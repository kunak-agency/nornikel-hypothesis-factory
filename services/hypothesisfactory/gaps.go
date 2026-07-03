package hypothesisfactory

import (
	"fmt"
	"strings"

	"hypothesis-factory/domain"
)

// gapOverlapThreshold — ниже этого word-overlap между claim'ом и темой
// (металл/точка потерь) claim не считается "покрывающим" тему. Используем
// ту же метрику, что и isGrounded (claims.go), не отдельную эвристику.
const gapOverlapThreshold = 0.2

// detectKnowledgeGaps — детерминированная (не LLM) проверка: для каждого
// металла/точки потерь из ProblemSpec есть ли среди извлечённых claims хотя
// бы один, реально её касающийся. Кейс явно требует "выявление пробелов в
// знаниях" — это честный сигнал "тут evidence мало/нет", а не LLM-домысел.
func detectKnowledgeGaps(spec domain.ProblemSpec, claims []domain.Claim) []string {
	haystacks := make([]string, len(claims))
	for i, c := range claims {
		haystacks[i] = normalizeForMatch(c.Subject + " " + c.Metric + " " + c.Quote)
	}
	covered := func(topic string) bool {
		norm := normalizeForMatch(topic)
		for _, h := range haystacks {
			if strings.Contains(h, norm) || wordOverlapRatio(norm, h) >= gapOverlapThreshold {
				return true
			}
		}
		return false
	}

	var gaps []string
	for _, metal := range spec.TargetMetals {
		if metal == "" || covered(metal) {
			continue
		}
		gaps = append(gaps, fmt.Sprintf(
			"По металлу %q в извлечённых claims нет явного покрытия — гипотезы по нему опираются на более слабое evidence, стоит расширить корпус или проверить вручную", metal))
	}
	for _, hotspot := range spec.LossHotspots {
		if hotspot == "" || covered(hotspot) {
			continue
		}
		gaps = append(gaps, fmt.Sprintf(
			"Точка потерь %q слабо покрыта evidence из базы знаний — возможно, требуется отдельное исследование", truncate(hotspot, 100)))
	}
	return gaps
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
