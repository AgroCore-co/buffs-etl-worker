// Package main é o entrypoint do ETL BUFFS.
// Inicializa HTTP server (net/http + Chi), PostgreSQL pool e graceful shutdown.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jaobarreto/buffs-etl-worker/internal/config"
	apphttp "github.com/jaobarreto/buffs-etl-worker/internal/http"
	"github.com/jaobarreto/buffs-etl-worker/internal/job"
	"github.com/jaobarreto/buffs-etl-worker/pkg/logger"
	"go.uber.org/zap"
)

func main() {
	// ── Logger ──────────────────────────────────────────────────────────
	log := logger.New()
	defer log.Sync()

	log.Info("Iniciando ETL BUFFS")

	// ── Config ──────────────────────────────────────────────────────────
	cfg := config.Load()

	if cfg.InternalKey == "" {
		log.Warn("BUFFS_ETL_INTERNAL_KEY não configurada — endpoints autenticados ficarão inacessíveis")
	}

	// ── PostgreSQL ──────────────────────────────────────────────────────
	poolCfg, err := pgxpool.ParseConfig(cfg.DB.URL)
	if err != nil {
		log.Fatal("Falha ao parsear DATABASE_URL", zap.Error(err))
	}
	poolCfg.MaxConns = cfg.DB.MaxConns
	poolCfg.MinConns = cfg.DB.MinConns
	poolCfg.MaxConnLifetime = cfg.DB.MaxConnLifetime
	poolCfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	cancel()
	if err != nil {
		log.Fatal("Falha ao conectar no PostgreSQL", zap.Error(err))
	}
	defer pool.Close()

	if err := pool.Ping(context.Background()); err != nil {
		log.Fatal("PostgreSQL não respondeu ao ping", zap.Error(err))
	}
	log.Info("PostgreSQL conectado", zap.String("url", maskDSN(cfg.DB.URL)))

	// ── Job Store (in-memory) ───────────────────────────────────────────
	jobs := job.NewStore(log)

	// Cleanup periódico de jobs antigos (a cada 30 min, remove jobs >2h)
	go func() {
		ticker := time.NewTicker(30 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			jobs.Cleanup(2 * time.Hour)
		}
	}()

	// ── HTTP Server (net/http + Chi) ────────────────────────────────────
	handler := apphttp.NewRouter(cfg, pool, jobs, log)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	go func() {
		log.Info("HTTP server ouvindo", zap.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("Falha ao iniciar servidor HTTP", zap.Error(err))
		}
	}()

	// ── Graceful Shutdown ───────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	log.Info("Sinal recebido, iniciando shutdown gracioso", zap.String("signal", sig.String()))

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("Erro no shutdown do HTTP server", zap.Error(err))
	}
	log.Info("HTTP server encerrado")

	log.Info("ETL BUFFS encerrado com sucesso")
}

// maskDSN mascara a senha da connection string para logs.
func maskDSN(dsn string) string {
	if len(dsn) > 20 {
		return dsn[:20] + "..."
	}
	return dsn
}
