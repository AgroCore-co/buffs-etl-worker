// Package main é o entrypoint do ETL BUFFS.
// Inicializa HTTP server (Fiber), Asynq worker, PostgreSQL pool e graceful shutdown.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jaobarreto/buffs-etl-worker/internal/config"
	apphttp "github.com/jaobarreto/buffs-etl-worker/internal/http"
	"github.com/jaobarreto/buffs-etl-worker/internal/job"
	"github.com/jaobarreto/buffs-etl-worker/internal/loader"
	"github.com/jaobarreto/buffs-etl-worker/internal/pipeline"
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

	// ── PostgreSQL ──────────────────────────────────────────────────────
	poolCfg, err := pgxpool.ParseConfig(cfg.DB.URL)
	if err != nil {
		log.Fatal("Falha ao parsear DATABASE_URL", zap.Error(err))
	}
	poolCfg.MaxConns = cfg.DB.MaxConns
	poolCfg.MinConns = cfg.DB.MinConns
	poolCfg.MaxConnLifetime = cfg.DB.MaxConnLifetime

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

	// ── Redis / Asynq ───────────────────────────────────────────────────
	redisOpt, err := asynq.ParseRedisURI(cfg.Redis.URL)
	if err != nil {
		log.Fatal("Falha ao parsear REDIS_URL", zap.Error(err))
	}

	asynqClient := asynq.NewClient(redisOpt)
	defer asynqClient.Close()

	asynqInspector := asynq.NewInspector(redisOpt)
	defer asynqInspector.Close()

	log.Info("Redis/Asynq configurado", zap.String("url", cfg.Redis.URL))

	// ── Asynq Worker (background) ──────────────────────────────────────
	asynqServer := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: cfg.Worker.Concurrency,
		Queues:      map[string]int{"default": 1},
		Logger:      newAsynqLogger(log),
	})

	pgLoader := loader.NewPostgresLoader(pool, log)
	pipelineEngine := pipeline.New(pgLoader, log)
	processor := job.NewProcessor(pipelineEngine, log)

	mux := asynq.NewServeMux()
	job.RegisterHandlers(mux, processor)

	go func() {
		if err := asynqServer.Run(mux); err != nil {
			log.Error("Asynq server encerrou com erro", zap.Error(err))
		}
	}()
	log.Info("Asynq worker iniciado", zap.Int("concurrency", cfg.Worker.Concurrency))

	// ── HTTP Server (Fiber) ─────────────────────────────────────────────
	app := apphttp.NewRouter(cfg, pool, asynqClient, asynqInspector, log)

	go func() {
		addr := fmt.Sprintf(":%d", cfg.Server.Port)
		log.Info("HTTP server ouvindo", zap.String("addr", addr))
		if err := app.Listen(addr); err != nil {
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

	// Para o worker Asynq
	asynqServer.Shutdown()
	log.Info("Asynq worker encerrado")

	// Para o HTTP server
	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
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

// asynqLogAdapter adapta zap.Logger para a interface asynq.Logger.
type asynqLogAdapter struct {
	logger *zap.Logger
}

func newAsynqLogger(l *zap.Logger) *asynqLogAdapter {
	return &asynqLogAdapter{logger: l.Named("asynq")}
}

func (a *asynqLogAdapter) Debug(args ...any) { a.logger.Debug(fmt.Sprint(args...)) }
func (a *asynqLogAdapter) Info(args ...any)  { a.logger.Info(fmt.Sprint(args...)) }
func (a *asynqLogAdapter) Warn(args ...any)  { a.logger.Warn(fmt.Sprint(args...)) }
func (a *asynqLogAdapter) Error(args ...any) { a.logger.Error(fmt.Sprint(args...)) }
func (a *asynqLogAdapter) Fatal(args ...any) { a.logger.Fatal(fmt.Sprint(args...)) }
