// Package extractor lê arquivos Excel usando excelize e retorna dados brutos.
package extractor

import (
	"fmt"
	"strings"

	"github.com/jaobarreto/buffs-etl-worker/pkg/apperror"
	"github.com/xuri/excelize/v2"
)

// ExcelData contém os dados extraídos de uma planilha Excel.
type ExcelData struct {
	Headers []string   // cabeçalhos da primeira linha
	Rows    [][]string // dados a partir da segunda linha
}

// Extract abre o arquivo .xlsx e extrai cabeçalhos + dados da aba "DADOS" ou da primeira aba.
// Retorna erro fatal se o arquivo estiver corrompido ou sem dados.
func Extract(filePath string) (*ExcelData, error) {
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return nil, apperror.BadRequest(apperror.CodeFileCorrupted,
			fmt.Sprintf("Não foi possível abrir o arquivo Excel: %v", err))
	}
	defer f.Close()

	sheet := findDataSheet(f)
	if sheet == "" {
		return nil, apperror.BadRequest(apperror.CodeInvalidFormat,
			"Nenhuma aba de dados encontrada. Esperado: 'DADOS', 'Dados', 'Sheet1' ou primeira aba disponível.")
	}

	rows, err := f.GetRows(sheet)
	if err != nil {
		return nil, apperror.BadRequest(apperror.CodeFileCorrupted,
			fmt.Sprintf("Erro ao ler aba '%s': %v", sheet, err))
	}

	if len(rows) < 2 {
		return nil, apperror.BadRequest(apperror.CodeInvalidFormat,
			"Planilha deve conter ao menos cabeçalho + 1 linha de dados")
	}

	return &ExcelData{
		Headers: rows[0],
		Rows:    rows[1:],
	}, nil
}

// RowCount retorna o número de linhas de dados (excluindo cabeçalho) sem carregar tudo em memória.
func RowCount(filePath string) (int, error) {
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return 0, apperror.BadRequest(apperror.CodeFileCorrupted,
			fmt.Sprintf("Não foi possível abrir o arquivo Excel: %v", err))
	}
	defer f.Close()

	sheet := findDataSheet(f)
	if sheet == "" {
		return 0, nil
	}

	rows, err := f.GetRows(sheet)
	if err != nil {
		return 0, err
	}

	if len(rows) <= 1 {
		return 0, nil
	}

	return len(rows) - 1, nil
}

// findDataSheet procura a aba de dados: "DADOS", "Dados", "dados", "Sheet1" ou primeira aba.
func findDataSheet(f *excelize.File) string {
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return ""
	}

	// Prioridade: aba chamada "DADOS" (case insensitive)
	for _, s := range sheets {
		if strings.EqualFold(s, "dados") {
			return s
		}
	}

	// Fallback: Sheet1
	for _, s := range sheets {
		if strings.EqualFold(s, "sheet1") {
			return s
		}
	}

	// Fallback: primeira aba
	return sheets[0]
}
