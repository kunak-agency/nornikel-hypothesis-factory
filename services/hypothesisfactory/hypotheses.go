package hypothesisfactory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"hypothesis-factory/domain"
	"hypothesis-factory/externalApi"
	"hypothesis-factory/pkg/logger"

	"github.com/google/uuid"
)

const hypothesisGenSystemPrompt = `Ты — старший технолог-исследователь, формулирующий проверяемые гипотезы по улучшению
схемы обогащения (снижение потерь металлов с хвостами). Тебе даны: ProblemSpec (цель, точки потерь,
доступное оборудование, ограничения) и evidence-pack — список фактов (claims), извлечённых из научной/
регламентной базы, каждый с id.

Сгенерируй 5-10 гипотез СТРОГО в виде JSON-массива. Каждая гипотеза должна:
- опираться на конкретные claim_id из evidence-pack (evidence_refs) — ссылайся минимум на один реальный
  claim_id, копируя его посимвольно (UUID) из evidence-pack; не выдумывай id и не оставляй гипотезу без
  подходящего claim, не выдумывай факты вне evidence-pack;
- быть лабораторно/промышленно проверяемой (конкретное изменение в схеме/режиме/оборудовании), а не общей идеей;
- учитывать доступное оборудование и ограничения из ProblemSpec.

Формат каждого элемента:
{
  "statement": "краткая формулировка гипотезы в духе технологического мозгового штурма",
  "mechanism": "объяснение физического/химического механизма, почему это должно сработать",
  "evidence_refs": ["claim_id", ...],
  "expected_kpi_effect": {"metric": "...", "direction": "increase|decrease", "magnitude": "качественная/количественная оценка"},
  "risks": ["технический риск", "экономический риск", ...],
  "novelty_reason": "чем это отличается от очевидного/уже известного решения",
  "verification_plan": [{"step": "...", "resource": "...", "success_criterion": "...", "estimated_duration": "напр. '1-2 недели'", "estimated_cost": "напр. '~200 т.р. на реагенты', если оценимо, иначе пустая строка"}]
}
ФОРМАТ ОТВЕТА: верни ТОЛЬКО JSON-массив и ничего больше. Первый символ ответа — [, последний — ].
Никакого текста до/после и никаких markdown-ограждений (` + "```" + ` и слова json).`

var genLanguageNames = map[string]string{"ru": "русском", "en": "английском", "zh": "китайском"}

