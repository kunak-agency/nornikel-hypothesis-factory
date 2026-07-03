// Package ingest orchestrates document ingestion: Docling parsing (via pyworker)
// -> semantic chunking -> BGE-M3 embedding -> persistence.
package ingest

import (
	"context"
	"fmt"

	"hypothesis-factory/internal/embed"
	"hypothesis-factory/internal/models"
	"hypothesis-factory/internal/store"
)

type Service struct {
	embedClient *embed.Client
	store       *store.Store
}

func New(embedClient *embed.Client, s *store.Store) *Service {
	return &Service{embedClient: embedClient, store: s}
}

type IngestRequest struct {
	Filename   string
	Data       []byte
	Title      string
	SourceType string // book | regulation | scheme | historical_case | report
	Domain     string
	Language   string
	Metadata   map[string]any
}

func (svc *Service) IngestDocument(ctx context.Context, req IngestRequest) (int, error) {
	if req.Domain == "" {
		req.Domain = "flotation"
	}
	if req.Language == "" {
		req.Language = "ru"
	}

	docID, err := svc.store.InsertDocument(ctx, models.Document{
		Title:      req.Title,
		SourceType: req.SourceType,
		FilePath:   req.Filename,
		Domain:     req.Domain,
		Language:   req.Language,
		Metadata:   req.Metadata,
	})
	if err != nil {
		return 0, fmt.Errorf("insert document: %w", err)
	}

	chunks, err := svc.embedClient.Ingest(ctx, req.Filename, req.Data)
	if err != nil {
		return 0, fmt.Errorf("docling ingest: %w", err)
	}
	if len(chunks) == 0 {
		return 0, nil
	}

	// Contextual embedding: prepend document title + section heading to the
	// text handed to the embedder (not to the stored/cited Content) so the
	// dense vector encodes which document/section a chunk belongs to. Cheap,
	// research-backed retrieval-quality lever for a one-time ingestion cost.
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = contextualizeForEmbedding(req.Title, c.Section, c.Content)
	}
	vectors, err := svc.embedClient.Embed(ctx, texts)
	if err != nil {
		return 0, fmt.Errorf("embed chunks: %w", err)
	}

	for i, c := range chunks {
		var vec []float32
		if i < len(vectors) {
			vec = vectors[i]
		}
		_, err := svc.store.InsertChunk(ctx, models.Chunk{
			DocumentID:  docID,
			Ordinal:     c.Ordinal,
			Section:     c.Section,
			Content:     c.Content,
			ContentType: c.ContentType,
			Embedding:   vec,
			Metadata:    c.Metadata,
		})
		if err != nil {
			return i, fmt.Errorf("insert chunk %d: %w", i, err)
		}
	}

	return len(chunks), nil
}

func contextualizeForEmbedding(title, section, content string) string {
	if title == "" && section == "" {
		return content
	}
	if section == "" {
		return "Документ: " + title + "\n" + content
	}
	return "Документ: " + title + " | Раздел: " + section + "\n" + content
}
