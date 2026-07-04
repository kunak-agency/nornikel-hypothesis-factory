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
	suffix   string // "" = базовый запрос как есть
	minFloor int    // сколько слотов гарантированы фасету, даже если его скор ниже других
}

// retrievalFacetsByDomain — детерминированная декомпозиция запроса на
// РАЗНЫЕ подтемы (не LLM-перефразы одного запроса), реранжируемые каждая
// своим запросом: единый реранкинг против общего минералогического запроса
// топит контент про оборудование ниже topK, даже если он был найден верным
// facet-запросом. minFloor — анти-голодание (гарантированный минимум слотов
// фасету), не процентная квота: остаток topK — открытая конкуренция по
// сырому (сигмоид-калиброванному, потому сравнимому между фасетами)
// реранкер-скору. Домен без записи в реестре — см. facetsForDomain.
var retrievalFacetsByDomain = map[string][]retrievalFacet{
	"flotation": {
		{suffix: "", minFloor: 3},
		{suffix: "оборудование и схема классификации: гидроциклоны, диаметр насадок, классификаторы, грохота, магнитная сепарация", minFloor: 2},
		{suffix: "измельчение: степень измельчения, крупность помола, футеровка мельниц, цепь измельчения", minFloor: 2},
	},
}

// facetsForDomain возвращает таксономию фасетов для домена; без записи в
// реестре — единственный base-query фасет с floor=topK (эквивалент
// single-query retrieval, без декомпозиции, но и без чужой таксономии).
func facetsForDomain(domainFilter string, topK int) []retrievalFacet {
	if facets, ok := retrievalFacetsByDomain[domainFilter]; ok {
		return facets
	}
	return []retrievalFacet{{suffix: "", minFloor: topK}}
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
			q := baseQuery
			if facet.suffix != "" {
				q = baseQuery + ". " + facet.suffix
			}
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
