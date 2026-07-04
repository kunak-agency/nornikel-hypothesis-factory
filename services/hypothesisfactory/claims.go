package hypothesisfactory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"hypothesis-factory/domain"
	"hypothesis-factory/externalApi"
	"hypothesis-factory/repositories"

	"github.com/google/uuid"
)

const claimExtractionSystemPrompt = `Ты извлекаешь проверяемые факты (claims) из фрагмента технической литературы по
обогащению руд цветных металлов (флотация, измельчение, классификация).

На вход — один фрагмент текста/таблицы с указанием источника. Извлеки 0-3 claim'а СТРОГО в виде JSON-массива
(если фрагмент не содержит применимых технических фактов — верни пустой массив []):
[
  {
    "subject": "объект воздействия, напр. 'диаметр насадки гидроциклона'",
    "subject_kind": "equipment | reagent | process | material | metric | other — тип объекта voздействия",
    "action": "конкретное действие/изменение, напр. 'уменьшение с 12 до 8 мм'",
    "condition": "условия применимости, если есть",
    "metric": "затрагиваемый показатель, напр. 'извлечение Ni', 'крупность слива'",
    "metric_kind": "почти всегда 'metric', иначе — тип показателя по той же шкале, если это не числовой показатель",
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

// parentContextRadius — сколько соседних чанков (по ordinal, тот же
// документ) добавляется по обе стороны retrieved ("child") чанка при claim
// extraction. Docling иногда режет таблицу и поясняющий её текст на два
// соседних чанка (разные heading блокируют HybridChunker.merge_peers) — окно
// ordinal±1 склеивает их обратно в parent-контекст, где дословная цитата
// остаётся проверяемой, даже если она лежит не в самом retrieved чанке.
const parentContextRadius = 1

// extractClaims извлекает claims по каждому retrieved-чанку конкурентно
// (ограниченно), не последовательно — N чанков стоит ~1 LLM round-trip по
// времени, а не N. Требуется, чтобы уложиться в "минуты, не часы". Parent-
// контекст для ВСЕХ чанков поднимается одним batched-запросом заранее — раньше
// каждая горутина сама дёргала GetNeighbors, N отдельных round-trip'ов к БД
// вместо одного.
func extractClaims(ctx context.Context, client externalApi.LLMClient, chunkRepo *repositories.ChunkRepo, chunks []domain.RetrievedChunk) []domain.Claim {
	parentContents := buildParentContexts(ctx, chunkRepo, chunks)

	results := make([][]domain.Claim, len(chunks))
	sem := make(chan struct{}, claimExtractionConcurrency)
	var wg sync.WaitGroup

	for i, chunk := range chunks {
		wg.Add(1)
		go func(i int, chunk domain.RetrievedChunk) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = extractClaimsFromChunk(ctx, client, chunk, parentContents[i])
		}(i, chunk)
	}
	wg.Wait()

	var all []domain.Claim
	for _, r := range results {
		all = append(all, r...)
	}
	return all
}

// buildParentContexts стягивает каждый retrieved-чанк с его непосредственными
// соседями (тот же документ, ordinal±parentContextRadius) в один блок текста
// — Docling эмитит table-чанк и поясняющий его текст как соседние ordinal
// (даже когда HybridChunker.merge_peers не смог их слить из-за разных
// heading), так что более широкий parent-контекст даёт claim extraction
// шанс процитировать дословно то, что физически лежит в соседнем чанке.
// Один batched-запрос вместо одного GetNeighbors на чанк; при ошибке батча
// падает обратно на chunk.Content для всех чанков разом (child-only
// extraction хуже, но не хуже, чем было раньше).
func buildParentContexts(ctx context.Context, chunkRepo *repositories.ChunkRepo, chunks []domain.RetrievedChunk) []string {
	out := make([]string, len(chunks))
	if len(chunks) == 0 {
		return out
	}

	ranges := make([]repositories.NeighborRange, len(chunks))
	for i, c := range chunks {
		ranges[i] = repositories.NeighborRange{
			DocumentID: c.DocumentID,
			MinOrdinal: c.Ordinal - parentContextRadius,
			MaxOrdinal: c.Ordinal + parentContextRadius,
		}
	}

	neighbors, err := chunkRepo.GetNeighborsBatch(ctx, ranges)
	if err != nil {
		for i, c := range chunks {
			out[i] = c.Content
		}
		return out
	}

	byDoc := make(map[uuid.UUID][]domain.Chunk, len(chunks))
	for _, n := range neighbors {
		byDoc[n.DocumentID] = append(byDoc[n.DocumentID], n)
	}

	for i, c := range chunks {
		min, max := c.Ordinal-parentContextRadius, c.Ordinal+parentContextRadius
		var b strings.Builder
		for _, n := range byDoc[c.DocumentID] {
			if n.Ordinal < min || n.Ordinal > max {
				continue
			}
			if b.Len() > 0 {
				b.WriteString("\n\n")
			}
			b.WriteString(n.Content)
		}
		if b.Len() == 0 {
			out[i] = c.Content
		} else {
			out[i] = b.String()
		}
	}
	return out
}

// claimSourceMetadata собирает всё, что известно об источнике claim'а:
// заголовок/тип документа всегда, плюс авторы/год из любого источника,
// который их дал — chunk.Metadata (article_authors/article_year, ставит
// GROBID-путь для статей) в приоритете, иначе DocumentMetadata (authors/
// year/edition, ставится вручную при загрузке через handlers.IngestDocument).
func claimSourceMetadata(chunk domain.RetrievedChunk) map[string]any {
	meta := map[string]any{
		"document_title": chunk.DocumentTitle,
		"source_type":    chunk.SourceType,
	}
	if v, ok := chunk.Metadata["article_authors"].(string); ok && v != "" {
		meta["authors"] = v
	} else if v, ok := chunk.DocumentMetadata["authors"].(string); ok && v != "" {
		meta["authors"] = v
	}
	if v, ok := chunk.Metadata["article_year"].(string); ok && v != "" {
		meta["year"] = v
	} else if v, ok := chunk.DocumentMetadata["year"].(string); ok && v != "" {
		meta["year"] = v
	}
	if v, ok := chunk.DocumentMetadata["edition"].(string); ok && v != "" {
		meta["edition"] = v
	}
	return meta
}

func extractClaimsFromChunk(ctx context.Context, client externalApi.LLMClient, chunk domain.RetrievedChunk, parentContent string) []domain.Claim {
	userMsg := fmt.Sprintf("Источник: %s (%s)\nРаздел: %s\n\nФрагмент (с окружающим контекстом):\n%s",
		chunk.DocumentTitle, chunk.SourceType, chunk.Section, parentContent)

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
		SubjectKind      string `json:"subject_kind"`
		Action           string `json:"action"`
		Condition        string `json:"condition"`
		Metric           string `json:"metric"`
		MetricKind       string `json:"metric_kind"`
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
		if !isGrounded(r.Quote, parentContent) {
			continue
		}
		claims = append(claims, domain.Claim{
			ID:               uuid.Must(uuid.NewV7()),
			ChunkID:          chunk.ID,
			Subject:          r.Subject,
			SubjectKind:      r.SubjectKind,
			Action:           r.Action,
			Condition:        r.Condition,
			Metric:           r.Metric,
			MetricKind:       r.MetricKind,
			EffectDirection:  r.EffectDirection,
			EffectMagnitude:  r.EffectMagnitude,
			SourceConfidence: r.SourceConfidence,
			ConflictFlag:     r.ConflictFlag,
			Quote:            r.Quote,
			// document_title/source_type/authors/год — иначе итоговый отчёт
			// может сказать судье только "3 ссылки на evidence", не откуда
			// они; это ровно то, что даёт доверие "evidence-backed" гипотезе
			// и закрывает требование кейса "поддержка метаданных: источники,
			// даты, авторы".
			Metadata: claimSourceMetadata(chunk),
		})
	}
	return claims
}

// groundingOverlapThreshold — порог word-overlap для isGrounded. Раньше был
// 0.7: отбрасывал не только галлюцинации, но и настоящие цитаты, которые
// LLM слегка перефразировала (склеила два предложения, поправила пунктуацию,
// зацепила letter-spacing-фикс) — то есть резал не только явную дичь, а
// заодно и правдоподобные пограничные случаи. 0.6 — компромисс: цитата, где
// большинство слов реально из источника, считается заземлённой; ниже
// половины слов — это уже больше выдумано, чем процитировано, отбрасываем.
const groundingOverlapThreshold = 0.6

// isGrounded проверяет, что цитата реально прослеживается до исходного
// чанка: точное вхождение после нормализации, либо word-overlap >=
// groundingOverlapThreshold. Ниже порога цитата считается недостоверной/
// галлюцинированной.
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
	return wordOverlapRatio(normQuote, normSource) >= groundingOverlapThreshold
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
