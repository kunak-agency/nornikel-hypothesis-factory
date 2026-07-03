package handlers

import (
	"hypothesis-factory/in"
	"hypothesis-factory/out"
	"hypothesis-factory/pkg/errs"
	"hypothesis-factory/services/hypothesisfactory"

	"github.com/gofiber/fiber/v2"
)

// CreateRun запускает пайплайн генерации гипотез. Синхронно выполняется
// только быстрый парсинг ProblemSpec (~1-2с); retrieval->claims->hypotheses->
// critique (~45-90с) уходит в фон — клиент получает run_id сразу (202) и
// поллит GET /runs/{runId} по полю status до done/failed.
// @Summary      Запуск генерации гипотез
// @Description  Принимает свободный текст (профиль хвостов/проблему), возвращает run сразу в статусе retrieving — дальше поллить GET /runs/{runId}.
// @Tags         runs
// @Accept       json
// @Produce      json
// @Param        body  body      in.CreateRunRequest  true  "Запрос на генерацию"
// @Success      202   {object}  out.RunResponse
// @Failure      422   {object}  errs.Error
// @Failure      500   {object}  errs.Error
// @Router       /runs [post]
func (h *Handler) CreateRun(c *fiber.Ctx) error {
	var body in.CreateRunRequest
	if err := c.BodyParser(&body); err != nil {
		return errs.NewBadRequestError("invalid json")
	}
	if err := h.validate.Struct(&body); err != nil {
		return err
	}

	run, err := h.services.Pipeline.StartRun(c.UserContext(), body.RawText, body.RawInput)
	if err != nil {
		return err
	}
	h.services.Pipeline.RunPipelineAsync(run)

	return c.Status(fiber.StatusAccepted).JSON(out.RunFromDomain(run, nil))
}

// GetRun возвращает текущий статус прогона и (если готовы) гипотезы.
// @Summary      Статус и результат прогона
// @Tags         runs
// @Produce      json
// @Param        runId  path      string  true  "UUID прогона"
// @Success      200    {object}  out.RunResponse
// @Failure      404    {object}  errs.Error
// @Router       /runs/{runId} [get]
func (h *Handler) GetRun(c *fiber.Ctx) error {
	run, err := h.services.Pipeline.GetRun(c.UserContext(), c.Params("runId"))
	if err != nil {
		return err
	}
	hyps, err := h.services.Pipeline.GetHypotheses(c.UserContext(), c.Params("runId"))
	if err != nil {
		return err
	}
	return c.JSON(out.RunFromDomain(run, hyps))
}

// ListRuns возвращает историю прогонов, самые новые первыми.
// @Summary      Список прогонов
// @Tags         runs
// @Produce      json
// @Param        page     query     int  false  "Номер страницы (>=1)"    default(1)
// @Param        perPage  query     int  false  "Размер страницы (1-100)" default(20)
// @Success      200      {object}  out.RunListResponse
// @Router       /runs [get]
func (h *Handler) ListRuns(c *fiber.Ctx) error {
	p := GetPagination(c)
	items, total, err := h.services.Pipeline.ListRuns(c.UserContext(), p.Offset(), p.Limit())
	if err != nil {
		return err
	}
	resp := out.RunListResponse{Items: make([]out.RunResponse, 0, len(items)), Total: total, Page: p.Page, PerPage: p.PerPage}
	for i := range items {
		resp.Items = append(resp.Items, out.RunFromDomain(&items[i], nil))
	}
	return c.JSON(resp)
}

// GetRunGraph отдаёт evidence-граф (источник → claim → гипотеза) для гипотез
// прогона — визуализация связей, которую явно требует кейс.
// @Summary      Граф evidence-связей прогона
// @Description  Узлы: entity (оборудование/показатель/реагент/...), claim, hypothesis. Рёбра: subject (entity→claim), affects (claim→entity), evidence (claim→hypothesis).
// @Tags         runs
// @Produce      json
// @Param        runId  path      string  true  "UUID прогона"
// @Success      200    {object}  out.GraphResponse
// @Failure      404    {object}  errs.Error
// @Router       /runs/{runId}/graph [get]
func (h *Handler) GetRunGraph(c *fiber.Ctx) error {
	g, err := h.services.Pipeline.BuildRunGraph(c.UserContext(), c.Params("runId"))
	if err != nil {
		return err
	}
	return c.JSON(out.GraphFromDomain(g))
}

// GetRunReportMarkdown отдаёт человекочитаемый Markdown-отчёт по прогону.
// @Summary      Markdown-отчёт по прогону
// @Tags         runs
// @Produce      text/markdown
// @Param        runId  path  string  true  "UUID прогона"
// @Success      200    {string}  string
// @Failure      404    {object}  errs.Error
// @Router       /runs/{runId}/report.md [get]
func (h *Handler) GetRunReportMarkdown(c *fiber.Ctx) error {
	run, err := h.services.Pipeline.GetRun(c.UserContext(), c.Params("runId"))
	if err != nil {
		return err
	}
	hyps, err := h.services.Pipeline.GetHypotheses(c.UserContext(), c.Params("runId"))
	if err != nil {
		return err
	}
	md := hypothesisfactory.RenderMarkdownReport(run.ProblemSpec, hyps)
	c.Set(fiber.HeaderContentType, "text/markdown; charset=utf-8")
	return c.SendString(md)
}
