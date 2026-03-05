package domain

import "time"

// ReproductionRecord representa um registro reprodutivo (tabela dadosreproducao).
type ReproductionRecord struct {
	IDBufala        string    // FK → bufalo.id_bufalo (fêmea, resolvido de brinco)
	IDBufalo        string    // FK → bufalo.id_bufalo (macho, resolvido de brinco, nullable)
	IDSemen         string    // FK → materialgenetico.id_material (nullable)
	IDOvulo         string    // FK → materialgenetico.id_material (nullable)
	TipoInseminacao string    // MN / IA / IATF / TE
	Status          string    // Positivo / Negativo / Pendente / Inconclusivo
	DtEvento        time.Time // Data do evento reprodutivo
	Ocorrencia      string    // Observação (máx. 255 chars)
	IDPropriedade   string    // FK → propriedade.id_propriedade
}

// Table retorna o nome da tabela no PostgreSQL.
func (ReproductionRecord) Table() string { return "dadosreproducao" }

// Columns retorna as colunas para COPY INTO.
func (ReproductionRecord) Columns() []string {
	return []string{
		"id_bufala", "id_bufalo", "id_semen", "id_ovulo",
		"tipo_inseminacao", "status", "dt_evento", "ocorrencia", "id_propriedade",
	}
}

// Values retorna os valores na mesma ordem de Columns().
func (r ReproductionRecord) Values() []any {
	return []any{
		r.IDBufala, nilIfEmpty(r.IDBufalo), nilIfEmpty(r.IDSemen), nilIfEmpty(r.IDOvulo),
		r.TipoInseminacao, nilIfEmpty(r.Status), r.DtEvento,
		nilIfEmpty(r.Ocorrencia), r.IDPropriedade,
	}
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
