package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"hypothesis-factory/pkg/errs"
	"hypothesis-factory/pkg/logger"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
)

type envelope struct {
	Error     *errs.Error `json:"error"`
	RequestID string      `json:"requestId,omitempty"`
}

// Errors реализует fiber.ErrorHandler — единая точка маппинга error -> HTTP-статус+JSON.
func (h *Handler) Errors(c *fiber.Ctx, err error) error {
	if err == nil {
		return nil
	}

	ctx := c.UserContext()
	rid := RequestIDFrom(c)

	if errors.Is(err, context.DeadlineExceeded) {
		de := &errs.Error{Type: errs.ErrTypeTimeout, Message: errs.ErrorTimeout}
		return h.write(c, ctx, http.StatusGatewayTimeout, err, de, rid)
	}
	if errors.Is(err, context.Canceled) {
		logger.LogWarningCtx(ctx, "client canceled: method=%s path=%s", c.Method(), c.Path())
		return nil
	}

	var de *errs.Error
	if errors.As(err, &de) {
		return h.write(c, ctx, de.HTTPStatus(), err, de, rid)
	}

	var fe *fiber.Error
	if errors.As(err, &fe) {
		return h.write(c, ctx, fe.Code, err, fromFiber(fe), rid)
	}

	var ve validator.ValidationErrors
	if errors.As(err, &ve) {
		de := errs.NewValidationError("request validation failed")
		for _, fe := range ve {
			de = de.WithDetails(errs.FieldError{Field: fe.Field(), Rule: fe.Tag(), Message: fe.Error()})
		}
		return h.write(c, ctx, de.HTTPStatus(), err, de, rid)
	}

	de = &errs.Error{Type: errs.ErrTypeInternal, Message: errs.ErrorInternal}
	return h.write(c, ctx, http.StatusInternalServerError, err, de, rid)
}

func (h *Handler) write(c *fiber.Ctx, ctx context.Context, status int, cause error, body *errs.Error, rid string) error {
	format := "request error: method=%s path=%s status=%d type=%s caller=%q err=%v"
	args := []interface{}{c.Method(), c.Path(), status, body.Type, body.Caller, cause}
	if status >= 500 {
		logger.LogErrorCtx(ctx, format, args...)
	} else {
		logger.LogInfoCtx(ctx, format, args...)
	}

	if !h.exposeDebug && body.Caller != "" {
		bc := *body
		bc.Caller = ""
		body = &bc
	}

	if werr := c.Status(status).JSON(envelope{Error: body, RequestID: rid}); werr != nil {
		logger.LogErrorCtx(ctx, "failed writing error response: request_id=%s err=%v", rid, werr)
		return werr
	}
	return nil
}

func fromFiber(fe *fiber.Error) *errs.Error {
	e := &errs.Error{Reason: fe.Message}
	switch fe.Code {
	case http.StatusNotFound:
		e.Type = errs.ErrTypeNotFound
		e.Message = errs.ErrorNotFound
	case http.StatusConflict:
		e.Type = errs.ErrTypeConflict
		e.Message = errs.ErrorConflict
	case http.StatusUnprocessableEntity:
		e.Type = errs.ErrTypeValidation
		e.Message = errs.ErrorValidation
	case http.StatusBadRequest:
		e.Type = errs.ErrTypeValidation
		e.Message = errs.ErrorBadRequest
	case http.StatusGatewayTimeout:
		e.Type = errs.ErrTypeTimeout
		e.Message = errs.ErrorTimeout
	default:
		if fe.Code >= 500 {
			e.Type = errs.ErrTypeInternal
			e.Message = errs.ErrorInternal
		} else {
			e.Type = errs.Type(strings.ToUpper(strings.ReplaceAll(http.StatusText(fe.Code), " ", "_")))
			e.Message = fe.Message
			e.Reason = ""
		}
	}
	return e
}
