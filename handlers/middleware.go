package handlers

import (
	"context"

	"hypothesis-factory/in"
	"hypothesis-factory/pkg/logger"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/requestid"
)

func InjectRequestID() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if rid, ok := c.Locals(requestid.ConfigDefault.ContextKey).(string); ok && rid != "" {
			ctx := context.WithValue(c.UserContext(), logger.RequestIDKey, rid)
			c.SetUserContext(ctx)
		}
		return c.Next()
	}
}

func RequestIDFrom(c *fiber.Ctx) string {
	if v, ok := c.UserContext().Value(logger.RequestIDKey).(string); ok {
		return v
	}
	return ""
}

const ctxKeyPagination = "pagination"

// Pagination читает ?page=/?perPage= из query и кладёт *in.Pagination в
// Locals. Вешается только на list-эндпоинты.
func Pagination() fiber.Handler {
	return func(c *fiber.Ctx) error {
		page := c.QueryInt("page", 0)
		perPage := c.QueryInt("perPage", 0)
		c.Locals(ctxKeyPagination, in.NewPagination(page, perPage))
		return c.Next()
	}
}

func GetPagination(c *fiber.Ctx) *in.Pagination {
	if v, ok := c.Locals(ctxKeyPagination).(*in.Pagination); ok && v != nil {
		return v
	}
	return in.NewPagination(0, 0)
}
