// Package transformer converte dados validados (strings) em structs tipadas do domínio.
// Normaliza datas, trata nulos, converte unidades.
package transformer

import (
	"strconv"
	"strings"
	"time"

	"github.com/jaobarreto/buffs-etl-worker/internal/domain"
	"github.com/jaobarreto/buffs-etl-worker/internal/mapper"
	"github.com/jaobarreto/buffs-etl-worker/internal/validator"
)

// Transform converte linhas validadas em records tipados do domínio.
func Transform(
	rows []validator.ValidatedRow,
	pt mapper.PipelineType,
	lookup domain.BrincoLookup,
	userID, propertyID string,
) []domain.Record {
	switch pt {
	case mapper.PipelineMilk:
		return transformMilk(rows, lookup, userID, propertyID)
	case mapper.PipelineWeight:
		return transformWeight(rows, lookup, userID)
	case mapper.PipelineReproduction:
		return transformReproduction(rows, lookup, propertyID)
	default:
		return nil
	}
}

// ── Milk ────────────────────────────────────────────────────────────────────

func transformMilk(rows []validator.ValidatedRow, lookup domain.BrincoLookup, userID, propertyID string) []domain.Record {
	records := make([]domain.Record, 0, len(rows))

	for _, row := range rows {
		brinco := row.Fields["id_bufala"]
		animal := lookup[brinco]
		if animal == nil {
			continue
		}

		rec := domain.MilkRecord{
			IDBufala:      animal.ID,
			IDUsuario:     userID,
			IDPropriedade: propertyID,
			QtOrdenha:     parseDecimal(row.Fields["qt_ordenha"]),
			Periodo:       normalizePeriodo(row.Fields["periodo"]),
			Ocorrencia:    truncate(row.Fields["ocorrencia"], 50),
			DtOrdenha:     parseDate(row.Fields["dt_ordenha"]),
		}
		records = append(records, rec)
	}

	return records
}

// ── Weight ──────────────────────────────────────────────────────────────────

func transformWeight(rows []validator.ValidatedRow, lookup domain.BrincoLookup, userID string) []domain.Record {
	records := make([]domain.Record, 0, len(rows))

	for _, row := range rows {
		brinco := row.Fields["id_bufalo"]
		animal := lookup[brinco]
		if animal == nil {
			continue
		}

		rec := domain.WeightRecord{
			IDBufalo:    animal.ID,
			IDUsuario:   userID,
			Peso:        parseDecimal(row.Fields["peso"]),
			DtRegistro:  parseDate(row.Fields["dt_registro"]),
			TipoPesagem: normalizeTipoPesagem(row.Fields["tipo_pesagem"]),
		}

		if bcs, ok := row.Fields["condicao_corporal"]; ok && bcs != "" {
			v := parseDecimal(bcs)
			rec.CondicaoCorporal = &v
		}

		records = append(records, rec)
	}

	return records
}

// ── Reproduction ────────────────────────────────────────────────────────────

func transformReproduction(rows []validator.ValidatedRow, lookup domain.BrincoLookup, propertyID string) []domain.Record {
	records := make([]domain.Record, 0, len(rows))

	for _, row := range rows {
		brincoFemea := row.Fields["id_bufala"]
		animalF := lookup[brincoFemea]
		if animalF == nil {
			continue
		}

		rec := domain.ReproductionRecord{
			IDBufala:        animalF.ID,
			TipoInseminacao: strings.ToUpper(strings.TrimSpace(row.Fields["tipo_inseminacao"])),
			Status:          normalizeStatus(row.Fields["status"]),
			DtEvento:        parseDate(row.Fields["dt_evento"]),
			Ocorrencia:      truncate(row.Fields["ocorrencia"], 255),
			IDPropriedade:   propertyID,
		}

		// Brinco do macho (opcional)
		if brincoMacho, ok := row.Fields["id_bufalo"]; ok && brincoMacho != "" {
			if animalM := lookup[brincoMacho]; animalM != nil {
				rec.IDBufalo = animalM.ID
			}
		}

		records = append(records, rec)
	}

	return records
}

// ── Helpers ─────────────────────────────────────────────────────────────────

var dateFormats = []string{
	"02/01/2006",
	"2/1/2006",
	"2006-01-02",
	"02-01-2006",
	"02/01/2006 15:04",
}

func parseDate(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, fmt := range dateFormats {
		if t, err := time.Parse(fmt, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func parseDecimal(s string) float64 {
	s = strings.TrimSpace(s)
	s = strings.Replace(s, ",", ".", 1)
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func normalizePeriodo(s string) string {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "M", "AM", "MANHÃ", "MANHA":
		return "M"
	case "T", "PM", "TARDE":
		return "T"
	case "U", "ÚNICO", "UNICO":
		return "U"
	default:
		return s
	}
}

func normalizeTipoPesagem(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "balança", "balanca":
		return "Balança"
	case "fita":
		return "Fita"
	case "estimativa visual", "estimativa", "visual":
		return "Estimativa Visual"
	default:
		return s
	}
}

func normalizeStatus(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "positivo", "+":
		return "Positivo"
	case "negativo", "-":
		return "Negativo"
	case "pendente":
		return "Pendente"
	case "inconclusivo":
		return "Inconclusivo"
	default:
		return s
	}
}

func truncate(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen]
	}
	return s
}
