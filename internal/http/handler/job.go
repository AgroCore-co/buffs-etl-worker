package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jaobarreto/buffs-etl-worker/internal/dto"
	"github.com/jaobarreto/buffs-etl-worker/internal/job"
	"go.uber.org/zap"
)

// JobHandler gerencia endpoint de status de jobs assíncronos.
type JobHandler struct {
	jobs   *job.Store
	logger *zap.Logger
}

// NewJobHandler cria um handler de jobs.
func NewJobHandler(jobs *job.Store, logger *zap.Logger) *JobHandler {
	return &JobHandler{jobs: jobs, logger: logger}
}

// HandleJobStatus retorna o status de um job assíncrono.
func (h *JobHandler) HandleJobStatus(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "id")
	if jobID == "" {
		writeJSON(w, http.StatusBadRequest, dto.ErrorResponse{
			Code:    "MISSING_JOB_ID",
			Message: "ID do job não fornecido",
		})
		return
	}

	userID := r.URL.Query().Get("usuarioId")
	if userID == "" {
		writeJSON(w, http.StatusBadRequest, dto.ErrorResponse{
			Code:    "MISSING_USER_ID",
			Message: "Query param usuarioId é obrigatório",
		})
		return
	}

	entry, ok, err := h.jobs.Get(jobID)
	if err != nil {
		h.logger.Error("Falha ao buscar status de job", zap.String("job_id", jobID), zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, dto.ErrorResponse{
			Code:    "INTERNAL_ERROR",
			Message: "Falha ao consultar status do job",
		})
		return
	}

	if !ok {
		writeJSON(w, http.StatusNotFound, dto.ErrorResponse{
			Code:    "JOB_NOT_FOUND",
			Message: "Job não encontrado. Ele pode ter expirado ou sido removido pela rotina de limpeza.",
		})
		return
	}

	if entry.UserID != userID {
		h.logger.Warn("Tentativa de acesso a job sem ownership",
			zap.String("job_id", jobID),
			zap.String("entry_user_id", entry.UserID),
			zap.String("request_user_id", userID),
		)
		writeJSON(w, http.StatusForbidden, dto.ErrorResponse{
			Code:    "FORBIDDEN",
			Message: "Você não tem permissão para consultar este job",
		})
		return
	}

	resp := dto.JobStatusResponse{
		JobID:  jobID,
		Status: entry.Status,
	}

	if entry.Result != nil {
		resp.Result = &dto.ImportResponse{
			Total:    entry.Result.Total,
			Imported: entry.Result.Imported,
			Skipped:  entry.Result.Skipped,
		}
		for _, e := range entry.Result.Errors {
			resp.Result.Errors = append(resp.Result.Errors, dto.RowIssue{
				Row: e.Row, Field: e.Field, Value: e.Value, Message: e.Message,
			})
		}
		for _, w := range entry.Result.Warnings {
			resp.Result.Warnings = append(resp.Result.Warnings, dto.RowIssue{
				Row: w.Row, Field: w.Field, Value: w.Value, Message: w.Message,
			})
		}
	}

	if entry.Error != "" {
		resp.Error = entry.Error
	}

	writeJSON(w, http.StatusOK, resp)
}
