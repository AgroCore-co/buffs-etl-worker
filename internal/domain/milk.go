package domain

import "time"

// MilkRecord representa um registro de pesagem de leite (tabela dadoslactacao).
type MilkRecord struct {
	IDBufala        string    // FK → bufalo.id_bufalo (resolvido de brinco)
	IDUsuario       string    // FK → usuario.id_usuario
	IDPropriedade   string    // FK → propriedade.id_propriedade
	IDCicloLactacao string    // FK → ciclolactacao.id_ciclo_lactacao (opcional)
	QtOrdenha       float64   // Quantidade produzida em litros
	Periodo         string    // M=Manhã, T=Tarde, U=Único
	Ocorrencia      string    // Observação livre (máx. 50 chars)
	DtOrdenha       time.Time // Data da ordenha
}

// Table retorna o nome da tabela no PostgreSQL.
func (MilkRecord) Table() string { return "dadoslactacao" }

// Columns retorna as colunas para COPY INTO (sem id e timestamps).
func (MilkRecord) Columns() []string {
	return []string{
		"id_bufala", "id_usuario", "id_propriedade",
		"qt_ordenha", "periodo", "ocorrencia", "dt_ordenha",
	}
}

// Values retorna os valores na mesma ordem de Columns().
func (r MilkRecord) Values() []any {
	return []any{
		r.IDBufala, r.IDUsuario, r.IDPropriedade,
		r.QtOrdenha, r.Periodo, nullIfEmpty(r.Ocorrencia), r.DtOrdenha,
	}
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
