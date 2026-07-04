package out

import (
	"time"

	"hypothesis-factory/domain"

	"github.com/google/uuid"
)

// ClaimResponse — один элемент evidence-pack прогона: извлечённый факт с
// дословной цитатой, источником и списком гипотез, которые его цитируют.
type ClaimResponse struct {
	ID               uuid.UUID      `json:"id"`
	Subject          string         `json:"subject"`
	Action           string         `json:"action"`
	Condition        string         `json:"condition,omitempty"`
	Metric           string         `json:"metric,omitempty"`
	EffectDirection  string         `json:"effectDirection"`
	EffectMagnitude  string         `json:"effectMagnitude,omitempty"`
	SourceConfidence string         `json:"sourceConfidence"`
	Quote            string         `json:"quote"`
	Source           map[string]any `json:"source"`
	CitedByHypIDs    []uuid.UUID    `json:"citedByHypothesisIds"`
	CreatedAt        time.Time      `json:"createdAt"`
}

func ClaimFromDomain(c *domain.Claim, citedBy []uuid.UUID) ClaimResponse {
	return ClaimResponse{
		ID:               c.ID,
		Subject:          c.Subject,
		Action:           c.Action,
		Condition:        c.Condition,
		Metric:           c.Metric,
		EffectDirection:  c.EffectDirection,
		EffectMagnitude:  c.EffectMagnitude,
		SourceConfidence: c.SourceConfidence,
		Quote:            c.Quote,
		Source:           c.Metadata,
		CitedByHypIDs:    citedBy,
		CreatedAt:        c.CreatedAt,
	}
}

type ClaimListResponse struct {
	Items []ClaimResponse `json:"items"`
	Total int             `json:"total"`
}

type FeedbackListResponse struct {
	Items []FeedbackResponse `json:"items"`
	Total int                `json:"total"`
}

// EntityReputationResponse — репутация сущности из графа фидбэка.
type EntityReputationResponse struct {
	EntityID      uuid.UUID `json:"entityId"`
	CanonicalName string    `json:"canonicalName"`
	Confirmed     int       `json:"confirmed"`
	Rejected      int       `json:"rejected"`
	NeedsRevision int       `json:"needsRevision"`
}

type EntityReputationListResponse struct {
	Items []EntityReputationResponse `json:"items"`
}
