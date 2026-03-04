package excel

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jaobarreto/buffs-etl-worker/internal/domain"
)

// ── Validadores de Campo ─────────────────────────────────────────────────────
// Cada validador recebe o valor bruto da célula e retorna erro se inválido.
// Os validadores NÃO convertem tipos — apenas garantem que os dados são
// semanticamente corretos antes do bulk insert.

// ValidateRequired verifica se o campo não está vazio.
func ValidateRequired(sheet string, row int, field, value string) *domain.RowError {
	if strings.TrimSpace(value) == "" {
		return &domain.RowError{
			Sheet:   sheet,
			Row:     row,
			Field:   field,
			Value:   value,
			Message: "campo obrigatório está vazio",
		}
	}
	return nil
}

// ValidateNumeric verifica se o valor é um número decimal válido.
// Aceita vírgula como separador decimal (comum em planilhas BR).
func ValidateNumeric(sheet string, row int, field, value string) *domain.RowError {
	if value == "" {
		return nil // Campo opcional vazio → ok
	}
	normalized := strings.Replace(value, ",", ".", 1)
	if _, err := strconv.ParseFloat(normalized, 64); err != nil {
		return &domain.RowError{
			Sheet:   sheet,
			Row:     row,
			Field:   field,
			Value:   value,
			Message: fmt.Sprintf("valor numérico inválido: '%s'", value),
		}
	}
	return nil
}

// ValidateDate verifica se o valor pode ser parseado como data.
// Aceita múltiplos formatos comuns em planilhas brasileiras.
// Também rejeita datas no futuro (ex: data de nascimento em 2099).
func ValidateDate(sheet string, row int, field, value string, allowFuture bool) *domain.RowError {
	if value == "" {
		return nil // Campo opcional vazio → ok
	}

	parsed, err := ParseDate(value)
	if err != nil {
		return &domain.RowError{
			Sheet:   sheet,
			Row:     row,
			Field:   field,
			Value:   value,
			Message: fmt.Sprintf("data inválida: '%s' (formatos aceitos: dd/mm/aaaa, aaaa-mm-dd)", value),
		}
	}

	if !allowFuture && parsed.After(time.Now()) {
		return &domain.RowError{
			Sheet:   sheet,
			Row:     row,
			Field:   field,
			Value:   value,
			Message: "data não pode ser no futuro",
		}
	}

	return nil
}

// ValidateSexo verifica se o valor é M ou F (case-insensitive).
func ValidateSexo(sheet string, row int, field, value string) *domain.RowError {
	if value == "" {
		return nil
	}
	upper := strings.ToUpper(strings.TrimSpace(value))
	if upper != "M" && upper != "F" {
		return &domain.RowError{
			Sheet:   sheet,
			Row:     row,
			Field:   field,
			Value:   value,
			Message: fmt.Sprintf("sexo deve ser 'M' ou 'F', recebido: '%s'", value),
		}
	}
	return nil
}

// ValidateMaxLength verifica se o valor não excede o tamanho máximo.
func ValidateMaxLength(sheet string, row int, field, value string, maxLen int) *domain.RowError {
	if len(value) > maxLen {
		return &domain.RowError{
			Sheet:   sheet,
			Row:     row,
			Field:   field,
			Value:   value,
			Message: fmt.Sprintf("excede tamanho máximo de %d caracteres (tem %d)", maxLen, len(value)),
		}
	}
	return nil
}

// ValidateBoolean verifica se o valor pode ser interpretado como booleano.
// Aceita: sim/não, s/n, true/false, 1/0, verdadeiro/falso.
func ValidateBoolean(sheet string, row int, field, value string) *domain.RowError {
	if value == "" {
		return nil
	}
	lower := strings.ToLower(strings.TrimSpace(value))
	validValues := map[string]bool{
		"sim": true, "não": true, "nao": true,
		"s": true, "n": true,
		"true": true, "false": true,
		"1": true, "0": true,
		"verdadeiro": true, "falso": true,
	}
	if !validValues[lower] {
		return &domain.RowError{
			Sheet:   sheet,
			Row:     row,
			Field:   field,
			Value:   value,
			Message: fmt.Sprintf("valor booleano inválido: '%s' (aceitos: sim/não, s/n, true/false, 1/0)", value),
		}
	}
	return nil
}

// ── Helpers de Conversão ─────────────────────────────────────────────────────

