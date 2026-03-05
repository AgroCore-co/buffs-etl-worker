// Package validator implementa validação de dados por linha do Excel.
// Cada regra retorna RowError (linha não importada) ou RowWarning (importa com flag).
package validator

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jaobarreto/buffs-etl-worker/internal/domain"
	"github.com/jaobarreto/buffs-etl-worker/internal/mapper"
	"github.com/jaobarreto/buffs-etl-worker/pkg/apperror"
)

// ValidatedRow contém uma linha validada pronta para transformação.
type ValidatedRow struct {
	Row    int               // número da linha no Excel (1-based)
	Fields map[string]string // db_column → valor string
}

// ValidationResult contém os resultados da validação de todas as linhas.
type ValidationResult struct {
	Valid    []ValidatedRow
	Errors   []*apperror.RowError
	Warnings []*apperror.RowError
}

// Validate valida todas as linhas mapeadas de acordo com o tipo de pipeline.
func Validate(
	rows []map[string]string,
	startRow int, // número da primeira linha de dados no Excel (geralmente 2)
	pt mapper.PipelineType,
	lookup domain.BrincoLookup,
) *ValidationResult {
	result := &ValidationResult{}

	_, required := mapper.GetMapForPipeline(pt)
	rules := getRules(pt)

	for i, row := range rows {
		excelRow := startRow + i
		rowErrors, rowWarnings := validateRow(excelRow, row, required, rules, lookup)

		if len(rowErrors) > 0 {
			result.Errors = append(result.Errors, rowErrors...)
		} else {
			result.Valid = append(result.Valid, ValidatedRow{Row: excelRow, Fields: row})
		}
		result.Warnings = append(result.Warnings, rowWarnings...)
	}

	return result
}

// ── Regras ──────────────────────────────────────────────────────────────────

// FieldRule define uma regra de validação para um campo.
type FieldRule struct {
	Field        string // db_column
	Required     bool
	Type         string  // "string", "date", "decimal", "integer", "enum"
	NoFuture     bool    // data não pode ser futura
	Min          float64 // valor numérico mínimo
	Max          float64 // valor numérico máximo (0 = sem limite)
	SuspectAbove float64 // valor acima disso gera warning
	SuspectBelow float64 // valor abaixo disso gera warning
	EnumValues   []string
	MaxLen       int
	NeedsLookup  bool // campo depende de resolução de brinco
	IsFemale     bool // brinco deve ser de fêmea
}

func getRules(pt mapper.PipelineType) []FieldRule {
	switch pt {
	case mapper.PipelineMilk:
		return milkRules
	case mapper.PipelineWeight:
		return weightRules
	case mapper.PipelineReproduction:
		return reproductionRules
	default:
		return nil
	}
}

var milkRules = []FieldRule{
	{Field: "id_bufala", Required: true, Type: "string", NeedsLookup: true},
	{Field: "dt_ordenha", Required: true, Type: "date", NoFuture: true},
	{Field: "qt_ordenha", Required: true, Type: "decimal", Min: 0, SuspectAbove: 60},
	{Field: "periodo", Type: "enum", EnumValues: []string{"M", "T", "U", "AM", "PM"}},
	{Field: "ocorrencia", Type: "string", MaxLen: 50},
}

var weightRules = []FieldRule{
	{Field: "id_bufalo", Required: true, Type: "string", NeedsLookup: true},
	{Field: "dt_registro", Required: true, Type: "date", NoFuture: true},
	{Field: "peso", Required: true, Type: "decimal", Min: 0.1, SuspectAbove: 1500},
	{Field: "tipo_pesagem", Type: "enum", EnumValues: []string{"Balança", "Balanca", "Fita", "Estimativa Visual", "Estimativa"}},
	{Field: "condicao_corporal", Type: "decimal", Min: 1.0, Max: 5.0},
}

var reproductionRules = []FieldRule{
	{Field: "id_bufala", Required: true, Type: "string", NeedsLookup: true, IsFemale: true},
	{Field: "id_bufalo", Type: "string", NeedsLookup: true},
	{Field: "dt_evento", Required: true, Type: "date", NoFuture: true},
	{Field: "tipo_inseminacao", Required: true, Type: "enum", EnumValues: []string{"MN", "IA", "IATF", "TE"}},
	{Field: "status", Type: "enum", EnumValues: []string{"Positivo", "Negativo", "Pendente", "Inconclusivo"}},
	{Field: "ocorrencia", Type: "string", MaxLen: 255},
}

// ── Engine de validação ─────────────────────────────────────────────────────

