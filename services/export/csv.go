// Package export рендерит финальный отчёт по прогону (гипотезы + ProblemSpec)
// в форматы, которые явно требует кейс — PDF, DOCX, CSV — поверх той же
// структурированной модели, что и Markdown-отчёт (RenderMarkdownReport).
package export

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"

	"hypothesis-factory/domain"

	"github.com/google/uuid"
)

var csvHeader = []string{
	"rank", "statement", "mechanism", "kpi_metric", "kpi_direction", "kpi_magnitude",
	"novelty_reason", "risks", "verification_plan", "evidence_strength", "feasibility",
	"impact", "novelty", "risk_penalty", "confidence", "total_score", "critic_notes",
	"evidence_refs_count", "evidence_sources",
}

// ToCSV — одна строка на гипотезу, для табличного просмотра/фильтрации в
// Excel; ProblemSpec в CSV не идёт (не табличные данные), только гипотезы.
// sources (может быть nil) — hypothesis.ID -> названия документов-источников
// evidence, см. hypothesisfactory.BuildEvidenceSources.
func ToCSV(hyps []domain.Hypothesis, sources map[uuid.UUID][]string) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("\xEF\xBB\xBF") // UTF-8 BOM — иначе Excel на Windows показывает кириллицу как мусор.
	w := csv.NewWriter(&buf)
	w.Comma = ';' // Excel в ru-локали ждёт ';' как разделитель CSV, не ','.

	if err := w.Write(csvHeader); err != nil {
		return nil, fmt.Errorf("write header: %w", err)
	}
	for _, h := range hyps {
		var verification []string
		for i, v := range h.VerificationPlan {
			verification = append(verification, fmt.Sprintf("%d. %s (%s; %s)", i+1, v.Step, v.Resource, v.SuccessCrit))
		}
		row := []string{
			strconv.Itoa(h.Rank),
			h.Statement,
			h.Mechanism,
			h.ExpectedKPIEffect.Metric,
			h.ExpectedKPIEffect.Direction,
			h.ExpectedKPIEffect.Magnitude,
			h.NoveltyReason,
			strings.Join(h.Risks, " | "),
			strings.Join(verification, " | "),
			formatFloat(h.Scores.EvidenceStrength),
			formatFloat(h.Scores.Feasibility),
			formatFloat(h.Scores.Impact),
			formatFloat(h.Scores.Novelty),
			formatFloat(h.Scores.RiskPenalty),
			formatFloat(h.Scores.Confidence),
			formatFloat(h.Scores.Total),
			h.CriticNotes,
			strconv.Itoa(len(h.EvidenceRefs)),
			strings.Join(sources[h.ID], " | "),
		}
		if err := w.Write(row); err != nil {
			return nil, fmt.Errorf("write row: %w", err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64)
}
