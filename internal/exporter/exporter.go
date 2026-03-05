// Package exporter gera planilhas Excel (.xlsx) a partir de dados do banco.
// Cada domínio (milk, weight, reproduction) possui sua própria query e formatação.
package exporter

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xuri/excelize/v2"
	"go.uber.org/zap"
)

// ExportParams contém os filtros para exportação.
type ExportParams struct {
	PropertyID string
	GroupID    string
	Maturity   string // novilha, primipara, multipara
	Sex        string // M, F
	From       time.Time
	To         time.Time
	Tipo       string // MN, IA, IATF, TE (reprodução)
	IncludeRef bool   // incluir aba ANIMAIS_REF
}

// Exporter gera planilhas Excel a partir de dados do banco.
type Exporter struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

// New cria um novo Exporter.
func New(pool *pgxpool.Pool, logger *zap.Logger) *Exporter {
	return &Exporter{pool: pool, logger: logger}
}

// ── Milk Export ─────────────────────────────────────────────────────────────

// ExportMilk gera uma planilha de pesagem de leite com dados do banco.
func (e *Exporter) ExportMilk(ctx context.Context, params ExportParams) (*excelize.File, error) {
	e.logger.Info("Exportando planilha de leite", zap.String("property_id", params.PropertyID))

	query, args := buildMilkQuery(params)

	rows, err := e.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("falha na query de leite: %w", err)
	}
	defer rows.Close()

	f := excelize.NewFile()
	sheet := "DADOS"
	idx, _ := f.NewSheet(sheet)
	f.DeleteSheet("Sheet1")
	f.SetActiveSheet(idx)

	// Cabeçalho
	headers := []string{"Brinco", "Data", "Qtd. Produzida (L)", "Turno", "Observação"}
	for i, h := range headers {
		cell := cellName(i, 1)
		f.SetCellValue(sheet, cell, h)
	}
	applyHeaderStyle(f, sheet, len(headers))

	row := 2
	for rows.Next() {
		var brinco, periodo, ocorrencia *string
		var qtOrdenha *float64
		var dtOrdenha *time.Time

		if err := rows.Scan(&brinco, &dtOrdenha, &qtOrdenha, &periodo, &ocorrencia); err != nil {
			e.logger.Error("Erro ao escanear linha de leite", zap.Error(err))
			continue
		}

		setCell(f, sheet, 0, row, derefStr(brinco))
		if dtOrdenha != nil {
			setCell(f, sheet, 1, row, dtOrdenha.Format("02/01/2006"))
		}
		setCell(f, sheet, 2, row, derefFloat(qtOrdenha))
		setCell(f, sheet, 3, row, decodePeriodo(derefStr(periodo)))
		setCell(f, sheet, 4, row, derefStr(ocorrencia))
		row++
	}

	if params.IncludeRef {
		e.addRefSheet(ctx, f, params.PropertyID, "F") // leite = só fêmeas
	}

	autoWidth(f, sheet, len(headers))

	e.logger.Info("Exportação de leite concluída", zap.Int("rows", row-2))
	return f, nil
}

func buildMilkQuery(params ExportParams) (string, []any) {
	q := `
		SELECT b.brinco, dl.dt_ordenha, dl.qt_ordenha, dl.periodo, dl.ocorrencia
		FROM dadoslactacao dl
		JOIN bufalo b ON b.id_bufalo = dl.id_bufala
		WHERE dl.id_propriedade = $1
		  AND dl.deleted_at IS NULL
		  AND b.deleted_at IS NULL
	`
	args := []any{params.PropertyID}
	idx := 2

	if params.GroupID != "" {
		q += fmt.Sprintf(" AND b.id_grupo = $%d", idx)
		args = append(args, params.GroupID)
		idx++
	}
	if params.Maturity != "" {
		q += fmt.Sprintf(" AND b.nivel_maturidade = $%d", idx)
		args = append(args, mapMaturity(params.Maturity))
		idx++
	}
	if !params.From.IsZero() {
		q += fmt.Sprintf(" AND dl.dt_ordenha >= $%d", idx)
		args = append(args, params.From)
		idx++
	}
	if !params.To.IsZero() {
		q += fmt.Sprintf(" AND dl.dt_ordenha <= $%d", idx)
		args = append(args, params.To)
		idx++
	}

	q += " ORDER BY dl.dt_ordenha DESC, b.brinco"
	return q, args
}

// ── Weight Export ───────────────────────────────────────────────────────────

