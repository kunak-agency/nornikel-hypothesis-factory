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

// retrievalFacet — фасет статичной доменной таксономии: topic —
// САМОСТОЯТЕЛЬНЫЙ текст запроса ("" = базовый запрос из ProblemSpec), не
// суффикс к базовому — склейка base+topic отравляет и эмбеддинг, и реранк:
// длинная минералогическая база доминирует, и реранкер честно топит
// специализированный контент (проверено напрямую: с base+suffix глава
// Поварова "3.4 Диаметр песковой насадки" не попадала в топ вообще, со
// standalone topic — топ-1 с score 0.978). minFloor — анти-голодание
// (гарантированный минимум слотов фасету), не процентная квота: остаток
// topK — открытая конкуренция по сырому (сигмоид-калиброванному, потому
// сравнимому между фасетами) реранкер-скору.
type retrievalFacet struct {
	topic    string
	minFloor int
}

// retrievalFacetsByDomain — статичная декомпозиция запроса на РАЗНЫЕ
// подтемы (не LLM-перефразы одного запроса) для фабрик БЕЗ структурированных
// данных об оборудовании; при наличии plant_equipment вместо тематических
// фасетов строятся per-type фасеты (см. buildFacetQueries). Первый элемент —
// всегда базовый фасет. Домен без записи в реестре — единственный base-query
// фасет (эквивалент single-query retrieval).
var retrievalFacetsByDomain = map[string][]retrievalFacet{
	"flotation": {
		{topic: "", minFloor: 3},
		{topic: "оборудование и классификация: гидроциклоны, спиральные классификаторы, грохота, магнитная сепарация", minFloor: 2},
		{topic: "измельчение: степень измельчения, крупность помола, раскрытие сростков минералов, замкнутый цикл измельчения с классификацией", minFloor: 2},
	},
}

// equipmentTypeLevers — регулировочные "рычаги" каждого типа аппарата
// (учебникового уровня знание: у гидроциклона ключевые параметры — насадка и
// разгрузочное отношение, у мельницы — футеровка и шаровая загрузка). Каждый
// тип, реально стоящий на фабрике (spec.EquipmentByType из plant_equipment),
// получает СВОЙ фасет "лейбл модели: рычаги" — так специфика запроса
// приходит из факта наличия аппарата на конкретной площадке, а не из
// зашитой в домен темы, и рычаги одного типа не разбавляются рычагами
// шести других (проверено A/B: свалка рычагов всех типов в один запрос
// снова топит главу Поварова "3.4 Диаметр песковой насадки" ниже общих
// глав; концентрированный одно-типовой запрос — топ-1 с 0.978).
var equipmentTypeLevers = map[string]struct{ label, levers string }{
	"hydrocyclone":   {"гидроциклон", "песковая насадка, диаметр насадки, разгрузочное отношение, крупность слива, давление на входе"},
	"mill":           {"мельница", "футеровка мельницы, шаровая загрузка, степень заполнения, циркулирующая нагрузка"},
	"classifier":     {"спиральный классификатор", "плотность слива классификатора, высота слива, эффективность классификации"},
	"screen":         {"грохот", "размер ячейки сита, эффективность грохочения, тонкое грохочение"},
	"flotation_cell": {"флотомашина", "аэрация, время флотации, уровень пульпы, реагентный режим"},
	"thickener":      {"сгуститель", "плотность сгущённого продукта, расход флокулянта"},
	"crusher":        {"дробилка", "ширина разгрузочной щели, степень дробления"},
}

// facetQuery — готовый к исполнению фасет: самостоятельный текст запроса +
// floor анти-голодания.
type facetQuery struct {
	query    string
	minFloor int
}

