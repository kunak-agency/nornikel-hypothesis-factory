package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "hypothesis-factory/docs"
	"hypothesis-factory/externalApi"
	"hypothesis-factory/handlers"
	"hypothesis-factory/pkg/logger"
	"hypothesis-factory/repositories"
	"hypothesis-factory/services"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/fiber/v2/middleware/requestid"
)

const bodyLimit = 200 * 1024 * 1024 // ingestion принимает крупные PDF (десятки МБ)

// @title           Фабрика гипотез — Hypothesis Factory API
// @version         1.0
// @description     Evidence-Backed Hypothesis Factory: генерация проверяемых технологических гипотез по обогащению руд на основе базы знаний (RAG).

// @host      localhost:8080
// @BasePath  /v1
func main() {
	repos, err := repositories.InitRepos()
	if err != nil {
		logger.LogCritical("init repos: %v", err)
		os.Exit(1)
	}
	if err := repos.MigrateDB(); err != nil {
		logger.LogCritical("migrate db: %v", err)
		os.Exit(1)
	}

	yandexFolderID := os.Getenv("YANDEX_FOLDER_ID")
	yandexModelURI := os.Getenv("YANDEX_MODEL_URI")
	if yandexModelURI == "" && yandexFolderID != "" {
		yandexModelURI = "gpt://" + yandexFolderID + "/yandexgpt/latest"
	}
	if os.Getenv("YANDEX_API_KEY") == "" || yandexFolderID == "" {
		logger.LogWarning("YANDEX_API_KEY / YANDEX_FOLDER_ID not set — LLM calls will fail until configured")
	}
	llm := externalApi.NewYandexClient(os.Getenv("YANDEX_API_KEY"), yandexFolderID, yandexModelURI)

	workerBaseURL := os.Getenv("WORKER_BASE_URL")
	if workerBaseURL == "" {
		workerBaseURL = "http://localhost:8090"
	}
	pyworker := externalApi.NewPyworkerClient(workerBaseURL)

	svc := services.NewServices(repos, llm, pyworker)
	hm := handlers.NewHandlerManager(svc)

	app := fiber.New(fiber.Config{
		AppName:      "hypothesis-factory",
		ErrorHandler: hm.Errors,
		BodyLimit:    bodyLimit,
		// Прогон пайплайна асинхронный (см. handlers.CreateRun), поэтому
		// долгие ответы тут не нужны — но ingestion больших PDF идёт
		// синхронно внутри одного запроса, отсюда большой ReadTimeout.
		ReadTimeout: 90 * time.Minute,
	})

	// Порядок структурно важен: requestid -> InjectRequestID -> recover -> cors.
	app.Use(requestid.New())
	app.Use(handlers.InjectRequestID())
	app.Use(recover.New(recover.Config{
		EnableStackTrace: os.Getenv("APP_ENV") != "production",
	}))

	origins := os.Getenv("ALLOWED_ORIGINS")
	if origins == "" {
		origins = "*"
	}
	app.Use(cors.New(cors.Config{
		AllowOrigins: origins,
		AllowMethods: "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		AllowHeaders: "Origin,Content-Type,Accept,Authorization",
	}))

	initRoutes(app, hm)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	listenErr := make(chan error, 1)
	go func() {
		listenErr <- app.Listen(":" + port)
	}()

	select {
	case err := <-listenErr:
		if err != nil {
			logger.LogCritical("listen failed: %v", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := app.ShutdownWithContext(shutdownCtx); err != nil && !errors.Is(err, context.Canceled) {
			logger.LogError("fiber shutdown: %v", err)
		}
	}
}
