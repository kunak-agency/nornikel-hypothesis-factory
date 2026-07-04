package in

import "hypothesis-factory/domain"

// RerankRequest — новые веса критериев для пересортировки готовых гипотез
// прогона (без повторных LLM-вызовов). Незаполненные поля = дефолтные веса.
type RerankRequest struct {
	RankingWeights domain.RankingWeights `json:"rankingWeights"`
}
