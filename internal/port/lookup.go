package port

import (
	"context"

	"github.com/jaobarreto/buffs-etl-worker/internal/domain"
)

// BrincoLoader define a interface para carregar o mapa de brincos→UUID de uma propriedade.
//
// A implementação concreta consulta o PostgreSQL.
// Em testes, pode ser substituída por um mock com mapa estático.
type BrincoLoader interface {
	// LoadBrincoMap carrega todos os brincos e seus UUIDs para uma propriedade.
	// Retorna um mapa brinco→id_bufalo para resolução O(1) durante o ETL.
	LoadBrincoMap(ctx context.Context, propriedadeID string) (domain.BrincoLookup, error)
}
