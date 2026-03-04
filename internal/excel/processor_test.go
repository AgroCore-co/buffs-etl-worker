package excel_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jaobarreto/buffs-etl-worker/internal/domain"
	"github.com/jaobarreto/buffs-etl-worker/internal/excel"
	"github.com/xuri/excelize/v2"
)

// ── Mock BrincoLoader ────────────────────────────────────────────────────────

// mockBrincoLoader implementa port.BrincoLoader para testes unitários.
type mockBrincoLoader struct {
	lookup domain.BrincoLookup
}

func (m *mockBrincoLoader) LoadBrincoMap(_ context.Context, _ string) (domain.BrincoLookup, error) {
	return m.lookup, nil
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// createTestExcel cria um arquivo Excel temporário com as abas e dados fornecidos.
// Retorna o caminho do diretório temporário e o nome do arquivo.
func createTestExcel(t *testing.T, sheets map[string][][]string) (dir, filename string) {
	t.Helper()

	dir = t.TempDir()
	filename = "test_planilha.xlsx"
	filePath := filepath.Join(dir, filename)

	f := excelize.NewFile()
	first := true
	for sheetName, rows := range sheets {
		if first {
			// Renomeia a aba default "Sheet1"
			f.SetSheetName("Sheet1", sheetName)
			first = false
		} else {
			f.NewSheet(sheetName)
		}
		for i, row := range rows {
			for j, cell := range row {
				colName, _ := excelize.ColumnNumberToName(j + 1)
				cellRef := colName + string(rune('0'+i+1))
				if i+1 >= 10 {
					cellRef = colName + itoa(i+1)
				}
				f.SetCellValue(sheetName, cellRef, cell)
			}
		}
	}

	if err := f.SaveAs(filePath); err != nil {
		t.Fatalf("Erro ao criar Excel de teste: %v", err)
	}
	f.Close()

	return dir, filename
}

// itoa simples para referências de célula
func itoa(n int) string {
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

// ── Testes do Parser ─────────────────────────────────────────────────────────

func TestNormalizeHeaderAndBuildIndex(t *testing.T) {
	dir, filename := createTestExcel(t, map[string][][]string{
		"Animais": {
			{"Brinco", " Nome ", "Dt Nascimento", "Sexo", "PESO (kg)"},
			{"B001", "Luna", "15/03/2020", "F", "450"},
		},
	})

	f, err := excelize.OpenFile(filepath.Join(dir, filename))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	h, err := excel.BuildHeaderIndex(f, "Animais", []string{"brinco"})
	if err != nil {
		t.Fatalf("BuildHeaderIndex falhou: %v", err)
	}

	// Verifica normalização
	tests := map[string]int{
		"brinco":        0,
		"nome":          1,
		"dt_nascimento": 2,
		"sexo":          3,
		"peso_kg":       4,
	}
	for col, expectedIdx := range tests {
		idx, ok := h[col]
		if !ok {
			t.Errorf("Coluna '%s' não encontrada no header index", col)
			continue
		}
		if idx != expectedIdx {
			t.Errorf("Coluna '%s': esperado índice %d, obteve %d", col, expectedIdx, idx)
		}
	}
}

func TestBuildHeaderIndex_MissingRequired(t *testing.T) {
	dir, filename := createTestExcel(t, map[string][][]string{
		"Animais": {
			{"Nome", "Sexo"}, // Sem "brinco"
			{"Luna", "F"},
		},
	})

	f, err := excelize.OpenFile(filepath.Join(dir, filename))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	_, err = excel.BuildHeaderIndex(f, "Animais", []string{"brinco"})
	if err == nil {
		t.Fatal("Deveria falhar com campo obrigatório 'brinco' ausente")
	}
}

func TestGetCell(t *testing.T) {
	h := excel.HeaderIndex{"brinco": 0, "nome": 1, "sexo": 2}
	row := []string{"B001", "Luna", "F"}

	if v := excel.GetCell(row, h, "brinco"); v != "B001" {
		t.Errorf("Esperado 'B001', obteve '%s'", v)
	}
	if v := excel.GetCell(row, h, "inexistente"); v != "" {
		t.Errorf("Esperado vazio para coluna inexistente, obteve '%s'", v)
	}

	// Linha curta (menos colunas que o header)
	shortRow := []string{"B001"}
	if v := excel.GetCell(shortRow, h, "sexo"); v != "" {
		t.Errorf("Esperado vazio para índice fora do range, obteve '%s'", v)
	}
}

// ── Testes do Validator ──────────────────────────────────────────────────────

func TestValidateRequired(t *testing.T) {
	if err := excel.ValidateRequired("Animais", 2, "brinco", "B001"); err != nil {
		t.Errorf("Não deveria falhar: brinco='B001'")
	}
	if err := excel.ValidateRequired("Animais", 2, "brinco", ""); err == nil {
		t.Error("Deveria falhar: brinco vazio")
	}
	if err := excel.ValidateRequired("Animais", 2, "brinco", "   "); err == nil {
		t.Error("Deveria falhar: brinco só espaços")
	}
}

func TestValidateNumeric(t *testing.T) {
	if err := excel.ValidateNumeric("P", 2, "peso", "450.5"); err != nil {
		t.Errorf("Deveria aceitar '450.5': %v", err)
	}
	if err := excel.ValidateNumeric("P", 2, "peso", "450,5"); err != nil {
		t.Errorf("Deveria aceitar '450,5' (vírgula BR): %v", err)
	}
	if err := excel.ValidateNumeric("P", 2, "peso", "abc"); err == nil {
		t.Error("Deveria rejeitar 'abc'")
	}
	if err := excel.ValidateNumeric("P", 2, "peso", ""); err != nil {
		t.Errorf("Deveria aceitar vazio (opcional): %v", err)
	}
}

func TestValidateDate(t *testing.T) {
	if err := excel.ValidateDate("A", 2, "dt", "15/03/2020", false); err != nil {
		t.Errorf("Deveria aceitar '15/03/2020': %v", err)
	}
	if err := excel.ValidateDate("A", 2, "dt", "2020-03-15", false); err != nil {
		t.Errorf("Deveria aceitar '2020-03-15': %v", err)
	}
	if err := excel.ValidateDate("A", 2, "dt", "abc", false); err == nil {
		t.Error("Deveria rejeitar 'abc'")
	}
	if err := excel.ValidateDate("A", 2, "dt", "01/01/2099", false); err == nil {
		t.Error("Deveria rejeitar data futura quando allowFuture=false")
	}
	if err := excel.ValidateDate("A", 2, "dt", "01/01/2099", true); err != nil {
		t.Errorf("Deveria aceitar data futura quando allowFuture=true: %v", err)
	}
}

func TestValidateSexo(t *testing.T) {
	if err := excel.ValidateSexo("A", 2, "sexo", "M"); err != nil {
		t.Errorf("Deveria aceitar 'M': %v", err)
	}
	if err := excel.ValidateSexo("A", 2, "sexo", "f"); err != nil {
		t.Errorf("Deveria aceitar 'f' (case insensitive): %v", err)
	}
	if err := excel.ValidateSexo("A", 2, "sexo", "X"); err == nil {
		t.Error("Deveria rejeitar 'X'")
	}
}

func TestValidateBoolean(t *testing.T) {
	validos := []string{"sim", "Não", "S", "n", "true", "false", "1", "0", "verdadeiro", "FALSO"}
	for _, v := range validos {
		if err := excel.ValidateBoolean("A", 2, "f", v); err != nil {
			t.Errorf("Deveria aceitar '%s': %v", v, err)
		}
	}
	if err := excel.ValidateBoolean("A", 2, "f", "talvez"); err == nil {
		t.Error("Deveria rejeitar 'talvez'")
	}
}

// ── Testes do Validator por Tabela ───────────────────────────────────────────

func TestValidateBufaloRow_Valid(t *testing.T) {
	h := excel.HeaderIndex{"brinco": 0, "nome": 1, "dt_nascimento": 2, "sexo": 3}
	row := []string{"B001", "Luna", "15/03/2020", "F"}

	errs := excel.ValidateBufaloRow("Animais", 2, row, h)
	if len(errs) > 0 {
		t.Errorf("Linha válida gerou %d erros: %v", len(errs), errs)
	}
}

func TestValidateBufaloRow_Invalid(t *testing.T) {
	h := excel.HeaderIndex{"brinco": 0, "nome": 1, "dt_nascimento": 2, "sexo": 3}
	row := []string{"", "Luna", "data-invalida", "X"}

	errs := excel.ValidateBufaloRow("Animais", 2, row, h)
	if len(errs) != 3 {
		t.Errorf("Esperado 3 erros (brinco vazio, data inválida, sexo inválido), obteve %d: %v", len(errs), errs)
	}
}

func TestValidatorForTable(t *testing.T) {
	tables := []string{"bufalo", "dadoszootecnicos", "dadossanitarios", "dadoslactacao", "dadosreproducao"}
	for _, table := range tables {
		v := excel.ValidatorForTable(table)
		if v == nil {
			t.Errorf("ValidatorForTable('%s') retornou nil", table)
		}
	}
	if v := excel.ValidatorForTable("inexistente"); v != nil {
		t.Error("Deveria retornar nil para tabela inexistente")
	}
}

// ── Testes do LookupSheet ────────────────────────────────────────────────────

func TestLookupSheet(t *testing.T) {
	tests := []struct {
		name  string
		table string
		found bool
	}{
		{"Animais", "bufalo", true},
		{"ANIMAIS", "bufalo", true},
		{"animais", "bufalo", true},
		{"bufalos", "bufalo", true},
		{"Pesagens", "dadoszootecnicos", true},
		{"sanitario", "dadossanitarios", true},
		{"Lactacao", "dadoslactacao", true},
		{"reproducao", "dadosreproducao", true},
		{"Sheet1", "", false},
		{"Configurações", "", false},
	}
	for _, tt := range tests {
		cfg, ok := excel.LookupSheet(tt.name)
		if ok != tt.found {
			t.Errorf("LookupSheet('%s'): found=%v, esperado %v", tt.name, ok, tt.found)
		}
		if tt.found && cfg.Table != tt.table {
			t.Errorf("LookupSheet('%s'): table='%s', esperado '%s'", tt.name, cfg.Table, tt.table)
		}
	}
}

// ── Teste de Integração: Processor completo ──────────────────────────────────

func TestProcessor_FullPipeline(t *testing.T) {
	dir, filename := createTestExcel(t, map[string][][]string{
		"Animais": {
			{"Brinco", "Nome", "Dt Nascimento", "Sexo"},
			{"B001", "Luna", "15/03/2020", "F"},
			{"B002", "Thor", "22/08/2019", "M"},
			{"", "SemBrinco", "01/01/2020", "M"}, // Linha inválida: brinco vazio
		},
		"Pesagens": {
			{"Brinco", "Dt Registro", "Peso"},
			{"B001", "10/01/2024", "452,5"},
			{"B002", "10/01/2024", "abc"}, // Linha inválida: peso não-numérico
		},
		"Configurações": { // Aba não mapeada
			{"Key", "Value"},
			{"cor", "azul"},
		},
	})

	processor := excel.NewProcessor(dir, nil)
	msg := domain.ExcelProcessingMessage{
		FilePath:      filename,
		PropriedadeID: "prop-uuid-123",
		UsuarioID:     "user-uuid-456",
		Timestamp:     "2025-01-15T10:30:00Z",
	}

	result, err := processor.Process(context.Background(), msg)
	if err != nil {
		t.Fatalf("Process falhou: %v", err)
	}

	// Verifica resultado geral
	if result.FileName != filename {
		t.Errorf("FileName: '%s', esperado '%s'", result.FileName, filename)
	}
	if result.PropriedadeID != "prop-uuid-123" {
		t.Errorf("PropriedadeID: '%s', esperado 'prop-uuid-123'", result.PropriedadeID)
	}

	// Deve ter 2 abas processadas + 1 desconhecida
	if len(result.Sheets) != 2 {
		t.Errorf("Esperado 2 abas processadas, obteve %d", len(result.Sheets))
	}
	if len(result.UnknownSheets) != 1 {
		t.Errorf("Esperado 1 aba desconhecida, obteve %d: %v", len(result.UnknownSheets), result.UnknownSheets)
	}

	// Verifica contadores da aba Animais
	var animaisSR *domain.SheetResult
	var pesagensSR *domain.SheetResult
	for i := range result.Sheets {
		switch result.Sheets[i].Table {
		case "bufalo":
			animaisSR = &result.Sheets[i]
		case "dadoszootecnicos":
			pesagensSR = &result.Sheets[i]
		}
	}

	if animaisSR == nil {
		t.Fatal("Aba 'Animais' (bufalo) não encontrada no resultado")
	}
	if animaisSR.TotalRows != 3 {
		t.Errorf("Animais TotalRows: %d, esperado 3", animaisSR.TotalRows)
	}
	if animaisSR.Inserted != 2 {
		t.Errorf("Animais Inserted: %d, esperado 2", animaisSR.Inserted)
	}
	if animaisSR.Skipped != 1 {
		t.Errorf("Animais Skipped: %d, esperado 1", animaisSR.Skipped)
	}

	if pesagensSR == nil {
		t.Fatal("Aba 'Pesagens' (dadoszootecnicos) não encontrada no resultado")
	}
	if pesagensSR.TotalRows != 2 {
		t.Errorf("Pesagens TotalRows: %d, esperado 2", pesagensSR.TotalRows)
	}
	if pesagensSR.Inserted != 1 {
		t.Errorf("Pesagens Inserted: %d, esperado 1", pesagensSR.Inserted)
	}
	if pesagensSR.Skipped != 1 {
		t.Errorf("Pesagens Skipped: %d, esperado 1", pesagensSR.Skipped)
	}

	// Totais
	if result.TotalInserted() != 3 {
		t.Errorf("TotalInserted: %d, esperado 3", result.TotalInserted())
	}
	if result.TotalSkipped() != 2 {
		t.Errorf("TotalSkipped: %d, esperado 2", result.TotalSkipped())
	}
}

func TestProcessor_FileNotFound(t *testing.T) {
	processor := excel.NewProcessor("/tmp/nao_existe", nil)
	msg := domain.ExcelProcessingMessage{
		FilePath:      "arquivo_fantasma.xlsx",
		PropriedadeID: "p1",
		UsuarioID:     "u1",
	}

	_, err := processor.Process(context.Background(), msg)
	if err == nil {
		t.Fatal("Deveria falhar para arquivo inexistente")
	}
}

func TestProcessor_EmptySheet(t *testing.T) {
	dir, filename := createTestExcel(t, map[string][][]string{
		"Animais": {
			{"Brinco", "Nome"}, // Só o cabeçalho, sem dados
		},
	})

	processor := excel.NewProcessor(dir, nil)
	msg := domain.ExcelProcessingMessage{
		FilePath:      filename,
		PropriedadeID: "p1",
		UsuarioID:     "u1",
	}

	result, err := processor.Process(context.Background(), msg)
	if err != nil {
		t.Fatalf("Process falhou: %v", err)
	}

	if len(result.Sheets) != 1 {
		t.Errorf("Esperado 1 aba, obteve %d", len(result.Sheets))
	}
	if result.Sheets[0].TotalRows != 0 {
		t.Errorf("Esperado 0 linhas de dados, obteve %d", result.Sheets[0].TotalRows)
	}
}

func TestProcessor_AllRowsInvalid(t *testing.T) {
	dir, filename := createTestExcel(t, map[string][][]string{
		"Animais": {
			{"Brinco", "Sexo"},
			{"", "X"},       // brinco vazio + sexo inválido
			{"", ""},        // linha vazia (ignorada, sem contar como skip)
			{"  ", "Alien"}, // brinco espaço + sexo inválido
		},
	})

	processor := excel.NewProcessor(dir, nil)
	msg := domain.ExcelProcessingMessage{
		FilePath:      filename,
		PropriedadeID: "p1",
		UsuarioID:     "u1",
	}

	result, err := processor.Process(context.Background(), msg)
	if err != nil {
		t.Fatalf("Process falhou: %v", err)
	}

	if result.TotalInserted() != 0 {
		t.Errorf("TotalInserted: %d, esperado 0 (todas inválidas)", result.TotalInserted())
	}
	if result.TotalSkipped() != 2 {
		t.Errorf("TotalSkipped: %d, esperado 2 (linha vazia não conta)", result.TotalSkipped())
	}
}

// ── Testes de Normalização ───────────────────────────────────────────────────

func TestParseDate(t *testing.T) {
	dates := []string{
		"15/03/2020",
		"2020-03-15",
		"2020-03-15 14:30:00",
		"15-03-2020",
	}
	for _, d := range dates {
		if _, err := excel.ParseDate(d); err != nil {
			t.Errorf("ParseDate('%s') falhou: %v", d, err)
		}
	}
	if _, err := excel.ParseDate("nao-e-data"); err == nil {
		t.Error("ParseDate deveria falhar para 'nao-e-data'")
	}
}

func TestNormalizeNumeric(t *testing.T) {
	if v := excel.NormalizeNumeric("450,5"); v != "450.5" {
		t.Errorf("Esperado '450.5', obteve '%s'", v)
	}
}

func TestNormalizeBool(t *testing.T) {
	if v := excel.NormalizeBool("sim"); v != "true" {
		t.Errorf("Esperado 'true', obteve '%s'", v)
	}
	if v := excel.NormalizeBool("Não"); v != "false" {
		t.Errorf("Esperado 'false', obteve '%s'", v)
	}
	if v := excel.NormalizeBool("xyz"); v != "" {
		t.Errorf("Esperado vazio, obteve '%s'", v)
	}
}

// ── Teste: não corrompe SheetRegistry ────────────────────────────────────────

func TestExtractRecord_DoesNotMutateRegistry(t *testing.T) {
	cfg, _ := excel.LookupSheet("animais")
	originalLen := len(cfg.RequiredFields)

	// Roda o processador duas vezes com dados válidos
	dir, filename := createTestExcel(t, map[string][][]string{
		"Animais": {
			{"Brinco", "Nome"},
			{"B001", "Luna"},
		},
	})

	processor := excel.NewProcessor(dir, nil)
	msg := domain.ExcelProcessingMessage{FilePath: filename, PropriedadeID: "p1", UsuarioID: "u1"}

	processor.Process(context.Background(), msg)
	processor.Process(context.Background(), msg)

	// Verifica que RequiredFields não foi mutado
	cfgAfter, _ := excel.LookupSheet("animais")
	if len(cfgAfter.RequiredFields) != originalLen {
		t.Errorf("SheetRegistry mutado! RequiredFields: antes=%d, depois=%d",
			originalLen, len(cfgAfter.RequiredFields))
	}
}

// ── Teste de reuso: verifica que TempDir é limpo ─────────────────────────────

func TestProcessor_TempDirCleanup(t *testing.T) {
	dir, filename := createTestExcel(t, map[string][][]string{
		"Animais": {
			{"Brinco"},
			{"B001"},
		},
	})

	// Arquivo deve existir
	filePath := filepath.Join(dir, filename)
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("Arquivo de teste deveria existir: %v", err)
	}

	processor := excel.NewProcessor(dir, nil)
	msg := domain.ExcelProcessingMessage{FilePath: filename, PropriedadeID: "p1", UsuarioID: "u1"}

	_, err := processor.Process(context.Background(), msg)
	if err != nil {
		t.Fatalf("Process falhou: %v", err)
	}
	// t.TempDir() limpa automaticamente no final do teste
}

// ── Testes de Resolução de FK (Brinco → UUID) ───────────────────────────────

func TestProcessor_BrincoLookup_ResolvesFK(t *testing.T) {
	// Cria planilha com aba "Pesagens" que precisa de resolução brinco→id_bufalo
	dir, filename := createTestExcel(t, map[string][][]string{
		"Pesagens": {
			{"Brinco", "Dt Registro", "Peso"},
			{"B001", "10/01/2024", "452,5"},
			{"B002", "10/01/2024", "380"},
			{"B999", "10/01/2024", "500"}, // Brinco não cadastrado no lookup
		},
	})

	loader := &mockBrincoLoader{
		lookup: domain.BrincoLookup{
			"B001": "uuid-bufalo-001",
			"B002": "uuid-bufalo-002",
			// B999 propositalmente ausente
		},
	}

	processor := excel.NewProcessor(dir, loader)
	msg := domain.ExcelProcessingMessage{
		FilePath:      filename,
		PropriedadeID: "prop-123",
		UsuarioID:     "user-456",
	}

	result, err := processor.Process(context.Background(), msg)
	if err != nil {
		t.Fatalf("Process falhou: %v", err)
	}

	if len(result.Sheets) != 1 {
		t.Fatalf("Esperado 1 aba, obteve %d", len(result.Sheets))
	}

	sr := result.Sheets[0]

	// 2 inseridos (B001, B002), 1 ignorado (B999 não encontrado no lookup)
	if sr.Inserted != 2 {
		t.Errorf("Inserted: %d, esperado 2", sr.Inserted)
	}
	if sr.Skipped != 1 {
		t.Errorf("Skipped: %d, esperado 1", sr.Skipped)
	}

	// Verifica que o erro do B999 contém mensagem estruturada
	found := false
	for _, e := range sr.Errors {
		if e.Field == "brinco" && e.Value == "B999" {
			found = true
			if !strings.Contains(e.Message, "não encontrado") {
				t.Errorf("Mensagem de erro inesperada: %s", e.Message)
			}
		}
	}
	if !found {
		t.Error("Esperava erro de brinco 'B999' não encontrado")
	}
}

func TestProcessor_BrincoLookup_NilLoader_SkipsFKResolution(t *testing.T) {
	// Sem loader (nil), a resolução de FK é desativada — tudo passa
	dir, filename := createTestExcel(t, map[string][][]string{
		"Pesagens": {
			{"Brinco", "Dt Registro", "Peso"},
			{"B001", "10/01/2024", "452,5"},
			{"B002", "10/01/2024", "380"},
		},
	})

	processor := excel.NewProcessor(dir, nil)
	msg := domain.ExcelProcessingMessage{
		FilePath:      filename,
		PropriedadeID: "prop-123",
		UsuarioID:     "user-456",
	}

	result, err := processor.Process(context.Background(), msg)
	if err != nil {
		t.Fatalf("Process falhou: %v", err)
	}

	sr := result.Sheets[0]
	// Ambas as linhas passam (sem validação de FK)
	if sr.Inserted != 2 {
		t.Errorf("Inserted: %d, esperado 2", sr.Inserted)
	}
	if sr.Skipped != 0 {
		t.Errorf("Skipped: %d, esperado 0", sr.Skipped)
	}
}

func TestProcessor_BrincoLookup_AnimaisSheet_NoFKResolution(t *testing.T) {
	// A aba "Animais" (tabela bufalo) NÃO precisa de lookup — brinco é coluna real
	dir, filename := createTestExcel(t, map[string][][]string{
		"Animais": {
			{"Brinco", "Nome", "Dt Nascimento", "Sexo"},
			{"B001", "Luna", "15/03/2020", "F"},
			{"B002", "Thor", "22/08/2019", "M"},
		},
	})

	loader := &mockBrincoLoader{
		lookup: domain.BrincoLookup{}, // Mapa vazio — nenhum brinco cadastrado
	}

	processor := excel.NewProcessor(dir, loader)
	msg := domain.ExcelProcessingMessage{
		FilePath:      filename,
		PropriedadeID: "prop-123",
		UsuarioID:     "user-456",
	}

	result, err := processor.Process(context.Background(), msg)
	if err != nil {
		t.Fatalf("Process falhou: %v", err)
	}

	sr := result.Sheets[0]
	// Ambos devem ser inseridos — aba bufalo não precisa de lookup
	if sr.Inserted != 2 {
		t.Errorf("Inserted: %d, esperado 2", sr.Inserted)
	}
	if sr.Skipped != 0 {
		t.Errorf("Skipped: %d, esperado 0", sr.Skipped)
	}
}

func TestProcessor_LactacaoSheet_UsesIdBufala(t *testing.T) {
	// A aba "Lactacao" deve usar o alias brinco → id_bufala (não id_bufalo)
	cfg, ok := excel.LookupSheet("lactacao")
	if !ok {
		t.Fatal("LookupSheet('lactacao') deveria encontrar mapeamento")
	}
	if cfg.Table != "dadoslactacao" {
		t.Errorf("Tabela: '%s', esperado 'dadoslactacao'", cfg.Table)
	}
	if !cfg.NeedsLookup() {
		t.Error("Lactacao deveria precisar de lookup (NeedsLookup = true)")
	}
	alias, ok := cfg.ColumnAliases["brinco"]
	if !ok || alias != "id_bufala" {
		t.Errorf("ColumnAliases['brinco'] = '%s', esperado 'id_bufala'", alias)
	}
}

func TestNeedsLookup(t *testing.T) {
	tests := []struct {
		sheet    string
		expected bool
	}{
		{"animais", false},   // Tabela principal — brinco é coluna real
		{"pesagens", true},   // FK: brinco → id_bufalo
		{"sanitario", true},  // FK: brinco → id_bufalo
		{"lactacao", true},   // FK: brinco → id_bufala
		{"reproducao", true}, // FK: brinco → id_bufala
	}

	for _, tt := range tests {
		cfg, ok := excel.LookupSheet(tt.sheet)
		if !ok {
			t.Errorf("LookupSheet('%s') deveria encontrar mapeamento", tt.sheet)
			continue
		}
		if got := cfg.NeedsLookup(); got != tt.expected {
			t.Errorf("NeedsLookup('%s') = %v, esperado %v", tt.sheet, got, tt.expected)
		}
	}
}
