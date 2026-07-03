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
	"hypothesis-factory/services/casefacts"

	"github.com/google/uuid"
)

type Service struct {
	repos    *repositories.Repos
	llm      externalApi.LLMClient
	pyworker *externalApi.PyworkerClient
	topK     int
}

func NewService(repos *repositories.Repos, llm externalApi.LLMClient, pyworker *externalApi.PyworkerClient) *Service {
	return &Service{repos: repos, llm: llm, pyworker: pyworker, topK: 15}
}

// StartRunOptions — то, что раньше было захардкожено на уровне Service
// (единственный домен "flotation") или вообще не настраивалось (язык
// вывода, веса ранжирования) — теперь параметры конкретного прогона, не
// сервиса: разные вызовы могут работать с разными предметными областями и
// разными приоритетами ранжирования без передеплоя.
type StartRunOptions struct {
	Domain         string
	Language       string
	RankingWeights domain.RankingWeights
	ExcludedTopics []string
}

func (o StartRunOptions) normalized() StartRunOptions {
	if o.Domain == "" {
		o.Domain = "flotation"
	}
	if o.Language == "" {
		o.Language = "ru"
	}
	return o
}

// StartRun — синхронная часть запроса: парсит ProblemSpec (один быстрый
// LLM-вызов, ~1-2с) и создаёт запись прогона. Остальной (медленный, ~45-90с)
// пайплайн запускается отдельно через RunPipelineAsync — HTTP-хендлер не
// блокируется на весь прогон, а сразу отдаёт run_id для поллинга статуса.
func (s *Service) StartRun(ctx context.Context, rawText string, rawInput map[string]any, opts StartRunOptions) (*domain.HypothesisRun, error) {
	if rawText == "" {
		return nil, errs.NewValidationError("raw_text is required")
	}
	opts = opts.normalized()

	spec, err := buildProblemSpec(ctx, s.llm, rawText)
	if err != nil {
		return nil, errs.Wrap(err, errs.ErrTypeInternal, "build problem spec")
	}

	run := &domain.HypothesisRun{
		ProblemSpec:    spec,
		RawInput:       rawInput,
		Domain:         opts.Domain,
		Language:       opts.Language,
		RankingWeights: opts.RankingWeights,
		ExcludedTopics: opts.ExcludedTopics,
		Status:         domain.RunStatusPending,
	}
	if err := s.repos.Runs.Create(ctx, run); err != nil {
		return nil, errs.Wrap(err, errs.ErrTypeInternal, "create run")
	}
	return run, nil
}

