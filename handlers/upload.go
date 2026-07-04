package handlers

import (
	"io"

	"hypothesis-factory/pkg/errs"

	"github.com/gofiber/fiber/v2"
)

// readUploadedFile читает поле multipart-формы в память — общее тело
// "FormFile → Open → ReadAll" для IngestDocument и CreateRunFromExcel.
// Возвращает исходное имя файла вместе с байтами (нужно вызывающей стороне
// для Document.FilePath/дефолтного заголовка).
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
