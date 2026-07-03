package handlers

import (
	"io"

	"hypothesis-factory/out"
	"hypothesis-factory/pkg/errs"
	"hypothesis-factory/services/knowledgebase"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// IngestDocument загружает документ в базу знаний.
// @Summary      Загрузка документа в базу знаний
// @Description  Парсит PDF/DOCX/XLSX/изображение через Docling, эмбеддит чанки BGE-M3, пишет в pgvector.
// @Tags         documents
// @Accept       multipart/form-data
// @Produce      json
// @Param        file        formData  file    true   "Файл документа"
// @Param        title       formData  string  false  "Заголовок (по умолчанию — имя файла)"
// @Param        sourceType  formData  string  false  "book | regulation | scheme | historical_case | report" default(report)
// @Param        domain      formData  string  false  "Домен знаний"                                          default(flotation)
// @Param        language    formData  string  false  "Язык"                                                  default(ru)
// @Success      200  {object}  out.IngestResponse
// @Failure      400  {object}  errs.Error
// @Failure      500  {object}  errs.Error
// @Router       /documents [post]
func (h *Handler) IngestDocument(c *fiber.Ctx) error {
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

	n, err := h.services.KnowledgeBase.Ingest(c.UserContext(), knowledgebase.IngestInput{
		Filename:   fileHeader.Filename,
		Data:       data,
		Title:      firstNonEmpty(c.FormValue("title"), fileHeader.Filename),
		SourceType: firstNonEmpty(c.FormValue("sourceType"), "report"),
		Domain:     c.FormValue("domain"),
		Language:   c.FormValue("language"),
	})
	if err != nil {
		return err
	}
	return c.JSON(out.IngestResponse{ChunksIngested: n})
}

// ListDocuments возвращает базу знаний целиком (документы редки, полноценная
// пагинация тут избыточна — UI показывает список сразу).
// @Summary      Список документов базы знаний
// @Tags         documents
// @Produce      json
// @Success      200  {object}  out.DocumentListResponse
// @Failure      500  {object}  errs.Error
// @Router       /documents [get]
func (h *Handler) ListDocuments(c *fiber.Ctx) error {
	items, err := h.services.KnowledgeBase.List(c.UserContext())
	if err != nil {
		return err
	}
	resp := out.DocumentListResponse{Items: make([]out.DocumentResponse, 0, len(items)), Total: len(items)}
	for i := range items {
		resp.Items = append(resp.Items, out.DocumentFromDomain(&items[i]))
	}
	return c.JSON(resp)
}

// DeleteDocument удаляет документ и каскадно его chunks/claims.
// @Summary      Удаление документа из базы знаний
// @Tags         documents
// @Param        documentId  path  string  true  "UUID документа"
// @Success      204  "Документ удалён"
// @Failure      404  {object}  errs.Error
// @Failure      500  {object}  errs.Error
// @Router       /documents/{documentId} [delete]
func (h *Handler) DeleteDocument(c *fiber.Ctx) error {
	id, err := uuid.Parse(c.Params("documentId"))
	if err != nil {
		return errs.NewValidationError("invalid documentId")
	}
	if err := h.services.KnowledgeBase.Delete(c.UserContext(), id); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
