// Package handler contém os handlers HTTP do ETL BUFFS.
package handler

import (
	"context"
	"net/http"
	"time"

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
func (h *HealthHandler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	services := make(map[string]string)

	// PostgreSQL
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
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

	code := http.StatusOK
	if status != "healthy" {
		code = http.StatusServiceUnavailable
	}

	writeJSON(w, code, dto.HealthResponse{
		Status:   status,
		Services: services,
	})
}
