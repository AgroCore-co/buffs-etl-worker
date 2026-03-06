// Package middleware contém os middlewares HTTP do ETL BUFFS.
package middleware

import (
	"net/http"

	"github.com/jaobarreto/buffs-etl-worker/internal/dto"
	"go.uber.org/zap"
)

// InternalKeyAuth cria um middleware que valida o header X-Internal-Key.
// Usado para autenticação inter-serviço (BUFFS API → ETL Worker).
func InternalKeyAuth(key string, logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			provided := r.Header.Get("X-Internal-Key")

			if provided == "" {
				logger.Debug("Requisição sem X-Internal-Key")
				writeJSON(w, http.StatusUnauthorized, dto.ErrorResponse{
					Code:    "UNAUTHORIZED",
					Message: "Header X-Internal-Key não fornecido",
				})
				return
			}

			if provided != key {
				logger.Warn("X-Internal-Key inválida")
				writeJSON(w, http.StatusUnauthorized, dto.ErrorResponse{
					Code:    "UNAUTHORIZED",
					Message: "X-Internal-Key inválida",
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