// ExportWeight gera uma planilha de pesagem do animal.
func (e *Exporter) ExportWeight(ctx context.Context, params ExportParams) (*excelize.File, error) {
	e.logger.Info("Exportando planilha de pesagem", zap.String("property_id", params.PropertyID))

	query, args := buildWeightQuery(params)

	rows, err := e.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("falha na query de pesagem: %w", err)
	}
	defer rows.Close()

	f := excelize.NewFile()
	sheet := "DADOS"
	idx, _ := f.NewSheet(sheet)
	f.DeleteSheet("Sheet1")
	f.SetActiveSheet(idx)

	headers := []string{"Brinco", "Data", "Peso (kg)", "Método", "Escore Corporal (BCS)", "Observação"}
	for i, h := range headers {
		cell := cellName(i, 1)
		f.SetCellValue(sheet, cell, h)
	}
	applyHeaderStyle(f, sheet, len(headers))

	row := 2
	for rows.Next() {
		var brinco, tipoPesagem *string
		var peso, bcs *float64
		var dtRegistro *time.Time

		if err := rows.Scan(&brinco, &dtRegistro, &peso, &tipoPesagem, &bcs); err != nil {
			e.logger.Error("Erro ao escanear linha de pesagem", zap.Error(err))
			continue
		}

		setCell(f, sheet, 0, row, derefStr(brinco))
		if dtRegistro != nil {
			setCell(f, sheet, 1, row, dtRegistro.Format("02/01/2006"))
		}
		setCell(f, sheet, 2, row, derefFloat(peso))
		setCell(f, sheet, 3, row, derefStr(tipoPesagem))
		setCell(f, sheet, 4, row, derefFloat(bcs))
		// Observação — sem coluna no banco para dadoszootecnicos
		row++
	}

	if params.IncludeRef {
		e.addRefSheet(ctx, f, params.PropertyID, "")
	}

	autoWidth(f, sheet, len(headers))

	e.logger.Info("Exportação de pesagem concluída", zap.Int("rows", row-2))
	return f, nil
}

func buildWeightQuery(params ExportParams) (string, []any) {
	q := `
		SELECT b.brinco, dz.dt_registro, dz.peso, dz.tipo_pesagem, dz.condicao_corporal
		FROM dadoszootecnicos dz
		JOIN bufalo b ON b.id_bufalo = dz.id_bufalo
		WHERE b.id_propriedade = $1
		  AND dz.deleted_at IS NULL
		  AND b.deleted_at IS NULL
	`
	args := []any{params.PropertyID}
	idx := 2

	if params.GroupID != "" {
		q += fmt.Sprintf(" AND b.id_grupo = $%d", idx)
		args = append(args, params.GroupID)
		idx++
	}
	if params.Maturity != "" {
		q += fmt.Sprintf(" AND b.nivel_maturidade = $%d", idx)
		args = append(args, mapMaturity(params.Maturity))
		idx++
	}
	if params.Sex != "" {
		q += fmt.Sprintf(" AND b.sexo = $%d", idx)
		args = append(args, strings.ToUpper(params.Sex[:1]))
		idx++
	}
	if !params.From.IsZero() {
		q += fmt.Sprintf(" AND dz.dt_registro >= $%d", idx)
		args = append(args, params.From)
		idx++
	}
	if !params.To.IsZero() {
		q += fmt.Sprintf(" AND dz.dt_registro <= $%d", idx)
		args = append(args, params.To)
		idx++
	}

	q += " ORDER BY dz.dt_registro DESC, b.brinco"
	return q, args
}

// ── Reproduction Export ─────────────────────────────────────────────────────

// ExportReproduction gera uma planilha de dados reprodutivos.
func (e *Exporter) ExportReproduction(ctx context.Context, params ExportParams) (*excelize.File, error) {
	e.logger.Info("Exportando planilha de reprodução", zap.String("property_id", params.PropertyID))

	query, args := buildReproductionQuery(params)

	rows, err := e.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("falha na query de reprodução: %w", err)
	}
	defer rows.Close()

	f := excelize.NewFile()
	sheet := "DADOS"
	idx, _ := f.NewSheet(sheet)
	f.DeleteSheet("Sheet1")
	f.SetActiveSheet(idx)

	headers := []string{"Brinco Fêmea", "Brinco Macho", "Data Evento", "Tipo", "Resultado DG", "Observação"}
	for i, h := range headers {
		cell := cellName(i, 1)
		f.SetCellValue(sheet, cell, h)
	}
	applyHeaderStyle(f, sheet, len(headers))

	row := 2
	for rows.Next() {
		var brincoF, brincoM, tipo, status, ocorrencia *string
		var dtEvento *time.Time

		if err := rows.Scan(&brincoF, &brincoM, &dtEvento, &tipo, &status, &ocorrencia); err != nil {
			e.logger.Error("Erro ao escanear linha de reprodução", zap.Error(err))
			continue
		}

		setCell(f, sheet, 0, row, derefStr(brincoF))
		setCell(f, sheet, 1, row, derefStr(brincoM))
		if dtEvento != nil {
			setCell(f, sheet, 2, row, dtEvento.Format("02/01/2006"))
		}
		setCell(f, sheet, 3, row, derefStr(tipo))
		setCell(f, sheet, 4, row, derefStr(status))
		setCell(f, sheet, 5, row, derefStr(ocorrencia))
		row++
	}

	if params.IncludeRef {
		e.addRefSheet(ctx, f, params.PropertyID, "F") // reprodução = fêmeas como principal
	}

	autoWidth(f, sheet, len(headers))

	e.logger.Info("Exportação de reprodução concluída", zap.Int("rows", row-2))
	return f, nil
}

