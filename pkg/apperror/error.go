// Package apperror define tipos de erro padronizados do ETL BUFFS.
// Todos os erros retornados pelas camadas internas devem ser do tipo *AppError.
package apperror

import (
	"fmt"
	"net/http"
)

// Code representa um código de erro da aplicação.
type Code string

const (
	// Erros fatais (abort total)
	CodeFileCorrupted    Code = "FILE_CORRUPTED"
	CodeInvalidFormat    Code = "INVALID_FORMAT"
	CodeMissingColumn    Code = "MISSING_COLUMN"
	CodeUnauthorized     Code = "UNAUTHORIZED"
	CodeForbidden        Code = "FORBIDDEN"
	CodePropertyNotFound Code = "PROPERTY_NOT_FOUND"
	CodeInternalError    Code = "INTERNAL_ERROR"
	CodeFileTooLarge     Code = "FILE_TOO_LARGE"
	CodeJobNotFound      Code = "JOB_NOT_FOUND"

	// Erros por linha (continua o import)
	CodeBrincoNotFound Code = "BRINCO_NOT_FOUND"
	CodeInvalidDate    Code = "INVALID_DATE"
	CodeInvalidValue   Code = "INVALID_VALUE"
	CodeDuplicate      Code = "DUPLICATE"
	CodeRequiredField  Code = "REQUIRED_FIELD"
	CodeInvalidEnum    Code = "INVALID_ENUM"

	// Warnings (importa com flag)
	CodeSuspectValue Code = "SUSPECT_VALUE"
)

// Severity indica a severidade do erro.
type Severity int

const (
	SeverityError   Severity = iota // Erro — linha não importada
	SeverityWarning                 // Warning — linha importada com flag
	SeverityFatal                   // Fatal — abort total do import
)

// AppError é o tipo de erro padrão da aplicação.
type AppError struct {
	Code     Code     `json:"code"`
	Message  string   `json:"message"`
	Severity Severity `json:"-"`
	HTTPCode int      `json:"-"`
	Cause    error    `json:"-"`
}

func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error { return e.Cause }

// RowError representa um erro de validação em uma linha específica da planilha.
type RowError struct {
	Row      int      `json:"row"`
	Field    string   `json:"field"`
	Value    string   `json:"value"`
	Message  string   `json:"message"`
	Code     Code     `json:"code"`
	Severity Severity `json:"-"`
}

func (e *RowError) Error() string {
	return fmt.Sprintf("row %d, field %q, value %q: %s", e.Row, e.Field, e.Value, e.Message)
}

// IsWarning retorna true se o erro é apenas um warning.
func (e *RowError) IsWarning() bool { return e.Severity == SeverityWarning }

// ── Construtores ────────────────────────────────────────────────────────────

func New(code Code, msg string) *AppError {
	return &AppError{Code: code, Message: msg, Severity: SeverityFatal, HTTPCode: http.StatusInternalServerError}
}

func Wrap(code Code, msg string, err error) *AppError {
	return &AppError{Code: code, Message: msg, Severity: SeverityFatal, HTTPCode: http.StatusInternalServerError, Cause: err}
}

func Unauthorized(msg string) *AppError {
	return &AppError{Code: CodeUnauthorized, Message: msg, Severity: SeverityFatal, HTTPCode: http.StatusUnauthorized}
}

func Forbidden(msg string) *AppError {
	return &AppError{Code: CodeForbidden, Message: msg, Severity: SeverityFatal, HTTPCode: http.StatusForbidden}
}

func BadRequest(code Code, msg string) *AppError {
	return &AppError{Code: code, Message: msg, Severity: SeverityFatal, HTTPCode: http.StatusBadRequest}
}

func NotFound(code Code, msg string) *AppError {
	return &AppError{Code: code, Message: msg, Severity: SeverityFatal, HTTPCode: http.StatusNotFound}
}

func Internal(msg string, err error) *AppError {
	return &AppError{Code: CodeInternalError, Message: msg, Severity: SeverityFatal, HTTPCode: http.StatusInternalServerError, Cause: err}
}

// NewRowError cria um erro de linha (nível Error — linha não importada).
func NewRowError(row int, field, value, msg string, code Code) *RowError {
	return &RowError{Row: row, Field: field, Value: value, Message: msg, Code: code, Severity: SeverityError}
}

// NewRowWarning cria um warning de linha (linha importada com flag).
func NewRowWarning(row int, field, value, msg string) *RowError {
	return &RowError{Row: row, Field: field, Value: value, Message: msg, Code: CodeSuspectValue, Severity: SeverityWarning}
}
