package handler

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jaobarreto/buffs-etl-worker/internal/dto"
)

// HealthHandler gerencia o endpoint de healthcheck.
type HealthHandler struct {
	pool *pgxpool.Pool
}

// NewHealthHandler cria um health handler.
func NewHealthHandler(pool *pgxpool.Pool) *HealthHandler {
	return &HealthHandler{pool: pool}
}

// HandleHealth verifica o status dos serviços.
func (h *HealthHandler) HandleHealth(c *fiber.Ctx) error {
	services := make(map[string]string)

	// PostgreSQL
	ctx, cancel := context.WithTimeout(c.Context(), 3*time.Second)
	defer cancel()

	if err := h.pool.Ping(ctx); err != nil {
		services["postgres"] = "unhealthy"
	} else {
		services["postgres"] = "healthy"
	}

	status := "healthy"
	for _, v := range services {
		if v != "healthy" {
			status = "degraded"
			break
		}
	}

	code := fiber.StatusOK
	if status != "healthy" {
		code = fiber.StatusServiceUnavailable
	}

	return c.Status(code).JSON(dto.HealthResponse{
		Status:   status,
		Services: services,
	})
}
