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

// UpdateVerificationPlan сохраняет отредактированную пользователем дорожную карту
// проверки гипотезы (визуальный конструктор из кейса). Полная замена массива шагов;
// правится только verificationPlan, остальные поля гипотезы машинные и неизменны.
// @Summary      Обновление дорожной карты проверки гипотезы
// @Tags         hypotheses
// @Accept       json
// @Produce      json
// @Param        hypothesisId  path      string                            true  "UUID гипотезы"
// @Param        body          body      in.UpdateVerificationPlanRequest  true  "Шаги дорожной карты"
// @Success      200  {object}  out.HypothesisResponse
// @Failure      404  {object}  errs.Error
// @Failure      422  {object}  errs.Error
// @Router       /hypotheses/{hypothesisId}/verification-plan [put]
func (h *Handler) UpdateVerificationPlan(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("hypothesisId"))
	if err != nil {
		return errs.NewValidationError("invalid hypothesisId")
	}

	var body in.UpdateVerificationPlanRequest
	if err := c.BodyParser(&body); err != nil {
		return errs.NewBadRequestError("invalid json")
	}
	if err := h.validate.Struct(&body); err != nil {
		return err
	}

	hyp, err := h.services.Pipeline.UpdateVerificationPlan(c.UserContext(), id, body.ToDomain())
	if err != nil {
		return err
	}
	return c.JSON(out.HypothesisFromDomain(hyp))
}

// ListHypothesisFeedback возвращает все экспертные оценки гипотезы.
// @Summary      Оценки гипотезы
// @Tags         hypotheses
// @Produce      json
// @Param        hypothesisId  path  string  true  "UUID гипотезы"
// @Success      200  {object}  out.FeedbackListResponse
// @Failure      422  {object}  errs.Error
// @Router       /hypotheses/{hypothesisId}/feedback [get]
func (h *Handler) ListHypothesisFeedback(c *fiber.Ctx) error {
	items, err := h.services.Pipeline.GetHypothesisFeedback(c.UserContext(), c.Params("hypothesisId"))
	if err != nil {
		return err
	}
	resp := out.FeedbackListResponse{Items: make([]out.FeedbackResponse, 0, len(items)), Total: len(items)}
	for i := range items {
		resp.Items = append(resp.Items, out.FeedbackFromDomain(&items[i]))
	}
	return c.JSON(resp)
}

// ListEntityReputations — репутация сущностей по накопленному фидбэку:
// видимая сторона "обучения на фидбэке".
// @Summary      Репутация сущностей (обучение на фидбэке)
// @Tags         entities
// @Produce      json
// @Success      200  {object}  out.EntityReputationListResponse
// @Failure      500  {object}  errs.Error
// @Router       /entities/reputation [get]
func (h *Handler) ListEntityReputations(c *fiber.Ctx) error {
	stats, err := h.services.Pipeline.EntityReputations(c.UserContext())
	if err != nil {
		return err
	}
	resp := out.EntityReputationListResponse{Items: make([]out.EntityReputationResponse, 0, len(stats))}
	for _, s := range stats {
		resp.Items = append(resp.Items, out.EntityReputationResponse{
			EntityID: s.EntityID, CanonicalName: s.CanonicalName,
			Confirmed: s.Confirmed, Rejected: s.Rejected, NeedsRevision: s.NeedsRevision,
		})
	}
	return c.JSON(resp)
}
