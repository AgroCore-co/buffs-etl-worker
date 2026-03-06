// Package dto contém os Request/Response DTOs para os endpoints HTTP.
package dto

import "time"

// ── Import DTOs ─────────────────────────────────────────────────────────────

// ImportResponse é a resposta de um import síncrono.
type ImportResponse struct {
	JobID    string     `json:"job_id,omitempty"`
	Total    int        `json:"total_rows"`
	Imported int        `json:"imported"`
	Skipped  int        `json:"skipped"`
	Errors   []RowIssue `json:"errors"`
	Warnings []RowIssue `json:"warnings"`
}

// RowIssue representa um erro ou warning de uma linha.
type RowIssue struct {
	Row     int    `json:"row"`
	Field   string `json:"field"`
	Value   string `json:"value"`
	Message string `json:"message"`
}

// AsyncImportResponse é retornado quando o import é processado assincronamente (202).
type AsyncImportResponse struct {
	JobID   string `json:"job_id"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

// ── Export DTOs ─────────────────────────────────────────────────────────────

// ExportQuery contém os parâmetros de filtro para exportação (nomes em português).
type ExportQuery struct {
	PropriedadeID string    `json:"propriedadeId"`
	GrupoID       string    `json:"grupoId"`
	Maturidade    string    `json:"maturidade"`
	Sexo          string    `json:"sexo"`
	Tipo          string    `json:"tipo"`
	De            time.Time `json:"de"`
	Ate           time.Time `json:"ate"`
	IncludeRef    *bool     `json:"include_ref"`
}

// ── Job DTOs ────────────────────────────────────────────────────────────────

// JobStatusResponse é a resposta do endpoint de status de job.
type JobStatusResponse struct {
	JobID  string          `json:"job_id"`
	Status string          `json:"status"`
	Result *ImportResponse `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// ── Error DTOs ──────────────────────────────────────────────────────────────

// ErrorResponse é a resposta padrão de erro.
type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ── Health DTOs ─────────────────────────────────────────────────────────────

// HealthResponse é a resposta do healthcheck.
type HealthResponse struct {
	Status   string            `json:"status"`
	Services map[string]string `json:"services"`
}
