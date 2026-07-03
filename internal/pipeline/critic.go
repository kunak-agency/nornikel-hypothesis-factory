package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"

	"hypothesis-factory/internal/llm"
	"hypothesis-factory/internal/models"
)

// A single LLM judge is unreliable (documented self-preference/position bias
// in LLM-as-judge research); we run three judges with distinct lenses in
// parallel and average their scores instead of trusting one opinion. Latency
// stays flat since the calls run concurrently, not serially.
var criticLenses = []struct {
	name   string
	prompt string
}{
	{
		name: "технолог",
		prompt: `Ты — скептически настроенный технолог-рецензент гипотез по обогащению руд. Оцени РЕАЛИЗУЕМОСТЬ:
можно ли внедрить это с текущим оборудованием и без остановки схемы? Не противоречит ли ограничениям ProblemSpec?`,
	},
	{
		name: "экономист",
		prompt: `Ты — скептически настроенный экономист-рецензент гипотез по обогащению руд. Оцени ОЖИДАЕМЫЙ ЭФФЕКТ И РИСКИ:
насколько правдоподобен заявленный эффект на KPI, какие экономические/операционные риски недооценены?`,
	},
	{
		name: "рецензент новизны",
		prompt: `Ты — скептически настроенный рецензент новизны гипотез по обогащению руд. Оцени НОВИЗНУ И СИЛУ EVIDENCE:
не является ли это просто известным стандартным решением? Не опирается ли на слабые/малочисленные источники (source_confidence low)? Есть ли противоречащие друг другу evidence claims?`,
	},
}

const criticResponseFormat = `

Оцени гипотезу и верни ТОЛЬКО JSON-объект по схеме ниже (все числовые оценки — целые от 0 до 10):
{
  "feasibility": <0-10>,
  "impact": <0-10>,
  "novelty": <0-10>,
  "risk_penalty": <0-10>,
  "critic_notes": "1-2 предложения: главное слабое место или почему гипотеза выдерживает критику с твоей точки зрения"
}
Пояснение к полям (не включай его в ответ): risk_penalty — чем выше значение, тем больше риск.
Первый символ ответа — {, последний — }. Никакого текста до/после и никаких markdown-ограждений (` + "```" + ` и слова json), никаких комментариев внутри JSON.`

type criticVerdict struct {
	Feasibility float64 `json:"feasibility"`
	Impact      float64 `json:"impact"`
	Novelty     float64 `json:"novelty"`
	RiskPenalty float64 `json:"risk_penalty"`
	CriticNotes string  `json:"critic_notes"`
}

// Critique scores each hypothesis via a 3-judge adversarial ensemble (run
// concurrently across hypotheses and judges), computes a deterministic
// evidence_strength from claim confidence/count (not left to LLM taste), and
// ranks by a transparent weighted formula.
func Critique(ctx context.Context, client llm.Client, spec models.ProblemSpec, claimByID map[string]models.Claim, hyps []models.Hypothesis) ([]models.Hypothesis, error) {
	specJSON, _ := json.Marshal(spec)

	// All hypotheses x judges run concurrently (not one hypothesis fully
	// critiqued before the next starts) — this is what keeps a 3-judge
	// ensemble inside the "minutes, not hours" budget instead of 3x'ing
	// wall-clock time serially.
	allVerdicts := make([][]*criticVerdict, len(hyps))
	var wg sync.WaitGroup
	for i := range hyps {
		h := &hyps[i]
		h.Scores.EvidenceStrength = evidenceStrength(h.EvidenceRefs, claimByID)

		hJSON, _ := json.Marshal(h)
		userMsg := fmt.Sprintf("ProblemSpec:\n%s\n\nГипотеза:\n%s", specJSON, hJSON)

		verdicts := make([]*criticVerdict, len(criticLenses))
		allVerdicts[i] = verdicts
		for li, lens := range criticLenses {
			wg.Add(1)
			go func(li int, lens struct {
				name   string
				prompt string
			}) {
				defer wg.Done()
				resp, err := client.Complete(ctx, llm.CompleteRequest{
					Messages: []llm.Message{
						{Role: "system", Content: lens.prompt + criticResponseFormat},
						{Role: "user", Content: userMsg},
					},
					Temperature: 0.2,
					MaxTokens:   400,
				})
				if err != nil {
					return
				}
				var v criticVerdict
				if json.Unmarshal([]byte(extractJSON(resp.Text)), &v) == nil {
					verdicts[li] = &v
				}
			}(li, lens)
		}
	}
	wg.Wait()

	for i := range hyps {
		h := &hyps[i]
		h.Scores, h.CriticNotes = aggregateVerdicts(allVerdicts[i], h.Scores.EvidenceStrength)
		h.Scores.Total = totalScore(h.Scores)
	}

	sort.SliceStable(hyps, func(i, j int) bool { return hyps[i].Scores.Total > hyps[j].Scores.Total })
	for i := range hyps {
		hyps[i].Rank = i + 1
	}
	return hyps, nil
}

