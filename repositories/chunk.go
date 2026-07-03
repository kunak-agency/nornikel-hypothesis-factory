package repositories

import (
	"context"

	"hypothesis-factory/domain"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"
	"gorm.io/gorm"
)

type ChunkRepo struct{ db *gorm.DB }

func NewChunkRepo(db *gorm.DB) *ChunkRepo { return &ChunkRepo{db: db} }

func (r *ChunkRepo) Create(ctx context.Context, c *domain.Chunk) error {
	return r.db.WithContext(ctx).Create(c).Error
}

// GetNeighbors возвращает чанки того же документа в окне [ordinal-radius,
// ordinal+radius], упорядоченные по ordinal. Docling эмитит table-чанк и
// поясняющий его текст как соседние ordinal внутри одного документа (даже
// когда HybridChunker.merge_peers не смог их слить из-за разных heading) —
// это даёт claim extraction более широкий ("parent") контекст, чем один
// retrieved ("child") чанк, без отдельного parent-хранилища.
func (r *ChunkRepo) GetNeighbors(ctx context.Context, documentID uuid.UUID, ordinal, radius int) ([]domain.Chunk, error) {
	var chunks []domain.Chunk
	err := r.db.WithContext(ctx).
		Where("document_id = ? AND ordinal BETWEEN ? AND ?", documentID, ordinal-radius, ordinal+radius).
		Order("ordinal ASC").
		Find(&chunks).Error
	return chunks, err
}

// hybridSearchRow — Metadata сознательно не выбирается: downstream (claim
// extraction) читает только ID/Content/Section/DocumentTitle/SourceType/скоринг.
type hybridSearchRow struct {
	ID            string
	DocumentID    string
	Ordinal       int
	Section       string
	Content       string
	ContentType   string
	DocumentTitle string
	SourceType    string
	LexicalScore  float64
	VectorScore   float64
	FusedScore    float64
}

// HybridSearch фьюзит лексический (ts_rank по русской FTS) и dense (cosine
// через pgvector HNSW) поиск взвешенной суммой (0.4 lex + 0.6 dense), затем
// возвращает top-N кандидатов для реранкинга/claim extraction.
func (r *ChunkRepo) HybridSearch(ctx context.Context, queryText string, queryEmbedding []float32, domainFilter string, limit int) ([]domain.RetrievedChunk, error) {
	var rows []hybridSearchRow
	err := r.db.WithContext(ctx).Raw(`
		WITH lexical AS (
			SELECT c.id, ts_rank(c.tsv, plainto_tsquery('russian', ?)) AS lscore
			FROM chunks c
			JOIN documents d ON d.id = c.document_id
			WHERE (? = '' OR d.domain = ?)
			  AND c.tsv @@ plainto_tsquery('russian', ?)
			ORDER BY lscore DESC
			LIMIT 50
		),
		dense AS (
			SELECT c.id, 1 - (c.embedding <=> ?) AS vscore
			FROM chunks c
			JOIN documents d ON d.id = c.document_id
			WHERE (? = '' OR d.domain = ?) AND c.embedding IS NOT NULL
			ORDER BY c.embedding <=> ?
			LIMIT 50
		),
		fused AS (
			SELECT COALESCE(l.id, dd.id) AS id,
			       COALESCE(l.lscore, 0) AS lscore,
			       COALESCE(dd.vscore, 0) AS vscore,
			       COALESCE(l.lscore, 0) * 0.4 + COALESCE(dd.vscore, 0) * 0.6 AS fused
			FROM lexical l
			FULL OUTER JOIN dense dd ON dd.id = l.id
		)
		SELECT c.id, c.document_id, c.ordinal, c.section, c.content, c.content_type,
		       d.title AS document_title, d.source_type,
		       f.lscore AS lexical_score, f.vscore AS vector_score, f.fused AS fused_score
		FROM fused f
		JOIN chunks c ON c.id = f.id
		JOIN documents d ON d.id = c.document_id
		ORDER BY f.fused DESC
		LIMIT ?
	`, queryText, domainFilter, domainFilter, queryText,
		pgvector.NewVector(queryEmbedding), domainFilter, domainFilter, pgvector.NewVector(queryEmbedding),
		limit).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	out := make([]domain.RetrievedChunk, 0, len(rows))
	for _, row := range rows {
		id, err := uuid.Parse(row.ID)
		if err != nil {
			return nil, err
		}
		docID, err := uuid.Parse(row.DocumentID)
		if err != nil {
			return nil, err
		}
		out = append(out, domain.RetrievedChunk{
			Chunk: domain.Chunk{
				ID:          id,
				DocumentID:  docID,
				Ordinal:     row.Ordinal,
				Section:     row.Section,
				Content:     row.Content,
				ContentType: row.ContentType,
			},
			DocumentTitle: row.DocumentTitle,
			SourceType:    row.SourceType,
			LexicalScore:  row.LexicalScore,
			VectorScore:   row.VectorScore,
			FusedScore:    row.FusedScore,
		})
	}
	return out, nil
}
