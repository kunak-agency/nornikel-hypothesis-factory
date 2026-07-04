// Package knowledgebase управляет ingestion документов (Docling-парсинг +
// BGE-M3 эмбеддинги через pyworker) и их жизненным циклом в базе знаний.
package knowledgebase

import (
	"context"
	"fmt"

	"hypothesis-factory/domain"
	"hypothesis-factory/externalApi"
	"hypothesis-factory/pkg/errs"
	"hypothesis-factory/repositories"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"
)

type Service struct {
	repos    *repositories.Repos
	pyworker *externalApi.PyworkerClient
}

func NewService(repos *repositories.Repos, pyworker *externalApi.PyworkerClient) *Service {
	return &Service{repos: repos, pyworker: pyworker}
}

type IngestInput struct {
	Filename   string
	Data       []byte
	Title      string
	SourceType string // book | regulation | scheme | historical_case | report | article
	Domain     string
	Language   string
	Metadata   map[string]any
}

// Ingest парсит документ через Docling, эмбеддит чанки контекстуализированным
// текстом (заголовок документа + секция перед контентом — дешёвый
// research-backed буст качества retrieval) и пишет всё в БД. Возвращает число
// созданных chunks.
func (s *Service) Ingest(ctx context.Context, in IngestInput) (int, error) {
	if in.Domain == "" {
		in.Domain = "flotation"
	}
	if in.Language == "" {
		in.Language = "ru"
	}

	doc := &domain.Document{
		Title:      in.Title,
		SourceType: in.SourceType,
		FilePath:   in.Filename,
		Domain:     in.Domain,
		Language:   in.Language,
		Metadata:   in.Metadata,
	}
	if err := s.repos.Documents.Create(ctx, doc); err != nil {
		return 0, errs.Wrap(err, errs.ErrTypeInternal, "create document")
	}
	// Document переживает только до конца этой функции без коммита чанков:
	// любой ранний return ниже (ingest/embed/insert failure) откатывает его
	// сам (ON DELETE CASCADE подчистит чанки, если insert успел частично
	// пройти) — иначе неудачная попытка (например, транзиентный сбой БД
	// посреди чанков) навсегда оставляет в базе знаний Document без
	// чанков или с частью чанков, которые пользователь не может отличить
	// от полноценного документа при повторной загрузке того же файла.
	rollbackDocument := func() {
		// Best-effort: если сам rollback тоже упадёт, orphan-Document
		// остаётся обнаружимым и вручную чистимым (как делалось в этой
		// сессии не раз) — заводить отдельный errs.ErrType ради этого не
		// стоит, исходная ошибка ingest/embed важнее для вызывающей стороны.
		_, _ = s.repos.Documents.Delete(ctx, doc.ID)
	}

	// sourceType=article — научная статья: GROBID (структура/цитируемость)
	// вместо Docling общего назначения, требует docker-compose.ingestion.yml
	// (grobid-сервис не часть runtime-сборки).
	var chunks []externalApi.IngestChunk
	var err error
	if in.SourceType == "article" {
		chunks, err = s.pyworker.IngestArticle(ctx, in.Filename, in.Data)
	} else {
		chunks, err = s.pyworker.Ingest(ctx, in.Filename, in.Data)
	}
	if err != nil {
		rollbackDocument()
		return 0, errs.Wrap(err, errs.ErrTypeInternal, "ingest failed")
	}
	if len(chunks) == 0 {
		return 0, nil
	}

	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = contextualizeForEmbedding(in.Title, c.Section, c.Content)
	}
	vectors, err := s.pyworker.Embed(ctx, texts)
	if err != nil {
		rollbackDocument()
		return 0, errs.Wrap(err, errs.ErrTypeInternal, "embed chunks failed")
	}

	domainChunks := make([]domain.Chunk, len(chunks))
	for i, c := range chunks {
		domainChunks[i] = domain.Chunk{
			DocumentID:  doc.ID,
			Ordinal:     c.Ordinal,
			Section:     c.Section,
			Content:     c.Content,
			ContentType: c.ContentType,
			Metadata:    c.Metadata,
		}
		if i < len(vectors) {
			v := pgvector.NewVector(vectors[i])
			domainChunks[i].Embedding = &v
		}
	}
	if err := s.repos.Chunks.CreateBatch(ctx, domainChunks); err != nil {
		rollbackDocument()
		return 0, errs.Wrap(err, errs.ErrTypeInternal, fmt.Sprintf("insert %d chunks", len(domainChunks)))
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

func (s *Service) List(ctx context.Context) ([]repositories.DocumentWithChunkCount, error) {
	return s.repos.Documents.List(ctx)
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	rows, err := s.repos.Documents.Delete(ctx, id)
	if err != nil {
		return errs.Wrap(err, errs.ErrTypeInternal, "delete document")
	}
	if rows == 0 {
		return errs.NewNotFoundError("document")
	}
	return nil
}
