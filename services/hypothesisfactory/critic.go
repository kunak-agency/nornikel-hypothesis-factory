package hypothesisfactory

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"hypothesis-factory/domain"
	"hypothesis-factory/externalApi"

	"github.com/google/uuid"
)

// Одна LLM-judge ненадёжна (задокументированный self-preference/position
// bias в LLM-as-judge исследованиях); запускаем три судьи с разными
// объективами параллельно и усредняем оценки вместо доверия одному мнению.
// Латентность не растёт — вызовы идут конкурентно, не последовательно.
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

func criticResponseFormat(language string) string {
	langName := genLanguageNames[language]
	if langName == "" {
		langName = genLanguageNames["ru"]
	}
	return fmt.Sprintf(`

Оцени гипотезу и верни ТОЛЬКО JSON-объект по схеме ниже (все числовые оценки — целые от 0 до 10):
{
  "feasibility": <0-10>,
  "impact": <0-10>,
  "novelty": <0-10>,
  "risk_penalty": <0-10>,
  "critic_notes": "1-2 предложения на %s языке: главное слабое место или почему гипотеза выдерживает критику с твоей точки зрения"
}
Пояснение к полям (не включай его в ответ): risk_penalty — чем выше значение, тем больше риск.
Первый символ ответа — {, последний — }. Никакого текста до/после и никаких markdown-ограждений (`+"```"+` и слова json), никаких комментариев внутри JSON.`, langName)
}

type criticVerdict struct {
	Feasibility float64 `json:"feasibility"`
	Impact      float64 `json:"impact"`
	Novelty     float64 `json:"novelty"`
	RiskPenalty float64 `json:"risk_penalty"`
	CriticNotes string  `json:"critic_notes"`
}

// critique оценивает каждую гипотезу через 3-judge состязательный ансамбль
// (конкурентно по всем гипотезам и судьям сразу — иначе 3x'или бы wall-clock
// последовательно), считает детерминированный evidence_strength из
// confidence/числа claims (не отдан на откуп LLM) и ранжирует по прозрачной
// формуле.
func critique(ctx context.Context, client externalApi.LLMClient, spec domain.ProblemSpec, claimByID map[string]domain.Claim,
	hyps []domain.Hypothesis, language string, weights domain.RankingWeights) []domain.Hypothesis {
	specJSON, _ := json.Marshal(spec)
	responseFormat := criticResponseFormat(language)

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
				resp, err := client.Complete(ctx, externalApi.CompleteRequest{
					Messages: []externalApi.Message{
						{Role: "system", Content: lens.prompt + responseFormat},
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
		h.Scores.Total = totalScore(h.Scores, weights)
	}

	sort.SliceStable(hyps, func(i, j int) bool { return hyps[i].Scores.Total > hyps[j].Scores.Total })
	for i := range hyps {
		hyps[i].Rank = i + 1
	}
	return hyps
}

// aggregateVerdicts усредняет оценки судей, которые вернули парсируемый JSON
// (судья, который не ответил валидным JSON, просто исключается, а не
// считается нулём), и конкатенирует заметки каждого судьи под его именем.
func aggregateVerdicts(verdicts []*criticVerdict, evidenceStrength float64) (domain.Scores, string) {
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
	s := domain.Scores{EvidenceStrength: evidenceStrength}
	if n > 0 {
		s.Feasibility = sumFeasible / float64(n)
		s.Impact = sumImpact / float64(n)
		s.Novelty = sumNovelty / float64(n)
		s.RiskPenalty = sumRisk / float64(n)
	}
	s.Confidence = confidenceFromScores(s)
	return s, strings.Join(notes, " ")
}

// evidenceStrength: 0-10, определяется числом и confidence процитированных
// claims. Намеренно детерминированный (не LLM-оценка), чтобы формула
// ранжирования оставалась полностью проверяемой.
func evidenceStrength(refs []uuid.UUID, claimByID map[string]domain.Claim) float64 {
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
	if sum > 10 {
		sum = 10
	}
	if sum < 0 {
		sum = 0
	}
	return sum
}

// defaultRankingWeights отдают приоритет проверяемости/эффекту над сырой
// новизной — кейс сильнее упирает на лабораторно проверяемые,
// бизнес-релевантные гипотезы, чем на "вау-радикальность". Переопределяются
// per-run через domain.RankingWeights ("режим экспертной настройки" из
// доп. пожеланий кейса) — незаполненные (nil) поля остаются дефолтными.
const (
	defaultWEvidence = 0.25
	defaultWFeasible = 0.20
	defaultWImpact   = 0.25
	defaultWNovelty  = 0.15
	defaultWRisk     = 0.15
)

func totalScore(s domain.Scores, w domain.RankingWeights) float64 {
	wEvidence := orDefault(w.Evidence, defaultWEvidence)
	wFeasible := orDefault(w.Feasibility, defaultWFeasible)
	wImpact := orDefault(w.Impact, defaultWImpact)
	wNovelty := orDefault(w.Novelty, defaultWNovelty)
	wRisk := orDefault(w.Risk, defaultWRisk)
	return wEvidence*s.EvidenceStrength + wFeasible*s.Feasibility + wImpact*s.Impact +
		wNovelty*s.Novelty - wRisk*s.RiskPenalty
}

func orDefault(v *float64, def float64) float64 {
	if v == nil {
		return def
	}
	return *v
}

func confidenceFromScores(s domain.Scores) float64 {
	c := (s.EvidenceStrength + s.Feasibility) / 2
	if c > 10 {
		c = 10
	}
	if c < 0 {
		c = 0
	}
	return c
}
