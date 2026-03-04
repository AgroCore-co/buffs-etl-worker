package excel

import "strings"

// SheetConfig descreve a configuração de uma aba suportada.
type SheetConfig struct {
	// Table é o nome da tabela PostgreSQL de destino.
	Table string
	// RequiredFields são os campos obrigatórios no cabeçalho do Excel.
	RequiredFields []string
	// OptionalFields são campos aceitos mas não obrigatórios.
	OptionalFields []string
	// ColumnAliases mapeia nome_excel → nome_coluna_db para campos que diferem
	// entre a planilha e o banco. Caso clássico: o fazendeiro escreve "brinco"
	// no Excel, mas a tabela dependente possui "id_bufalo" ou "id_bufala" como FK.
	//
	// Se nil, os nomes do Excel são usados diretamente como nomes de coluna.
	// Exemplo: {"brinco": "id_bufalo"} — ao gerar o INSERT, "brinco" vira "id_bufalo"
	// e o valor é resolvido de string (ex: "B001") para UUID via BrincoLookup.
	ColumnAliases map[string]string
}

// SheetRegistry mapeia nome de aba (normalizado para lowercase) para configuração.
var SheetRegistry = map[string]SheetConfig{
	// ── Animais / Búfalos ────────────────────────────────────────────
	// Tabela principal: "brinco" é coluna real, sem alias.
	"animais": {
		Table:          "bufalo",
		RequiredFields: []string{"brinco"},
		OptionalFields: []string{
			"nome", "dt_nascimento", "sexo", "nivel_maturidade",
			"origem", "categoria", "microchip", "brinco_original",
			"registro_prov", "registro_def", "motivo_inativo",
		},
		// Sem ColumnAliases — brinco é coluna real na tabela bufalo.
	},
	"bufalos": {
		Table:          "bufalo",
		RequiredFields: []string{"brinco"},
		OptionalFields: []string{
			"nome", "dt_nascimento", "sexo", "nivel_maturidade",
			"origem", "categoria", "microchip", "brinco_original",
			"registro_prov", "registro_def", "motivo_inativo",
		},
	},
	// ── Pesagens / Dados Zootécnicos ─────────────────────────────────
	// FK: brinco → id_bufalo (dadoszootecnicos.id_bufalo)
	"pesagens": {
		Table:          "dadoszootecnicos",
		RequiredFields: []string{"brinco", "dt_registro"},
		OptionalFields: []string{
			"peso", "condicao_corporal", "cor_pelagem",
			"formato_chifre", "porte_corporal", "tipo_pesagem",
		},
		ColumnAliases: map[string]string{"brinco": "id_bufalo"},
	},
	"zootecnicos": {
		Table:          "dadoszootecnicos",
		RequiredFields: []string{"brinco", "dt_registro"},
		OptionalFields: []string{
			"peso", "condicao_corporal", "cor_pelagem",
			"formato_chifre", "porte_corporal", "tipo_pesagem",
		},
		ColumnAliases: map[string]string{"brinco": "id_bufalo"},
	},
	// ── Sanitário / Dados Sanitários ─────────────────────────────────
	// FK: brinco → id_bufalo (dadossanitarios.id_bufalo)
	"sanitario": {
		Table:          "dadossanitarios",
		RequiredFields: []string{"brinco", "dt_aplicacao"},
		OptionalFields: []string{
			"dosagem", "unidade_medida", "doenca",
			"necessita_retorno", "dt_retorno", "observacao",
		},
		ColumnAliases: map[string]string{"brinco": "id_bufalo"},
	},
	"vacinacao": {
		Table:          "dadossanitarios",
		RequiredFields: []string{"brinco", "dt_aplicacao"},
		OptionalFields: []string{
			"dosagem", "unidade_medida", "doenca",
			"necessita_retorno", "dt_retorno", "observacao",
		},
		ColumnAliases: map[string]string{"brinco": "id_bufalo"},
	},
	// ── Lactação / Dados de Lactação ─────────────────────────────────
	// FK: brinco → id_bufala (dadoslactacao.id_bufala — nota: "bufala", não "bufalo")
	"lactacao": {
		Table:          "dadoslactacao",
		RequiredFields: []string{"brinco", "dt_ordenha"},
		OptionalFields: []string{
			"qt_ordenha", "periodo", "ocorrencia",
		},
		ColumnAliases: map[string]string{"brinco": "id_bufala"},
	},
	"ordenha": {
		Table:          "dadoslactacao",
		RequiredFields: []string{"brinco", "dt_ordenha"},
		OptionalFields: []string{
			"qt_ordenha", "periodo", "ocorrencia",
		},
		ColumnAliases: map[string]string{"brinco": "id_bufala"},
	},
	// ── Reprodução / Dados Reprodutivos ──────────────────────────────
	// FK: brinco → id_bufala (dadosreproducao.id_bufala — animal registrado)
	"reproducao": {
		Table:          "dadosreproducao",
		RequiredFields: []string{"brinco", "dt_evento"},
		OptionalFields: []string{
			"tipo_inseminacao", "status", "tipo_parto", "ocorrencia",
		},
		ColumnAliases: map[string]string{"brinco": "id_bufala"},
	},
}

// LookupSheet busca a configuração de uma aba pelo nome (case-insensitive).
func LookupSheet(sheetName string) (SheetConfig, bool) {
	normalized := strings.ToLower(strings.TrimSpace(sheetName))
	cfg, ok := SheetRegistry[normalized]
	return cfg, ok
}

// NeedsLookup retorna true se a config da aba exige resolução de brinco → UUID.
func (cfg SheetConfig) NeedsLookup() bool {
	if cfg.ColumnAliases == nil {
		return false
	}
	_, has := cfg.ColumnAliases["brinco"]
	return has
}
