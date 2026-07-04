package hypothesisfactory

import (
	"context"
	"strconv"

	"hypothesis-factory/domain"
	"hypothesis-factory/pkg/errs"

	"github.com/google/uuid"
)

// Graph — тройка "источник(entity) → claim → гипотеза", буквально то, что
// требует кейс ("визуальное представление связей"). Не отдельное хранилище:
// строится запросом по уже существующим claims/hypotheses/entities.
type GraphNode struct {
	ID    string
	Type  string // entity | claim | hypothesis
	Label string
	Kind  string // entity: EntityKind*; hypothesis: "rank_N"; claim: source_confidence
}

type GraphEdge struct {
	ID    string
	From  string
	To    string
	Type  string // subject | affects | evidence
	Label string
}

type Graph struct {
	Nodes []GraphNode
	Edges []GraphEdge
}

// BuildRunGraph строит граф evidence-цепочки для гипотез конкретного
// прогона: только claims, реально процитированные в hypotheses.evidence_refs
// (не все claims прогона) — это ответ на вопрос "почему именно эта гипотеза",
// а не дамп всей извлечённой evidence. Entity-узлы при этом накоплены не
// только из этого прогона (resolveEntities мог переиспользовать сущности из
// прошлых прогонов) — граф растёт между прогонами.
func (s *Service) BuildRunGraph(ctx context.Context, runIDStr string) (Graph, error) {
	runID, err := uuid.Parse(runIDStr)
	if err != nil {
		return Graph{}, errs.NewValidationError("invalid run id")
	}

	// Явная проверка существования прогона: GetByRunID ниже вернёт пустой срез
	// (без ошибки) и для несуществующего прогона, из-за чего граф раньше отдавал
	// 200 с пустым графом вместо 404. GetByID отдаёт типизированный NotFound.
	if _, err := s.repos.Runs.GetByID(ctx, runID); err != nil {
		return Graph{}, err
	}

	hyps, err := s.repos.Hypotheses.GetByRunID(ctx, runID)
	if err != nil {
		return Graph{}, errs.Wrap(err, errs.ErrTypeInternal, "get hypotheses")
	}

	var refLists [][]uuid.UUID
	for _, h := range hyps {
		refLists = append(refLists, h.EvidenceRefs)
	}
	claimIDs := uniqueUUIDs(refLists...)

	claims, err := s.repos.Claims.GetByIDs(ctx, claimIDs)
	if err != nil {
		return Graph{}, errs.Wrap(err, errs.ErrTypeInternal, "get claims")
	}

	var subjectIDs, metricIDs []uuid.UUID
	for _, c := range claims {
		if c.SubjectEntityID != nil {
			subjectIDs = append(subjectIDs, *c.SubjectEntityID)
		}
		if c.MetricEntityID != nil {
			metricIDs = append(metricIDs, *c.MetricEntityID)
		}
	}
	entityIDs := uniqueUUIDs(subjectIDs, metricIDs)
	entities, err := s.repos.Entities.GetByIDs(ctx, entityIDs)
	if err != nil {
		return Graph{}, errs.Wrap(err, errs.ErrTypeInternal, "get entities")
	}

	var g Graph
	for _, e := range entities {
		g.Nodes = append(g.Nodes, GraphNode{ID: e.ID.String(), Type: "entity", Label: e.CanonicalName, Kind: e.Kind})
	}

	claimByID := make(map[string]domain.Claim, len(claims))
	for _, c := range claims {
		claimByID[c.ID.String()] = c
		g.Nodes = append(g.Nodes, GraphNode{ID: c.ID.String(), Type: "claim", Label: c.Action, Kind: c.SourceConfidence})
		if c.SubjectEntityID != nil {
			g.Edges = append(g.Edges, GraphEdge{
				ID: "subj-" + c.ID.String(), From: c.SubjectEntityID.String(), To: c.ID.String(),
				Type: "subject", Label: c.Subject,
			})
		}
		if c.MetricEntityID != nil {
			g.Edges = append(g.Edges, GraphEdge{
				ID: "aff-" + c.ID.String(), From: c.ID.String(), To: c.MetricEntityID.String(),
				Type: "affects", Label: c.EffectDirection,
			})
		}
	}

	for _, h := range hyps {
		g.Nodes = append(g.Nodes, GraphNode{
			ID: h.ID.String(), Type: "hypothesis", Label: h.Statement, Kind: "rank_" + strconv.Itoa(h.Rank),
		})
		for _, ref := range h.EvidenceRefs {
			if _, ok := claimByID[ref.String()]; ok {
				g.Edges = append(g.Edges, GraphEdge{
					ID: "ev-" + ref.String() + "-" + h.ID.String(), From: ref.String(), To: h.ID.String(), Type: "evidence",
				})
			}
		}
	}

	return g, nil
}
