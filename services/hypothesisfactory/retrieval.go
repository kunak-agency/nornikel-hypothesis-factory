package hypothesisfactory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"hypothesis-factory/domain"
	"hypothesis-factory/externalApi"
	"hypothesis-factory/repositories"
)

// retrievalFacet — фасет запроса + минимальная гарантированная "квота
// анти-голодания" (не доля topK — см. ниже, почему это разные вещи).
type retrievalFacet struct {
	// topic — САМОСТОЯТЕЛЬНЫЙ текст запроса фасета ("" = базовый запрос из
	// ProblemSpec). Не суффикс к базовому: склейка base+topic отравляет и
	// эмбеддинг, и реранк — длинная минералогическая база доминирует, и
	// реранкер честно топит специализированный контент (проверено напрямую:
	// с base+suffix глава Поварова "3.4 Диаметр песковой насадки" не
	// попадала в топ вообще, со standalone topic — топ-1 с score 0.978).
	topic string
	// useEquipment — добавить в запрос фасета реальное оборудование фабрики
	// из spec.AvailableEquipment: делает фасет конкретным для этой площадки
	// (гидроциклон какой модели, какие мельницы), а не абстрактной темой.
	useEquipment bool
	minFloor     int // сколько слотов гарантированы фасету, даже если его скор ниже других
}

// retrievalFacetsByDomain — детерминированная декомпозиция запроса на
// РАЗНЫЕ подтемы (не LLM-перефразы одного запроса), каждая ищется и
// реранжируется своим самостоятельным запросом. minFloor — анти-голодание
// (гарантированный минимум слотов фасету), не процентная квота: остаток
// topK — открытая конкуренция по сырому (сигмоид-калиброванному, потому
// сравнимому между фасетами) реранкер-скору. Домен без записи в реестре —
// см. facetsForDomain.
var retrievalFacetsByDomain = map[string][]retrievalFacet{
	"flotation": {
		{topic: "", minFloor: 3},
		{topic: "оборудование и классификация: гидроциклоны, песковая насадка, влияние диаметра насадки на крупность слива и потери металла, спиральные классификаторы, грохота, магнитная сепарация", useEquipment: true, minFloor: 2},
		{topic: "измельчение: степень измельчения, крупность помола, раскрытие сростков минералов, футеровка мельниц, замкнутый цикл измельчения с классификацией", useEquipment: true, minFloor: 2},
	},
}

// buildFacetQuery — самостоятельный текст запроса фасета: тема + (для
// equipment-фасетов) реальное оборудование фабрики из ProblemSpec. Из
// записей оборудования берётся только модель (текст до скобки с позицией в
// схеме): "ГЦ-660 (Линии 4-2, 5-3, поз. 4-2-ГЦ-660...)" тянет в эмбеддинг
// запроса номера линий/позиций — шум, разбавляющий тематическую близость
// к литературе про сам аппарат.
func buildFacetQuery(baseQuery string, facet retrievalFacet, spec domain.ProblemSpec) string {
	if facet.topic == "" {
		return baseQuery
	}
	q := facet.topic
	if facet.useEquipment && len(spec.AvailableEquipment) > 0 {
		models := make([]string, 0, len(spec.AvailableEquipment))
		for _, e := range spec.AvailableEquipment {
			if i := strings.Index(e, " ("); i > 0 {
				e = e[:i]
			}
			models = append(models, e)
		}
		q += ". Оборудование фабрики: " + strings.Join(models, ", ")
	}
	return q
}

// facetsForDomain возвращает таксономию фасетов для домена; без записи в
// реестре — единственный base-query фасет с floor=topK (эквивалент
// single-query retrieval, без декомпозиции, но и без чужой таксономии).
func facetsForDomain(domainFilter string, topK int) []retrievalFacet {
	if facets, ok := retrievalFacetsByDomain[domainFilter]; ok {
		return facets
	}
	return []retrievalFacet{{topic: "", minFloor: topK}}
}

// retrieve — facet-декомпозированный гибридный поиск: каждый фасет параллельно
// получает свой embed+HybridSearch+rerank; итоговый topK — floor-проход
// (анти-голодание) плюс open-pool проход (остаток бюджета из объединённого
// пула по скору, dedup по chunk ID).
func retrieve(ctx context.Context, chunks *repositories.ChunkRepo, pyworker *externalApi.PyworkerClient, spec domain.ProblemSpec, domainFilter string, topK int) ([]domain.RetrievedChunk, error) {
	baseQuery := buildRetrievalQuery(spec)
	facets := facetsForDomain(domainFilter, topK)

	type facetResult struct {
		candidates []domain.RetrievedChunk
		err        error
	}
	results := make([]facetResult, len(facets))
	var wg sync.WaitGroup
	for i, facet := range facets {
		wg.Add(1)
		go func(i int, facet retrievalFacet) {
			defer wg.Done()
			q := buildFacetQuery(baseQuery, facet, spec)
			vec, err := pyworker.EmbedOne(ctx, q)
			if err != nil {
				results[i] = facetResult{err: fmt.Errorf("embed facet query: %w", err)}
				return
			}
			cands, err := chunks.HybridSearch(ctx, q, vec, domainFilter, 40)
			if err != nil {
				results[i] = facetResult{err: err}
				return
			}
			if len(cands) == 0 {
				results[i] = facetResult{}
				return
			}
			texts := make([]string, len(cands))
			for j, c := range cands {
				texts[j] = c.Content
			}
			if scores, err := pyworker.Rerank(ctx, q, texts); err == nil && len(scores) == len(cands) {
				for j := range cands {
					cands[j].FusedScore = scores[j]
				}
				sortByFusedScoreDesc(cands)
			}
			results[i] = facetResult{candidates: cands}
		}(i, facet)
	}
	wg.Wait()

	var lastErr error
	succeeded := false
	seen := make(map[string]bool)
	var candidates []domain.RetrievedChunk

	for i, r := range results {
		if r.err != nil {
			lastErr = r.err
			continue
		}
		succeeded = true
		taken := 0
		for _, c := range r.candidates {
			if taken >= facets[i].minFloor {
				break
			}
			id := c.ID.String()
			if seen[id] {
				continue
			}
			seen[id] = true
			candidates = append(candidates, c)
			taken++
		}
	}
	if !succeeded {
		return nil, fmt.Errorf("all retrieval facets failed: %w", lastErr)
	}

	var pool []domain.RetrievedChunk
	for _, r := range results {
		for _, c := range r.candidates {
			if !seen[c.ID.String()] {
				pool = append(pool, c)
			}
		}
	}
	sortByFusedScoreDesc(pool)
	remaining := topK - len(candidates)
	for _, c := range pool {
		if remaining <= 0 {
			break
		}
		id := c.ID.String()
		if seen[id] {
			continue
		}
		seen[id] = true
		candidates = append(candidates, c)
		remaining--
	}

	return candidates, nil
}

func buildRetrievalQuery(spec domain.ProblemSpec) string {
	var b strings.Builder
	b.WriteString(spec.TargetKPI)
	if len(spec.TargetMetals) > 0 {
		b.WriteString(". Металлы: " + strings.Join(spec.TargetMetals, ", "))
	}
	if len(spec.LossHotspots) > 0 {
		b.WriteString(". Точки потерь: " + strings.Join(spec.LossHotspots, "; "))
	}
	if spec.Plant != "" {
		b.WriteString(". Фабрика: " + spec.Plant)
	}
	return b.String()
}

func sortByFusedScoreDesc(cs []domain.RetrievedChunk) {
	sort.Slice(cs, func(i, j int) bool { return cs[i].FusedScore > cs[j].FusedScore })
}
