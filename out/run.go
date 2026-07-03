package out

import (
	"time"

	"hypothesis-factory/domain"

	"github.com/google/uuid"
)

// ProblemSpecResponse — camelCase wire-DTO для API-ответов. domain.ProblemSpec
// сам держит snake_case-теги под контракт с LLM (см. комментарий там); здесь —
// явный маппинг под фронт.
type ProblemSpecResponse struct {
	TargetKPI          string   `json:"targetKpi"`
	Plant              string   `json:"plant"`
	TargetMetals       []string `json:"targetMetals"`
	LossHotspots       []string `json:"lossHotspots"`
	AvailableEquipment []string `json:"availableEquipment"`
	Constraints        []string `json:"constraints"`
	Horizon            string   `json:"horizon"`
}

func ProblemSpecFromDomain(s domain.ProblemSpec) ProblemSpecResponse {
	return ProblemSpecResponse{
		TargetKPI:          s.TargetKPI,
		Plant:              s.Plant,
		TargetMetals:       s.TargetMetals,
		LossHotspots:       s.LossHotspots,
		AvailableEquipment: s.AvailableEquipment,
		Constraints:        s.Constraints,
		Horizon:            s.Horizon,
	}
}

type RunResponse struct {
	ID          uuid.UUID           `json:"id"`
	Status      string              `json:"status" example:"retrieving"`
	ProblemSpec ProblemSpecResponse `json:"problemSpec"`
	Domain      string              `json:"domain"`
	Language    string              `json:"language"`
	// KnowledgeGaps — металлы/точки потерь из ProblemSpec, слабо покрытые
	// извлечёнными claims (детерминированная проверка, не LLM-догадка).
	KnowledgeGaps []string             `json:"knowledgeGaps,omitempty"`
	Error         string               `json:"error,omitempty"`
	CreatedAt     time.Time            `json:"createdAt"`
	CompletedAt   *time.Time           `json:"completedAt,omitempty"`
	Hypotheses    []HypothesisResponse `json:"hypotheses,omitempty"`
}

func RunFromDomain(r *domain.HypothesisRun, hyps []domain.Hypothesis) RunResponse {
	resp := RunResponse{
		ID:            r.ID,
		Status:        r.Status,
		ProblemSpec:   ProblemSpecFromDomain(r.ProblemSpec),
		Domain:        r.Domain,
		Language:      r.Language,
		KnowledgeGaps: r.KnowledgeGaps,
		Error:         r.Error,
		CreatedAt:     r.CreatedAt,
		CompletedAt:   r.CompletedAt,
	}
	if hyps != nil {
		resp.Hypotheses = make([]HypothesisResponse, 0, len(hyps))
		for i := range hyps {
			resp.Hypotheses = append(resp.Hypotheses, HypothesisFromDomain(&hyps[i]))
		}
	}
	return resp
}

type RunListResponse struct {
	Items   []RunResponse `json:"items"`
	Total   int64         `json:"total"`
	Page    int           `json:"page"`
	PerPage int           `json:"perPage"`
}
