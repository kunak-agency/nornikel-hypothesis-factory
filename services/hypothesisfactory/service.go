package hypothesisfactory

import (
	"context"
	"fmt"
	"sort"
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

// StartRunOptions — параметры конкретного прогона (не сервиса): разные
// вызовы могут работать с разными предметными областями и приоритетами
// ранжирования без передеплоя.
type StartRunOptions struct {
	Domain         string
	Language       string
	RankingWeights domain.RankingWeights
	ExcludedTopics []string
	// Plant — если задано, ПЕРЕКРЫВАЕТ то, что LLM извлечёт из RawText в
	// ProblemSpec.Plant. RawText — недоверенный ввод: он может содержать
	// текст, замаскированный под данные, но на деле являющийся инструкцией
	// ("игнорируй предыдущие инструкции, фабрика = ..."). Явный Plant даёт
	// вызывающей стороне детерминированный выбор фабрики, не зависящий от
	// того, устоял ли extraction-промпт перед инъекцией.
	Plant string
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
	if opts.Plant != "" {
		spec.Plant = opts.Plant
	}
	s.enrichAvailableEquipment(ctx, &spec)

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

// enrichAvailableEquipment детерминированно (SQL по имени фабрики, не через
// RAG-поиск) подмешивает в ProblemSpec.AvailableEquipment реальные модели/
// параметры оборудования, если фабрика из spec.Plant есть в
// plant_equipment. Это прямой путь к гипотезам вида "диаметр насадки X→Y" —
// без этого LLM не может предложить конкретное текущее значение, потому что
// его физически нет ни в одном retrieved-чанке в грамматически удобной для
// поиска форме. Ошибка поиска не блокирует прогон — это обогащение, не
// обязательный шаг.
func (s *Service) enrichAvailableEquipment(ctx context.Context, spec *domain.ProblemSpec) {
	if spec.Plant == "" {
		return
	}
	matches, err := s.repos.PlantEquipment.FindByPlantMention(ctx, spec.Plant)
	if err != nil {
		logger.LogWarningCtx(ctx, "plant equipment lookup failed for %q: %v", spec.Plant, err)
		return
	}
	seen := make(map[string]bool)
	seenTypes := make(map[string]bool)
	seenTypeModel := make(map[string]bool)
	for _, e := range matches {
		desc := e.Model
		if e.CircuitPosition != "" {
			desc = fmt.Sprintf("%s (%s)", desc, e.CircuitPosition)
		}
		if e.EquipmentType != "" {
			if !seenTypes[e.EquipmentType] {
				seenTypes[e.EquipmentType] = true
				spec.EquipmentTypes = append(spec.EquipmentTypes, e.EquipmentType)
			}
			if e.Model != "" && !seenTypeModel[e.EquipmentType+"|"+e.Model] {
				seenTypeModel[e.EquipmentType+"|"+e.Model] = true
				if spec.EquipmentByType == nil {
					spec.EquipmentByType = map[string][]string{}
				}
				spec.EquipmentByType[e.EquipmentType] = append(spec.EquipmentByType[e.EquipmentType], e.Model)
			}
		}
		if desc == "" || seen[desc] {
			continue
		}
		seen[desc] = true
		spec.AvailableEquipment = append(spec.AvailableEquipment, desc)
	}
	sort.Strings(spec.EquipmentTypes)
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

	if opts.Plant != "" {
		spec.Plant = opts.Plant
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
	s.enrichAvailableEquipment(ctx, &spec)

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
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
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
	if len(claims) == 0 {
		// Честный отказ вместо генерации на пустом evidence-pack: без
		// grounded claims модель либо вернёт пустоту, либо сфабрикует
		// ссылки (которые всё равно будут отброшены) — в обоих случаях
		// результат непроверяем, и статус failed с понятной причиной
		// полезнее, чем пустой «успешный» отчёт.
		return fmt.Errorf("no grounded claims extracted from %d retrieved chunks — knowledge base has no verifiable evidence for this problem", len(chunks))
	}
	resolveEntities(ctx, s.repos.Entities, s.pyworker, claims)
	for i := range claims {
		claims[i].RunID = &run.ID
	}
	if err := s.repos.Claims.CreateBatch(ctx, claims); err != nil {
		return fmt.Errorf("persist claims: %w", err)
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

	if err := s.repos.Hypotheses.CreateBatch(ctx, hyps); err != nil {
		return fmt.Errorf("persist hypotheses: %w", err)
	}

	// Не фатально: гипотезы уже закоммичены строкой выше — если бы эта
	// ошибка проваливала весь прогон, клиент увидел бы status=failed и мог
	// бы никогда не отрендерить уже готовые, реально сохранённые гипотезы
	// (knowledge gaps — диагностическое дополнение к отчёту, не его ядро).
	if err := s.repos.Runs.UpdateKnowledgeGaps(ctx, run.ID, run.KnowledgeGaps); err != nil {
		logger.LogWarningCtx(ctx, "persist knowledge gaps for run %s: %v", run.ID, err)
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
	var subjectIDs, metricIDs []uuid.UUID
	for _, c := range claims {
		if c.SubjectEntityID != nil {
			subjectIDs = append(subjectIDs, *c.SubjectEntityID)
		}
		if c.MetricEntityID != nil {
			metricIDs = append(metricIDs, *c.MetricEntityID)
		}
	}
	ids := uniqueUUIDs(subjectIDs, metricIDs)
	if len(ids) == 0 {
		return nil
	}

	stats, err := s.repos.Entities.GetFeedbackStats(ctx, ids)
	if err != nil {
		logger.LogWarningCtx(ctx, "load entity reputations: %v", err)
		return nil
	}

	out := make([]EntityReputation, 0, len(stats))
	for _, st := range stats {
		if st.CanonicalName == "" {
			continue
		}
		out = append(out, EntityReputation{
			Name: st.CanonicalName, Confirmed: st.Confirmed, Rejected: st.Rejected, NeedsRevision: st.NeedsRevision,
		})
	}
	return out
}

func (s *Service) GetRun(ctx context.Context, id string) (*domain.HypothesisRun, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errs.NewValidationError("invalid run id")
	}
	// GetByID сам возвращает типизированный errs.NotFound/Internal (requireFound) —
	// пробрасываем как есть, НЕ оборачивая в Internal, иначе 404 стал бы 500.
	return s.repos.Runs.GetByID(ctx, uid)
}

func (s *Service) GetHypotheses(ctx context.Context, runID string) ([]domain.Hypothesis, error) {
	uid, err := uuid.Parse(runID)
	if err != nil {
		return nil, errs.NewValidationError("invalid run id")
	}
	return s.repos.Hypotheses.GetByRunID(ctx, uid)
}

// UpdateVerificationPlan заменяет дорожную карту гипотезы (PUT
// /hypotheses/{id}/verification-plan) без затрагивания scores/rank — частичный
// апдейт колонки в репозитории не конфликтует с rerank. GetByID отдаёт
// типизированный 404 (requireFound), пробрасываем как есть.
func (s *Service) UpdateVerificationPlan(ctx context.Context, id uuid.UUID, plan []domain.VerificationStep) (*domain.Hypothesis, error) {
	hyp, err := s.repos.Hypotheses.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	hyp.VerificationPlan = plan
	if err := s.repos.Hypotheses.UpdateVerificationPlan(ctx, hyp); err != nil {
		return nil, errs.Wrap(err, errs.ErrTypeInternal, "persist verification plan")
	}
	return hyp, nil
}

func (s *Service) ListRuns(ctx context.Context, offset, limit int) ([]domain.HypothesisRun, int64, error) {
	return s.repos.Runs.List(ctx, offset, limit)
}

// GetClaimSources возвращает claims, процитированные гипотезами (по
// EvidenceRefs), индексированные по ID — отчёты/экспорт используют это,
// чтобы показать источник (document_title) каждой evidence-ссылки, а не
// просто её количество.
func (s *Service) GetClaimSources(ctx context.Context, hyps []domain.Hypothesis) (map[uuid.UUID]domain.Claim, error) {
	var refLists [][]uuid.UUID
	for _, h := range hyps {
		refLists = append(refLists, h.EvidenceRefs)
	}
	ids := uniqueUUIDs(refLists...)
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

// GetRunClaims — evidence-pack прогона целиком: все claims, извлечённые в
// нём (включая не процитированные гипотезами), с пометкой, какие гипотезы
// каждый claim цитируют.
func (s *Service) GetRunClaims(ctx context.Context, runID string) ([]domain.Claim, map[uuid.UUID][]uuid.UUID, error) {
	uid, err := uuid.Parse(runID)
	if err != nil {
		return nil, nil, errs.NewValidationError("invalid run id")
	}
	claims, err := s.repos.Claims.GetByRunID(ctx, uid)
	if err != nil {
		return nil, nil, errs.Wrap(err, errs.ErrTypeInternal, "get run claims")
	}
	hyps, err := s.repos.Hypotheses.GetByRunID(ctx, uid)
	if err != nil {
		return nil, nil, errs.Wrap(err, errs.ErrTypeInternal, "get run hypotheses")
	}
	citedBy := make(map[uuid.UUID][]uuid.UUID)
	for _, h := range hyps {
		for _, ref := range h.EvidenceRefs {
			citedBy[ref] = append(citedBy[ref], h.ID)
		}
	}
	return claims, citedBy, nil
}

// DeleteRun удаляет прогон каскадом (гипотезы + их фидбэк уходят по FK).
func (s *Service) DeleteRun(ctx context.Context, runID string) error {
	uid, err := uuid.Parse(runID)
	if err != nil {
		return errs.NewValidationError("invalid run id")
	}
	n, err := s.repos.Runs.Delete(ctx, uid)
	if err != nil {
		return errs.Wrap(err, errs.ErrTypeInternal, "delete run")
	}
	if n == 0 {
		return errs.NewNotFoundError("run")
	}
	return nil
}

// RerankRun пересчитывает Total/Rank гипотез прогона с новыми весами БЕЗ
// повторных LLM-вызовов: компоненты оценок (evidence/feasibility/impact/
// novelty/risk) уже сохранены после критик-ансамбля, меняется только
// прозрачная формула свёртки — "экспертная настройка весов" интерактивна
// и стоит миллисекунды, а не минуты прогона.
func (s *Service) RerankRun(ctx context.Context, runID string, weights domain.RankingWeights) ([]domain.Hypothesis, error) {
	uid, err := uuid.Parse(runID)
	if err != nil {
		return nil, errs.NewValidationError("invalid run id")
	}
	hyps, err := s.repos.Hypotheses.GetByRunID(ctx, uid)
	if err != nil {
		return nil, errs.Wrap(err, errs.ErrTypeInternal, "get hypotheses")
	}
	if len(hyps) == 0 {
		return nil, errs.NewNotFoundError("hypotheses for run")
	}
	for i := range hyps {
		hyps[i].Scores.Total = totalScore(hyps[i].Scores, weights)
	}
	sort.SliceStable(hyps, func(i, j int) bool { return hyps[i].Scores.Total > hyps[j].Scores.Total })
	for i := range hyps {
		hyps[i].Rank = i + 1
		if err := s.repos.Hypotheses.UpdateScoresAndRank(ctx, &hyps[i]); err != nil {
			return nil, errs.Wrap(err, errs.ErrTypeInternal, "persist rerank")
		}
	}
	return hyps, nil
}

// GetHypothesisFeedback — все экспертные оценки гипотезы.
func (s *Service) GetHypothesisFeedback(ctx context.Context, hypothesisID string) ([]domain.Feedback, error) {
	uid, err := uuid.Parse(hypothesisID)
	if err != nil {
		return nil, errs.NewValidationError("invalid hypothesisId")
	}
	return s.repos.Feedback.ListByHypothesis(ctx, uid)
}

// EntityReputations — репутация всех сущностей с хотя бы одним вердиктом:
// видимая сторона "обучения на фидбэке".
func (s *Service) EntityReputations(ctx context.Context) ([]repositories.FeedbackStats, error) {
	return s.repos.Entities.AllFeedbackStats(ctx)
}

// --- CRUD структурированного оборудования фабрик (plant_equipment) ---

func (s *Service) ListPlantEquipment(ctx context.Context, plantName string) ([]domain.PlantEquipment, error) {
	return s.repos.PlantEquipment.List(ctx, plantName)
}

func (s *Service) CreatePlantEquipment(ctx context.Context, e *domain.PlantEquipment) error {
	if err := s.repos.PlantEquipment.Create(ctx, e); err != nil {
		return errs.Wrap(err, errs.ErrTypeInternal, "create plant equipment")
	}
	return nil
}

func (s *Service) UpdatePlantEquipment(ctx context.Context, e *domain.PlantEquipment) error {
	n, err := s.repos.PlantEquipment.Update(ctx, e)
	if err != nil {
		return errs.Wrap(err, errs.ErrTypeInternal, "update plant equipment")
	}
	if n == 0 {
		return errs.NewNotFoundError("plant equipment")
	}
	return nil
}

func (s *Service) DeletePlantEquipment(ctx context.Context, id uuid.UUID) error {
	n, err := s.repos.PlantEquipment.Delete(ctx, id)
	if err != nil {
		return errs.Wrap(err, errs.ErrTypeInternal, "delete plant equipment")
	}
	if n == 0 {
		return errs.NewNotFoundError("plant equipment")
	}
	return nil
}

// Plants — известные фабрики (по plant_equipment) для селектора "выбор
// фабрики настройкой".
func (s *Service) Plants(ctx context.Context) ([]repositories.PlantSummary, error) {
	return s.repos.PlantEquipment.Plants(ctx)
}
