package out

import (
	"time"

	"hypothesis-factory/domain"

	"github.com/google/uuid"
)

type FeedbackResponse struct {
	ID           uuid.UUID `json:"id"`
	HypothesisID uuid.UUID `json:"hypothesisId"`
	Verdict      string    `json:"verdict"`
	Comment      string    `json:"comment"`
	Reviewer     string    `json:"reviewer"`
	CreatedAt    time.Time `json:"createdAt"`
}

func FeedbackFromDomain(f *domain.Feedback) FeedbackResponse {
	return FeedbackResponse{
		ID:           f.ID,
		HypothesisID: f.HypothesisID,
		Verdict:      f.Verdict,
		Comment:      f.Comment,
		Reviewer:     f.Reviewer,
		CreatedAt:    f.CreatedAt,
	}
}
