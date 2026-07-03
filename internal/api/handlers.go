package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/google/uuid"

	"hypothesis-factory/internal/ingest"
	"hypothesis-factory/internal/models"
	"hypothesis-factory/internal/pipeline"
)

func renderReport(spec models.ProblemSpec, hyps []models.Hypothesis) string {
	return pipeline.RenderMarkdownReport(spec, hyps)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// POST /documents (multipart/form-data: file, title, source_type, domain, language)
// Ingests a knowledge-base document: book, regulation, scheme, or historical
// hypothesis example (Гипотезы *.docx / Хвосты *.xlsx pairs).
func (s *Server) handleIngestDocument(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(200 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "parse multipart form: "+err.Error())
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing file: "+err.Error())
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "read file: "+err.Error())
		return
	}

	req := ingest.IngestRequest{
		Filename:   header.Filename,
		Data:       data,
		Title:      firstNonEmpty(r.FormValue("title"), header.Filename),
		SourceType: firstNonEmpty(r.FormValue("source_type"), "report"),
		Domain:     r.FormValue("domain"),
		Language:   r.FormValue("language"),
	}

	n, err := s.ingest.IngestDocument(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ingest: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"chunks_ingested": n})
}

type createRunRequest struct {
	RawText  string         `json:"raw_text"`
	RawInput map[string]any `json:"raw_input"`
}

// POST /runs {"raw_text": "...", "raw_input": {...}}
// Runs the full pipeline: ProblemSpec -> retrieval -> claims -> hypotheses -> critic.
func (s *Server) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	var req createRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "decode body: "+err.Error())
		return
	}
	if req.RawText == "" {
		writeError(w, http.StatusBadRequest, "raw_text is required")
		return
	}

	result, err := s.orchestrator.Run(r.Context(), req.RawText, req.RawInput)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "pipeline run: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	runID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid run id")
		return
	}
	run, err := s.store.GetRun(r.Context(), runID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	hyps, err := s.store.GetHypothesesByRun(r.Context(), runID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": run, "hypotheses": hyps})
}

func (s *Server) handleGetRunReportMarkdown(w http.ResponseWriter, r *http.Request) {
	runID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid run id")
		return
	}
	run, err := s.store.GetRun(r.Context(), runID)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	hyps, err := s.store.GetHypothesesByRun(r.Context(), runID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	md := renderReport(run.ProblemSpec, hyps)
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(md))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
