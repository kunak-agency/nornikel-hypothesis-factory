package handlers

import (
	"os"

	"hypothesis-factory/services"

	"github.com/go-playground/validator/v10"
)

type Handler struct {
	exposeDebug bool // если false, errs.Error.Caller вырезается из ответов
	validate    *validator.Validate
	services    *services.Services
}

func NewHandlerManager(svc *services.Services) *Handler {
	return &Handler{
		exposeDebug: os.Getenv("APP_ENV") != "production",
		validate:    validator.New(validator.WithRequiredStructEnabled()),
		services:    svc,
	}
}
