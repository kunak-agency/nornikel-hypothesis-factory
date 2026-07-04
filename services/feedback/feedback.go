// Package feedback принимает экспертную оценку гипотез (confirmed/rejected/
// needs_revision). Оценки питают "обучение на фидбэке": репутация сущностей
// (см. hypothesisfactory.loadEntityReputations) подмешивается в следующие
// прогоны генерации.
package feedback

import (
	"context"

	"hypothesis-factory/domain"
	"hypothesis-factory/pkg/errs"
	"hypothesis-factory/repositories"

	"github.com/google/uuid"
)

type Service struct {
	repos *repositories.Repos
}

func NewService(repos *repositories.Repos) *Service {
	return &Service{repos: repos}
}

type SubmitInput struct {
	HypothesisID uuid.UUID
	Verdict      string
	Comment      string
	Reviewer     string
}

func (s *Service) Submit(ctx context.Context, in SubmitInput) (*domain.Feedback, error) {
	h, err := s.repos.Hypotheses.GetByID(ctx, in.HypothesisID)
	if err != nil {
		return nil, errs.Wrap(err, errs.ErrTypeInternal, "get hypothesis")
	}
	if h == nil {
		return nil, errs.NewNotFoundError("hypothesis")
	}

	fb := &domain.Feedback{
		HypothesisID: in.HypothesisID,
		Verdict:      in.Verdict,
		Comment:      in.Comment,
		Reviewer:     in.Reviewer,
	}
	if err := s.repos.Feedback.Create(ctx, fb); err != nil {
		return nil, errs.Wrap(err, errs.ErrTypeInternal, "create feedback")
	}
	return fb, nil
}
