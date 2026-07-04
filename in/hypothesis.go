package in

import "hypothesis-factory/domain"

// UpdateVerificationPlanRequest — полная замена дорожной карты проверки гипотезы.
// Визуальный конструктор на фронте отдаёт весь массив шагов целиком (PUT-семантика,
// идемпотентно). Правится ТОЛЬКО дорожная карта; statement/mechanism/scores/
// evidenceRefs остаются машинными (обоснованы источниками) и здесь не принимаются.
type UpdateVerificationPlanRequest struct {
	// min=1: `required` на слайсе проверяет лишь non-nil, а JSON `[]` декодится в
	// непустой len-0 слайс и прошёл бы валидацию — пустой массив молча стёр бы
	// сохранённую дорожную карту. min=1 закрывает это (пустой массив → 422).
	VerificationPlan []VerificationStepInput `json:"verificationPlan" validate:"required,min=1,max=50,dive"`
}

type VerificationStepInput struct {
	Step              string `json:"step"              validate:"required" example:"Лабораторный тест флотации при pH 9.5"`
	Resource          string `json:"resource"          example:"Лаборатория ОФ, реагенты"`
	SuccessCriterion  string `json:"successCriterion"  example:"Извлечение Ni +1.5% против базы"`
	EstimatedDuration string `json:"estimatedDuration" example:"1-2 недели"`
	EstimatedCost     string `json:"estimatedCost"     example:"~200 т.р. на реагенты"`
}

// ToDomain мапит вход в доменную модель. Поле successCriterion → SuccessCrit
// (json-тег домена — successCriterion, имя поля — SuccessCrit).
func (r UpdateVerificationPlanRequest) ToDomain() []domain.VerificationStep {
	out := make([]domain.VerificationStep, 0, len(r.VerificationPlan))
	for _, s := range r.VerificationPlan {
		out = append(out, domain.VerificationStep{
			Step:              s.Step,
			Resource:          s.Resource,
			SuccessCrit:       s.SuccessCriterion,
			EstimatedDuration: s.EstimatedDuration,
			EstimatedCost:     s.EstimatedCost,
		})
	}
	return out
}
