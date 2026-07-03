package hypothesisfactory

import (
	"fmt"
	"sort"
	"strings"

	"hypothesis-factory/domain"

	"github.com/google/uuid"
)

// BuildEvidenceSources сводит claims, процитированные каждой гипотезой, к
// списку уникальных отсортированных источников (заголовок + авторы/год,
// если известны) — то, что видит судья вместо голого "3 ссылки на
// evidence". claimsByID — из Service.GetClaimSources.
func BuildEvidenceSources(hyps []domain.Hypothesis, claimsByID map[uuid.UUID]domain.Claim) map[uuid.UUID][]string {
	out := make(map[uuid.UUID][]string, len(hyps))
	for _, h := range hyps {
		seen := map[string]bool{}
		var titles []string
		for _, ref := range h.EvidenceRefs {
			c, ok := claimsByID[ref]
			if !ok {
				continue
			}
			label := formatSourceLabel(c.Metadata)
			if label == "" || seen[label] {
				continue
			}
			seen[label] = true
			titles = append(titles, label)
		}
		sort.Strings(titles)
		out[h.ID] = titles
	}
	return out
}

// formatSourceLabel строит человекочитаемую ссылку из метаданных claim'а:
// "Заголовок (авторы, год[, издание])" — то, чего не хватало в отчёте до
// сих пор (authors/year собирались при ingestion, но никуда не долетали).
func formatSourceLabel(meta map[string]any) string {
	title, _ := meta["document_title"].(string)
	if title == "" {
		return ""
	}
	var extras []string
	if v, ok := meta["authors"].(string); ok && v != "" {
		extras = append(extras, v)
	}
	if v, ok := meta["year"].(string); ok && v != "" {
		extras = append(extras, v)
	}
	if v, ok := meta["edition"].(string); ok && v != "" {
		extras = append(extras, v)
	}
	if len(extras) == 0 {
		return title
	}
	return fmt.Sprintf("%s (%s)", title, strings.Join(extras, ", "))
}

// RenderMarkdownReport собирает отчёт: ранжированные карточки гипотез с
// механизмом, evidence, рисками и roadmap проверки — формат "карточка, не
// свободный текст", которого ждёт кейс. sources — из BuildEvidenceSources,
// может быть nil (тогда просто не показывает источники). knowledgeGaps — из
// HypothesisRun.KnowledgeGaps, тоже может быть nil.
func RenderMarkdownReport(spec domain.ProblemSpec, hyps []domain.Hypothesis, sources map[uuid.UUID][]string, knowledgeGaps []string) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# Фабрика гипотез — отчёт\n\n")
	fmt.Fprintf(&b, "**Цель:** %s\n\n", spec.TargetKPI)
	if spec.Plant != "" {
		fmt.Fprintf(&b, "**Фабрика:** %s\n\n", spec.Plant)
	}
	if len(spec.TargetMetals) > 0 {
		fmt.Fprintf(&b, "**Целевые металлы:** %s\n\n", strings.Join(spec.TargetMetals, ", "))
	}
	if len(spec.LossHotspots) > 0 {
		fmt.Fprintf(&b, "**Точки потерь:**\n")
		for _, h := range spec.LossHotspots {
			fmt.Fprintf(&b, "- %s\n", h)
		}
		b.WriteString("\n")
	}
	if len(spec.Constraints) > 0 {
		fmt.Fprintf(&b, "**Ограничения:**\n")
		for _, c := range spec.Constraints {
			fmt.Fprintf(&b, "- %s\n", c)
		}
		b.WriteString("\n")
	}
	if len(knowledgeGaps) > 0 {
		fmt.Fprintf(&b, "**Пробелы в знаниях (слабое покрытие evidence):**\n")
		for _, g := range knowledgeGaps {
			fmt.Fprintf(&b, "- %s\n", g)
		}
		b.WriteString("\n")
	}

	b.WriteString("---\n\n")

	for _, h := range hyps {
		fmt.Fprintf(&b, "## %d. %s\n\n", h.Rank, h.Statement)
		fmt.Fprintf(&b, "**Механизм:** %s\n\n", h.Mechanism)
		fmt.Fprintf(&b, "**Ожидаемый эффект на KPI:** %s — %s (%s)\n\n",
			h.ExpectedKPIEffect.Metric, h.ExpectedKPIEffect.Direction, h.ExpectedKPIEffect.Magnitude)
		fmt.Fprintf(&b, "**Новизна:** %s\n\n", h.NoveltyReason)

		if len(h.Risks) > 0 {
			b.WriteString("**Риски:**\n")
			for _, r := range h.Risks {
				fmt.Fprintf(&b, "- %s\n", r)
			}
			b.WriteString("\n")
		}

		if len(h.VerificationPlan) > 0 {
			b.WriteString("**Дорожная карта проверки:**\n")
			for i, v := range h.VerificationPlan {
				fmt.Fprintf(&b, "%d. %s (ресурсы: %s; критерий успеха: %s)\n", i+1, v.Step, v.Resource, v.SuccessCrit)
			}
			b.WriteString("\n")
		}

		fmt.Fprintf(&b, "**Оценки:** evidence=%.1f, feasibility=%.1f, impact=%.1f, novelty=%.1f, risk_penalty=%.1f, confidence=%.1f, **total=%.1f**\n\n",
			h.Scores.EvidenceStrength, h.Scores.Feasibility, h.Scores.Impact, h.Scores.Novelty, h.Scores.RiskPenalty, h.Scores.Confidence, h.Scores.Total)

		if h.CriticNotes != "" {
			fmt.Fprintf(&b, "**Замечание рецензента:** %s\n\n", h.CriticNotes)
		}
		if titles := sources[h.ID]; len(titles) > 0 {
			fmt.Fprintf(&b, "**Источники (%d ссылок на evidence):** %s\n\n", len(h.EvidenceRefs), strings.Join(titles, "; "))
		} else {
			fmt.Fprintf(&b, "**Ссылок на evidence:** %d\n\n", len(h.EvidenceRefs))
		}
		b.WriteString("---\n\n")
	}

	return b.String()
}
