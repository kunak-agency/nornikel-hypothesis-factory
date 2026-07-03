package handlers

import (
	"hypothesis-factory/in"
	"hypothesis-factory/out"
	"hypothesis-factory/pkg/errs"
	"hypothesis-factory/services/feedback"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// SubmitFeedback фиксирует экспертную оценку гипотезы (confirmed/rejected/
// needs_revision) — задел под "обучение на фидбэке" из кейса.
// @Summary      Оценка гипотезы экспертом
// @Tags         hypotheses
// @Accept       json
// @Produce      json
// @Param        hypothesisId  path      string                      true  "UUID гипотезы"
// @Param        body          body      in.SubmitFeedbackRequest   true  "Вердикт"
// @Success      201  {object}  out.FeedbackResponse
// @Failure      404  {object}  errs.Error
// @Failure      422  {object}  errs.Error
// @Router       /hypotheses/{hypothesisId}/feedback [post]
func (h *Handler) SubmitFeedback(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("hypothesisId"))
	if err != nil {
		return errs.NewValidationError("invalid hypothesisId")
	}

	var body in.SubmitFeedbackRequest
	if err := c.BodyParser(&body); err != nil {
		return errs.NewBadRequestError("invalid json")
	}
	if err := h.validate.Struct(&body); err != nil {
		return err
	}

	fb, err := h.services.Feedback.Submit(c.UserContext(), feedback.SubmitInput{
		HypothesisID: id,
		Verdict:      body.Verdict,
		Comment:      body.Comment,
		Reviewer:     body.Reviewer,
	})
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(out.FeedbackFromDomain(fb))
}
