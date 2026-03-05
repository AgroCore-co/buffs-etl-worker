package domain
package domain

// Record é a interface comum para todos os tipos de registro do ETL.
// Permite que o loader e o pipeline trabalhem de forma genérica.
type Record interface {
	Table() string
	Columns() []string
	Values() []any
}
































type BrincoLookup map[string]*Animal// Carregado uma única vez por request de import, a partir do PostgreSQL.// BrincoLookup mapeia brinco (string) → Animal.}	IDPropriedade   string // FK → propriedade.id_propriedade	IDGrupo         string // FK → grupo.id_grupo	NivelMaturidade string // nível de maturidade	Sexo            string // M ou F	Brinco          string // identificador visual	ID              string // id_bufalo (UUID)type Animal struct {// Animal contém os dados de um búfalo para lookup.}	Message string `json:"message"`	Value   string `json:"value"`	Field   string `json:"field"`	Row     int    `json:"row"`type RowIssue struct {// RowIssue representa um erro ou warning de uma linha do Excel (para serialização JSON).}	Warnings []RowIssue  `json:"warnings"`	Errors   []RowIssue  `json:"errors"`	Skipped  int         `json:"skipped"`	Imported int         `json:"imported"`	Total    int         `json:"total_rows"`	JobID    string      `json:"job_id,omitempty"`type ImportResult struct {// ImportResult é o resultado final do processamento de um arquivo Excel.