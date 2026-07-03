// Package store implements persistence and hybrid (lexical + dense) retrieval
// over the knowledge base using plain PostgreSQL: pg_trgm/tsvector for lexical
// search and pgvector/HNSW for dense search. No separate vector DB needed.
package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"

	"hypothesis-factory/internal/models"
)

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func marshalJSON(v any) []byte {
	if v == nil {
		return []byte("{}")
	}
	b, _ := json.Marshal(v)
	return b
}

// ---------- Documents ----------

func (s *Store) InsertDocument(ctx context.Context, d models.Document) (uuid.UUID, error) {
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO documents (id, title, source_type, file_path, domain, language, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, d.ID, d.Title, d.SourceType, d.FilePath, d.Domain, d.Language, marshalJSON(d.Metadata))
	return d.ID, err
}

// ---------- Chunks ----------

func (s *Store) InsertChunk(ctx context.Context, c models.Chunk) (uuid.UUID, error) {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	var emb any
	if len(c.Embedding) > 0 {
		emb = pgvector.NewVector(c.Embedding)
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO chunks (id, document_id, ordinal, section, content, content_type, embedding, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, c.ID, c.DocumentID, c.Ordinal, c.Section, c.Content, c.ContentType, emb, marshalJSON(c.Metadata))
	return c.ID, err
}

// HybridSearch fuses lexical (ts_rank over Russian FTS) and dense (cosine via
// pgvector HNSW) retrieval with simple weighted-sum fusion, then returns the
// top N fused candidates for downstream reranking / claim extraction.
func (s *Store) HybridSearch(ctx context.Context, queryText string, queryEmbedding []float32, domain string, limit int) ([]models.RetrievedChunk, error) {
	rows, err := s.pool.Query(ctx, `
		WITH lexical AS (
			SELECT c.id, ts_rank(c.tsv, plainto_tsquery('russian', $1)) AS lscore
			FROM chunks c
			JOIN documents d ON d.id = c.document_id
			WHERE ($3 = '' OR d.domain = $3)
			  AND c.tsv @@ plainto_tsquery('russian', $1)
			ORDER BY lscore DESC
			LIMIT 50
		),
		dense AS (
			SELECT c.id, 1 - (c.embedding <=> $2) AS vscore
			FROM chunks c
			JOIN documents d ON d.id = c.document_id
			WHERE ($3 = '' OR d.domain = $3) AND c.embedding IS NOT NULL
			ORDER BY c.embedding <=> $2
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
		SELECT c.id, c.document_id, c.ordinal, c.section, c.content, c.content_type, c.metadata,
		       d.title, d.source_type, f.lscore, f.vscore, f.fused
		FROM fused f
		JOIN chunks c ON c.id = f.id
		JOIN documents d ON d.id = c.document_id
		ORDER BY f.fused DESC
		LIMIT $4
	`, queryText, pgvector.NewVector(queryEmbedding), domain, limit)
	if err != nil {
		return nil, fmt.Errorf("hybrid search: %w", err)
	}
	defer rows.Close()

	var out []models.RetrievedChunk
	for rows.Next() {
		var rc models.RetrievedChunk
		var metaRaw []byte
		if err := rows.Scan(&rc.ID, &rc.DocumentID, &rc.Ordinal, &rc.Section, &rc.Content, &rc.ContentType,
			&metaRaw, &rc.DocumentTitle, &rc.SourceType, &rc.LexicalScore, &rc.VectorScore, &rc.FusedScore); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(metaRaw, &rc.Metadata)
		out = append(out, rc)
	}
	return out, rows.Err()
}

// ---------- Claims ----------

func (s *Store) InsertClaim(ctx context.Context, c models.Claim) (uuid.UUID, error) {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO claims (id, chunk_id, subject, action, condition, metric, effect_direction,
		                     effect_magnitude, source_confidence, conflict_flag, quote, metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`, c.ID, c.ChunkID, c.Subject, c.Action, c.Condition, c.Metric, c.EffectDirection,
		c.EffectMagnitude, c.SourceConfidence, c.ConflictFlag, c.Quote, marshalJSON(c.Metadata))
	return c.ID, err
}

func (s *Store) GetClaimsByChunkIDs(ctx context.Context, chunkIDs []uuid.UUID) ([]models.Claim, error) {
	if len(chunkIDs) == 0 {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, chunk_id, subject, action, condition, metric, effect_direction,
		       effect_magnitude, source_confidence, conflict_flag, quote, metadata
		FROM claims WHERE chunk_id = ANY($1)
	`, chunkIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Claim
	for rows.Next() {
		var c models.Claim
		var metaRaw []byte
		if err := rows.Scan(&c.ID, &c.ChunkID, &c.Subject, &c.Action, &c.Condition, &c.Metric,
			&c.EffectDirection, &c.EffectMagnitude, &c.SourceConfidence, &c.ConflictFlag, &c.Quote, &metaRaw); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(metaRaw, &c.Metadata)
		out = append(out, c)
	}
	return out, rows.Err()
}

// ---------- Runs & Hypotheses ----------

func (s *Store) CreateRun(ctx context.Context, spec models.ProblemSpec, rawInput map[string]any) (uuid.UUID, error) {
	id := uuid.New()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO hypothesis_runs (id, problem_spec, raw_input, status)
		VALUES ($1, $2, $3, 'pending')
	`, id, marshalJSON(spec), marshalJSON(rawInput))
	return id, err
}

func (s *Store) UpdateRunStatus(ctx context.Context, runID uuid.UUID, status string) error {
	_, err := s.pool.Exec(ctx, `UPDATE hypothesis_runs SET status = $2 WHERE id = $1`, runID, status)
	return err
}

func (s *Store) InsertHypothesis(ctx context.Context, h models.Hypothesis) (uuid.UUID, error) {
	if h.ID == uuid.Nil {
		h.ID = uuid.New()
	}
	refs := make([]string, 0, len(h.EvidenceRefs))
	for _, r := range h.EvidenceRefs {
		refs = append(refs, r.String())
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO hypotheses (id, run_id, statement, mechanism, evidence_refs, expected_kpi_effect,
		                         risks, novelty_reason, verification_plan, scores, critic_notes, rank)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`, h.ID, h.RunID, h.Statement, h.Mechanism, refs, marshalJSON(h.ExpectedKPIEffect),
		marshalJSON(h.Risks), h.NoveltyReason, marshalJSON(h.VerificationPlan), marshalJSON(h.Scores), h.CriticNotes, h.Rank)
	return h.ID, err
}

func (s *Store) GetHypothesesByRun(ctx context.Context, runID uuid.UUID) ([]models.Hypothesis, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, run_id, statement, mechanism, evidence_refs, expected_kpi_effect, risks,
		       novelty_reason, verification_plan, scores, critic_notes, rank
		FROM hypotheses WHERE run_id = $1 ORDER BY rank ASC NULLS LAST
	`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Hypothesis
	for rows.Next() {
		var h models.Hypothesis
		var refs []uuid.UUID
		var kpiRaw, risksRaw, planRaw, scoresRaw []byte
		if err := rows.Scan(&h.ID, &h.RunID, &h.Statement, &h.Mechanism, &refs, &kpiRaw, &risksRaw,
			&h.NoveltyReason, &planRaw, &scoresRaw, &h.CriticNotes, &h.Rank); err != nil {
			return nil, err
		}
		h.EvidenceRefs = refs
		_ = json.Unmarshal(kpiRaw, &h.ExpectedKPIEffect)
		_ = json.Unmarshal(risksRaw, &h.Risks)
		_ = json.Unmarshal(planRaw, &h.VerificationPlan)
		_ = json.Unmarshal(scoresRaw, &h.Scores)
		out = append(out, h)
	}
	return out, rows.Err()
}

func (s *Store) GetRun(ctx context.Context, runID uuid.UUID) (models.HypothesisRun, error) {
	var r models.HypothesisRun
	var specRaw, rawRaw []byte
	err := s.pool.QueryRow(ctx, `
		SELECT id, problem_spec, raw_input, status, created_at, completed_at
		FROM hypothesis_runs WHERE id = $1
	`, runID).Scan(&r.ID, &specRaw, &rawRaw, &r.Status, &r.CreatedAt, &r.CompletedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return r, fmt.Errorf("run not found: %s", runID)
		}
		return r, err
	}
	_ = json.Unmarshal(specRaw, &r.ProblemSpec)
	_ = json.Unmarshal(rawRaw, &r.RawInput)
	return r, nil
}
