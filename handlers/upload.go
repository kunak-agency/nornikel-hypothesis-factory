package handlers

import (
	"io"

	"hypothesis-factory/pkg/errs"

	"github.com/gofiber/fiber/v2"
)

// readUploadedFile reads a multipart form file field into memory — the
// shared "FormFile → Open → ReadAll" body duplicated across IngestDocument
// and CreateRunFromExcel. Returns the original filename alongside the bytes
// since callers need it for Document.FilePath/default title.
func readUploadedFile(c *fiber.Ctx, field string) (data []byte, filename string, err error) {
	fileHeader, err := c.FormFile(field)
	if err != nil {
		return nil, "", errs.NewBadRequestError("missing file: " + err.Error())
	}
	file, err := fileHeader.Open()
	if err != nil {
		return nil, "", errs.NewBadRequestError("open file: " + err.Error())
	}
	defer file.Close()
	data, err = io.ReadAll(file)
	if err != nil {
		return nil, "", errs.NewInternalError("read file: " + err.Error())
	}
	return data, fileHeader.Filename, nil
}
