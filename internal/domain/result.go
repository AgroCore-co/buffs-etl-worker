// Package domain contém as entidades centrais do domínio do worker.
package domain

import "fmt"

// ── Resultado de Processamento ───────────────────────────────────────────────

// RowError representa um erro de validação em uma linha específica da planilha.
type RowError struct {
	// Sheet é o nome da aba onde o erro ocorreu (ex: "Animais").
	Sheet string
	// Row é o número da linha na planilha (1-based, contando o cabeçalho).
	Row int
	// Field é o nome do campo que falhou na validação (ex: "peso").
	Field string
	// Value é o valor original que causou o erro.
	Value string
	// Message descreve o motivo da falha.
	Message string
}

// Error implementa a interface error para RowError.
func (e RowError) Error() string {
	return fmt.Sprintf("Aba '%s', Linha %d, Campo '%s' (valor='%s'): %s",
		e.Sheet, e.Row, e.Field, e.Value, e.Message)
}

// SheetResult armazena o resultado do processamento de uma única aba da planilha.
type SheetResult struct {
	// Sheet é o nome da aba processada.
	Sheet string
	// Table é o nome da tabela PostgreSQL de destino.
	Table string
	// TotalRows é o número total de linhas de dados (sem cabeçalho).
	TotalRows int
	// Inserted é o número de linhas inseridas com sucesso.
	Inserted int
	// Skipped é o número de linhas ignoradas por erro de validação.
	Skipped int
	// Errors contém os erros detalhados de cada linha ignorada.
	Errors []RowError
}

// ProcessingResult é o resultado final do processamento completo de um arquivo Excel.
type ProcessingResult struct {
	// FileName é o nome do arquivo processado.
	FileName string
	// PropriedadeID é o UUID da propriedade associada.
	PropriedadeID string
	// UsuarioID é o UUID do usuário que fez o upload.
	UsuarioID string
	// Sheets contém os resultados individuais de cada aba processada.
	Sheets []SheetResult
	// UnknownSheets são abas encontradas no Excel que não possuem mapeamento.
	UnknownSheets []string
}

// TotalInserted retorna o total de linhas inseridas em todas as abas.
func (r *ProcessingResult) TotalInserted() int {
	total := 0
	for _, s := range r.Sheets {
		total += s.Inserted
	}
	return total
}

// TotalSkipped retorna o total de linhas ignoradas em todas as abas.
func (r *ProcessingResult) TotalSkipped() int {
	total := 0
	for _, s := range r.Sheets {
		total += s.Skipped
	}
	return total
}

// AllErrors retorna todos os erros de todas as abas em uma lista plana.
func (r *ProcessingResult) AllErrors() []RowError {
	var all []RowError
	for _, s := range r.Sheets {
		all = append(all, s.Errors...)
	}
	return all
}
