package handlers

import (
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
// @Param        sourceType  formData  string  false  "book | regulation | scheme | historical_case | report | article | patent (article = GROBID + Semantic Scholar; patent — через обычный Docling-путь, structured-парсинг формулы изобретения не делаем — используйте authors/year для инвентора/даты подачи)" default(report)
// @Param        domain      formData  string  false  "Домен знаний"                                          default(flotation)
// @Param        language    formData  string  false  "Язык"                                                  default(ru)
// @Param        authors     formData  string  false  "Авторы (через запятую), если известны"
// @Param        year        formData  string  false  "Год издания/публикации, если известен"
// @Param        edition     formData  string  false  "Издание/версия, если известно"
// @Success      200  {object}  out.IngestResponse
// @Failure      400  {object}  errs.Error
// @Failure      500  {object}  errs.Error
// @Router       /documents [post]
func (h *Handler) IngestDocument(c *fiber.Ctx) error {
	data, filename, err := readUploadedFile(c, "file")
	if err != nil {
		return err
	}

	// authors/year/edition — свободные поля, а не пересказ LLM; сейчас для
	// большинства документов эти данные уже "зашиты" в Title текстом
	// куратором корпуса, но структурные поля дают отчётам возможность
	// цитировать точнее (и это единственный путь дать их для документов, у
	// которых Title — не человекочитаемая библиографическая запись).
	metadata := map[string]any{}
	if authors := c.FormValue("authors"); authors != "" {
		metadata["authors"] = authors
	}
	if year := c.FormValue("year"); year != "" {
		metadata["year"] = year
	}
	if edition := c.FormValue("edition"); edition != "" {
		metadata["edition"] = edition
	}

	n, err := h.services.KnowledgeBase.Ingest(c.UserContext(), knowledgebase.IngestInput{
		Filename:   filename,
		Data:       data,
		Title:      firstNonEmpty(c.FormValue("title"), filename),
		SourceType: firstNonEmpty(c.FormValue("sourceType"), "report"),
		Domain:     c.FormValue("domain"),
		Language:   c.FormValue("language"),
		Metadata:   metadata,
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
