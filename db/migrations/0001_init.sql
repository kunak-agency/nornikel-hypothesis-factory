-- Hypothesis Factory schema
-- Evidence-Backed Hypothesis Factory: documents -> chunks -> claims -> hypotheses

CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- ============ Knowledge base ============

CREATE TABLE documents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title TEXT NOT NULL,
    source_type TEXT NOT NULL,              -- book | regulation | scheme | historical_case | report
    file_path TEXT,
    domain TEXT NOT NULL DEFAULT 'flotation', -- extensible: flotation | metallurgy | polymers ...
    language TEXT NOT NULL DEFAULT 'ru',
    metadata JSONB NOT NULL DEFAULT '{}',    -- authors, date, plant, equipment, etc.
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE chunks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    ordinal INT NOT NULL,                    -- position within document
    section TEXT,                            -- heading / table name / scheme title
    content TEXT NOT NULL,
    content_type TEXT NOT NULL DEFAULT 'text', -- text | table | figure_caption
    embedding vector(1024),                  -- BGE-M3 dense dim
    tsv tsvector GENERATED ALWAYS AS (to_tsvector('russian', content)) STORED,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX chunks_embedding_hnsw_idx ON chunks USING hnsw (embedding vector_cosine_ops);
CREATE INDEX chunks_tsv_idx ON chunks USING gin (tsv);
CREATE INDEX chunks_document_id_idx ON chunks (document_id);

-- ============ Claim-level evidence ============

CREATE TABLE claims (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    chunk_id UUID NOT NULL REFERENCES chunks(id) ON DELETE CASCADE,
    subject TEXT NOT NULL,                   -- e.g. "гидроциклон, диаметр насадки"
    action TEXT NOT NULL,                    -- e.g. "уменьшение диаметра с 12 до 8 мм"
    condition TEXT,                          -- context / prerequisites
    metric TEXT,                             -- KPI affected, e.g. "потери Ni в хвостах"
    effect_direction TEXT,                   -- increase | decrease | neutral | mixed
    effect_magnitude TEXT,                   -- qualitative or quantitative
    source_confidence TEXT NOT NULL DEFAULT 'medium', -- low | medium | high
    conflict_flag BOOLEAN NOT NULL DEFAULT false,
    quote TEXT NOT NULL,                     -- exact citation basis
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX claims_chunk_id_idx ON claims (chunk_id);

-- ============ Runs / requests ============

CREATE TABLE hypothesis_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    problem_spec JSONB NOT NULL,             -- parsed ProblemSpec (target KPI, constraints, plant, ...)
    raw_input JSONB NOT NULL DEFAULT '{}',   -- original tailings profile / free-text request
    status TEXT NOT NULL DEFAULT 'pending',  -- pending | retrieving | extracting | generating | critiquing | done | failed
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);

CREATE TABLE hypotheses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id UUID NOT NULL REFERENCES hypothesis_runs(id) ON DELETE CASCADE,
    statement TEXT NOT NULL,
    mechanism TEXT NOT NULL,
    evidence_refs UUID[] NOT NULL DEFAULT '{}', -- claims.id[]
    expected_kpi_effect JSONB NOT NULL DEFAULT '{}', -- {metric, direction, magnitude}
    risks JSONB NOT NULL DEFAULT '[]',
    novelty_reason TEXT,
    verification_plan JSONB NOT NULL DEFAULT '[]',  -- ordered list of experiments
    scores JSONB NOT NULL DEFAULT '{}',       -- {evidence_strength, feasibility, impact, novelty, risk_penalty, confidence, total}
    critic_notes TEXT,
    rank INT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX hypotheses_run_id_idx ON hypotheses (run_id);

-- ============ Expert feedback loop ============

CREATE TABLE feedback (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    hypothesis_id UUID NOT NULL REFERENCES hypotheses(id) ON DELETE CASCADE,
    verdict TEXT NOT NULL,                   -- confirmed | rejected | needs_revision
    comment TEXT,
    reviewer TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