// buildFacetQueries строит список фасетов прогона. База — всегда запрос из
// ProblemSpec. Если у фабрики есть структурированное оборудование
// (plant_equipment), каждый его тип получает свой концентрированный фасет
// "лейбл + модели + рычаги"; иначе — статичные тематические фасеты домена.
// Floor'ы per-type фасетов урезаются, если типов так много, что floor'ы
// съели бы весь topK, не оставив open-pool ни одного слота.
func buildFacetQueries(baseQuery, domainFilter string, spec domain.ProblemSpec, topK int) []facetQuery {
	static, ok := retrievalFacetsByDomain[domainFilter]
	if !ok {
		return []facetQuery{{query: baseQuery, minFloor: topK}}
	}

	out := []facetQuery{{query: baseQuery, minFloor: static[0].minFloor}}

	if len(spec.EquipmentByType) > 0 {
		floorBudget := topK - static[0].minFloor - 4 // ≥4 слота открытому пулу
		for _, t := range spec.EquipmentTypes {
			tl, known := equipmentTypeLevers[t]
			if !known {
				continue
			}
			floor := 1
			if floorBudget <= 0 {
				floor = 0
			}
			floorBudget -= floor
			// Без кодов моделей: буквальное "ГЦ-660" в запросе взвинчивает
			// реранкер-скор таблиц характеристик и схем, где код встречается
			// дословно, топя литературу о ВЛИЯНИИ параметров аппарата.
			// Модели и так доходят до генерации через AvailableEquipment.
			q := tl.label + ": " + tl.levers + " — влияние на технологические показатели и потери металла"
			out = append(out, facetQuery{query: q, minFloor: floor})
		}
		return out
	}

	for _, f := range static[1:] {
		out = append(out, facetQuery{query: f.topic, minFloor: f.minFloor})
	}
	return out
}

// retrieve — facet-декомпозированный гибридный поиск: каждый фасет параллельно
// получает свой embed+HybridSearch+rerank; итоговый topK — floor-проход
// (анти-голодание) плюс open-pool проход (остаток бюджета из объединённого
// пула по скору, dedup по chunk ID).
func retrieve(ctx context.Context, chunks *repositories.ChunkRepo, pyworker *externalApi.PyworkerClient, spec domain.ProblemSpec, domainFilter string, topK int) ([]domain.RetrievedChunk, error) {
	baseQuery := buildRetrievalQuery(spec)
	facets := buildFacetQueries(baseQuery, domainFilter, spec, topK)

	type facetResult struct {
		candidates []domain.RetrievedChunk
		err        error
	}
	results := make([]facetResult, len(facets))
	var wg sync.WaitGroup
	for i, facet := range facets {
		wg.Add(1)
		go func(i int, facet facetQuery) {
			defer wg.Done()
			q := facet.query
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

	// floorScoreThreshold — floor гарантирует фасету слоты только для
	// кандидатов, которые сам реранкер считает скорее релевантными (сигмоид-
	// скор ≥ 0.5): анти-голодание защищает сильный контент слабого фасета от
	// вытеснения, а не проталкивает в топ мусор фасета, которому в корпусе
	// вообще нечего предъявить (наблюдалось: floor отдавал слот кандидату со
	// скором 0.08).
	const floorScoreThreshold = 0.5
	for i, r := range results {
		if r.err != nil {
			lastErr = r.err
			continue
		}
		succeeded = true
		taken := 0
		for _, c := range r.candidates {
			if taken >= facets[i].minFloor || c.FusedScore < floorScoreThreshold {
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

	// Открытый пул: остаток topK по скору, но не больше poolCapPerFacet
	// добавочных слотов одному специализированному фасету — фасет, чей
	// запрос почти дословно совпал с каким-то документом корпуса, иначе
	// затапливает пул однотипным контентом (наблюдалось: 4 чанка про
	// дробление в топ-15 при задаче о потерях в тонких классах). Базовый
	// фасет (i==0) не ограничивается — это сама формулировка проблемы.
	const poolCapPerFacet = 2
	type poolEntry struct {
		chunk    domain.RetrievedChunk
		facetIdx int
	}
	var pool []poolEntry
	for i, r := range results {
		for _, c := range r.candidates {
			if !seen[c.ID.String()] {
				pool = append(pool, poolEntry{chunk: c, facetIdx: i})
			}
		}
	}
	sort.Slice(pool, func(a, b int) bool { return pool[a].chunk.FusedScore > pool[b].chunk.FusedScore })
	remaining := topK - len(candidates)
	poolTaken := make(map[int]int)
	for _, p := range pool {
		if remaining <= 0 {
			break
		}
		id := p.chunk.ID.String()
		if seen[id] {
			continue
		}
		if p.facetIdx != 0 && poolTaken[p.facetIdx] >= poolCapPerFacet {
			continue
		}
		seen[id] = true
		poolTaken[p.facetIdx]++
		candidates = append(candidates, p.chunk)
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
