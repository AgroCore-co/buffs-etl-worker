// Package loader implementa bulk insert no PostgreSQL usando o protocolo COPY.
// Nunca faz INSERT linha a linha — usa exclusivamente pgx.CopyFrom.
package loader

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jaobarreto/buffs-etl-worker/internal/domain"
	"go.uber.org/zap"
)

// PostgresLoader realiza bulk insert via protocolo COPY do PostgreSQL.
type PostgresLoader struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

// NewPostgresLoader cria um loader com connection pool e logger.
func NewPostgresLoader(pool *pgxpool.Pool, logger *zap.Logger) *PostgresLoader {
	return &PostgresLoader{pool: pool, logger: logger}
}

// Load insere registros em batch via COPY dentro de uma transação.
// Retorna o número de linhas inseridas com sucesso.
func (l *PostgresLoader) Load(ctx context.Context, records []domain.Record) (int64, error) {
	if len(records) == 0 {
		return 0, nil
	}

	first := records[0]
	table := first.Table()
	columns := first.Columns()

	l.logger.Info("Iniciando bulk insert via COPY",
		zap.String("table", table),
		zap.Int("records", len(records)),
		zap.Strings("columns", columns),
	)

	// Inicia transação
	tx, err := l.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("falha ao iniciar transação: %w", err)
	}
	defer tx.Rollback(ctx)

	// Prepara rows para COPY
	rows := make([][]any, 0, len(records))
	for _, rec := range records {
		rows = append(rows, rec.Values())
	}

	// Executa COPY INTO
	count, err := tx.CopyFrom(
		ctx,
		pgx.Identifier{table},
		columns,
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		l.logger.Error("Falha no COPY INTO",
			zap.String("table", table),
			zap.Error(err),
		)
		return 0, fmt.Errorf("falha no COPY INTO %s: %w", table, err)
	}

	// Commit da transação
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("falha no commit: %w", err)
	}

	l.logger.Info("Bulk insert concluído",
		zap.String("table", table),
		zap.Int64("inserted", count),
	)

	return count, nil
}

// LoadBrincoMap carrega todos os brincos e seus dados para uma propriedade.
// Retorna um map[brinco]→*Animal para resolução O(1) durante o ETL.
func (l *PostgresLoader) LoadBrincoMap(ctx context.Context, propertyID string) (domain.BrincoLookup, error) {
	l.logger.Debug("Carregando mapa de brincos", zap.String("property_id", propertyID))

	query := `
		SELECT id_bufalo, brinco, sexo, nivel_maturidade, id_grupo, id_propriedade
		FROM bufalo
		WHERE id_propriedade = $1
		  AND deleted_at IS NULL
		  AND brinco IS NOT NULL
		  AND brinco != ''
	`

	rows, err := l.pool.Query(ctx, query, propertyID)
	if err != nil {
		return nil, fmt.Errorf("falha ao carregar brincos: %w", err)
	}
	defer rows.Close()

	lookup := make(domain.BrincoLookup)
	for rows.Next() {
		var a domain.Animal
		var sexo, maturidade, grupo *string

		if err := rows.Scan(&a.ID, &a.Brinco, &sexo, &maturidade, &grupo, &a.IDPropriedade); err != nil {
			return nil, fmt.Errorf("falha ao escanear brinco: %w", err)
		}

		if sexo != nil {
			a.Sexo = *sexo
		}
		if maturidade != nil {
			a.NivelMaturidade = *maturidade
		}
		if grupo != nil {
			a.IDGrupo = *grupo
		}

		lookup[a.Brinco] = &a
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erro durante iteração dos brincos: %w", err)
	}

	l.logger.Info("Mapa de brincos carregado",
		zap.String("property_id", propertyID),
		zap.Int("total_animals", len(lookup)),
	)

	return lookup, nil
}

// CheckPropertyAccess verifica se o usuário tem acesso à propriedade.
// Retorna true se o usuário é dono ou está na tabela usuariopropriedade.
func (l *PostgresLoader) CheckPropertyAccess(ctx context.Context, userAuthID, propertyID string) (bool, string, error) {
	// Primeiro resolve auth_id → id_usuario
	var userID string
	err := l.pool.QueryRow(ctx,
		`SELECT id_usuario FROM usuario WHERE auth_id = $1`, userAuthID,
	).Scan(&userID)
	if err != nil {
		return false, "", nil // usuário não encontrado
	}

	// Verifica se é dono
	var count int
	err = l.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM propriedade WHERE id_propriedade = $1 AND id_dono = $2 AND deleted_at IS NULL`,
		propertyID, userID,
	).Scan(&count)
	if err != nil {
		return false, "", err
	}
	if count > 0 {
		return true, userID, nil
	}

	// Verifica tabela de associação
	err = l.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM usuariopropriedade WHERE id_propriedade = $1 AND id_usuario = $2`,
		propertyID, userID,
	).Scan(&count)
	if err != nil {
		return false, "", err
	}

	return count > 0, userID, nil
}
