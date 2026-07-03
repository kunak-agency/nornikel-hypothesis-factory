package hypothesisfactory

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"hypothesis-factory/domain"
	"hypothesis-factory/externalApi"
	"hypothesis-factory/repositories"
)

// retrievalFacet — фасет запроса + доля topK-бюджета, гарантированно
// зарезервированная за ним. Веса, не равные доли — базовый факт остаётся
// главным сигналом (он же формулировка проблемы), остальные закрывают
// конкретный систематический слепой угол.
type retrievalFacet struct {
	suffix string // "" = базовый запрос как есть
	weight float64
}

// retrievalFacets — детерминированная (не LLM-генерируемая на лету)
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
// Фасеты сейчас specific для домена "flotation" — при добавлении новых
// доменов потребуется своя таксономия фасетов на домен (TODO, не блокер).
var retrievalFacets = []retrievalFacet{
	{suffix: "", weight: 0.5},
	{suffix: "оборудование и схема классификации: гидроциклоны, диаметр насадок, классификаторы, грохота, магнитная сепарация", weight: 0.25},
	{suffix: "измельчение: степень измельчения, крупность помола, футеровка мельниц, цепь измельчения", weight: 0.25},
}

// retrieve — facet-декомпозированный гибридный (лексика+вектор) поиск:
// параллельно по каждому фасету — свой embed, свой HybridSearch, свой
// reranking СВОИМ запросом — затем каждый фасет отдаёт свою зарезервированную
// долю итогового topK (дедуп по chunk ID: чанк, уже добавленный фасетом
// повыше приоритетом, не дублируется фасетом пониже) перед (дорогими)
// вызовами claim extraction.
func retrieve(ctx context.Context, chunks *repositories.ChunkRepo, pyworker *externalApi.PyworkerClient, spec domain.ProblemSpec, domainFilter string, topK int) ([]domain.RetrievedChunk, error) {
	baseQuery := buildRetrievalQuery(spec)

	type facetResult struct {
		candidates []domain.RetrievedChunk
		err        error
	}
	results := make([]facetResult, len(retrievalFacets))
	var wg sync.WaitGroup
	for i, facet := range retrievalFacets {
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
	for i, r := range results {
		if r.err != nil {
			lastErr = r.err
			continue
		}
		succeeded = true
		budget := int(float64(topK)*retrievalFacets[i].weight + 0.5)
		if budget < 1 {
			budget = 1
		}
		taken := 0
		for _, c := range r.candidates {
			if taken >= budget {
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
	for i := 1; i < len(cs); i++ {
		for j := i; j > 0 && cs[j].FusedScore > cs[j-1].FusedScore; j-- {
			cs[j], cs[j-1] = cs[j-1], cs[j]
		}
	}
}
