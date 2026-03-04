package excel

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/jaobarreto/buffs-etl-worker/internal/domain"
	"github.com/jaobarreto/buffs-etl-worker/internal/port"
	"github.com/xuri/excelize/v2"
)

// ── Processor ────────────────────────────────────────────────────────────────
// Orquestra o processamento completo de um arquivo Excel:
// 1. Abre o arquivo
// 2. Carrega o mapa brinco→UUID uma vez na memória (O(1) por lookup)
// 3. Identifica abas mapeadas
// 4. Lê cabeçalhos e valida campos obrigatórios
// 5. Itera linha a linha com validação rigorosa
// 6. Resolve brinco→id_bufalo para tabelas dependentes
// 7. Acumula registros válidos para bulk insert
// 8. Gera relatório final com logs estruturados

// Processor é o orquestrador de ETL para arquivos Excel.
type Processor struct {
	basePath     string
	brincoLoader port.BrincoLoader // nil = sem resolução de FK (ex: testes unitários)
}

// NewProcessor cria um Processor com o diretório base dos uploads.
// brincoLoader pode ser nil — nesse caso, a resolução de brinco→UUID é desativada.
func NewProcessor(basePath string, brincoLoader port.BrincoLoader) *Processor {
	return &Processor{basePath: basePath, brincoLoader: brincoLoader}
}

// Process executa o ETL completo de um arquivo Excel recebido via mensagem RabbitMQ.
// Retorna o resultado detalhado do processamento com contadores e erros por aba.
//
// O método NUNCA retorna erro para linhas inválidas — elas são logadas e puladas.
// Erro é retornado apenas para falhas de infraestrutura (arquivo não encontrado, DB fora, etc.).
func (p *Processor) Process(ctx context.Context, msg domain.ExcelProcessingMessage) (*domain.ProcessingResult, error) {
	filePath := filepath.Join(p.basePath, msg.FilePath)

	log.Printf("[ETL] Abrindo arquivo: %s", filePath)

	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("erro ao abrir arquivo Excel '%s': %w", filePath, err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.Printf("[ETL] Aviso: erro ao fechar arquivo Excel: %v", err)
		}
	}()

	result := &domain.ProcessingResult{
		FileName:      msg.FilePath,
		PropriedadeID: msg.PropriedadeID,
		UsuarioID:     msg.UsuarioID,
	}

	// ── Carrega brinco lookup uma vez para todas as abas ─────────────
	var brincoMap domain.BrincoLookup
	if p.brincoLoader != nil {
		brincoMap, err = p.brincoLoader.LoadBrincoMap(ctx, msg.PropriedadeID)
		if err != nil {
			return nil, fmt.Errorf("erro ao carregar mapa de brincos: %w", err)
		}
		log.Printf("[ETL] Brinco lookup: %d animais carregados para propriedade %s",
			len(brincoMap), msg.PropriedadeID)
	}

	sheets := f.GetSheetList()
	log.Printf("[ETL] Abas encontradas: %s", strings.Join(sheets, ", "))

	for _, sheetName := range sheets {
		cfg, ok := LookupSheet(sheetName)
		if !ok {
			log.Printf("[ETL] Aba '%s' ignorada (sem mapeamento para tabela)", sheetName)
			result.UnknownSheets = append(result.UnknownSheets, sheetName)
			continue
		}

		sheetResult := p.processSheet(f, sheetName, cfg, msg, brincoMap)
		result.Sheets = append(result.Sheets, sheetResult)
	}

	// ── Log estruturado final ────────────────────────────────────────
	p.logSummary(result)

	return result, nil
}

