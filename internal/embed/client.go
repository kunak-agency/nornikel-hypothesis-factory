// Package embed talks to the pyworker sidecar for Docling ingestion and
// BGE-M3 embeddings/reranking, keeping heavy ML deps out of the Go binary.
package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

func New(baseURL string) *Client {
	// 30 min: Docling parsing large PDFs (layout/OCR models on CPU) is a
	// one-time ingestion cost, not an interactive request — the pipeline's
	// per-query calls (embed/rerank) return in seconds and never approach this.
	return &Client{BaseURL: baseURL, HTTP: &http.Client{Timeout: 30 * time.Minute}}
}

type IngestChunk struct {
	Ordinal     int            `json:"ordinal"`
	Section     string         `json:"section"`
	Content     string         `json:"content"`
	ContentType string         `json:"content_type"`
	Metadata    map[string]any `json:"metadata"`
}

func (c *Client) Ingest(ctx context.Context, filename string, data []byte) ([]IngestChunk, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", filename)
	if err != nil {
		return nil, err
	}
	if _, err := fw.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/ingest", &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pyworker ingest: status %d: %s", resp.StatusCode, string(raw))
	}
	var out struct {
		Chunks []IngestChunk `json:"chunks"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("pyworker ingest: decode: %w", err)
	}
	return out.Chunks, nil
}

func (c *Client) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	payload, _ := json.Marshal(map[string]any{"texts": texts})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/embed", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pyworker embed: status %d: %s", resp.StatusCode, string(raw))
	}
	var out struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("pyworker embed: decode: %w", err)
	}
	return out.Embeddings, nil
}

func (c *Client) EmbedOne(ctx context.Context, text string) ([]float32, error) {
	vecs, err := c.Embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("pyworker embed: empty result")
	}
	return vecs[0], nil
}

func (c *Client) Rerank(ctx context.Context, query string, candidates []string) ([]float64, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	payload, _ := json.Marshal(map[string]any{"query": query, "candidates": candidates})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/rerank", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pyworker rerank: status %d: %s", resp.StatusCode, string(raw))
	}
	var out struct {
		Scores []float64 `json:"scores"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("pyworker rerank: decode: %w", err)
	}
	return out.Scores, nil
}