func buildReproductionQuery(params ExportParams) (string, []any) {
	q := `
		SELECT bf.brinco, bm.brinco, dr.dt_evento, dr.tipo_inseminacao, dr.status, dr.ocorrencia
		FROM dadosreproducao dr
		JOIN bufalo bf ON bf.id_bufalo = dr.id_bufala
		LEFT JOIN bufalo bm ON bm.id_bufalo = dr.id_bufalo
		WHERE dr.id_propriedade = $1
		  AND dr.deleted_at IS NULL
		  AND bf.deleted_at IS NULL
	`
	args := []any{params.PropertyID}
	idx := 2

	if params.GroupID != "" {
		q += fmt.Sprintf(" AND bf.id_grupo = $%d", idx)
		args = append(args, params.GroupID)
		idx++
	}
	if params.Tipo != "" {
		q += fmt.Sprintf(" AND dr.tipo_inseminacao = $%d", idx)
		args = append(args, strings.ToUpper(params.Tipo))
		idx++
	}
	if !params.From.IsZero() {
		q += fmt.Sprintf(" AND dr.dt_evento >= $%d", idx)
		args = append(args, params.From)
		idx++
	}
	if !params.To.IsZero() {
		q += fmt.Sprintf(" AND dr.dt_evento <= $%d", idx)
		args = append(args, params.To)
		idx++
	}

	q += " ORDER BY dr.dt_evento DESC, bf.brinco"
	return q, args
}

// ── Aba ANIMAIS_REF ─────────────────────────────────────────────────────────

// addRefSheet adiciona uma aba com a lista de brincos disponíveis na propriedade.
func (e *Exporter) addRefSheet(ctx context.Context, f *excelize.File, propertyID, sexFilter string) {
	sheet := "ANIMAIS_REF"
	f.NewSheet(sheet)

	headers := []string{"Brinco", "Nome", "Sexo", "Maturidade", "Grupo"}
	for i, h := range headers {
		f.SetCellValue(sheet, cellName(i, 1), h)
	}
	applyHeaderStyle(f, sheet, len(headers))

	q := `
		SELECT b.brinco, b.nome, b.sexo, b.nivel_maturidade, g.nome_grupo
		FROM bufalo b
		LEFT JOIN grupo g ON g.id_grupo = b.id_grupo
		WHERE b.id_propriedade = $1
		  AND b.deleted_at IS NULL
		  AND b.brinco IS NOT NULL
	`
	args := []any{propertyID}
	if sexFilter != "" {
		q += " AND b.sexo = $2"
		args = append(args, sexFilter)
	}
	q += " ORDER BY b.brinco"

	rows, err := e.pool.Query(ctx, q, args...)
	if err != nil {
		e.logger.Error("Erro ao carregar ANIMAIS_REF", zap.Error(err))
		return
	}
	defer rows.Close()

	row := 2
	for rows.Next() {
		var brinco, nome, sexo, maturidade, grupo *string
		if err := rows.Scan(&brinco, &nome, &sexo, &maturidade, &grupo); err != nil {
			continue
		}
		setCell(f, sheet, 0, row, derefStr(brinco))
		setCell(f, sheet, 1, row, derefStr(nome))
		setCell(f, sheet, 2, row, derefStr(sexo))
		setCell(f, sheet, 3, row, decodeMaturidade(derefStr(maturidade)))
		setCell(f, sheet, 4, row, derefStr(grupo))
		row++
	}

	autoWidth(f, sheet, len(headers))
}

// ── Template Generation ─────────────────────────────────────────────────────