func validateRow(
	row int,
	fields map[string]string,
	required []string,
	rules []FieldRule,
	lookup domain.BrincoLookup,
) (errors []*apperror.RowError, warnings []*apperror.RowError) {
	for _, rule := range rules {
		val, exists := fields[rule.Field]

		// Campos obrigatórios
		if rule.Required && (!exists || val == "") {
			errors = append(errors, apperror.NewRowError(
				row, rule.Field, "", "Campo obrigatório não preenchido", apperror.CodeRequiredField,
			))
			continue
		}

		if !exists || val == "" {
			continue // campo opcional vazio → ok
		}

		// Resolução de brinco
		if rule.NeedsLookup && lookup != nil {
			animal, found := lookup[val]
			if !found {
				errors = append(errors, apperror.NewRowError(
					row, rule.Field, val, "Brinco não encontrado na propriedade", apperror.CodeBrincoNotFound,
				))
				continue
			}
			if rule.IsFemale && animal != nil && animal.Sexo != "" && strings.ToUpper(animal.Sexo) != "F" {
				errors = append(errors, apperror.NewRowError(
					row, rule.Field, val, "Brinco deve ser de uma fêmea", apperror.CodeInvalidValue,
				))
				continue
			}
		}

		// Validação por tipo
		switch rule.Type {
		case "date":
			errs, warns := validateDate(row, rule, val)
			errors = append(errors, errs...)
			warnings = append(warnings, warns...)
		case "decimal":
			errs, warns := validateDecimal(row, rule, val)
			errors = append(errors, errs...)
			warnings = append(warnings, warns...)
		case "integer":
			errs, warns := validateInteger(row, rule, val)
			errors = append(errors, errs...)
			warnings = append(warnings, warns...)
		case "enum":
			errs, _ := validateEnum(row, rule, val)
			errors = append(errors, errs...)
		case "string":
			if rule.MaxLen > 0 && len(val) > rule.MaxLen {
				errors = append(errors, apperror.NewRowError(
					row, rule.Field, val,
					fmt.Sprintf("Texto excede o limite de %d caracteres", rule.MaxLen),
					apperror.CodeInvalidValue,
				))
			}
		}
	}

	return errors, warnings
}

// ── Validadores de tipo ─────────────────────────────────────────────────────

var dateFormats = []string{
	"02/01/2006",       // DD/MM/YYYY
	"2/1/2006",         // D/M/YYYY
	"2006-01-02",       // YYYY-MM-DD (ISO)
	"02-01-2006",       // DD-MM-YYYY
	"02/01/2006 15:04", // DD/MM/YYYY HH:MM
}

func validateDate(row int, rule FieldRule, val string) (errors, warnings []*apperror.RowError) {
	var parsed time.Time
	var err error

	for _, fmt := range dateFormats {
		parsed, err = time.Parse(fmt, strings.TrimSpace(val))
		if err == nil {
			break
		}
	}

	if err != nil {
		errors = append(errors, apperror.NewRowError(
			row, rule.Field, val, "Data em formato inválido (esperado DD/MM/AAAA)", apperror.CodeInvalidDate,
		))
		return
	}

	if rule.NoFuture && parsed.After(time.Now()) {
		errors = append(errors, apperror.NewRowError(
			row, rule.Field, val, "Data não pode ser futura", apperror.CodeInvalidDate,
		))
	}

	return
}

func validateDecimal(row int, rule FieldRule, val string) (errors, warnings []*apperror.RowError) {
	// Aceita vírgula como separador decimal
	normalized := strings.Replace(val, ",", ".", 1)
	f, err := strconv.ParseFloat(normalized, 64)
	if err != nil {
		errors = append(errors, apperror.NewRowError(
			row, rule.Field, val, "Valor numérico inválido", apperror.CodeInvalidValue,
		))
		return
	}

	if f < rule.Min {
		errors = append(errors, apperror.NewRowError(
			row, rule.Field, val,
			fmt.Sprintf("Valor deve ser >= %.2f", rule.Min),
			apperror.CodeInvalidValue,
		))
		return
	}

	if rule.Max > 0 && f > rule.Max {
		errors = append(errors, apperror.NewRowError(
			row, rule.Field, val,
			fmt.Sprintf("Valor deve ser <= %.2f", rule.Max),
			apperror.CodeInvalidValue,
		))
		return
	}

	if rule.SuspectAbove > 0 && f > rule.SuspectAbove {
		warnings = append(warnings, apperror.NewRowWarning(
			row, rule.Field, val,
			fmt.Sprintf("Valor acima do esperado (> %.0f)", rule.SuspectAbove),
		))
	}

	return
}

func validateInteger(row int, rule FieldRule, val string) (errors, warnings []*apperror.RowError) {
	i, err := strconv.Atoi(strings.TrimSpace(val))
	if err != nil {
		errors = append(errors, apperror.NewRowError(
			row, rule.Field, val, "Valor inteiro inválido", apperror.CodeInvalidValue,
		))
		return
	}

	f := float64(i)
	if f < rule.Min {
		errors = append(errors, apperror.NewRowError(
			row, rule.Field, val,
			fmt.Sprintf("Valor deve ser >= %.0f", rule.Min),
			apperror.CodeInvalidValue,
		))
	}

	if rule.Max > 0 && f > rule.Max {
		errors = append(errors, apperror.NewRowError(
			row, rule.Field, val,
			fmt.Sprintf("Valor deve ser <= %.0f", rule.Max),
			apperror.CodeInvalidValue,
		))
	}

	return
}

func validateEnum(row int, rule FieldRule, val string) (errors, warnings []*apperror.RowError) {
	upper := strings.ToUpper(strings.TrimSpace(val))
	for _, allowed := range rule.EnumValues {
		if strings.ToUpper(allowed) == upper {
			return
		}
	}

	errors = append(errors, apperror.NewRowError(
		row, rule.Field, val,
		fmt.Sprintf("Valor deve ser um de: %s", strings.Join(rule.EnumValues, ", ")),
		apperror.CodeInvalidEnum,
	))
	return
}
