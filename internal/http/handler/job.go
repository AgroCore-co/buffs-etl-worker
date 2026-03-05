package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/hibiken/asynq"
	"github.com/jaobarreto/buffs-etl-worker/internal/dto"
	"go.uber.org/zap"
)

// JobHandler gerencia o endpoint de status de jobs.
type JobHandler struct {
	inspector *asynq.Inspector
	logger    *zap.Logger
}

// NewJobHandler cria um handler de jobs.
func NewJobHandler(inspector *asynq.Inspector, logger *zap.Logger) *JobHandler {
	return &JobHandler{inspector: inspector, logger: logger}
}

// HandleJobStatus retorna o status de um job assíncrono.
func (h *JobHandler) HandleJobStatus(c *fiber.Ctx) error {
	jobID := c.Params("id")
	if jobID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(dto.ErrorResponse{
			Code:    "MISSING_JOB_ID",
			Message: "Job ID é obrigatório",
		})
	}

	// Busca a task por ID em todas as filas
	info, err := h.inspector.GetTaskInfo("default", jobID)
	if err != nil {
		h.logger.Debug("Job não encontrado", zap.String("job_id", jobID), zap.Error(err))
		return c.Status(fiber.StatusNotFound).JSON(dto.ErrorResponse{
			Code:    "JOB_NOT_FOUND",
			Message: "Job não encontrado",
		})
	}

	status := mapTaskState(info.State)

	return c.JSON(dto.JobStatusResponse{
		JobID:  jobID,
		Status: status,
	})
}

func mapTaskState(state asynq.TaskState) string {
	switch state {
	case asynq.TaskStatePending:
		return "PENDING"
	case asynq.TaskStateActive:
		return "PROCESSING"
	case asynq.TaskStateCompleted:
		return "COMPLETED"
	case asynq.TaskStateRetry:
		return "RETRYING"
	case asynq.TaskStateArchived:
		return "FAILED"
	default:
		return "UNKNOWN"
	}
}
