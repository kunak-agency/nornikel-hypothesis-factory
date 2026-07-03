package api

import (
	"log/slog"
	"net/http"

	"hypothesis-factory/internal/ingest"
	"hypothesis-factory/internal/pipeline"
	"hypothesis-factory/internal/store"
)

type Server struct {
	orchestrator *pipeline.Orchestrator
	ingest       *ingest.Service
	store        *store.Store
	log          *slog.Logger
}

func NewServer(orch *pipeline.Orchestrator, ing *ingest.Service, st *store.Store, log *slog.Logger) *Server {
	return &Server{orchestrator: orch, ingest: ing, store: st, log: log}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("POST /documents", s.handleIngestDocument)
	mux.HandleFunc("POST /runs", s.handleCreateRun)
	mux.HandleFunc("GET /runs/{id}", s.handleGetRun)
	mux.HandleFunc("GET /runs/{id}/report.md", s.handleGetRunReportMarkdown)
	return withLogging(s.log, mux)
}

func withLogging(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Info("request", "method", r.Method, "path", r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