// dateFormats são os formatos de data aceitos, do mais específico ao mais genérico.
var dateFormats = []string{
	"2006-01-02T15:04:05Z07:00", // ISO 8601 (vem do Excel serializado)
	"2006-01-02 15:04:05",       // SQL datetime
	"2006-01-02",                // ISO date
	"02/01/2006",                // dd/mm/yyyy (BR)
	"2/1/2006",                  // d/m/yyyy (BR sem zero à esquerda)
	"02-01-2006",                // dd-mm-yyyy
	"01/02/2006",                // mm/dd/yyyy (US, fallback)
}

// ParseDate tenta parsear uma string de data usando múltiplos formatos.
func ParseDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	for _, layout := range dateFormats {
		if t, err := time.Parse(layout, value); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("formato de data não reconhecido: '%s'", value)
}

// NormalizeNumeric converte separador decimal de vírgula para ponto.
func NormalizeNumeric(value string) string {
	return strings.Replace(strings.TrimSpace(value), ",", ".", 1)
}

// NormalizeBool converte valores brasileiros para "true"/"false".
func NormalizeBool(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	switch lower {
	case "sim", "s", "true", "1", "verdadeiro":
		return "true"
	case "não", "nao", "n", "false", "0", "falso":
		return "false"
	default:
		return ""
	}
}

// ── Validação por Tabela ─────────────────────────────────────────────────────
// Cada função abaixo valida uma linha inteira para uma tabela específica,
// retornando todos os erros encontrados (fail-fast por linha, continua no lote).

// ValidateBufaloRow valida uma linha da aba "Animais" → tabela bufalo.
func ValidateBufaloRow(sheet string, rowNum int, row []string, h HeaderIndex) []domain.RowError {
	var errs []domain.RowError

	collect := func(e *domain.RowError) {
		if e != nil {
			errs = append(errs, *e)
		}
	}

	collect(ValidateRequired(sheet, rowNum, "brinco", GetCell(row, h, "brinco")))
	collect(ValidateMaxLength(sheet, rowNum, "brinco", GetCell(row, h, "brinco"), 10))
	collect(ValidateMaxLength(sheet, rowNum, "nome", GetCell(row, h, "nome"), 20))
	collect(ValidateDate(sheet, rowNum, "dt_nascimento", GetCell(row, h, "dt_nascimento"), false))
	collect(ValidateSexo(sheet, rowNum, "sexo", GetCell(row, h, "sexo")))
	collect(ValidateMaxLength(sheet, rowNum, "nivel_maturidade", GetCell(row, h, "nivel_maturidade"), 1))
	collect(ValidateMaxLength(sheet, rowNum, "origem", GetCell(row, h, "origem"), 10))
	collect(ValidateMaxLength(sheet, rowNum, "categoria", GetCell(row, h, "categoria"), 3))
	collect(ValidateMaxLength(sheet, rowNum, "microchip", GetCell(row, h, "microchip"), 50))

	return errs
}

// ValidateZootecnicoRow valida uma linha da aba "Pesagens" → tabela dadoszootecnicos.
func ValidateZootecnicoRow(sheet string, rowNum int, row []string, h HeaderIndex) []domain.RowError {
	var errs []domain.RowError

	collect := func(e *domain.RowError) {
		if e != nil {
			errs = append(errs, *e)
		}
	}

	collect(ValidateRequired(sheet, rowNum, "brinco", GetCell(row, h, "brinco")))
	collect(ValidateRequired(sheet, rowNum, "dt_registro", GetCell(row, h, "dt_registro")))
	collect(ValidateDate(sheet, rowNum, "dt_registro", GetCell(row, h, "dt_registro"), false))
	collect(ValidateNumeric(sheet, rowNum, "peso", GetCell(row, h, "peso")))
	collect(ValidateNumeric(sheet, rowNum, "condicao_corporal", GetCell(row, h, "condicao_corporal")))
	collect(ValidateMaxLength(sheet, rowNum, "cor_pelagem", GetCell(row, h, "cor_pelagem"), 30))
	collect(ValidateMaxLength(sheet, rowNum, "formato_chifre", GetCell(row, h, "formato_chifre"), 30))
	collect(ValidateMaxLength(sheet, rowNum, "porte_corporal", GetCell(row, h, "porte_corporal"), 30))
	collect(ValidateMaxLength(sheet, rowNum, "tipo_pesagem", GetCell(row, h, "tipo_pesagem"), 50))

	return errs
}

