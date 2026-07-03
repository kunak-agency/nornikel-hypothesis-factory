package repositories

import (
	"context"
	"encoding/json"
	"strings"
	"unicode"

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

// hybridSearchRow — c.metadata/d.metadata выбираются как raw JSON ([]byte),
// не через GORM-сериализатор (это raw SQL, не gorm.Find) — распаковываются
// вручную в HybridSearch. Нужны для authors/year/edition (document.metadata)
// и article_authors/article_year (chunk.metadata из GROBID-пути) — без этого
// claim extraction не может процитировать источник детальнее заголовка.
type hybridSearchRow struct {
	ID               string
	DocumentID       string
	Ordinal          int
	Section          string
	Content          string
	ContentType      string
	DocumentTitle    string
	SourceType       string
	ChunkMetadata    []byte
	DocumentMetadata []byte
	LexicalScore     float64
	VectorScore      float64
	FusedScore       float64
}

// buildOrTsQuery превращает свободный текст в OR-запрос для to_tsquery
// ("слово1 | слово2 | ..."). plainto_tsquery — который мы использовали
// раньше — AND'ит ВСЕ слова запроса: для короткой (2-4 слова) фразы это
// нормально, но facet-декомпозированные запросы (base + "оборудование и
// схема классификации: гидроциклоны, диаметр насадок, ...") — это 15-20
// слов, и требовать буквального совпадения ВСЕХ них в одном чанке означает
// НОЛЬ совпадений почти всегда (подтверждено: реальный чанк с "ГЦ-660",
// который лексически идеально релевантен, не проходил фильтр вообще — вся
// лексическая часть гибридного поиска молча не работала для таких запросов,
// retrieval держался только на векторной части). to_tsquery применяет ту же
// стемминг-нормализацию к каждому слову, что и plainto_tsquery — просто с
// OR вместо AND между ними, так что один совпавший термин уже даёт чанку
// шанс попасть в кандидаты, а ts_rank сам взвесит по количеству/весу
// совпадений.
func buildOrTsQuery(text string) string {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	seen := make(map[string]bool, len(fields))
	terms := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.ToLower(f)
		if f == "" || seen[f] {
			continue
		}
		seen[f] = true
		terms = append(terms, f)
	}
	return strings.Join(terms, " | ")
}

// HybridSearch фьюзит лексический (ts_rank по русской FTS) и dense (cosine
// через pgvector HNSW) поиск взвешенной суммой (0.4 lex + 0.6 dense), затем
// возвращает top-N кандидатов для реранкинга/claim extraction.
func (r *ChunkRepo) HybridSearch(ctx context.Context, queryText string, queryEmbedding []float32, domainFilter string, limit int) ([]domain.RetrievedChunk, error) {
	orQuery := buildOrTsQuery(queryText)
	var rows []hybridSearchRow
	err := r.db.WithContext(ctx).Raw(`
		WITH lexical AS (
			SELECT c.id, ts_rank(c.tsv, to_tsquery('russian', ?)) AS lscore
			FROM chunks c
			JOIN documents d ON d.id = c.document_id
			WHERE (? = '' OR d.domain = ?)
			  AND ? <> ''
			  AND c.tsv @@ to_tsquery('russian', ?)
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
		       c.metadata AS chunk_metadata, d.metadata AS document_metadata,
		       f.lscore AS lexical_score, f.vscore AS vector_score, f.fused AS fused_score
		FROM fused f
		JOIN chunks c ON c.id = f.id
		JOIN documents d ON d.id = c.document_id
		ORDER BY f.fused DESC
		LIMIT ?
	`, orQuery, domainFilter, domainFilter, orQuery, orQuery,
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
		var chunkMeta, docMeta map[string]any
		if len(row.ChunkMetadata) > 0 {
			_ = json.Unmarshal(row.ChunkMetadata, &chunkMeta)
		}
		if len(row.DocumentMetadata) > 0 {
			_ = json.Unmarshal(row.DocumentMetadata, &docMeta)
		}
		out = append(out, domain.RetrievedChunk{
			Chunk: domain.Chunk{
				ID:          id,
				DocumentID:  docID,
				Ordinal:     row.Ordinal,
				Section:     row.Section,
				Content:     row.Content,
				ContentType: row.ContentType,
				Metadata:    chunkMeta,
			},
			DocumentTitle:    row.DocumentTitle,
			SourceType:       row.SourceType,
			DocumentMetadata: docMeta,
			LexicalScore:     row.LexicalScore,
			VectorScore:      row.VectorScore,
			FusedScore:    row.FusedScore,
		})
	}
	return out, nil
}
