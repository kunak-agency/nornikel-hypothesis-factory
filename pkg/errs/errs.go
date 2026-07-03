// Package errs содержит типизированную модель ошибок приложения.
// Ошибки несут Type (маппится в HTTP-статус), безопасный для клиента Message,
// опциональные Reason/Details/Cause и автоматически захваченные данные о Caller.
package errs

import (
	"errors"
	"fmt"
	"net/http"
	"path"
	"runtime"
)

// Type категоризует доменные ошибки. HTTP-слой маппит Type в статус-код.
type Type string

const (
	ErrTypeValidation Type = "VALIDATION" // 422
	ErrTypeNotFound   Type = "NOT_FOUND"  // 404
	ErrTypeConflict   Type = "CONFLICT"   // 409
	ErrTypeTimeout    Type = "TIMEOUT"    // 504
	ErrTypeInternal   Type = "INTERNAL"   // 500
)

const (
	ErrorNotFound   = "nothing was found"
	ErrorBadRequest = "bad request"
	ErrorValidation = "validation error"
	ErrorConflict   = "conflict"
	ErrorTimeout    = "request timed out"
	ErrorInternal   = "internal error"
)

// FieldError сообщает об одной ошибке валидации на уровне поля.
type FieldError struct {
	Field   string `json:"field"`
	Rule    string `json:"rule,omitempty"`
	Message string `json:"message"`
}

// Error — каноническая доменная ошибка. Безопасно сериализуется в JSON как есть.
type Error struct {
	Type    Type         `json:"type"`
	Message string       `json:"message"`
	Reason  string       `json:"reason,omitempty"`
	Details []FieldError `json:"details,omitempty"`
	Caller  string       `json:"caller,omitempty"`
	cause   error
}

func (e *Error) Error() string {
	s := fmt.Sprintf("%s: %s", e.Type, e.Message)
	if e.Reason != "" {
		s += ": " + e.Reason
	}
	if e.cause != nil {
		s += ": " + e.cause.Error()
	}
	return s
}

func (e *Error) Unwrap() error { return e.cause }

// Is сравнивает по Type, чтобы sentinel вида &Error{Type: ErrTypeNotFound}
// можно было сопоставлять через errors.Is.
func (e *Error) Is(target error) bool {
	var t *Error
	if !errors.As(target, &t) {
		return false
	}
	return e.Type == t.Type
}

func (e *Error) WithCause(err error) *Error { e.cause = err; return e }
func (e *Error) WithReason(r string) *Error { e.Reason = r; return e }
func (e *Error) WithDetails(d ...FieldError) *Error {
	e.Details = append(e.Details, d...)
	return e
}

// HTTPStatus возвращает общепринятый HTTP-статус для этой ошибки.
func (e *Error) HTTPStatus() int {
	switch e.Type {
	case ErrTypeValidation:
		return http.StatusUnprocessableEntity
	case ErrTypeNotFound:
		return http.StatusNotFound
	case ErrTypeConflict:
		return http.StatusConflict
	case ErrTypeTimeout:
		return http.StatusGatewayTimeout
	}
	return http.StatusInternalServerError
}

func NewError(t Type, message string) *Error { return build(t, message, "") }
func NewValidationError(reason string) *Error {
	return build(ErrTypeValidation, ErrorValidation, reason)
}
func NewBadRequestError(reason string) *Error {
	return build(ErrTypeValidation, ErrorBadRequest, reason)
}
func NewNotFoundError(reason string) *Error { return build(ErrTypeNotFound, ErrorNotFound, reason) }
func NewConflictError(reason string) *Error { return build(ErrTypeConflict, ErrorConflict, reason) }
func NewTimeoutError(reason string) *Error  { return build(ErrTypeTimeout, ErrorTimeout, reason) }
func NewInternalError(reason string) *Error { return build(ErrTypeInternal, ErrorInternal, reason) }

// Wrap превращает любую нижележащую ошибку в Error заданного типа,
// прикрепляя оригинал как cause. Используется на границах (БД, внешний API).
func Wrap(err error, t Type, message string) *Error {
	e := build(t, message, "")
	e.cause = err
	return e
}

// build конструирует *Error и захватывает место вызова пользовательского кода как Caller.
func build(t Type, message, reason string) *Error {
	pc, file, line, ok := runtime.Caller(2)
	caller := ""
	if ok {
		fname := "?"
		if fn := runtime.FuncForPC(pc); fn != nil {
			fname = fn.Name()
		}
		caller = fmt.Sprintf("%s at %s:%d", fname, path.Base(file), line)
	}
	return &Error{Type: t, Message: message, Reason: reason, Caller: caller}
}
