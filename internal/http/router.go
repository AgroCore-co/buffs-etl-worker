// Package http configura o router Fiber com todos os endpoints do ETL BUFFS.
package http

import (
	"github.com/gofiber/adaptor/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/jaobarreto/buffs-etl-worker/internal/config"
	"github.com/jaobarreto/buffs-etl-worker/internal/exporter"
	"github.com/jaobarreto/buffs-etl-worker/internal/http/handler"
	"github.com/jaobarreto/buffs-etl-worker/internal/http/middleware"
	"github.com/jaobarreto/buffs-etl-worker/internal/loader"
	"github.com/jaobarreto/buffs-etl-worker/internal/mapper"
	"github.com/jaobarreto/buffs-etl-worker/internal/pipeline"
	"go.uber.org/zap"
)

// NewRouter cria e configura a aplicação Fiber com todos os endpoints.
func NewRouter(
	cfg *config.Config,
	pool *pgxpool.Pool,
	asynqClient *asynq.Client,
	asynqInspector *asynq.Inspector,
	logger *zap.Logger,
) *fiber.App {
	app := fiber.New(fiber.Config{
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		BodyLimit:    int(cfg.Upload.MaxFileSize) + 1024*1024, // margem para multipart overhead
		ErrorHandler: globalErrorHandler(logger),
	})

	// ── Middlewares globais ──────────────────────────────────────────────
	app.Use(recover.New())
	app.Use(requestid.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,OPTIONS",
		AllowHeaders: "Authorization,Content-Type",
	}))
	app.Use(middleware.RequestLogger(logger))

	// ── Dependências ────────────────────────────────────────────────────
	pgLoader := loader.NewPostgresLoader(pool, logger)
	pipelineEngine := pipeline.New(pgLoader, logger)
	exp := exporter.New(pool, logger)
	rateLimiter := middleware.NewRateLimiter(cfg.RateLimit.MaxImportsPerHour)

	// ── Handlers ────────────────────────────────────────────────────────
	importHandler := handler.NewImportHandler(pipelineEngine, pgLoader, asynqClient, cfg, logger)
	exportHandler := handler.NewExportHandler(exp, pgLoader, logger)
	jobHandler := handler.NewJobHandler(asynqInspector, logger)
	healthHandler := handler.NewHealthHandler(pool)

	// ── Rotas públicas ──────────────────────────────────────────────────
	app.Get("/health", healthHandler.HandleHealth)
	app.Get("/metrics", adaptor.HTTPHandler(promhttp.Handler()))

	// ── Rotas autenticadas ──────────────────────────────────────────────
	auth := app.Group("", middleware.JWTAuth(cfg.JWT.Secret, logger))

	// Import (com rate limiting)
	importGroup := auth.Group("/import", rateLimiter.CheckImportLimit())
	importGroup.Post("/milk", importHandler.HandleImport(mapper.PipelineMilk))
	importGroup.Post("/weight", importHandler.HandleImport(mapper.PipelineWeight))
	importGroup.Post("/reproduction", importHandler.HandleImport(mapper.PipelineReproduction))

	// Export
	exportGroup := auth.Group("/export")
	exportGroup.Get("/milk", exportHandler.HandleExportMilk)
	exportGroup.Get("/weight", exportHandler.HandleExportWeight)
	exportGroup.Get("/reproduction", exportHandler.HandleExportReproduction)

	// Templates
	templateGroup := auth.Group("/template")
	templateGroup.Get("/milk", exportHandler.HandleTemplateMilk)
	templateGroup.Get("/weight", exportHandler.HandleTemplateWeight)
	templateGroup.Get("/reproduction", exportHandler.HandleTemplateReproduction)

	// Jobs
	auth.Get("/jobs/:id/status", jobHandler.HandleJobStatus)

	return app
}

// globalErrorHandler trata erros não capturados.
func globalErrorHandler(logger *zap.Logger) fiber.ErrorHandler {
	return func(c *fiber.Ctx, err error) error {
		code := fiber.StatusInternalServerError
		msg := "Erro interno do servidor"

		if e, ok := err.(*fiber.Error); ok {
			code = e.Code
			msg = e.Message
		}

		logger.Error("Erro não tratado",
			zap.Int("status", code),
			zap.Error(err),
			zap.String("path", c.Path()),
		)

		return c.Status(code).JSON(fiber.Map{
			"code":    "INTERNAL_ERROR",
			"message": msg,
		})
	}
}
