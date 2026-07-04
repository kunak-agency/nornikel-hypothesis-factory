package handlers

import (
	"context"
	"encoding/json"

	"hypothesis-factory/domain"
	"hypothesis-factory/in"
	"hypothesis-factory/out"
	"hypothesis-factory/pkg/errs"
	"hypothesis-factory/services/export"
	"hypothesis-factory/services/hypothesisfactory"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
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

	opts := hypothesisfactory.StartRunOptions{
		Domain:         body.Domain,
		Language:       body.Language,
		ExcludedTopics: body.ExcludedTopics,
		Plant:          body.Plant,
	}
	if body.RankingWeights != nil {
		opts.RankingWeights = *body.RankingWeights
	}

	run, err := h.services.Pipeline.StartRun(c.UserContext(), body.RawText, body.RawInput, opts)
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
// @Param        file            formData  file    true   "Хвосты *.xlsx"
// @Param        rawText         formData  string  false  "Свободный текст с доп. контекстом (цель, оборудование, ограничения)"
// @Param        domain          formData  string  false  "База знаний какого домена используется"                              default(flotation)
// @Param        language        formData  string  false  "Язык гипотез: ru|en|zh"                                              default(ru)
// @Param        rankingWeights  formData  string  false  "JSON domain.RankingWeights — режим экспертной настройки весов"
// @Param        excludedTopics  formData  string  false  "JSON-массив строк — направления, которые генерация должна обходить"
// @Success      202  {object}  out.RunResponse
// @Failure      400  {object}  errs.Error
// @Failure      422  {object}  errs.Error
// @Failure      500  {object}  errs.Error
// @Router       /runs/from-excel [post]
func (h *Handler) CreateRunFromExcel(c *fiber.Ctx) error {
	data, _, err := readUploadedFile(c, "file")
	if err != nil {
		return err
	}

	var rawInput map[string]any
	if raw := c.FormValue("rawInput"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &rawInput); err != nil {
			return errs.NewBadRequestError("invalid rawInput json: " + err.Error())
		}
	}

	opts := hypothesisfactory.StartRunOptions{
		Domain:   c.FormValue("domain"),
		Language: c.FormValue("language"),
		Plant:    c.FormValue("plant"),
	}
	if raw := c.FormValue("rankingWeights"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &opts.RankingWeights); err != nil {
			return errs.NewBadRequestError("invalid rankingWeights json: " + err.Error())
		}
	}
	if raw := c.FormValue("excludedTopics"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &opts.ExcludedTopics); err != nil {
			return errs.NewBadRequestError("invalid excludedTopics json: " + err.Error())
		}
	}

	run, err := h.services.Pipeline.StartRunFromExcel(c.UserContext(), data, c.FormValue("rawText"), rawInput, opts)
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

// loadRunReportData — общая для всех report.* хендлеров последовательность:
// run → hypotheses → claim sources → evidence sources.
func (h *Handler) loadRunReportData(ctx context.Context, runID string) (*domain.HypothesisRun, []domain.Hypothesis, map[uuid.UUID][]string, error) {
	run, err := h.services.Pipeline.GetRun(ctx, runID)
	if err != nil {
		return nil, nil, nil, err
	}
	hyps, err := h.services.Pipeline.GetHypotheses(ctx, runID)
	if err != nil {
		return nil, nil, nil, err
	}
	claims, err := h.services.Pipeline.GetClaimSources(ctx, hyps)
	if err != nil {
		return nil, nil, nil, err
	}
	return run, hyps, hypothesisfactory.BuildEvidenceSources(hyps, claims), nil
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
	run, hyps, sources, err := h.loadRunReportData(c.UserContext(), c.Params("runId"))
	if err != nil {
		return err
	}
	md := hypothesisfactory.RenderMarkdownReport(run.ProblemSpec, hyps, sources, run.KnowledgeGaps)
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
	run, hyps, sources, err := h.loadRunReportData(c.UserContext(), c.Params("runId"))
	if err != nil {
		return err
	}
	pdfBytes, err := export.ToPDF(run.ProblemSpec, hyps, sources, run.KnowledgeGaps)
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
	run, hyps, sources, err := h.loadRunReportData(c.UserContext(), c.Params("runId"))
	if err != nil {
		return err
	}
	docxBytes, err := export.ToDOCX(run.ProblemSpec, hyps, sources, run.KnowledgeGaps)
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
	_, hyps, sources, err := h.loadRunReportData(c.UserContext(), c.Params("runId"))
	if err != nil {
		return err
	}
	csvBytes, err := export.ToCSV(hyps, sources)
	if err != nil {
		return errs.NewInternalError("render csv: " + err.Error())
	}
	c.Set(fiber.HeaderContentType, "text/csv; charset=utf-8")
	c.Set(fiber.HeaderContentDisposition, `attachment; filename="report.csv"`)
	return c.Send(csvBytes)
}

// GetRunReportJira отдаёт гипотезы прогона как задачи на верификацию в
// формате Jira Cloud bulk-import (issue/bulk payload) — файл для ручного/
// автоматического импорта, готовый уйти тем же телом прямым POST-запросом,
// если появится живой API-ключ инстанса.
// @Summary      Экспорт гипотез как задач в Jira-совместимом JSON
// @Tags         runs
// @Produce      json
// @Param        runId       path   string  true   "UUID прогона"
// @Param        projectKey  query  string  true   "Ключ проекта Jira, напр. HYP"
// @Param        issueType   query  string  false  "Тип задачи"  default(Task)
// @Success      200  {file}    file
// @Failure      400  {object}  errs.Error
// @Failure      404  {object}  errs.Error
// @Failure      500  {object}  errs.Error
// @Router       /runs/{runId}/report.jira.json [get]
func (h *Handler) GetRunReportJira(c *fiber.Ctx) error {
	_, hyps, sources, err := h.loadRunReportData(c.UserContext(), c.Params("runId"))
	if err != nil {
		return err
	}

	projectKey := c.Query("projectKey")
	if projectKey == "" {
		return errs.NewBadRequestError("projectKey query param is required")
	}
	jiraBytes, err := export.ToJiraJSON(hyps, sources, projectKey, c.Query("issueType"))
	if err != nil {
		return errs.NewInternalError("render jira export: " + err.Error())
	}
	c.Set(fiber.HeaderContentType, "application/json")
	c.Set(fiber.HeaderContentDisposition, `attachment; filename="report.jira.json"`)
	return c.Send(jiraBytes)
}
