package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"hypothesis-factory/internal/api"
	"hypothesis-factory/internal/config"
	"hypothesis-factory/internal/db"
	"hypothesis-factory/internal/embed"
	"hypothesis-factory/internal/ingest"
	"hypothesis-factory/internal/llm"
	"hypothesis-factory/internal/pipeline"
	"hypothesis-factory/internal/store"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := config.Load()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	pool, err := db.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("connect db", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if cfg.YandexAPIKey == "" || cfg.YandexFolderID == "" {
		log.Warn("YANDEX_API_KEY / YANDEX_FOLDER_ID not set — LLM calls will fail until configured")
	}
	llmClient := llm.NewYandexClient(cfg.YandexAPIKey, cfg.YandexFolderID, cfg.YandexModelURI)
	embedClient := embed.New(cfg.WorkerBaseURL)

	s := store.New(pool)
	ing := ingest.New(embedClient, s)
	orch := pipeline.NewOrchestrator(llmClient, embedClient, s, "flotation", 15)

	srv := api.NewServer(orch, ing, s, log)

	httpServer := &http.Server{
		Addr:        ":" + cfg.Port,
		Handler:     srv.Routes(),
		ReadTimeout: 5 * time.Minute,
		// 30 min: /documents can proxy a large-PDF Docling parse (one-time
		// ingestion, minutes is fine); /runs pipeline calls finish in well
		// under this even with the multi-judge critic ensemble.
		WriteTimeout: 30 * time.Minute,
	}

	go func() {
		log.Info("listening", "port", cfg.Port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	_ = httpServer.Shutdown(shutdownCtx)
}
