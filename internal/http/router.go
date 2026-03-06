// Package http configura o router Chi com todos os endpoints do ETL BUFFS.
package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jaobarreto/buffs-etl-worker/internal/config"
	"github.com/jaobarreto/buffs-etl-worker/internal/exporter"
	"github.com/jaobarreto/buffs-etl-worker/internal/http/handler"
	"github.com/jaobarreto/buffs-etl-worker/internal/http/middleware"
	"github.com/jaobarreto/buffs-etl-worker/internal/job"
	"github.com/jaobarreto/buffs-etl-worker/internal/loader"
	"github.com/jaobarreto/buffs-etl-worker/internal/mapper"
	"github.com/jaobarreto/buffs-etl-worker/internal/pipeline"
	"go.uber.org/zap"
)

// NewRouter cria e configura o router Chi com todos os endpoints.
func NewRouter(
	cfg *config.Config,
	pool *pgxpool.Pool,
	jobs *job.Store,
	logger *zap.Logger,
) http.Handler {
	r := chi.NewRouter()

	// ── Middlewares globais ──────────────────────────────────────────────
	r.Use(chimw.Recoverer)
	r.Use(chimw.RequestID)
	r.Use(middleware.RequestLogger(logger))

	// ── Dependências ────────────────────────────────────────────────────
	pgLoader := loader.NewPostgresLoader(pool, logger)
	pipelineEngine := pipeline.New(pgLoader, logger)
	exp := exporter.New(pool, logger)

	// ── Handlers ────────────────────────────────────────────────────────
	importHandler := handler.NewImportHandler(pipelineEngine, cfg, jobs, logger)
	exportHandler := handler.NewExportHandler(exp, logger)
	jobHandler := handler.NewJobHandler(jobs, logger)
	healthHandler := handler.NewHealthHandler(pool)

	// ── Rota pública ────────────────────────────────────────────────────
	r.Get("/health", healthHandler.HandleHealth)

	// ── Rotas autenticadas (X-Internal-Key) ─────────────────────────────
	r.Group(func(r chi.Router) {
		r.Use(middleware.InternalKeyAuth(cfg.InternalKey, logger))

		// Import (rotas em português)
		r.Post("/import/leite", importHandler.HandleImport(mapper.PipelineMilk))
		r.Post("/import/pesagem", importHandler.HandleImport(mapper.PipelineWeight))
		r.Post("/import/reproducao", importHandler.HandleImport(mapper.PipelineReproduction))

		// Export (rotas em português)
		r.Get("/export/leite", exportHandler.HandleExportMilk)
		r.Get("/export/pesagem", exportHandler.HandleExportWeight)
		r.Get("/export/reproducao", exportHandler.HandleExportReproduction)

		// Jobs
		r.Get("/jobs/{id}/status", jobHandler.HandleJobStatus)
	})

	return r
}