// StartRunFromExcel — то же самое, что StartRun, но loss_hotspots и
// затронутые металлы берутся детерминированным парсингом профиля хвостов
// (Хвосты *.xlsx), а не пересказом LLM: сама LLM используется только для
// качественных полей (target_kpi/оборудование/ограничения) из свободного
// текста, если он передан. Числа из файла — источник истины, LLM их не
// видит и не может исказить.
func (s *Service) StartRunFromExcel(ctx context.Context, excelData []byte, rawText string, rawInput map[string]any, opts StartRunOptions) (*domain.HypothesisRun, error) {
	opts = opts.normalized()
	facts, err := casefacts.ParseTailingsExcel(excelData)
	if err != nil {
		return nil, errs.NewValidationError("parse tailings excel: " + err.Error())
	}

	var spec domain.ProblemSpec
	if rawText != "" {
		spec, err = buildProblemSpec(ctx, s.llm, rawText)
		if err != nil {
			return nil, errs.Wrap(err, errs.ErrTypeInternal, "build problem spec")
		}
	}

	hotspots := casefacts.BuildLossHotspots(facts, 3)
	if len(hotspots) > 0 {
		spec.LossHotspots = hotspots
	}
	metalSet := map[string]bool{}
	for _, m := range spec.TargetMetals {
		metalSet[m] = true
	}
	for _, m := range facts.Metals {
		if !metalSet[m.Symbol] {
			spec.TargetMetals = append(spec.TargetMetals, m.Symbol)
			metalSet[m.Symbol] = true
		}
	}

	if rawInput == nil {
		rawInput = map[string]any{}
	}
	rawInput["case_facts"] = facts
	if len(facts.Warnings) > 0 {
		logger.LogWarningCtx(ctx, "case facts parse warnings: %v", facts.Warnings)
	}

	run := &domain.HypothesisRun{
		ProblemSpec:    spec,
		RawInput:       rawInput,
		Domain:         opts.Domain,
		Language:       opts.Language,
		RankingWeights: opts.RankingWeights,
		ExcludedTopics: opts.ExcludedTopics,
		Status:         domain.RunStatusPending,
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
	chunks, err := retrieve(ctx, s.repos.Chunks, s.pyworker, run.ProblemSpec, run.Domain, s.topK)
	if err != nil {
		return fmt.Errorf("retrieve: %w", err)
	}
	if len(chunks) == 0 {
		return fmt.Errorf("no relevant chunks found in knowledge base for domain %q — ingest documents first", run.Domain)
	}

	if err := s.repos.Runs.UpdateStatus(ctx, run.ID, domain.RunStatusExtracting); err != nil {
		return fmt.Errorf("update status extracting: %w", err)
	}
	claims := extractClaims(ctx, s.llm, s.repos.Chunks, chunks)
	resolveEntities(ctx, s.repos.Entities, s.pyworker, claims)
	for i := range claims {
		if err := s.repos.Claims.Create(ctx, &claims[i]); err != nil {
			return fmt.Errorf("persist claim: %w", err)
		}
	}

	// "Выявление пробелов в знаниях" из функциональных требований —
	// детерминированная проверка покрытия claims по каждому металлу/точке
	// потерь из ProblemSpec, не LLM-догадка. Ошибку не возвращает — это
	// диагностика, а не блокер пайплайна.
	run.KnowledgeGaps = detectKnowledgeGaps(run.ProblemSpec, claims)

	// "Обучение на фидбэке" через граф памяти: сущности из текущих claims
	// резолвятся (resolveEntities выше) в те же Entity, что и в прошлых
	// прогонах (embedding similarity dedup) — так что история подтверждений/
	// отклонений по каждой сущности уже накоплена в графе и её можно поднять
	// по entity_id, не храня отдельный лог "похожих гипотез".
	entityReputations := s.loadEntityReputations(ctx, claims)

	if err := s.repos.Runs.UpdateStatus(ctx, run.ID, domain.RunStatusGenerating); err != nil {
		return fmt.Errorf("update status generating: %w", err)
	}
	hyps, err := generateHypotheses(ctx, s.llm, run.ProblemSpec, claims, run.Language, run.ExcludedTopics, entityReputations)
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
	hyps = critique(ctx, s.llm, run.ProblemSpec, claimByID, hyps, run.Language, run.RankingWeights)

	for i := range hyps {
		if err := s.repos.Hypotheses.Create(ctx, &hyps[i]); err != nil {
			return fmt.Errorf("persist hypothesis: %w", err)
		}
	}

	if err := s.repos.Runs.UpdateKnowledgeGaps(ctx, run.ID, run.KnowledgeGaps); err != nil {
		return fmt.Errorf("persist knowledge gaps: %w", err)
	}

	return s.repos.Runs.MarkDone(ctx, run.ID)
}

// EntityReputation — сколько раз claims об этой сущности (по CanonicalName)
// цитировались гипотезами с тем или иным экспертным вердиктом в прошлых
// прогонах — подмешивается в промпт генерации как "обучение на фидбэке"
// (см. loadEntityReputations).
type EntityReputation struct {
	Name          string
	Confirmed     int
	Rejected      int
	NeedsRevision int
}

// loadEntityReputations поднимает историю фидбэка по сущностям, к которым
// резолвились claims текущего прогона (resolveEntities выше уже отработал —
// SubjectEntityID/MetricEntityID заполнены). Ошибка не блокирует
// генерацию — отсутствие репутации просто равно "истории пока нет".
func (s *Service) loadEntityReputations(ctx context.Context, claims []domain.Claim) []EntityReputation {
	idSet := make(map[uuid.UUID]struct{})
	for _, c := range claims {
		if c.SubjectEntityID != nil {
			idSet[*c.SubjectEntityID] = struct{}{}
		}
		if c.MetricEntityID != nil {
			idSet[*c.MetricEntityID] = struct{}{}
		}
	}
	if len(idSet) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}

	stats, err := s.repos.Entities.GetFeedbackStats(ctx, ids)
	if err != nil {
		logger.LogWarningCtx(ctx, "load entity reputations: %v", err)
		return nil
	}
	if len(stats) == 0 {
		return nil
	}
	statIDs := make([]uuid.UUID, len(stats))
	for i, st := range stats {
		statIDs[i] = st.EntityID
	}
	entities, err := s.repos.Entities.GetByIDs(ctx, statIDs)
	if err != nil {
		return nil
	}
	nameByID := make(map[uuid.UUID]string, len(entities))
	for _, e := range entities {
		nameByID[e.ID] = e.CanonicalName
	}

	out := make([]EntityReputation, 0, len(stats))
	for _, st := range stats {
		name, ok := nameByID[st.EntityID]
		if !ok || name == "" {
			continue
		}
		out = append(out, EntityReputation{
			Name: name, Confirmed: st.Confirmed, Rejected: st.Rejected, NeedsRevision: st.NeedsRevision,
		})
	}
	return out
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

// GetClaimSources возвращает claims, процитированные гипотезами (по
// EvidenceRefs), индексированные по ID — отчёты/экспорт используют это,
// чтобы показать источник (document_title) каждой evidence-ссылки, а не
// просто её количество.
func (s *Service) GetClaimSources(ctx context.Context, hyps []domain.Hypothesis) (map[uuid.UUID]domain.Claim, error) {
	idSet := make(map[uuid.UUID]struct{})
	for _, h := range hyps {
		for _, ref := range h.EvidenceRefs {
			idSet[ref] = struct{}{}
		}
	}
	ids := make([]uuid.UUID, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	claims, err := s.repos.Claims.GetByIDs(ctx, ids)
	if err != nil {
		return nil, errs.Wrap(err, errs.ErrTypeInternal, "get claim sources")
	}
	out := make(map[uuid.UUID]domain.Claim, len(claims))
	for _, c := range claims {
		out[c.ID] = c
	}
	return out, nil
}
