package hypothesisfactory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"hypothesis-factory/domain"
	"hypothesis-factory/externalApi"

	"github.com/google/uuid"
)

const claimExtractionSystemPrompt = `Ты извлекаешь проверяемые факты (claims) из фрагмента технической литературы по
обогащению руд цветных металлов (флотация, измельчение, классификация).

На вход — один фрагмент текста/таблицы с указанием источника. Извлеки 0-3 claim'а СТРОГО в виде JSON-массива
(если фрагмент не содержит применимых технических фактов — верни пустой массив []):
[
  {
    "subject": "объект воздействия, напр. 'диаметр насадки гидроциклона'",
    "action": "конкретное действие/изменение, напр. 'уменьшение с 12 до 8 мм'",
    "condition": "условия применимости, если есть",
    "metric": "затрагиваемый показатель, напр. 'извлечение Ni', 'крупность слива'",
    "effect_direction": "increase | decrease | neutral | mixed | unspecified",
    "effect_magnitude": "количественная или качественная оценка эффекта, если есть в тексте; иначе пустая строка",
    "source_confidence": "low | medium | high (high — если есть числа/эксперимент; medium — есть механизм без цифр; low — просто упомянутое действие/идея без обоснования)",
    "conflict_flag": false,
    "quote": "точная цитата-основание из фрагмента, СЛОВО В СЛОВО как в тексте (не длиннее 300 символов)"
  }
]
Фрагмент может быть не только фрагментом учебника с измеренным эффектом, но и списком технологических идей/действий
без обоснования (напр. список из мозгового штурма технолога — "уменьшить диаметр насадки гидроциклона",
"опробовать магнитную сепарацию"). В этом случае извлекай subject+action как есть, effect_direction="unspecified",
source_confidence="low" — сам факт, что это действие было предложено/опробовано на реальной фабрике, тоже ценное
evidence, даже без измеренного эффекта.
Правила для "quote" (нарушение = claim молча отбрасывается автоматической проверкой на дословность):
- копируй ОДИН непрерывный участок исходного текста ровно как он написан (тот же порядок слов, те же цифры, те же символы);
- НЕ добавляй "...", НЕ склеивай куски из разных мест, НЕ исправляй опечатки, НЕ переводи, НЕ меняй окончания;
- если нужная мысль длиннее 300 символов — возьми более короткий непрерывный кусок того же предложения, но не выкидывай слова из середины;
- цитата должна дословно содержаться в присланном фрагменте; если подходящей дословной цитаты нет — не создавай этот claim.
Не придумывай факты, которых нет в тексте. Не переноси общие фразы без конкретики.

ФОРМАТ ОТВЕТА: верни ТОЛЬКО JSON-массив и ничего больше. Первый символ ответа — [, последний — ].
Никакого текста до/после, никаких markdown-ограждений (` + "```" + ` и слова json). Если применимых фактов нет — верни ровно [].`

const claimExtractionConcurrency = 8

// extractClaims извлекает claims по каждому retrieved-чанку конкурентно
// (ограниченно), не последовательно — N чанков стоит ~1 LLM round-trip по
// времени, а не N. Требуется, чтобы уложиться в "минуты, не часы".
func extractClaims(ctx context.Context, client externalApi.LLMClient, chunks []domain.RetrievedChunk) []domain.Claim {
	results := make([][]domain.Claim, len(chunks))
	sem := make(chan struct{}, claimExtractionConcurrency)
	var wg sync.WaitGroup

	for i, chunk := range chunks {
		wg.Add(1)
		go func(i int, chunk domain.RetrievedChunk) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = extractClaimsFromChunk(ctx, client, chunk)
		}(i, chunk)
	}
	wg.Wait()

	var all []domain.Claim
	for _, r := range results {
		all = append(all, r...)
	}
	return all
}

