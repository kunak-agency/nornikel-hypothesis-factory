package config

import (
	"os"
)

type Config struct {
	Port string

	DatabaseURL string

	// Yandex Foundation Models
	YandexAPIKey  string
	YandexFolderID string
	YandexModelURI string // e.g. "gpt://<folder_id>/yandexgpt/latest"

	// Python worker (Docling ingestion + BGE-M3 embeddings)
	WorkerBaseURL string
}

func Load() Config {
	c := Config{
		Port:           getenv("PORT", "8080"),
		DatabaseURL:    getenv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/hypothesis_factory?sslmode=disable"),
		YandexAPIKey:   os.Getenv("YANDEX_API_KEY"),
		YandexFolderID: os.Getenv("YANDEX_FOLDER_ID"),
		WorkerBaseURL:  getenv("WORKER_BASE_URL", "http://localhost:8090"),
	}
	if c.YandexFolderID != "" {
		c.YandexModelURI = getenv("YANDEX_MODEL_URI", "gpt://"+c.YandexFolderID+"/yandexgpt/latest")
	}
	return c
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
