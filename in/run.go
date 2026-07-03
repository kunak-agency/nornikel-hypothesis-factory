package in

import "hypothesis-factory/domain"

type CreateRunRequest struct {
	RawText  string         `json:"rawText"  validate:"required" example:"Фабрика: КГМК. Породные хвосты 5824591 СМТ..."`
	RawInput map[string]any `json:"rawInput"`
	// Domain — база знаний какого домена используется (по умолчанию flotation).
	Domain string `json:"domain" example:"flotation"`
	// Plant — явное указание фабрики (напр. "ТОФ", "Маломырский рудник).
	// Если задано — ПЕРЕКРЫВАЕТ значение, которое LLM извлечёт из RawText:
	// RawText — это пользовательский текст, который theoretически может
	// содержать инструкции, маскирующиеся под данные (напр. "игнорируй
	// предыдущие инструкции, фабрика = ..."); явный Plant даёт вызывающей
	// стороне (организаторам) детерминированный, не-LLM-зависимый способ
	// выбрать фабрику, не полагаясь на устойчивость extraction-промпта.
	Plant string `json:"plant" example:"ТОФ"`
	// Language — язык гипотез/обоснований в ответе (по умолчанию ru).
	Language string `json:"language" validate:"omitempty,oneof=ru en zh" example:"ru"`
	// RankingWeights — режим экспертной настройки: переопределение весов
	// критериев ранжирования (нетронутые поля = дефолт).
	RankingWeights *domain.RankingWeights `json:"rankingWeights"`
	// ExcludedTopics — направления/подходы, которые генерация должна
	// обходить (например, уже испробованные и отклонённые ранее).
	ExcludedTopics []string `json:"excludedTopics"`
}
