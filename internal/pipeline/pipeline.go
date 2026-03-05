// Package pipeline orquestra o fluxo ETL: Extract → Map → Validate → Transform → Load.
// Cada pipeline de domínio (milk, weight, reproduction) reutiliza a mesma engine.
package pipeline

import (
	"context"
	"fmt"

	"github.com/jaobarreto/buffs-etl-worker/internal/domain"
	"github.com/jaobarreto/buffs-etl-worker/internal/extractor"
	"github.com/jaobarreto/buffs-etl-worker/internal/loader"
	"github.com/jaobarreto/buffs-etl-worker/internal/mapper"
	"github.com/jaobarreto/buffs-etl-worker/internal/transformer"
	"github.com/jaobarreto/buffs-etl-worker/internal/validator"
	"github.com/jaobarreto/buffs-etl-worker/pkg/apperror"
	"go.uber.org/zap"
)

// Pipeline orquestra o processamento completo de um arquivo Excel.
type Pipeline struct {
	loader *loader.PostgresLoader
	logger *zap.Logger
}

// New cria uma nova instância do pipeline.
func New(loader *loader.PostgresLoader, logger *zap.Logger) *Pipeline {
	return &Pipeline{loader: loader, logger: logger}
}

// RunParams são os parâmetros para executar um pipeline.
type RunParams struct {
	FilePath   string
	PropertyID string
	UserID     string
	Type       mapper.PipelineType
}

// Run executa o pipeline ETL completo e retorna o resultado.
func (p *Pipeline) Run(ctx context.Context, params RunParams) (*domain.ImportResult, error) {
	p.logger.Info("Iniciando pipeline ETL",
		zap.String("type", string(params.Type)),
		zap.String("file", params.FilePath),
		zap.String("property_id", params.PropertyID),
	)

	result := &domain.ImportResult{}

	// ── 1. Extract ──────────────────────────────────────────────────────
	data, err := extractor.Extract(params.FilePath)
	if err != nil {
		return nil, err
	}
	result.Total = len(data.Rows)

	p.logger.Info("Extração concluída",
		zap.Int("total_rows", result.Total),
		zap.Int("header_cols", len(data.Headers)),
	)

	// ── 2. Map Headers ──────────────────────────────────────────────────
	colMap, required := mapper.GetMapForPipeline(params.Type)
	headerMap, missing := mapper.MapHeaders(data.Headers, colMap, required)

	if len(missing) > 0 {
		return nil, apperror.BadRequest(apperror.CodeMissingColumn,
			fmt.Sprintf("Colunas obrigatórias ausentes: %v", missing))
	}

	// Mapeia todas as linhas
	mappedRows := make([]map[string]string, 0, len(data.Rows))
	for _, row := range data.Rows {
		mapped := mapper.MapRow(row, headerMap)
		if len(mapped) > 0 { // ignora linhas completamente vazias
			mappedRows = append(mappedRows, mapped)
		}
	}

	p.logger.Info("Mapeamento concluído",
		zap.Int("mapped_rows", len(mappedRows)),
	)

	// ── 3. Load Brinco Lookup ───────────────────────────────────────────
	lookup, err := p.loader.LoadBrincoMap(ctx, params.PropertyID)
	if err != nil {
		return nil, apperror.Internal("Falha ao carregar mapa de brincos", err)
	}

	// ── 4. Validate ─────────────────────────────────────────────────────
	valResult := validator.Validate(mappedRows, 2, params.Type, lookup)

	for _, e := range valResult.Errors {
		result.Errors = append(result.Errors, domain.RowIssue{
			Row: e.Row, Field: e.Field, Value: e.Value, Message: e.Message,
		})
	}
	for _, w := range valResult.Warnings {
		result.Warnings = append(result.Warnings, domain.RowIssue{
			Row: w.Row, Field: w.Field, Value: w.Value, Message: w.Message,
		})
	}

	result.Skipped = len(valResult.Errors)

	p.logger.Info("Validação concluída",
		zap.Int("valid", len(valResult.Valid)),
		zap.Int("errors", len(valResult.Errors)),
		zap.Int("warnings", len(valResult.Warnings)),
	)

	if len(valResult.Valid) == 0 {
		result.Imported = 0
		return result, nil
	}

	// ── 5. Transform ────────────────────────────────────────────────────
	records := transformer.Transform(valResult.Valid, params.Type, lookup, params.UserID, params.PropertyID)

	p.logger.Info("Transformação concluída", zap.Int("records", len(records)))

	// ── 6. Load ─────────────────────────────────────────────────────────
	count, err := p.loader.Load(ctx, records)
	if err != nil {
		return nil, apperror.Internal("Falha ao inserir dados no banco", err)
	}

	result.Imported = int(count)

	p.logger.Info("Pipeline ETL concluído",
		zap.String("type", string(params.Type)),
		zap.Int("total", result.Total),
		zap.Int("imported", result.Imported),
		zap.Int("skipped", result.Skipped),
	)

	return result, nil
}
