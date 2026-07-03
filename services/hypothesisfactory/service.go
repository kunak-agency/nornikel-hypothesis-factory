package hypothesisfactory

import (
	"context"
	"fmt"
	"time"

	"hypothesis-factory/domain"
	"hypothesis-factory/externalApi"
	"hypothesis-factory/pkg/errs"
	"hypothesis-factory/pkg/logger"
	"hypothesis-factory/repositories"

	"github.com/google/uuid"
)

type Service struct {
	repos        *repositories.Repos
	llm          externalApi.LLMClient
	pyworker     *externalApi.PyworkerClient
	domainFilter string
	topK         int
}

func NewService(repos *repositories.Repos, llm externalApi.LLMClient, pyworker *externalApi.PyworkerClient) *Service {
	return &Service{repos: repos, llm: llm, pyworker: pyworker, domainFilter: "flotation", topK: 15}
}

// StartRun — синхронная часть запроса: парсит ProblemSpec (один быстрый
// LLM-вызов, ~1-2с) и создаёт запись прогона. Остальной (медленный, ~45-90с)
// пайплайн запускается отдельно через RunPipelineAsync — HTTP-хендлер не
// блокируется на весь прогон, а сразу отдаёт run_id для поллинга статуса.
func (s *Service) StartRun(ctx context.Context, rawText string, rawInput map[string]any) (*domain.HypothesisRun, error) {
	if rawText == "" {
		return nil, errs.NewValidationError("raw_text is required")
	}

	spec, err := buildProblemSpec(ctx, s.llm, rawText)
	if err != nil {
		return nil, errs.Wrap(err, errs.ErrTypeInternal, "build problem spec")
	}

	run := &domain.HypothesisRun{
		ProblemSpec: spec,
		RawInput:    rawInput,
		Status:      domain.RunStatusPending,
	}
	if err := s.repos.Runs.Create(ctx, run); err != nil {
		return nil, errs.Wrap(err, errs.ErrTypeInternal, "create run")
	}
	return run, nil
}

// RunPipelineAsync запускает retrieval->claims->hypotheses->critique в фоне
// со своим контекстом (не привязан к контексту HTTP-запроса, который к этому
// моменту уже завершился ответом 202).
func (s *Service) RunPipelineAsync(run *domain.HypothesisRun) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if err := s.runPipeline(ctx, run); err != nil {
			logger.LogErrorCtx(ctx, "pipeline run %s failed: %v", run.ID, err)
			if markErr := s.repos.Runs.MarkFailed(ctx, run.ID, err.Error()); markErr != nil {
				logger.LogErrorCtx(ctx, "mark run %s failed: %v", run.ID, markErr)
			}
		}
	}()
}

func (s *Service) runPipeline(ctx context.Context, run *domain.HypothesisRun) error {
	if err := s.repos.Runs.UpdateStatus(ctx, run.ID, domain.RunStatusRetrieving); err != nil {
		return fmt.Errorf("update status retrieving: %w", err)
	}
	chunks, err := retrieve(ctx, s.repos.Chunks, s.pyworker, run.ProblemSpec, s.domainFilter, s.topK)
	if err != nil {
		return fmt.Errorf("retrieve: %w", err)
	}
	if len(chunks) == 0 {
		return fmt.Errorf("no relevant chunks found in knowledge base for domain %q — ingest documents first", s.domainFilter)
	}

	if err := s.repos.Runs.UpdateStatus(ctx, run.ID, domain.RunStatusExtracting); err != nil {
		return fmt.Errorf("update status extracting: %w", err)
	}
	claims := extractClaims(ctx, s.llm, chunks)
	resolveEntities(ctx, s.repos.Entities, s.pyworker, claims)
	for i := range claims {
		if err := s.repos.Claims.Create(ctx, &claims[i]); err != nil {
			return fmt.Errorf("persist claim: %w", err)
		}
	}

	if err := s.repos.Runs.UpdateStatus(ctx, run.ID, domain.RunStatusGenerating); err != nil {
		return fmt.Errorf("update status generating: %w", err)
	}
	hyps, err := generateHypotheses(ctx, s.llm, run.ProblemSpec, claims)
	if err != nil {
		return fmt.Errorf("generate hypotheses: %w", err)
	}
	for i := range hyps {
		hyps[i].RunID = run.ID
	}

	if err := s.repos.Runs.UpdateStatus(ctx, run.ID, domain.RunStatusCritiquing); err != nil {
		return fmt.Errorf("update status critiquing: %w", err)
	}
	claimByID := make(map[string]domain.Claim, len(claims))
	for _, c := range claims {
		claimByID[c.ID.String()] = c
	}
	hyps = critique(ctx, s.llm, run.ProblemSpec, claimByID, hyps)

	for i := range hyps {
		if err := s.repos.Hypotheses.Create(ctx, &hyps[i]); err != nil {
			return fmt.Errorf("persist hypothesis: %w", err)
		}
	}

	return s.repos.Runs.MarkDone(ctx, run.ID)
}

func (s *Service) GetRun(ctx context.Context, id string) (*domain.HypothesisRun, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errs.NewValidationError("invalid run id")
	}
	run, err := s.repos.Runs.GetByID(ctx, uid)
	if err != nil {
		return nil, errs.Wrap(err, errs.ErrTypeInternal, "get run")
	}
	if run == nil {
		return nil, errs.NewNotFoundError("run")
	}
	return run, nil
}

func (s *Service) GetHypotheses(ctx context.Context, runID string) ([]domain.Hypothesis, error) {
	uid, err := uuid.Parse(runID)
	if err != nil {
		return nil, errs.NewValidationError("invalid run id")
	}
	return s.repos.Hypotheses.GetByRunID(ctx, uid)
}

func (s *Service) ListRuns(ctx context.Context, offset, limit int) ([]domain.HypothesisRun, int64, error) {
	return s.repos.Runs.List(ctx, offset, limit)
}