func generateHypotheses(ctx context.Context, client externalApi.LLMClient, spec domain.ProblemSpec, claims []domain.Claim,
	language string, excludedTopics []string, entityReputations []EntityReputation) ([]domain.Hypothesis, error) {
	specJSON, _ := json.MarshalIndent(spec, "", "  ")

	type evidenceItem struct {
		ClaimID string `json:"claim_id"`
		Subject string `json:"subject"`
		Action  string `json:"action"`
		Effect  string `json:"effect"`
		Quote   string `json:"quote"`
	}
	evidence := make([]evidenceItem, 0, len(claims))
	claimByID := make(map[string]domain.Claim, len(claims))
	for _, c := range claims {
		id := c.ID.String()
		claimByID[id] = c
		evidence = append(evidence, evidenceItem{
			ClaimID: id,
			Subject: c.Subject,
			Action:  c.Action,
			Effect:  strings.TrimSpace(c.EffectDirection + " " + c.EffectMagnitude),
			Quote:   c.Quote,
		})
	}
	evidenceJSON, _ := json.MarshalIndent(evidence, "", "  ")

	var b strings.Builder
	fmt.Fprintf(&b, "ProblemSpec:\n%s\n\nEvidence-pack (%d claims):\n%s", specJSON, len(evidence), evidenceJSON)

	langName := genLanguageNames[language]
	if langName == "" {
		langName = genLanguageNames["ru"]
	}
	fmt.Fprintf(&b, "\n\nЯзык ответа: пиши все текстовые значения (statement, mechanism, risks, "+
		"novelty_reason, verification_plan и т.д.) на %s языке. Ключи JSON и claim_id не переводи.", langName)

	if len(excludedTopics) > 0 {
		fmt.Fprintf(&b, "\n\nИсключённые направления (режим экспертной настройки) — НЕ предлагай гипотезы по "+
			"следующим темам/подходам, эксперт их уже отклонил или счёл нерелевантными: %s", strings.Join(excludedTopics, "; "))
	}

	if len(entityReputations) > 0 {
		b.WriteString("\n\nИстория экспертного фидбэка по сущностям из прошлых прогонов (обучение на фидбэке — " +
			"учитывай как сигнал, не как жёсткий запрет: сущность с историей отклонений может быть предложена " +
			"снова, но только с более сильным обоснованием, чем раньше):\n")
		for _, rep := range entityReputations {
			fmt.Fprintf(&b, "- %s: подтверждено %d, отклонено %d, требует доработки %d\n",
				rep.Name, rep.Confirmed, rep.Rejected, rep.NeedsRevision)
		}
	}

	resp, err := client.Complete(ctx, externalApi.CompleteRequest{
		Messages: []externalApi.Message{
			{Role: "system", Content: hypothesisGenSystemPrompt},
			{Role: "user", Content: b.String()},
		},
		Temperature: 0.4,
		MaxTokens:   6000,
	})
	if err != nil {
		return nil, fmt.Errorf("hypothesis generation llm call: %w", err)
	}

	var raw []struct {
		Statement         string   `json:"statement"`
		Mechanism         string   `json:"mechanism"`
		EvidenceRefs      []string `json:"evidence_refs"`
		ExpectedKPIEffect struct {
			Metric    string `json:"metric"`
			Direction string `json:"direction"`
			Magnitude string `json:"magnitude"`
		} `json:"expected_kpi_effect"`
		Risks            []string `json:"risks"`
		NoveltyReason    string   `json:"novelty_reason"`
		VerificationPlan []struct {
			Step              string `json:"step"`
			Resource          string `json:"resource"`
			SuccessCrit       string `json:"success_criterion"`
			EstimatedDuration string `json:"estimated_duration"`
			EstimatedCost     string `json:"estimated_cost"`
		} `json:"verification_plan"`
	}
	if err := json.Unmarshal([]byte(extractJSON(resp.Text)), &raw); err != nil {
		return nil, fmt.Errorf("hypothesis generation parse: %w (raw=%s)", err, resp.Text)
	}

	out := make([]domain.Hypothesis, 0, len(raw))
	for _, r := range raw {
		var refs []uuid.UUID
		for _, id := range r.EvidenceRefs {
			if _, ok := claimByID[id]; ok {
				if u, err := uuid.Parse(id); err == nil {
					refs = append(refs, u)
				}
			}
		}
		// Гипотеза, все evidence_refs которой оказались выдуманными
		// (не совпали ни с одним реальным claim_id), отбрасывается целиком:
		// «evidence-backed» — это контракт системы, а не пожелание к LLM,
		// и без единой реальной ссылки гипотеза непроверяема по построению.
		if len(refs) == 0 {
			logger.LogWarningCtx(ctx, "hypothesis dropped, no valid evidence refs: %q", r.Statement)
			continue
		}
		plan := make([]domain.VerificationStep, 0, len(r.VerificationPlan))
		for _, p := range r.VerificationPlan {
			plan = append(plan, domain.VerificationStep{
				Step: p.Step, Resource: p.Resource, SuccessCrit: p.SuccessCrit,
				EstimatedDuration: p.EstimatedDuration, EstimatedCost: p.EstimatedCost,
			})
		}
		out = append(out, domain.Hypothesis{
			ID:            uuid.Must(uuid.NewV7()),
			Statement:     r.Statement,
			Mechanism:     r.Mechanism,
			EvidenceRefs:  refs,
			NoveltyReason: r.NoveltyReason,
			Risks:         r.Risks,
			ExpectedKPIEffect: domain.KPIEffect{
				Metric: r.ExpectedKPIEffect.Metric, Direction: r.ExpectedKPIEffect.Direction, Magnitude: r.ExpectedKPIEffect.Magnitude,
			},
			VerificationPlan: plan,
		})
	}
	return out, nil
}
