package hypothesisfactory

import (
	"fmt"
	"strings"

	"hypothesis-factory/domain"
)

// RenderMarkdownReport собирает отчёт: ранжированные карточки гипотез с
// механизмом, evidence, рисками и roadmap проверки — формат "карточка, не
// свободный текст", которого ждёт кейс.
func RenderMarkdownReport(spec domain.ProblemSpec, hyps []domain.Hypothesis) string {
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
		fmt.Fprintf(&b, "**Ссылок на evidence:** %d\n\n", len(h.EvidenceRefs))
		b.WriteString("---\n\n")
	}

	return b.String()
}
