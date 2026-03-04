package excel

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/xuri/excelize/v2"
)

// ── Header Mapping ───────────────────────────────────────────────────────────
// O mapeamento dinâmico de cabeçalho permite que as planilhas tenham colunas
// em qualquer ordem. O Go lê a primeira linha, normaliza os nomes e cria um
// dicionário map[string]int para resolver cada campo por posição.

// HeaderIndex mapeia nome_coluna_normalizado → índice (0-based) na planilha.
type HeaderIndex map[string]int

// normalizeHeader converte um nome de cabeçalho para forma canônica:
// lowercase, sem espaços extras, sem acentos comuns, underscores ao invés de espaços.
//
// Exemplos:
//
//	"Brinco"           → "brinco"
//	" Data Nascimento " → "data_nascimento"
//	"Dt Nascimento"    → "dt_nascimento"
//	"PESO (kg)"        → "peso_kg"
func normalizeHeader(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.ToLower(s)
	// Remove acentos comuns do português
	replacer := strings.NewReplacer(
		"á", "a", "à", "a", "ã", "a", "â", "a",
		"é", "e", "ê", "e",
		"í", "i",
		"ó", "o", "ô", "o", "õ", "o",
		"ú", "u", "ü", "u",
		"ç", "c",
	)
	s = replacer.Replace(s)
	// Substitui qualquer sequência de caracteres não-alfanuméricos por underscore
	var b strings.Builder
	lastWasUnderscore := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastWasUnderscore = false
		} else if !lastWasUnderscore {
			b.WriteRune('_')
			lastWasUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

// BuildHeaderIndex lê a primeira linha (cabeçalho) de uma aba e retorna
// um mapeamento normalizado de cada coluna para seu índice (0-based).
//
// Se a aba estiver vazia ou sem cabeçalho, retorna erro.
// Se campos obrigatórios (requiredFields) estiverem ausentes, retorna erro.
func BuildHeaderIndex(f *excelize.File, sheet string, requiredFields []string) (HeaderIndex, error) {
	rows, err := f.GetRows(sheet)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler aba '%s': %w", sheet, err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("aba '%s' está vazia (sem cabeçalho)", sheet)
	}

	headerRow := rows[0]
	index := make(HeaderIndex, len(headerRow))
	for i, cell := range headerRow {
		normalized := normalizeHeader(cell)
		if normalized != "" {
			index[normalized] = i
		}
	}

	// Valida campos obrigatórios
	var missing []string
	for _, field := range requiredFields {
		if _, ok := index[field]; !ok {
			missing = append(missing, field)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("aba '%s': campos obrigatórios ausentes no cabeçalho: %s",
			sheet, strings.Join(missing, ", "))
	}

	return index, nil
}

// GetCell retorna o valor trimado de uma coluna pelo nome normalizado.
// Retorna "" se a coluna não existir no headerIndex ou se o índice estiver
// fora do range da linha (linhas com menos colunas que o cabeçalho).
func GetCell(row []string, headerIndex HeaderIndex, colName string) string {
	idx, ok := headerIndex[colName]
	if !ok || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

// GetDataRows lê todas as linhas de dados de uma aba (exclui o cabeçalho).
// Retorna as linhas e o offset para calcular o número real da linha na planilha
// (offset=2 porque linha 1 é cabeçalho, Excel é 1-based).
func GetDataRows(f *excelize.File, sheet string) ([][]string, error) {
	rows, err := f.GetRows(sheet)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler linhas da aba '%s': %w", sheet, err)
	}
	if len(rows) <= 1 {
		return nil, nil // Só cabeçalho ou vazio → sem dados
	}
	return rows[1:], nil // Exclui cabeçalho
}
