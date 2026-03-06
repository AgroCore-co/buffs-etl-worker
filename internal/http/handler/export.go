package handler

import (
	"fmt"
	"net/http"
	"time"

	"github.com/jaobarreto/buffs-etl-worker/internal/dto"
	"github.com/jaobarreto/buffs-etl-worker/internal/exporter"
	"github.com/xuri/excelize/v2"
	"go.uber.org/zap"
)

// ExportHandler gerencia endpoints de exportação de planilhas.
type ExportHandler struct {
	exporter *exporter.Exporter
	logger   *zap.Logger
}

// NewExportHandler cria um handler de export.
func NewExportHandler(exp *exporter.Exporter, logger *zap.Logger) *ExportHandler {
	return &ExportHandler{exporter: exp, logger: logger}
}

// HandleExportMilk exporta planilha de pesagem de leite.
func (h *ExportHandler) HandleExportMilk(w http.ResponseWriter, r *http.Request) {
	params, ok := h.parseExportParams(w, r)
	if !ok {
		return
	}
	file, err := h.exporter.ExportMilk(r.Context(), *params)
	if err != nil {
		h.logger.Error("Erro ao exportar leite", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, dto.ErrorResponse{
			Code: "EXPORT_ERROR", Message: "Falha ao gerar planilha de leite",
		})
		return
	}
	h.sendExcel(w, file, "pesagem_leite")
}

// HandleExportWeight exporta planilha de pesagem do animal.
func (h *ExportHandler) HandleExportWeight(w http.ResponseWriter, r *http.Request) {
	params, ok := h.parseExportParams(w, r)
	if !ok {
		return
	}
	file, err := h.exporter.ExportWeight(r.Context(), *params)
	if err != nil {
		h.logger.Error("Erro ao exportar pesagem", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, dto.ErrorResponse{
			Code: "EXPORT_ERROR", Message: "Falha ao gerar planilha de pesagem",
		})
		return
	}
	h.sendExcel(w, file, "pesagem_animal")
}

// HandleExportReproduction exporta planilha de dados reprodutivos.
func (h *ExportHandler) HandleExportReproduction(w http.ResponseWriter, r *http.Request) {
	params, ok := h.parseExportParams(w, r)
	if !ok {
		return
	}
	file, err := h.exporter.ExportReproduction(r.Context(), *params)
	if err != nil {
		h.logger.Error("Erro ao exportar reprodução", zap.Error(err))
		writeJSON(w, http.StatusInternalServerError, dto.ErrorResponse{
			Code: "EXPORT_ERROR", Message: "Falha ao gerar planilha de reprodução",
		})
		return
	}
	h.sendExcel(w, file, "reproducao")
}

// ── Helpers ─────────────────────────────────────────────────────────────────

// parseExportParams extrai os filtros da query string (nomes em português).
// BUFFS API já validou auth/permissões — aqui só faz parsing.
func (h *ExportHandler) parseExportParams(w http.ResponseWriter, r *http.Request) (*exporter.ExportParams, bool) {
	propertyID := r.URL.Query().Get("propriedadeId")
	if propertyID == "" {
		writeJSON(w, http.StatusBadRequest, dto.ErrorResponse{
			Code: "MISSING_PROPERTY_ID", Message: "Query param propriedadeId é obrigatório",
		})
		return nil, false
	}

	params := &exporter.ExportParams{
		PropertyID: propertyID,
		GroupID:    r.URL.Query().Get("grupoId"),
		Maturity:   r.URL.Query().Get("maturidade"),
		Sex:        r.URL.Query().Get("sexo"),
		Tipo:       r.URL.Query().Get("tipo"),
		IncludeRef: r.URL.Query().Get("include_ref") != "false",
	}

	if de := r.URL.Query().Get("de"); de != "" {
		if t, err := time.Parse("2006-01-02", de); err == nil {
			params.From = t
		}
	}
	if ate := r.URL.Query().Get("ate"); ate != "" {
		if t, err := time.Parse("2006-01-02", ate); err == nil {
			// Inclui o dia inteiro: avança para início do dia seguinte
			params.To = t.AddDate(0, 0, 1)
		}
	}

	return params, true
}

func (h *ExportHandler) sendExcel(w http.ResponseWriter, file *excelize.File, name string) {
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("%s_%s.xlsx", name, timestamp)

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	file.Write(w)
}
