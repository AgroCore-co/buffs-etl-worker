package handler

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/jaobarreto/buffs-etl-worker/internal/config"
	"github.com/jaobarreto/buffs-etl-worker/internal/dto"
	"github.com/jaobarreto/buffs-etl-worker/internal/extractor"
	"github.com/jaobarreto/buffs-etl-worker/internal/job"
	"github.com/jaobarreto/buffs-etl-worker/internal/mapper"
	"github.com/jaobarreto/buffs-etl-worker/internal/pipeline"
	"github.com/jaobarreto/buffs-etl-worker/pkg/apperror"
	"go.uber.org/zap"
)

// ImportHandler gerencia endpoints de importação de planilhas.
type ImportHandler struct {
	pipeline *pipeline.Pipeline
	cfg      *config.Config
	jobs     *job.Store
	logger   *zap.Logger
}

// NewImportHandler cria um handler de import com todas as dependências.
func NewImportHandler(
	p *pipeline.Pipeline,
	cfg *config.Config,
	jobs *job.Store,
	logger *zap.Logger,
) *ImportHandler {
	return &ImportHandler{
		pipeline: p,
		cfg:      cfg,
		jobs:     jobs,
		logger:   logger,
	}
}

// HandleImport processa o upload e importação de uma planilha Excel.
// Se o arquivo tiver mais linhas que o threshold, processa assincronamente via goroutine.
func (h *ImportHandler) HandleImport(pipelineType mapper.PipelineType) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Valida propriedadeId e usuarioId (enviados pela BUFFS API)
		propertyID := r.URL.Query().Get("propriedadeId")
		if propertyID == "" {
			writeJSON(w, http.StatusBadRequest, dto.ErrorResponse{
				Code:    "MISSING_PROPERTY_ID",
				Message: "Query param propriedadeId é obrigatório",
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

		// 2. Recebe o arquivo (multipart/form-data, campo "file")
		if err := r.ParseMultipartForm(h.cfg.Upload.MaxFileSize + 1024*1024); err != nil {
			writeJSON(w, http.StatusBadRequest, dto.ErrorResponse{
				Code:    "MISSING_FILE",
				Message: "Arquivo Excel não enviado. Use multipart/form-data com campo 'file'.",
			})
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, dto.ErrorResponse{
				Code:    "MISSING_FILE",
				Message: "Arquivo Excel não enviado. Use multipart/form-data com campo 'file'.",
			})
			return
		}
		defer file.Close()

		// 3. Valida tamanho
		if header.Size > h.cfg.Upload.MaxFileSize {
			writeJSON(w, http.StatusBadRequest, dto.ErrorResponse{
				Code:    string(apperror.CodeFileTooLarge),
				Message: fmt.Sprintf("Arquivo excede o limite de %dMB", h.cfg.Upload.MaxFileSize/(1024*1024)),
			})
			return
		}

		// 4. Valida extensão
		ext := filepath.Ext(header.Filename)
		if ext != ".xlsx" && ext != ".xls" {
			writeJSON(w, http.StatusBadRequest, dto.ErrorResponse{
				Code:    string(apperror.CodeInvalidFormat),
				Message: "Apenas arquivos .xlsx e .xls são aceitos",
			})
			return
		}

		// 5. Salva em disco temporário
		os.MkdirAll(h.cfg.Upload.TempDir, 0755)
		fileName := fmt.Sprintf("%s_%d%s", propertyID, time.Now().UnixMilli(), ext)
		filePath := filepath.Join(h.cfg.Upload.TempDir, fileName)

		dst, err := os.Create(filePath)
		if err != nil {
			h.logger.Error("Falha ao criar arquivo temporário", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, dto.ErrorResponse{
				Code:    "INTERNAL_ERROR",
				Message: "Falha ao salvar arquivo temporário",
			})
			return
		}
		if _, err := io.Copy(dst, file); err != nil {
			dst.Close()
			os.Remove(filePath)
			h.logger.Error("Falha ao salvar arquivo", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, dto.ErrorResponse{
				Code:    "INTERNAL_ERROR",
				Message: "Falha ao salvar arquivo temporário",
			})
			return
		}
		dst.Close()

		// 6. Conta linhas para decidir sync vs async
		rowCount, err := extractor.RowCount(filePath)
		if err != nil {
			os.Remove(filePath)
			writeJSON(w, http.StatusBadRequest, dto.ErrorResponse{
				Code:    string(apperror.CodeFileCorrupted),
				Message: "Não foi possível ler o arquivo Excel",
			})
			return
		}

		// 7. Processamento assíncrono para arquivos grandes
		if rowCount > h.cfg.Worker.AsyncThreshold {
			jobID := h.jobs.Create(filePath, pipelineType, propertyID, userID)

			go h.jobs.Process(jobID, h.pipeline)

			h.logger.Info("Import enviado para processamento assíncrono",
				zap.String("job_id", jobID),
				zap.Int("rows", rowCount),
			)

			writeJSON(w, http.StatusAccepted, dto.AsyncImportResponse{
				JobID:   jobID,
				Message: fmt.Sprintf("Planilha com %d linhas enviada para processamento assíncrono", rowCount),
				Status:  "PROCESSING",
			})
			return
		}

		// 8. Processamento síncrono
		defer os.Remove(filePath)

		result, err := h.pipeline.Run(r.Context(), pipeline.RunParams{
			FilePath:   filePath,
			PropertyID: propertyID,
			UserID:     userID,
			Type:       pipelineType,
		})

		if err != nil {
			if appErr, ok := err.(*apperror.AppError); ok {
				writeJSON(w, appErr.HTTPCode, dto.ErrorResponse{
					Code:    string(appErr.Code),
					Message: appErr.Message,
				})
				return
			}
			h.logger.Error("Erro no pipeline", zap.Error(err))
			writeJSON(w, http.StatusInternalServerError, dto.ErrorResponse{
				Code:    "INTERNAL_ERROR",
				Message: "Erro interno no processamento",
			})
			return
		}

		// Converte domain.ImportResult → dto.ImportResponse
		resp := dto.ImportResponse{
			Total:    result.Total,
			Imported: result.Imported,
			Skipped:  result.Skipped,
		}
		for _, e := range result.Errors {
			resp.Errors = append(resp.Errors, dto.RowIssue{
				Row: e.Row, Field: e.Field, Value: e.Value, Message: e.Message,
			})
		}
		for _, w := range result.Warnings {
			resp.Warnings = append(resp.Warnings, dto.RowIssue{
				Row: w.Row, Field: w.Field, Value: w.Value, Message: w.Message,
			})
		}

		writeJSON(w, http.StatusOK, resp)
	}
}
