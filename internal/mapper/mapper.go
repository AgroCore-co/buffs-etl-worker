// Package mapper normaliza cabeçalhos de planilha Excel para nomes internos do sistema.
// Tolerante a variações de case, espaços e acentuação.
package mapper

import (
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// FieldMapping define como um campo do Excel mapeia para uma coluna do banco.
type FieldMapping struct {
	DBColumn    string // nome da coluna no PostgreSQL
	NeedsLookup bool   // true se precisa resolver brinco→UUID
	IsFemale    bool   // true se o brinco deve ser de uma fêmea (para reprodução)
}

// ColumnMap mapeia nome normalizado do cabeçalho Excel → FieldMapping.
type ColumnMap map[string]FieldMapping

// PipelineType representa o tipo de pipeline (domínio).
type PipelineType string

const (
	PipelineMilk         PipelineType = "leite"
	PipelineWeight       PipelineType = "pesagem"
	PipelineReproduction PipelineType = "reproducao"
)

// ── Registries ──────────────────────────────────────────────────────────────

// MilkColumnMap define o mapeamento de colunas da planilha de leite.
var MilkColumnMap = ColumnMap{
	"brinco":            {DBColumn: "id_bufala", NeedsLookup: true},
	"data":              {DBColumn: "dt_ordenha"},
	"qtd produzida (l)": {DBColumn: "qt_ordenha"},
	"qtd produzida":     {DBColumn: "qt_ordenha"},
	"quantidade (l)":    {DBColumn: "qt_ordenha"},
	"litros":            {DBColumn: "qt_ordenha"},
	"turno":             {DBColumn: "periodo"},
	"periodo":           {DBColumn: "periodo"},
	"escore ccs":        {DBColumn: "_escore_ccs"}, // campo sem coluna no DB — ignorado no loader
	"ccs":               {DBColumn: "_escore_ccs"},
	"observacao":        {DBColumn: "ocorrencia"},
	"observação":        {DBColumn: "ocorrencia"},
	"obs":               {DBColumn: "ocorrencia"},
}

// MilkRequiredFields são os campos obrigatórios.
var MilkRequiredFields = []string{"id_bufala", "dt_ordenha", "qt_ordenha"}

// WeightColumnMap define o mapeamento de colunas da planilha de pesagem.
var WeightColumnMap = ColumnMap{
	"brinco":                {DBColumn: "id_bufalo", NeedsLookup: true},
	"data":                  {DBColumn: "dt_registro"},
	"peso (kg)":             {DBColumn: "peso"},
	"peso":                  {DBColumn: "peso"},
	"metodo":                {DBColumn: "tipo_pesagem"},
	"método":                {DBColumn: "tipo_pesagem"},
	"escore corporal (bcs)": {DBColumn: "condicao_corporal"},
	"escore corporal":       {DBColumn: "condicao_corporal"},
	"bcs":                   {DBColumn: "condicao_corporal"},
	"condicao corporal":     {DBColumn: "condicao_corporal"},
	"gmd (kg/dia)":          {DBColumn: "_gmd"}, // calculado — ignorado no loader
	"gmd":                   {DBColumn: "_gmd"},
	"observacao":            {DBColumn: "_observacao"}, // sem coluna no dadoszootecnicos
	"observação":            {DBColumn: "_observacao"},
}

// WeightRequiredFields são os campos obrigatórios.
var WeightRequiredFields = []string{"id_bufalo", "dt_registro", "peso"}

// ReproductionColumnMap define o mapeamento de colunas da planilha de reprodução.
var ReproductionColumnMap = ColumnMap{
	"brinco femea":             {DBColumn: "id_bufala", NeedsLookup: true, IsFemale: true},
	"brinco fêmea":             {DBColumn: "id_bufala", NeedsLookup: true, IsFemale: true},
	"brinco macho":             {DBColumn: "id_bufalo", NeedsLookup: true},
	"data evento":              {DBColumn: "dt_evento"},
	"data":                     {DBColumn: "dt_evento"},
	"tipo":                     {DBColumn: "tipo_inseminacao"},
	"tipo inseminacao":         {DBColumn: "tipo_inseminacao"},
	"tipo inseminação":         {DBColumn: "tipo_inseminacao"},
	"cod material genetico":    {DBColumn: "id_semen"},
	"código material genético": {DBColumn: "id_semen"},
	"touro / doadora":          {DBColumn: "_touro_doadora"}, // sem coluna direta
	"touro/doadora":            {DBColumn: "_touro_doadora"},
	"resultado dg":             {DBColumn: "status"},
	"diagnostico gestacao":     {DBColumn: "status"},
	"data dg":                  {DBColumn: "_data_dg"},        // sem coluna no DB
	"previsao parto":           {DBColumn: "_previsao_parto"}, // sem coluna no DB
	"observacao":               {DBColumn: "ocorrencia"},
	"observação":               {DBColumn: "ocorrencia"},
}

// ReproductionRequiredFields são os campos obrigatórios.
var ReproductionRequiredFields = []string{"id_bufala", "dt_evento", "tipo_inseminacao"}

// ── Funções de mapeamento ──────────────────────────────────────────────────

// MapHeaders recebe os cabeçalhos do Excel e retorna o mapeamento
// coluna_index → FieldMapping. Retorna também a lista de campos obrigatórios faltantes.
func MapHeaders(headers []string, colMap ColumnMap, required []string) (map[int]FieldMapping, []string) {
	result := make(map[int]FieldMapping, len(headers))
	found := make(map[string]bool)

	for i, h := range headers {
		key := normalizeHeader(h)
		if fm, ok := colMap[key]; ok {
			result[i] = fm
			found[fm.DBColumn] = true
		}
	}

	var missing []string
	for _, req := range required {
		if !found[req] {
			missing = append(missing, req)
		}
	}

	return result, missing
}

// GetMapForPipeline retorna o ColumnMap e required fields para o tipo de pipeline.
func GetMapForPipeline(pt PipelineType) (ColumnMap, []string) {
	switch pt {
	case PipelineMilk:
		return MilkColumnMap, MilkRequiredFields
	case PipelineWeight:
		return WeightColumnMap, WeightRequiredFields
	case PipelineReproduction:
		return ReproductionColumnMap, ReproductionRequiredFields
	default:
		return nil, nil
	}
}

// normalizeHeader normaliza um cabeçalho: lowercase, remove acentos/pontuação, trim espaços.
func normalizeHeader(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ToLower(s)
	s = removeAccents(s)
	// Remove pontuação que não faz parte do conteúdo semântico (ex: "Qtd." → "Qtd")
	s = strings.ReplaceAll(s, ".", "")
	// Colapsa múltiplos espaços em um
	s = strings.Join(strings.Fields(s), " ")
	return s
}

// removeAccents remove acentos de uma string usando unicode NFD decomposition.
func removeAccents(s string) string {
	t := transform.Chain(
		norm.NFD,
		runes.Remove(runes.In(unicode.Mn)),
		norm.NFC,
	)
	result, _, err := transform.String(t, s)
	if err != nil {
		return s
	}
	return result
}

// MapRow aplica o mapeamento de headers a uma linha de dados.
// Retorna map[db_column]→valor para as colunas mapeadas.
func MapRow(row []string, headerMap map[int]FieldMapping) map[string]string {
	mapped := make(map[string]string, len(headerMap))
	for i, fm := range headerMap {
		if i < len(row) {
			val := strings.TrimSpace(row[i])
			if val != "" {
				mapped[fm.DBColumn] = val
			}
		}
	}
	return mapped
}