func extractClaimsFromChunk(ctx context.Context, client externalApi.LLMClient, chunk domain.RetrievedChunk) []domain.Claim {
	userMsg := fmt.Sprintf("Источник: %s (%s)\nРаздел: %s\n\nФрагмент:\n%s",
		chunk.DocumentTitle, chunk.SourceType, chunk.Section, chunk.Content)

	resp, err := client.Complete(ctx, externalApi.CompleteRequest{
		Messages: []externalApi.Message{
			{Role: "system", Content: claimExtractionSystemPrompt},
			{Role: "user", Content: userMsg},
		},
		Temperature: 0.0,
		MaxTokens:   1200,
	})
	if err != nil {
		return nil
	}

	var raw []struct {
		Subject          string `json:"subject"`
		Action           string `json:"action"`
		Condition        string `json:"condition"`
		Metric           string `json:"metric"`
		EffectDirection  string `json:"effect_direction"`
		EffectMagnitude  string `json:"effect_magnitude"`
		SourceConfidence string `json:"source_confidence"`
		ConflictFlag     bool   `json:"conflict_flag"`
		Quote            string `json:"quote"`
	}
	text := extractJSON(resp.Text)
	if strings.TrimSpace(text) == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		// Пропускаем сломанный JSON, а не валим весь прогон — шумные исходники
		// это явное требование кейса, а не крайний случай.
		return nil
	}

	var claims []domain.Claim
	for _, r := range raw {
		if r.Subject == "" || r.Action == "" {
			continue
		}
		// Детерминированная проверка grounding: цитата, которую нельзя
		// проследить до исходного чанка, отбрасывается. Исследования RAG
		// citation faithfulness показывают, что модели цитируют
		// правдоподобный, но никогда не встречавшийся в источнике текст;
		// валидный JSON этого не ловит — нужна проверка вне LLM-вызова.
		if !isGrounded(r.Quote, chunk.Content) {
			continue
		}
		claims = append(claims, domain.Claim{
			ID:               uuid.Must(uuid.NewV7()),
			ChunkID:          chunk.ID,
			Subject:          r.Subject,
			Action:           r.Action,
			Condition:        r.Condition,
			Metric:           r.Metric,
			EffectDirection:  r.EffectDirection,
			EffectMagnitude:  r.EffectMagnitude,
			SourceConfidence: r.SourceConfidence,
			ConflictFlag:     r.ConflictFlag,
			Quote:            r.Quote,
		})
	}
	return claims
}

// isGrounded проверяет, что цитата реально прослеживается до исходного
// чанка: точное вхождение после нормализации, либо word-overlap >= 0.7 (чтобы
// мелкая нормализация пробелов/пунктуации моделью не отбрасывала настоящую
// цитату). Ниже порога цитата считается недостоверной/галлюцинированной.
func isGrounded(quote, sourceContent string) bool {
	quote = strings.TrimSpace(quote)
	if quote == "" {
		return false
	}
	normQuote := normalizeForMatch(quote)
	normSource := normalizeForMatch(sourceContent)
	if strings.Contains(normSource, normQuote) {
		return true
	}
	return wordOverlapRatio(normQuote, normSource) >= 0.7
}

func normalizeForMatch(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if r == ' ' || r == '\n' || r == '\t' || r == '\r' {
			if !prevSpace {
				b.WriteRune(' ')
			}
			prevSpace = true
			continue
		}
		b.WriteRune(r)
		prevSpace = false
	}
	return strings.TrimSpace(b.String())
}

func wordOverlapRatio(quote, source string) float64 {
	quoteWords := strings.Fields(quote)
	if len(quoteWords) == 0 {
		return 0
	}
	sourceSet := make(map[string]struct{}, len(strings.Fields(source)))
	for _, w := range strings.Fields(source) {
		sourceSet[w] = struct{}{}
	}
	matched := 0
	for _, w := range quoteWords {
		if _, ok := sourceSet[w]; ok {
			matched++
		}
	}
	return float64(matched) / float64(len(quoteWords))
}
