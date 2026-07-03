package main

import (
	"hypothesis-factory/handlers"

	"github.com/gofiber/fiber/v2"
	fiberSwagger "github.com/gofiber/swagger"
)

// initRoutes регистрирует все маршруты. /health и /swagger живут на root,
// доменный API — под /v1/.
func initRoutes(app *fiber.App, hm *handlers.Handler) {
	app.Get("/health", hm.Health)
	app.Get("/swagger/*", fiberSwagger.HandlerDefault)

	v1 := app.Group("/v1")

	documents := v1.Group("/documents")
	documents.Post("", hm.IngestDocument)
	documents.Get("", hm.ListDocuments)
	documents.Delete("/:documentId", hm.DeleteDocument)

	runs := v1.Group("/runs")
	runs.Post("", hm.CreateRun)
	runs.Get("", handlers.Pagination(), hm.ListRuns)
	runs.Get("/:runId", hm.GetRun)
	runs.Get("/:runId/report.md", hm.GetRunReportMarkdown)

	hypotheses := v1.Group("/hypotheses")
	hypotheses.Post("/:hypothesisId/feedback", hm.SubmitFeedback)
}
