package domain

import "time"

// WeightRecord representa um registro de pesagem do animal (tabela dadoszootecnicos).
type WeightRecord struct {
	IDBufalo         string    // FK → bufalo.id_bufalo (resolvido de brinco)
	IDUsuario        string    // FK → usuario.id_usuario (do JWT)
	Peso             float64   // Peso em kg
	CondicaoCorporal *float64  // BCS 1.0-5.0 (nullable)
	DtRegistro       time.Time // Data da pesagem
	TipoPesagem      string    // Balança / Fita / Estimativa Visual
}

// Table retorna o nome da tabela no PostgreSQL.
func (WeightRecord) Table() string { return "dadoszootecnicos" }

// Columns retorna as colunas para COPY INTO.
func (WeightRecord) Columns() []string {
	return []string{
		"id_bufalo", "id_usuario",
		"peso", "condicao_corporal", "dt_registro", "tipo_pesagem",
	}
}

// Values retorna os valores na mesma ordem de Columns().
func (r WeightRecord) Values() []any {
	return []any{
		r.IDBufalo, r.IDUsuario,
		r.Peso, r.CondicaoCorporal, r.DtRegistro, nullIfEmptyStr(r.TipoPesagem),
	}
}

func nullIfEmptyStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
