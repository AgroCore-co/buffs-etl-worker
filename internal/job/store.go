// Package job gerencia jobs assíncronos de importação com persistência em PostgreSQL.
package job

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jaobarreto/buffs-etl-worker/internal/domain"
	"github.com/jaobarreto/buffs-etl-worker/internal/mapper"
	"github.com/jaobarreto/buffs-etl-worker/internal/pipeline"
	"go.uber.org/zap"
)

// Status possíveis de um job.
const (
	StatusProcessing = "PROCESSING"
	StatusDone       = "DONE"
	StatusFailed     = "FAILED"
)

// Entry representa um job em execução ou concluído.
type Entry struct {
	FilePath   string
	Type       mapper.PipelineType
	PropertyID string
	UserID     string
	Status     string
	Result     *domain.ImportResult
	Error      string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Store gerencia jobs assíncronos com persistência em PostgreSQL.
type Store struct {
	db     *pgxpool.Pool
	logger *zap.Logger
}

// NewStore cria um novo store de jobs.

func NewStore(ctx context.Context, db *pgxpool.Pool, logger *zap.Logger) (*Store, error) {
	s := &Store{db: db, logger: logger}

	if err := s.ensureSchema(ctx); err != nil {
		return nil, err
	}

	if err := s.markStaleProcessingJobs(ctx); err != nil {
		return nil, err
	}

	return s, nil
}

// Create registra um novo job e retorna o ID.
func (s *Store) Create(filePath string, pType mapper.PipelineType, propertyID, userID string) (string, error) {
	jobID := fmt.Sprintf("job_%d_%d", time.Now().UnixMilli(), time.Now().UnixNano()%1_000_000)
	now := time.Now().UTC()

	_, err := s.db.Exec(context.Background(), `
		INSERT INTO etl_jobs (
			job_id, file_path, pipeline_type, property_id, user_id,
			status, result_json, error_message, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, NULL, NULL, $7, $7)
	`, jobID, filePath, string(pType), propertyID, userID, StatusProcessing, now)
	if err != nil {
		return "", err
	}

	return jobID, nil
}

// Get retorna o estado de um job.
func (s *Store) Get(id string) (*Entry, bool, error) {
	entry := &Entry{}
	var pipelineType string
	var resultJSON []byte
	var errorMessage *string

	err := s.db.QueryRow(context.Background(), `
		SELECT file_path, pipeline_type, property_id, user_id, status,
		       result_json, error_message, created_at, updated_at
		FROM etl_jobs
		WHERE job_id = $1
	`, id).Scan(
		&entry.FilePath,
		&pipelineType,
		&entry.PropertyID,
		&entry.UserID,
		&entry.Status,
		&resultJSON,
		&errorMessage,
		&entry.CreatedAt,
		&entry.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}

	entry.Type = mapper.PipelineType(pipelineType)
	if errorMessage != nil {
		entry.Error = *errorMessage
	}

	if len(resultJSON) > 0 {
		var result domain.ImportResult
		if err := json.Unmarshal(resultJSON, &result); err != nil {
			s.logger.Warn("Falha ao desserializar resultado de job", zap.String("job_id", id), zap.Error(err))
		} else {
			entry.Result = &result
		}
	}

	return entry, true, nil
}

// Process executa o pipeline ETL em background (chamado como goroutine).
func (s *Store) Process(jobID string, p *pipeline.Pipeline) {
	entry, ok, err := s.Get(jobID)
	if err != nil {
		s.logger.Error("Falha ao recuperar job para processamento", zap.String("job_id", jobID), zap.Error(err))
		_ = s.markFailed(jobID, "Falha interna ao recuperar job")
		return
	}

	if !ok {
		s.logger.Error("Job não encontrado para processamento", zap.String("job_id", jobID))
		return
	}

	s.logger.Info("Iniciando processamento assíncrono",
		zap.String("job_id", jobID),
		zap.String("type", string(entry.Type)),
		zap.String("file", entry.FilePath),
	)

	defer os.Remove(entry.FilePath)

	ctx := context.Background()
	result, err := p.Run(ctx, pipeline.RunParams{
		FilePath:   entry.FilePath,
		PropertyID: entry.PropertyID,
		UserID:     entry.UserID,
		Type:       entry.Type,
	})

	if err != nil {
		if markErr := s.markFailed(jobID, err.Error()); markErr != nil {
			s.logger.Error("Falha ao persistir erro de job", zap.String("job_id", jobID), zap.Error(markErr))
		}
		s.logger.Error("Job falhou",
			zap.String("job_id", jobID),
			zap.Error(err),
		)
		return
	}

	if markErr := s.markDone(jobID, result); markErr != nil {
		s.logger.Error("Falha ao persistir conclusão de job", zap.String("job_id", jobID), zap.Error(markErr))
		return
	}

	s.logger.Info("Job concluído",
		zap.String("job_id", jobID),
		zap.Int("imported", result.Imported),
		zap.Int("errors", len(result.Errors)),
	)
}

// Cleanup remove jobs mais antigos que a duração especificada.
// Pode ser chamado periodicamente para evitar crescimento indefinido da tabela.
func (s *Store) Cleanup(maxAge time.Duration) {
	cutoff := time.Now().Add(-maxAge)
	_, err := s.db.Exec(context.Background(), `
		DELETE FROM etl_jobs
		WHERE created_at < $1 AND status <> $2
	`, cutoff, StatusProcessing)
	if err != nil {
		s.logger.Warn("Falha no cleanup de jobs", zap.Error(err))
	}
}

func (s *Store) ensureSchema(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS etl_jobs (
			job_id TEXT PRIMARY KEY,
			file_path TEXT NOT NULL,
			pipeline_type TEXT NOT NULL,
			property_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			status TEXT NOT NULL,
			result_json JSONB,
			error_message TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_etl_jobs_status_created_at
		ON etl_jobs (status, created_at DESC)
	`)
	return err
}

func (s *Store) markStaleProcessingJobs(ctx context.Context) error {
	cmd, err := s.db.Exec(ctx, `
		UPDATE etl_jobs
		SET status = $1,
			error_message = $2,
			updated_at = NOW()
		WHERE status = $3
	`, StatusFailed, "Processamento interrompido por reinício do worker", StatusProcessing)
	if err != nil {
		return err
	}

	if cmd.RowsAffected() > 0 {
		s.logger.Warn("Jobs em processamento foram marcados como FAILED após reinício",
			zap.Int64("jobs_atualizados", cmd.RowsAffected()),
		)
	}

	return nil
}

func (s *Store) markDone(jobID string, result *domain.ImportResult) error {
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(context.Background(), `
		UPDATE etl_jobs
		SET status = $2,
			result_json = $3,
			error_message = NULL,
			updated_at = NOW()
		WHERE job_id = $1
	`, jobID, StatusDone, resultJSON)
	return err
}

func (s *Store) markFailed(jobID, errMsg string) error {
	_, err := s.db.Exec(context.Background(), `
		UPDATE etl_jobs
		SET status = $2,
			error_message = $3,
			updated_at = NOW()
		WHERE job_id = $1
	`, jobID, StatusFailed, errMsg)
	return err
}
