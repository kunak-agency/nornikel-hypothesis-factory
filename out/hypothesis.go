package out

import (
	"time"

	"hypothesis-factory/domain"

	"github.com/google/uuid"
)

type HypothesisResponse struct {
	ID                uuid.UUID                  `json:"id"`
	RunID             uuid.UUID                  `json:"runId"`
	Statement         string                     `json:"statement"`
	Mechanism         string                     `json:"mechanism"`
	EvidenceRefs      []uuid.UUID                `json:"evidenceRefs"`
	ExpectedKPIEffect domain.KPIEffect           `json:"expectedKpiEffect"`
	Risks             []string                   `json:"risks"`
	NoveltyReason     string                     `json:"noveltyReason"`
	VerificationPlan  []domain.VerificationStep  `json:"verificationPlan"`
	Scores            domain.Scores              `json:"scores"`
	CriticNotes       string                     `json:"criticNotes"`
	Rank              int                        `json:"rank"`
	CreatedAt         time.Time                  `json:"createdAt"`
}

func HypothesisFromDomain(h *domain.Hypothesis) HypothesisResponse {
	return HypothesisResponse{
		ID:                h.ID,
		RunID:             h.RunID,
		Statement:         h.Statement,
		Mechanism:         h.Mechanism,
		EvidenceRefs:      h.EvidenceRefs,
		ExpectedKPIEffect: h.ExpectedKPIEffect,
		Risks:             h.Risks,
		NoveltyReason:     h.NoveltyReason,
		VerificationPlan:  h.VerificationPlan,
		Scores:            h.Scores,
		CriticNotes:       h.CriticNotes,
		Rank:              h.Rank,
		CreatedAt:         h.CreatedAt,
	}
}

type HypothesisListResponse struct {
	Items []HypothesisResponse `json:"items"`
	Total int                  `json:"total"`
}
