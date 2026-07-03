package out

import (
	"time"

	"hypothesis-factory/repositories"

	"github.com/google/uuid"
)

type DocumentResponse struct {
	ID         uuid.UUID `json:"id"`
	Title      string    `json:"title"`
	SourceType string    `json:"sourceType" example:"book"`
	Domain     string    `json:"domain"     example:"flotation"`
	Language   string    `json:"language"   example:"ru"`
	ChunkCount int64     `json:"chunkCount"`
	CreatedAt  time.Time `json:"createdAt"`
}

type DocumentListResponse struct {
	Items []DocumentResponse `json:"items"`
	Total int                `json:"total"`
}

func DocumentFromDomain(d *repositories.DocumentWithChunkCount) DocumentResponse {
	return DocumentResponse{
		ID:         d.ID,
		Title:      d.Title,
		SourceType: d.SourceType,
		Domain:     d.Domain,
		Language:   d.Language,
		ChunkCount: d.ChunkCount,
		CreatedAt:  d.CreatedAt,
	}
}

type IngestResponse struct {
	ChunksIngested int `json:"chunksIngested"`
}
