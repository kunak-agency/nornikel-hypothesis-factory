package handlers

import "github.com/gofiber/fiber/v2"

// Health — infrastructure liveness probe.
// @Summary      Проверка живости сервиса
// @Description  Возвращает {"status":"ok"}, если приложение поднято и обслуживает запросы.
// @Tags         health
// @Produce      json
// @Success      200  {object}  map[string]string
// @Router       /health [get]
func (h *Handler) Health(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"status": "ok"})
}
