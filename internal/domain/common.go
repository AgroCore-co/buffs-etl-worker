// Package domain contém as entidades centrais de negócio do ETL BUFFS.
// Structs puras sem dependência de infraestrutura.
package domain

// Record é a interface comum para todos os tipos de registro do ETL.
// Permite que o loader e o pipeline trabalhem de forma genérica.
type Record interface {
	Table() string
	Columns() []string
	Values() []any
}

// ImportResult é o resultado final do processamento de um arquivo Excel.
type ImportResult struct {
	JobID    string     `json:"job_id,omitempty"`
	Total    int        `json:"total_rows"`
	Imported int        `json:"imported"`
	Skipped  int        `json:"skipped"`
	Errors   []RowIssue `json:"errors"`
	Warnings []RowIssue `json:"warnings"`
}

// RowIssue representa um erro ou warning de uma linha do Excel (para serialização JSON).
type RowIssue struct {
	Row     int    `json:"row"`
	Field   string `json:"field"`
	Value   string `json:"value"`
	Message string `json:"message"`
}

// Animal contém os dados de um búfalo para lookup.
type Animal struct {
	ID              string // id_bufalo (UUID)
	Brinco          string // identificador visual
	Sexo            string // M ou F
	NivelMaturidade string // nível de maturidade
	IDGrupo         string // FK → grupo.id_grupo
	IDPropriedade   string // FK → propriedade.id_propriedade
}

// BrincoLookup mapeia brinco (string) → Animal.
// Carregado uma única vez por request de import, a partir do PostgreSQL.
type BrincoLookup map[string]*Animal
