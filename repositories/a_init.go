package repositories

import (
	"errors"
	"fmt"
	"os"
	"time"

	"hypothesis-factory/domain"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Repos struct {
	db             *gorm.DB
	Documents      *DocumentRepo
	Chunks         *ChunkRepo
	Claims         *ClaimRepo
	Entities       *EntityRepo
	Runs           *HypothesisRunRepo
	Hypotheses     *HypothesisRepo
	Feedback       *FeedbackRepo
	PlantEquipment *PlantEquipmentRepo
}

func InitRepos() (*Repos, error) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return nil, errors.New("DATABASE_URL not set")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql.DB: %w", err)
	}
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(10 * time.Minute)

	return &Repos{
		db:             db,
		Documents:      NewDocumentRepo(db),
		Chunks:         NewChunkRepo(db),
		Claims:         NewClaimRepo(db),
		Entities:       NewEntityRepo(db),
		Runs:           NewHypothesisRunRepo(db),
		Hypotheses:     NewHypothesisRepo(db),
		Feedback:       NewFeedbackRepo(db),
		PlantEquipment: NewPlantEquipmentRepo(db),
	}, nil
}

// MigrateDB — гибрид AutoMigrate (базовые колонки/индексы по доменным моделям)
// и raw SQL для того, что GORM-теги не умеют выразить: расширение pgvector,
// генерируемая tsvector-колонка для лексического поиска, HNSW-индекс по
// вектору и GIN-индекс по tsvector.
func (r *Repos) MigrateDB() error {
	if err := r.db.Exec(`CREATE EXTENSION IF NOT EXISTS vector`).Error; err != nil {
		return fmt.Errorf("create extension vector: %w", err)
	}

	if err := r.db.AutoMigrate(
		&domain.Document{},
		&domain.Chunk{},
		&domain.Entity{},
		&domain.Claim{},
		&domain.HypothesisRun{},
		&domain.Hypothesis{},
		&domain.Feedback{},
		&domain.PlantEquipment{},
	); err != nil {
		return fmt.Errorf("automigrate: %w", err)
	}

	// tsvector для гибридного (лексика+вектор) поиска — GORM-теги не умеют
	// GENERATED ALWAYS AS, поэтому колонка и её GIN-индекс добавляются raw SQL.
	if err := r.db.Exec(`
		ALTER TABLE chunks ADD COLUMN IF NOT EXISTS tsv tsvector
		GENERATED ALWAYS AS (to_tsvector('russian', content)) STORED
	`).Error; err != nil {
		return fmt.Errorf("add tsv column: %w", err)
	}
	if err := r.db.Exec(`CREATE INDEX IF NOT EXISTS chunks_tsv_idx ON chunks USING gin (tsv)`).Error; err != nil {
		return fmt.Errorf("create tsv index: %w", err)
	}

	// HNSW по dense-эмбеддингу (BGE-M3, cosine) — GORM-теги векторные индексы
	// pgvector не выражают.
	if err := r.db.Exec(`
		CREATE INDEX IF NOT EXISTS chunks_embedding_hnsw_idx ON chunks
		USING hnsw (embedding vector_cosine_ops)
	`).Error; err != nil {
		return fmt.Errorf("create hnsw index: %w", err)
	}

	// HNSW по эмбеддингу сущностей — используется entity resolution (find
	// nearest existing entity перед созданием новой) при claim extraction.
	if err := r.db.Exec(`
		CREATE INDEX IF NOT EXISTS entities_embedding_hnsw_idx ON entities
		USING hnsw (embedding vector_cosine_ops)
	`).Error; err != nil {
		return fmt.Errorf("create entities hnsw index: %w", err)
	}

	// ON DELETE CASCADE между сущностями — AutoMigrate без явной GORM-связи
	// (belongs-to/has-many поля) FK вообще не создаёт, а хотим именно
	// каскадное удаление (удалили документ — ушли его chunks/claims).
	fks := []struct{ name, table, sql string }{
		{"fk_chunks_document", "chunks", `ALTER TABLE chunks ADD CONSTRAINT fk_chunks_document
			FOREIGN KEY (document_id) REFERENCES documents(id) ON DELETE CASCADE`},
		{"fk_claims_chunk", "claims", `ALTER TABLE claims ADD CONSTRAINT fk_claims_chunk
			FOREIGN KEY (chunk_id) REFERENCES chunks(id) ON DELETE CASCADE`},
		{"fk_claims_subject_entity", "claims", `ALTER TABLE claims ADD CONSTRAINT fk_claims_subject_entity
			FOREIGN KEY (subject_entity_id) REFERENCES entities(id) ON DELETE SET NULL`},
		{"fk_claims_metric_entity", "claims", `ALTER TABLE claims ADD CONSTRAINT fk_claims_metric_entity
			FOREIGN KEY (metric_entity_id) REFERENCES entities(id) ON DELETE SET NULL`},
		{"fk_hypotheses_run", "hypotheses", `ALTER TABLE hypotheses ADD CONSTRAINT fk_hypotheses_run
			FOREIGN KEY (run_id) REFERENCES hypothesis_runs(id) ON DELETE CASCADE`},
		{"fk_feedback_hypothesis", "feedbacks", `ALTER TABLE feedbacks ADD CONSTRAINT fk_feedback_hypothesis
			FOREIGN KEY (hypothesis_id) REFERENCES hypotheses(id) ON DELETE CASCADE`},
	}
	for _, fk := range fks {
		if err := r.addConstraintIfMissing(fk.table, fk.name, fk.sql); err != nil {
			return fmt.Errorf("add constraint %s: %w", fk.name, err)
		}
	}

	return nil
}

// addConstraintIfMissing выполняет ALTER TABLE ADD CONSTRAINT идемпотентно —
// Postgres не поддерживает "ADD CONSTRAINT IF NOT EXISTS", поэтому проверяем
// pg_constraint вручную перед выполнением.
func (r *Repos) addConstraintIfMissing(table, constraintName, sql string) error {
	var exists bool
	if err := r.db.Raw(`SELECT EXISTS (
		SELECT 1 FROM pg_constraint c
		JOIN pg_class t ON t.oid = c.conrelid
		WHERE t.relname = ? AND c.conname = ?
	)`, table, constraintName).Scan(&exists).Error; err != nil {
		return err
	}
	if exists {
		return nil
	}
	return r.db.Exec(sql).Error
}
