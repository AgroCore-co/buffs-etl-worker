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

	entry, ok := h.jobs.Get(jobID)
	if !ok {
		writeJSON(w, http.StatusNotFound, dto.ErrorResponse{
			Code:    "JOB_NOT_FOUND",
			Message: "Job não encontrado",
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
