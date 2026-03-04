// Package repository implementa os adapters de acesso a dados (PostgreSQL).
// Segue o padrão Ports & Adapters — implementa as interfaces definidas em port/.
package repository

import (
"context"
"fmt"
"log"
"time"

"github.com/jackc/pgx/v5/pgxpool"
"github.com/jaobarreto/buffs-etl-worker/internal/domain"
"github.com/jaobarreto/buffs-etl-worker/internal/port"
)

// Garante em tempo de compilação que PostgresBrincoLoader implementa port.BrincoLoader.
var _ port.BrincoLoader = (*PostgresBrincoLoader)(nil)

// PostgresBrincoLoader carrega o mapa de brincos do PostgreSQL via pgxpool.
type PostgresBrincoLoader struct {
	pool *pgxpool.Pool
}

// NewPostgresBrincoLoader cria uma instância com o connection pool fornecido.
func NewPostgresBrincoLoader(pool *pgxpool.Pool) *PostgresBrincoLoader {
	return &PostgresBrincoLoader{pool: pool}
}

// LoadBrincoMap faz SELECT id_bufalo, brinco FROM bufalo WHERE id_propriedade = $1
// e retorna um map[brinco]→id_bufalo para resolução O(1) de FKs.
//
// Filtra apenas animais ativos (deleted_at IS NULL) e com brinco preenchido.
// Usa o índice idx_bufalo_propriedade para performance.
func (r *PostgresBrincoLoader) LoadBrincoMap(ctx context.Context, propriedadeID string) (domain.BrincoLookup, error) {
	start := time.Now()

	query := `
		SELECT id_bufalo, brinco
		FROM bufalo
		WHERE id_propriedade = $1
		  AND deleted_at IS NULL
		  AND brinco IS NOT NULL
		  AND brinco <> ''
	`

	rows, err := r.pool.Query(ctx, query, propriedadeID)
	if err != nil {
		return nil, fmt.Errorf("erro ao carregar brincos da propriedade %s: %w", propriedadeID, err)
	}
	defer rows.Close()

	lookup := make(domain.BrincoLookup)
	for rows.Next() {
		var idBufalo, brinco string
		if err := rows.Scan(&idBufalo, &brinco); err != nil {
			return nil, fmt.Errorf("erro ao ler linha de brinco: %w", err)
		}
		lookup[brinco] = idBufalo
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erro iterando resultados de brincos: %w", err)
	}

	elapsed := time.Since(start)
	log.Printf("[ETL] Brinco lookup carregado: %d animais da propriedade %s em %v",
len(lookup), propriedadeID, elapsed)

	return lookup, nil
}