// GenerateMilkTemplate gera uma planilha vazia de leite com validações.
func (e *Exporter) GenerateMilkTemplate(ctx context.Context, propertyID string, includeRef bool) (*excelize.File, error) {
	f := excelize.NewFile()
	sheet := "DADOS"
	idx, _ := f.NewSheet(sheet)
	f.DeleteSheet("Sheet1")
	f.SetActiveSheet(idx)

	headers := []string{"Brinco", "Data", "Qtd. Produzida (L)", "Turno", "Observação"}
	for i, h := range headers {
		f.SetCellValue(sheet, cellName(i, 1), h)
	}
	applyHeaderStyle(f, sheet, len(headers))

	// Data validation: Turno
	addDropdown(f, sheet, "D2", "D1000", `"AM,PM,Único"`)

	autoWidth(f, sheet, len(headers))

	if includeRef {
		e.addRefSheet(ctx, f, propertyID, "F")
	}

	return f, nil
}

// GenerateWeightTemplate gera uma planilha vazia de pesagem com validações.
func (e *Exporter) GenerateWeightTemplate(ctx context.Context, propertyID string, includeRef bool) (*excelize.File, error) {
	f := excelize.NewFile()
	sheet := "DADOS"
	idx, _ := f.NewSheet(sheet)
	f.DeleteSheet("Sheet1")
	f.SetActiveSheet(idx)

	headers := []string{"Brinco", "Data", "Peso (kg)", "Método", "Escore Corporal (BCS)", "Observação"}
	for i, h := range headers {
		f.SetCellValue(sheet, cellName(i, 1), h)
	}
	applyHeaderStyle(f, sheet, len(headers))

	addDropdown(f, sheet, "D2", "D1000", `"Balança,Fita,Estimativa Visual"`)

	autoWidth(f, sheet, len(headers))

	if includeRef {
		e.addRefSheet(ctx, f, propertyID, "")
	}

	return f, nil
}

// GenerateReproductionTemplate gera uma planilha vazia de reprodução com validações.
func (e *Exporter) GenerateReproductionTemplate(ctx context.Context, propertyID string, includeRef bool) (*excelize.File, error) {
	f := excelize.NewFile()
	sheet := "DADOS"
	idx, _ := f.NewSheet(sheet)
	f.DeleteSheet("Sheet1")
	f.SetActiveSheet(idx)

	headers := []string{"Brinco Fêmea", "Brinco Macho", "Data Evento", "Tipo", "Resultado DG", "Observação"}
	for i, h := range headers {
		f.SetCellValue(sheet, cellName(i, 1), h)
	}
	applyHeaderStyle(f, sheet, len(headers))

	addDropdown(f, sheet, "D2", "D1000", `"MN,IA,IATF,TE"`)
	addDropdown(f, sheet, "E2", "E1000", `"Positivo,Negativo,Pendente,Inconclusivo"`)

	autoWidth(f, sheet, len(headers))

	if includeRef {
		e.addRefSheet(ctx, f, propertyID, "")
	}

	return f, nil
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func cellName(col, row int) string {
	colLetter := string(rune('A' + col))
	return fmt.Sprintf("%s%d", colLetter, row)
}

func setCell(f *excelize.File, sheet string, col, row int, val any) {
	f.SetCellValue(sheet, cellName(col, row), val)
}

func applyHeaderStyle(f *excelize.File, sheet string, numCols int) {
	style, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 11, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"2E7D32"}},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	for i := 0; i < numCols; i++ {
		f.SetCellStyle(sheet, cellName(i, 1), cellName(i, 1), style)
	}
}

func autoWidth(f *excelize.File, sheet string, numCols int) {
	for i := 0; i < numCols; i++ {
		col := string(rune('A' + i))
		f.SetColWidth(sheet, col, col, 20)
	}
}

func addDropdown(f *excelize.File, sheet, from, to, formula string) {
	dv := excelize.NewDataValidation(true)
	dv.Sqref = fmt.Sprintf("%s:%s", from, to)
	dv.SetDropList(strings.Split(strings.Trim(formula, `"`), ","))
	f.AddDataValidation(sheet, dv)
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefFloat(f *float64) any {
	if f == nil {
		return ""
	}
	return *f
}

func decodePeriodo(s string) string {
	switch s {
	case "M":
		return "AM"
	case "T":
		return "PM"
	case "U":
		return "Único"
	default:
		return s
	}
}

func decodeMaturidade(s string) string {
	switch strings.ToUpper(s) {
	case "N":
		return "Novilha"
	case "P":
		return "Primípara"
	case "M":
		return "Multípara"
	default:
		return s
	}
}

func mapMaturity(s string) string {
	switch strings.ToLower(s) {
	case "novilha":
		return "N"
	case "primipara", "primípara":
		return "P"
	case "multipara", "multípara":
		return "M"
	default:
		return s
	}
}
