package hypothesisfactory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"hypothesis-factory/domain"
	"hypothesis-factory/externalApi"

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
  "verification_plan": [{"step": "...", "resource": "...", "success_criterion": "..."}]
}
ФОРМАТ ОТВЕТА: верни ТОЛЬКО JSON-массив и ничего больше. Первый символ ответа — [, последний — ].
Никакого текста до/после и никаких markdown-ограждений (` + "```" + ` и слова json).`

func generateHypotheses(ctx context.Context, client externalApi.LLMClient, spec domain.ProblemSpec, claims []domain.Claim) ([]domain.Hypothesis, error) {
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

	userMsg := fmt.Sprintf("ProblemSpec:\n%s\n\nEvidence-pack (%d claims):\n%s", specJSON, len(evidence), evidenceJSON)

	resp, err := client.Complete(ctx, externalApi.CompleteRequest{
		Messages: []externalApi.Message{
			{Role: "system", Content: hypothesisGenSystemPrompt},
			{Role: "user", Content: userMsg},
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
			Step        string `json:"step"`
			Resource    string `json:"resource"`
			SuccessCrit string `json:"success_criterion"`
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
		plan := make([]domain.VerificationStep, 0, len(r.VerificationPlan))
		for _, p := range r.VerificationPlan {
			plan = append(plan, domain.VerificationStep{Step: p.Step, Resource: p.Resource, SuccessCrit: p.SuccessCrit})
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
