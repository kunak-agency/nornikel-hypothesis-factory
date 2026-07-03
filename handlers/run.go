package handlers

import (
	"encoding/json"
	"io"

	"hypothesis-factory/in"
	"hypothesis-factory/out"
	"hypothesis-factory/pkg/errs"
	"hypothesis-factory/services/export"
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

// CreateRunFromExcel запускает пайплайн из профиля хвостов (Хвосты *.xlsx —
// формат кейса). В отличие от CreateRun, loss_hotspots и затронутые металлы
// считаются кодом из файла, а не пересказом LLM: свободный текст (если
// передан) используется только для качественных полей ProblemSpec.
// @Summary      Запуск генерации гипотез из профиля хвостов (Excel)
// @Description  Принимает Хвосты *.xlsx — числа (классы крупности, минеральные формы, Ni/Cu) парсятся детерминированно, без LLM. rawText опционален — дополняет качественные поля (цель, оборудование, ограничения).
// @Tags         runs
// @Accept       multipart/form-data
// @Produce      json
// @Param        file     formData  file    true   "Хвосты *.xlsx"
// @Param        rawText  formData  string  false  "Свободный текст с доп. контекстом (цель, оборудование, ограничения)"
// @Success      202  {object}  out.RunResponse
// @Failure      400  {object}  errs.Error
// @Failure      422  {object}  errs.Error
// @Failure      500  {object}  errs.Error
// @Router       /runs/from-excel [post]
func (h *Handler) CreateRunFromExcel(c *fiber.Ctx) error {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		return errs.NewBadRequestError("missing file: " + err.Error())
	}
	file, err := fileHeader.Open()
	if err != nil {
		return errs.NewBadRequestError("open file: " + err.Error())
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		return errs.NewInternalError("read file: " + err.Error())
	}

	var rawInput map[string]any
	if raw := c.FormValue("rawInput"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &rawInput); err != nil {
			return errs.NewBadRequestError("invalid rawInput json: " + err.Error())
		}
	}

	run, err := h.services.Pipeline.StartRunFromExcel(c.UserContext(), data, c.FormValue("rawText"), rawInput)
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

// GetRunReportPDF отдаёт отчёт по прогону в PDF.
// @Summary      PDF-отчёт по прогону
// @Tags         runs
// @Produce      application/pdf
// @Param        runId  path  string  true  "UUID прогона"
// @Success      200    {file}    file
// @Failure      404    {object}  errs.Error
// @Failure      500    {object}  errs.Error
// @Router       /runs/{runId}/report.pdf [get]
func (h *Handler) GetRunReportPDF(c *fiber.Ctx) error {
	run, err := h.services.Pipeline.GetRun(c.UserContext(), c.Params("runId"))
	if err != nil {
		return err
	}
	hyps, err := h.services.Pipeline.GetHypotheses(c.UserContext(), c.Params("runId"))
	if err != nil {
		return err
	}
	pdfBytes, err := export.ToPDF(run.ProblemSpec, hyps)
	if err != nil {
		return errs.NewInternalError("render pdf: " + err.Error())
	}
	c.Set(fiber.HeaderContentType, "application/pdf")
	c.Set(fiber.HeaderContentDisposition, `attachment; filename="report.pdf"`)
	return c.Send(pdfBytes)
}

// GetRunReportDOCX отдаёт отчёт по прогону в DOCX.
// @Summary      DOCX-отчёт по прогону
// @Tags         runs
// @Produce      application/vnd.openxmlformats-officedocument.wordprocessingml.document
// @Param        runId  path  string  true  "UUID прогона"
// @Success      200    {file}    file
// @Failure      404    {object}  errs.Error
// @Failure      500    {object}  errs.Error
// @Router       /runs/{runId}/report.docx [get]
func (h *Handler) GetRunReportDOCX(c *fiber.Ctx) error {
	run, err := h.services.Pipeline.GetRun(c.UserContext(), c.Params("runId"))
	if err != nil {
		return err
	}
	hyps, err := h.services.Pipeline.GetHypotheses(c.UserContext(), c.Params("runId"))
	if err != nil {
		return err
	}
	docxBytes, err := export.ToDOCX(run.ProblemSpec, hyps)
	if err != nil {
		return errs.NewInternalError("render docx: " + err.Error())
	}
	c.Set(fiber.HeaderContentType, "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	c.Set(fiber.HeaderContentDisposition, `attachment; filename="report.docx"`)
	return c.Send(docxBytes)
}

// GetRunReportCSV отдаёт гипотезы прогона в CSV (одна строка на гипотезу).
// @Summary      CSV-отчёт по прогону
// @Tags         runs
// @Produce      text/csv
// @Param        runId  path  string  true  "UUID прогона"
// @Success      200    {file}    file
// @Failure      404    {object}  errs.Error
// @Failure      500    {object}  errs.Error
// @Router       /runs/{runId}/report.csv [get]
func (h *Handler) GetRunReportCSV(c *fiber.Ctx) error {
	if _, err := h.services.Pipeline.GetRun(c.UserContext(), c.Params("runId")); err != nil {
		return err
	}
	hyps, err := h.services.Pipeline.GetHypotheses(c.UserContext(), c.Params("runId"))
	if err != nil {
		return err
	}
	csvBytes, err := export.ToCSV(hyps)
	if err != nil {
		return errs.NewInternalError("render csv: " + err.Error())
	}
	c.Set(fiber.HeaderContentType, "text/csv; charset=utf-8")
	c.Set(fiber.HeaderContentDisposition, `attachment; filename="report.csv"`)
	return c.Send(csvBytes)
}