// ValidateSanitarioRow valida uma linha da aba "Sanitario" → tabela dadossanitarios.
func ValidateSanitarioRow(sheet string, rowNum int, row []string, h HeaderIndex) []domain.RowError {
	var errs []domain.RowError

	collect := func(e *domain.RowError) {
		if e != nil {
			errs = append(errs, *e)
		}
	}

	collect(ValidateRequired(sheet, rowNum, "brinco", GetCell(row, h, "brinco")))
	collect(ValidateRequired(sheet, rowNum, "dt_aplicacao", GetCell(row, h, "dt_aplicacao")))
	collect(ValidateDate(sheet, rowNum, "dt_aplicacao", GetCell(row, h, "dt_aplicacao"), false))
	collect(ValidateNumeric(sheet, rowNum, "dosagem", GetCell(row, h, "dosagem")))
	collect(ValidateMaxLength(sheet, rowNum, "unidade_medida", GetCell(row, h, "unidade_medida"), 20))
	collect(ValidateMaxLength(sheet, rowNum, "doenca", GetCell(row, h, "doenca"), 100))
	collect(ValidateBoolean(sheet, rowNum, "necessita_retorno", GetCell(row, h, "necessita_retorno")))
	collect(ValidateDate(sheet, rowNum, "dt_retorno", GetCell(row, h, "dt_retorno"), true))
	collect(ValidateMaxLength(sheet, rowNum, "observacao", GetCell(row, h, "observacao"), 255))

	return errs
}

// ValidateLactacaoRow valida uma linha da aba "Lactacao" → tabela dadoslactacao.
func ValidateLactacaoRow(sheet string, rowNum int, row []string, h HeaderIndex) []domain.RowError {
	var errs []domain.RowError

	collect := func(e *domain.RowError) {
		if e != nil {
			errs = append(errs, *e)
		}
	}

	collect(ValidateRequired(sheet, rowNum, "brinco", GetCell(row, h, "brinco")))
	collect(ValidateRequired(sheet, rowNum, "dt_ordenha", GetCell(row, h, "dt_ordenha")))
	collect(ValidateDate(sheet, rowNum, "dt_ordenha", GetCell(row, h, "dt_ordenha"), false))
	collect(ValidateNumeric(sheet, rowNum, "qt_ordenha", GetCell(row, h, "qt_ordenha")))
	collect(ValidateMaxLength(sheet, rowNum, "periodo", GetCell(row, h, "periodo"), 1))
	collect(ValidateMaxLength(sheet, rowNum, "ocorrencia", GetCell(row, h, "ocorrencia"), 50))

	return errs
}

// ValidateReproducaoRow valida uma linha da aba "Reproducao" → tabela dadosreproducao.
func ValidateReproducaoRow(sheet string, rowNum int, row []string, h HeaderIndex) []domain.RowError {
	var errs []domain.RowError

	collect := func(e *domain.RowError) {
		if e != nil {
			errs = append(errs, *e)
		}
	}

	collect(ValidateRequired(sheet, rowNum, "brinco", GetCell(row, h, "brinco")))
	collect(ValidateRequired(sheet, rowNum, "dt_evento", GetCell(row, h, "dt_evento")))
	collect(ValidateDate(sheet, rowNum, "dt_evento", GetCell(row, h, "dt_evento"), false))
	collect(ValidateMaxLength(sheet, rowNum, "tipo_inseminacao", GetCell(row, h, "tipo_inseminacao"), 50))
	collect(ValidateMaxLength(sheet, rowNum, "status", GetCell(row, h, "status"), 20))
	collect(ValidateMaxLength(sheet, rowNum, "tipo_parto", GetCell(row, h, "tipo_parto"), 20))
	collect(ValidateMaxLength(sheet, rowNum, "ocorrencia", GetCell(row, h, "ocorrencia"), 255))

	return errs
}

// RowValidator é a assinatura de uma função de validação de linha.
// Recebe o nome da aba, o número da linha, os dados da linha e o header index.
type RowValidator func(sheet string, rowNum int, row []string, h HeaderIndex) []domain.RowError

// ValidatorForTable retorna o validador adequado para uma tabela PostgreSQL.
func ValidatorForTable(table string) RowValidator {
	switch table {
	case "bufalo":
		return ValidateBufaloRow
	case "dadoszootecnicos":
		return ValidateZootecnicoRow
	case "dadossanitarios":
		return ValidateSanitarioRow
	case "dadoslactacao":
		return ValidateLactacaoRow
	case "dadosreproducao":
		return ValidateReproducaoRow
	default:
		return nil
	}
}