// aggregateVerdicts averages the surviving judge verdicts (a judge that
// failed to return parseable JSON is simply excluded, not treated as a zero)
// and concatenates each judge's note under its lens name for traceability.
func aggregateVerdicts(verdicts []*criticVerdict, evidenceStrength float64) (models.Scores, string) {
	var sumFeasible, sumImpact, sumNovelty, sumRisk float64
	var notes []string
	n := 0
	for i, v := range verdicts {
		if v == nil {
			continue
		}
		sumFeasible += v.Feasibility
		sumImpact += v.Impact
		sumNovelty += v.Novelty
		sumRisk += v.RiskPenalty
		if v.CriticNotes != "" {
			notes = append(notes, fmt.Sprintf("[%s] %s", criticLenses[i].name, v.CriticNotes))
		}
		n++
	}
	s := models.Scores{EvidenceStrength: evidenceStrength}
	if n > 0 {
		s.Feasibility = sumFeasible / float64(n)
		s.Impact = sumImpact / float64(n)
		s.Novelty = sumNovelty / float64(n)
		s.RiskPenalty = sumRisk / float64(n)
	}
	s.Confidence = confidenceFromScores(s)
	return s, strings.Join(notes, " ")
}

// evidenceStrength: 0-10, driven by number and confidence of cited claims.
// Deliberately deterministic (not LLM-scored) so the ranking formula stays
// auditable end to end.
func evidenceStrength(refs []uuid.UUID, claimByID map[string]models.Claim) float64 {
	if len(refs) == 0 {
		return 0
	}
	var sum float64
	for _, r := range refs {
		claim, ok := claimByID[r.String()]
		if !ok {
			continue
		}
		switch claim.SourceConfidence {
		case "high":
			sum += 3
		case "medium":
			sum += 2
		case "low":
			sum += 1
		default:
			sum += 1
		}
		if claim.ConflictFlag {
			sum -= 1
		}
	}
	score := sum
	if score > 10 {
		score = 10
	}
	if score < 0 {
		score = 0
	}
	return score
}

func totalScore(s models.Scores) float64 {
	// Weights favor verifiability/impact over raw novelty, per case emphasis
	// on lab-checkable, business-relevant hypotheses over "wow radicalism".
	const (
		wEvidence = 0.25
		wFeasible = 0.20
		wImpact   = 0.25
		wNovelty  = 0.15
		wRisk     = 0.15
	)
	return wEvidence*s.EvidenceStrength + wFeasible*s.Feasibility + wImpact*s.Impact +
		wNovelty*s.Novelty - wRisk*s.RiskPenalty
}

func confidenceFromScores(s models.Scores) float64 {
	c := (s.EvidenceStrength + s.Feasibility) / 2
	if c > 10 {
		c = 10
	}
	if c < 0 {
		c = 0
	}
	return c
}
