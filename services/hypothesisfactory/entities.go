package hypothesisfactory

import (
	"context"
	"strings"

	"hypothesis-factory/domain"
	"hypothesis-factory/externalApi"
	"hypothesis-factory/repositories"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"
)

// entityResolutionThreshold — cosine distance (не similarity!) ниже которого
// новая сущность считается тем же узлом графа, что уже существующий. pgvector
// "<=>" для cosine возвращает distance = 1 - similarity, т.е. 0.10 означает
// ~90% похожести. Порог намеренно консервативный: лучше два близких, но
// разных узла ("диаметр насадки гидроциклона" vs "тип гидроциклона"), чем
// ложное склеивание разных по сути сущностей.
const entityResolutionThreshold = 0.10

// resolveEntities резолвит subject/metric каждого claim в domain.Entity —
// существующую похожую (переиспользуем id) или новую. Это и есть
// "склеивание похожих сущностей": одна и та же сущность, упомянутая в разных
// источниках или прогонах, схлопывается в один узел графа, а не дублируется
// на каждый claim. Мутирует claims на месте (проставляет *EntityID).
//
// Деградация: если embed-вызов не удался, граф просто не обогащается для
// этого набора claims — сами claims (и остальной пайплайн) не ломаются,
// граф — надстройка, а не критический путь.
func resolveEntities(ctx context.Context, entities *repositories.EntityRepo, pyworker *externalApi.PyworkerClient, claims []domain.Claim) {
	if len(claims) == 0 {
		return
	}

	texts := make([]string, 0, len(claims)*2)
	for i := range claims {
		texts = append(texts, claims[i].Subject, claims[i].Metric)
	}
	vectors, err := pyworker.Embed(ctx, texts)
	if err != nil || len(vectors) != len(texts) {
		return
	}

	for i := range claims {
		subjectVec := vectors[i*2]
		metricVec := vectors[i*2+1]

		if id, err := resolveOneEntity(ctx, entities, subjectVec, claims[i].Subject, claims[i].SubjectKind); err == nil {
			claims[i].SubjectEntityID = id
		}
		if strings.TrimSpace(claims[i].Metric) != "" {
			if id, err := resolveOneEntity(ctx, entities, metricVec, claims[i].Metric, claims[i].MetricKind); err == nil {
				claims[i].MetricEntityID = id
			}
		}
	}
}

func resolveOneEntity(ctx context.Context, entities *repositories.EntityRepo, embedding []float32, name, kind string) (*uuid.UUID, error) {
	kind = normalizeEntityKind(kind)

	existing, distance, err := entities.FindNearest(ctx, embedding, kind)
	if err != nil {
		return nil, err
	}
	if existing != nil && distance <= entityResolutionThreshold {
		return &existing.ID, nil
	}

	v := pgvector.NewVector(embedding)
	e := &domain.Entity{CanonicalName: name, Kind: kind, Embedding: &v}
	if err := entities.Create(ctx, e); err != nil {
		return nil, err
	}
	return &e.ID, nil
}

func normalizeEntityKind(kind string) string {
	switch kind {
	case domain.EntityKindEquipment, domain.EntityKindMetric, domain.EntityKindReagent,
		domain.EntityKindProcess, domain.EntityKindMaterial:
		return kind
	default:
		return domain.EntityKindOther
	}
}
