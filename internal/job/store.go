// Package job gerencia jobs assíncronos de importação via goroutines e sync.Map.
// Substituiu o Asynq/Redis por uma solução in-memory simples.
package job

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

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
}

// Store gerencia jobs assíncronos em memória.
type Store struct {
	mu      sync.RWMutex
	jobs    map[string]*Entry
	counter int
	logger  *zap.Logger
}

// NewStore cria um novo store de jobs.
func NewStore(logger *zap.Logger) *Store {
	return &Store{
		jobs:   make(map[string]*Entry),
		logger: logger,
	}
}

// Create registra um novo job e retorna o ID.
func (s *Store) Create(filePath string, pType mapper.PipelineType, propertyID, userID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.counter++
	id := fmt.Sprintf("job_%d_%d", time.Now().UnixMilli(), s.counter)

	s.jobs[id] = &Entry{
		FilePath:   filePath,
		Type:       pType,
		PropertyID: propertyID,
		UserID:     userID,
		Status:     StatusProcessing,
		CreatedAt:  time.Now(),
	}

	return id
}

// Get retorna o estado de um job.
func (s *Store) Get(id string) (*Entry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.jobs[id]
	return e, ok
}

// Process executa o pipeline ETL em background (chamado como goroutine).
func (s *Store) Process(jobID string, p *pipeline.Pipeline) {
	s.mu.RLock()
	entry, ok := s.jobs[jobID]
	s.mu.RUnlock()

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

	s.mu.Lock()
	defer s.mu.Unlock()

	if err != nil {
		entry.Status = StatusFailed
		entry.Error = err.Error()
		s.logger.Error("Job falhou",
			zap.String("job_id", jobID),
			zap.Error(err),
		)
		return
	}

	entry.Status = StatusDone
	entry.Result = result
	s.logger.Info("Job concluído",
		zap.String("job_id", jobID),
		zap.Int("imported", result.Imported),
		zap.Int("errors", len(result.Errors)),
	)
}

// Cleanup remove jobs mais antigos que a duração especificada.
// Pode ser chamado periodicamente para evitar memory leak.
func (s *Store) Cleanup(maxAge time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	for id, entry := range s.jobs {
		if entry.CreatedAt.Before(cutoff) && entry.Status != StatusProcessing {
			delete(s.jobs, id)
		}
	}
}
