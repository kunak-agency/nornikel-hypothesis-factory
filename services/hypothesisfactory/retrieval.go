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

// retrievalFacetsByDomain — детерминированная (не LLM-генерируемая на лету)
// декомпозиция запроса на аспекты предметной области. Deep-research
// (2026-07, 106 агентов, авторитетные источники) подтвердил: наивный
// paraphrase-style multi-query (RAG-Fusion — LLM перефразирует один и тот же
// запрос) не даёт статистически значимого прироста recall и может даже
// ухудшать precision на узкоспециализированных корпусах; а facet-based
// декомпозиция (РАЗНЫЕ подтемы, не перефразы) с reranking каждого фасета
// своим запросом — воспроизведённо работает (MRR@10 +36.7%, answer F1
// +11.6% в исследованиях: decomposition расширяет recall-охват, reranker
// восстанавливает precision — одно без другого не работает).
//
// Важный урок с первой попытки этого фикса: реранкинг ОБЪЕДИНЁННОГО пула
// ПРОТИВ ЕДИНОГО базового (минералогического) запроса заново топит
// оборудование ниже topK — реранкер честно видит, что чанк про гидроциклоны
// низко релевантен тексту вида "потери Ni в классе -71+45 мкм", даже если
// чанк был найден правильным facet-запросом. Чтобы facet-based decomposition
// реально работала, каждый фасет должен реранжироваться СВОИМ запросом и
// иметь зарезервированную долю топ-K слотов — иначе финальная глобальная
// сортировка по единому скору откатывает всё к поведению single-query.
//
// Базовый запрос из ProblemSpec естественно покрывает минералогию/реагентику
// (LossHotspots уже упоминает минеральные формы), но систематически
// недотягивается до оборудования/схемы измельчения-классификации — на живом
// сравнении с экспертными гипотезами (2 независимых кейса) система выдавала
// 0 гипотез про оборудование при 100% экспертных гипотез именно про него,
// хотя контент в базе есть (проверено: десятки/сотни релевантных чанков) —
// просто не долетал до top-K по чисто минералогическому запросу. Эти два
// фасета добавлены явно, чтобы закрыть систематический слепой угол.
//
// Фасеты хранятся per-domain в retrievalFacetsByDomain (см. facetsForDomain
// ниже) — домен без записи в реестре откатывается на единственный
// base-query фасет вместо того, чтобы молча получить таксономию "flotation".
//
// minFloor — не "справедливая доля", а анти-голодание: гарантия, что фасет
// не обнулится структурно из-за того, что его тема хуже ложится в лексику
// запроса. Раньше здесь был жёсткий %-бюджет (50/25/25) — тот же самый
// изъян, что и у single-query: если у фасета "оборудование" по факту 20
// сильных кандидатов, а у базового — 3 слабых, жёсткий процент всё равно
// режет оборудование до квоты, вместо того чтобы отдать простаивающие слоты
// туда, где контент реально лучше. Теперь: каждый фасет гарантированно
// получает minFloor лучших своих кандидатов, а ВЕСЬ остаток topK — открытая
// конкуренция по сырому reranker-скору между всеми фасетами разом (скор
// bge-reranker сигмоид-калиброван, т.е. интерпретируется как P(релевантно
// своему запросу) и потому сравним между разными facet-запросами).
var retrievalFacetsByDomain = map[string][]retrievalFacet{
	"flotation": {
		{suffix: "", minFloor: 3},
		{suffix: "оборудование и схема классификации: гидроциклоны, диаметр насадок, классификаторы, грохота, магнитная сепарация", minFloor: 2},
		{suffix: "измельчение: степень измельчения, крупность помола, футеровка мельниц, цепь измельчения", minFloor: 2},
	},
}

// facetsForDomain возвращает таксономию фасетов для конкретного домена —
// domainFilter уже был обязательным параметром retrieve() и раньше просто
// игнорировался при выборе фасетов, из-за чего второй домен молча получал бы
// facet-таксономию "flotation" вместо своей. Для домена без записи в реестре
// используется единственный base-query фасет с floor=topK — эквивалент
// single-query retrieval (без выгоды декомпозиции, но и без искусственного
// урезания топа под чужую тематику).
func facetsForDomain(domainFilter string, topK int) []retrievalFacet {
	if facets, ok := retrievalFacetsByDomain[domainFilter]; ok {
		return facets
	}
	return []retrievalFacet{{suffix: "", minFloor: topK}}
}

// retrieve — facet-декомпозированный гибридный (лексика+вектор) поиск:
// параллельно по каждому фасету — свой embed, свой HybridSearch, свой
// reranking СВОИМ запросом. Отбор в итоговый topK — два прохода: (1) каждый
// фасет берёт свои minFloor лучших (анти-голодание), (2) весь остаток
// бюджета заполняется лучшими по скору кандидатами из ОБЪЕДИНЁННОГО пула
// (dedup по chunk ID), независимо от того, какому фасету они принадлежат —
// сильный фасет с запасом хорошего контента не режется искусственно, слабый
// не разбавляется мусором ради процента.
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
			// Реранкинг СВОИМ (facet-специфичным) запросом — ключевое
			// отличие от первой (не сработавшей) версии фикса: так топ
			// кандидатов фасета "оборудование" реально про оборудование,
			// а не про то, что близко к минералогии базового запроса.
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

	// Проход 1: анти-голодание — каждый фасет забирает свои minFloor лучших,
	// пока их не отобрал более приоритетный фасет.
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

	// Проход 2: открытый пул — весь остаток topK заполняется лучшими по
	// reranker-скору кандидатами из всех фасетов сразу (сигмоид-скор
	// сравним между разными facet-запросами, это не сырое косинусное
	// сходство с разными точками отсчёта).
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