// processSheet processa uma única aba da planilha.
func (p *Processor) processSheet(
	f *excelize.File,
	sheetName string,
	cfg SheetConfig,
	msg domain.ExcelProcessingMessage,
	brincoMap domain.BrincoLookup,
) domain.SheetResult {
	sr := domain.SheetResult{
		Sheet: sheetName,
		Table: cfg.Table,
	}

	// 1. Monta header index
	headerIndex, err := BuildHeaderIndex(f, sheetName, cfg.RequiredFields)
	if err != nil {
		log.Printf("[ETL][%s] Erro no cabeçalho: %v — aba inteira ignorada", sheetName, err)
		sr.Errors = append(sr.Errors, domain.RowError{
			Sheet:   sheetName,
			Row:     1,
			Field:   "cabeçalho",
			Message: err.Error(),
		})
		return sr
	}

	log.Printf("[ETL][%s] Cabeçalho mapeado → tabela '%s' | Colunas: %v",
		sheetName, cfg.Table, headerKeys(headerIndex))

	// 2. Lê linhas de dados
	dataRows, err := GetDataRows(f, sheetName)
	if err != nil {
		log.Printf("[ETL][%s] Erro ao ler dados: %v", sheetName, err)
		return sr
	}
	sr.TotalRows = len(dataRows)

	if sr.TotalRows == 0 {
		log.Printf("[ETL][%s] Aba sem dados (apenas cabeçalho)", sheetName)
		return sr
	}

	// 3. Busca o validador adequado para esta tabela
	validator := ValidatorForTable(cfg.Table)
	if validator == nil {
		log.Printf("[ETL][%s] Sem validador para tabela '%s' — aba ignorada", sheetName, cfg.Table)
		return sr
	}

	// Verifica se esta aba precisa de resolução brinco→UUID
	needsLookup := cfg.NeedsLookup()

	// 4. Valida linha a linha — acumula válidas, pula inválidas
	var validRows []map[string]string

	for i, row := range dataRows {
		rowNum := i + 2 // +2 porque: índice 0-based + 1 (cabeçalho) + 1 (1-based)

		// Pula linhas completamente vazias
		if isEmptyRow(row) {
			continue
		}

		// 4a. Validação de campos (tipo, formato, required)
		rowErrors := validator(sheetName, rowNum, row, headerIndex)
		if len(rowErrors) > 0 {
			sr.Skipped++
			sr.Errors = append(sr.Errors, rowErrors...)
			for _, e := range rowErrors {
				log.Printf("[ETL][%s] SKIP Linha %d: %s", sheetName, rowNum, e.Message)
			}
			continue
		}

		// 4b. Resolução de FK: brinco → UUID
		if needsLookup && brincoMap != nil {
			brinco := GetCell(row, headerIndex, "brinco")
			if _, found := brincoMap[brinco]; !found {
				sr.Skipped++
				sr.Errors = append(sr.Errors, domain.RowError{
					Sheet:   sheetName,
					Row:     rowNum,
					Field:   "brinco",
					Value:   brinco,
					Message: fmt.Sprintf("animal de brinco '%s' não encontrado na base de dados desta propriedade", brinco),
				})
				log.Printf("[WARN] Aba '%s', Linha %d: Ignorada. Animal de brinco '%s' não encontrado na base de dados desta propriedade.",
					sheetName, rowNum, brinco)
				continue
			}
		}

		// 4c. Extrai record com aliases e resolução de FK aplicados
		record := extractRecord(row, headerIndex, cfg, brincoMap)
		record["id_propriedade"] = msg.PropriedadeID
		validRows = append(validRows, record)
	}

	sr.Inserted = len(validRows)

	log.Printf("[ETL][%s] Validação concluída: %d válidas, %d ignoradas de %d total",
		sheetName, sr.Inserted, sr.Skipped, sr.TotalRows)

	// TODO: 5. Bulk insert dos validRows no PostgreSQL
	// Será implementado quando o repository layer de escrita estiver pronto.
	if len(validRows) > 0 {
		log.Printf("[ETL][%s] %d registros prontos para INSERT na tabela '%s'",
			sheetName, len(validRows), cfg.Table)
	}

	return sr
}

// logSummary imprime o relatório final estruturado do processamento.
func (p *Processor) logSummary(result *domain.ProcessingResult) {
	log.Println("════════════════════════════════════════════════════════════")
	log.Printf("[ETL] RESUMO DO PROCESSAMENTO")
	log.Printf("[ETL] Arquivo: %s", result.FileName)
	log.Printf("[ETL] Propriedade: %s | Usuário: %s", result.PropriedadeID, result.UsuarioID)
	log.Println("────────────────────────────────────────────────────────────")

	for _, sr := range result.Sheets {
		log.Printf("[ETL] Aba '%s' → tabela '%s': %d inseridos, %d ignorados (de %d)",
			sr.Sheet, sr.Table, sr.Inserted, sr.Skipped, sr.TotalRows)
	}

	if len(result.UnknownSheets) > 0 {
		log.Printf("[ETL] Abas não mapeadas (ignoradas): %s",
			strings.Join(result.UnknownSheets, ", "))
	}

	totalInserted := result.TotalInserted()
	totalSkipped := result.TotalSkipped()
	allErrors := result.AllErrors()

	log.Println("────────────────────────────────────────────────────────────")
	log.Printf("[ETL] TOTAL: %d inseridos com sucesso, %d linhas ignoradas",
		totalInserted, totalSkipped)

	if len(allErrors) > 0 {
		log.Printf("[ETL] ERROS DETALHADOS (%d):", len(allErrors))
		for _, e := range allErrors {
			log.Printf("[ETL]   → %s", e.Error())
		}
	}

	log.Println("════════════════════════════════════════════════════════════")
}

// ── Helpers internos ─────────────────────────────────────────────────────────

// headerKeys retorna os nomes das colunas mapeadas (para log).
func headerKeys(h HeaderIndex) []string {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	return keys
}

// isEmptyRow verifica se todos os campos da linha estão vazios.
func isEmptyRow(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}

// extractRecord extrai os campos de uma linha como map[coluna_db]→valor.
// Aplica ColumnAliases para renomear campos (ex: "brinco" → "id_bufalo")
// e resolve o valor via BrincoLookup quando aplicável.
func extractRecord(row []string, h HeaderIndex, cfg SheetConfig, brincoMap domain.BrincoLookup) map[string]string {
	record := make(map[string]string)

	// Copia para não mutar o slice original do SheetRegistry
	allFields := make([]string, 0, len(cfg.RequiredFields)+len(cfg.OptionalFields))
	allFields = append(allFields, cfg.RequiredFields...)
	allFields = append(allFields, cfg.OptionalFields...)

	for _, field := range allFields {
		value := GetCell(row, h, field)
		if value == "" {
			continue
		}

		// Aplica alias: campo do Excel → coluna do DB
		dbColumn := field
		if cfg.ColumnAliases != nil {
			if alias, ok := cfg.ColumnAliases[field]; ok {
				dbColumn = alias
				// Se é um alias de brinco e temos lookup, resolve para UUID
				if field == "brinco" && brincoMap != nil {
					if uuid, found := brincoMap[value]; found {
						value = uuid
					}
				}
			}
		}

		record[dbColumn] = value
	}

	return record
}
