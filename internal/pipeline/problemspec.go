// Package pipeline implements the Evidence-Backed Hypothesis Factory chain:
// ProblemSpec -> hybrid retrieval -> claim extraction -> hypothesis generation
// -> critic/ranker -> report. Each stage is a narrow LLM call with a strict
// JSON schema, not one long prompt, so the run stays interactive (minutes).
package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"hypothesis-factory/internal/llm"
	"hypothesis-factory/internal/models"
)

const problemSpecSystemPrompt = `Ты — технолог-аналитик обогатительной фабрики (флотация руд цветных металлов, Норникель).
Тебе дают сырой текст: свободное описание проблемы и/или данные анализа хвостов (объёмы, содержание
элементов 28 (Ni) и 29 (Cu), разбивка по классам крупности, минералогический состав — раскрытый/закрытый
Pnt/Cp, миллерит и т.д.).

Твоя задача — извлечь структурированный ProblemSpec СТРОГО в формате JSON, без пояснений вне JSON:
{
  "target_kpi": "краткая формулировка целевого показателя, напр. 'снижение потерь Ni в породных хвостах'",
  "plant": "название фабрики/подразделения, если указано, иначе пустая строка",
  "target_metals": ["Ni" | "Cu" | ...],
  "loss_hotspots": ["конкретные точки потерь: класс крупности + минеральная форма + оценка вклада, напр. '-71+45 мкм: закрытый Pnt/Cp, ~78% потерь Ni в этом классе'"],
  "available_equipment": ["упомянутое или типовое оборудование цепи: мельницы, гидроциклоны, классификаторы, флотомашины, грохота"],
  "constraints": ["бюджетные/нормативные/сырьевые ограничения, если есть"],
  "horizon": "желаемый горизонт проверки гипотез, если указан, иначе пустая строка"
}

Если данных не хватает — не выдумывай числа, оставляй поле пустым/пустым списком, но обязательно попытайся
определить loss_hotspots из минералогической разбивки, т.к. это ключевой вход для генерации гипотез.

ФОРМАТ ОТВЕТА: верни ТОЛЬКО JSON-объект и ничего больше. Первый символ ответа — {, последний — }.
Никакого текста до или после, никаких markdown-ограждений (никаких ` + "```" + ` и слова json), никаких комментариев внутри JSON.`

// BuildProblemSpec extracts a structured ProblemSpec from free-text and/or
// ingested tailings-report chunks (already parsed by Docling upstream).
func BuildProblemSpec(ctx context.Context, client llm.Client, rawText string) (models.ProblemSpec, error) {
	resp, err := client.Complete(ctx, llm.CompleteRequest{
		Messages: []llm.Message{
			{Role: "system", Content: problemSpecSystemPrompt},
			{Role: "user", Content: rawText},
		},
		Temperature: 0.1,
		MaxTokens:   1500,
	})
	if err != nil {
		return models.ProblemSpec{}, fmt.Errorf("problemspec llm call: %w", err)
	}

	var spec models.ProblemSpec
	if err := json.Unmarshal([]byte(extractJSON(resp.Text)), &spec); err != nil {
		return models.ProblemSpec{}, fmt.Errorf("problemspec parse: %w (raw=%s)", err, resp.Text)
	}
	return spec, nil
}

// extractJSON strips markdown code fences and leading/trailing prose that
// chat-tuned LLMs sometimes add despite "JSON only" instructions.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)

	start := strings.IndexAny(s, "{[")
	end := strings.LastIndexAny(s, "}]")
	if start == -1 || end == -1 || end < start {
		return s
	}
	return s[start : end+1]
}
