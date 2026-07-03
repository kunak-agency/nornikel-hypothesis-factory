package pipeline

import (
	"context"
	"fmt"
	"strings"

	"hypothesis-factory/internal/embed"
	"hypothesis-factory/internal/models"
	"hypothesis-factory/internal/store"
)

// Retrieve runs hybrid (lexical + dense) search seeded by the ProblemSpec,
// then reranks with bge-reranker-v2-m3 to stabilize the top set before the
// (expensive) claim-extraction LLM calls.
func Retrieve(ctx context.Context, s *store.Store, embedClient *embed.Client, spec models.ProblemSpec, domain string, topK int) ([]models.RetrievedChunk, error) {
	query := buildRetrievalQuery(spec)

	queryVec, err := embedClient.EmbedOne(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	candidates, err := s.HybridSearch(ctx, query, queryVec, domain, 40)
	if err != nil {
		return nil, fmt.Errorf("hybrid search: %w", err)
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	texts := make([]string, len(candidates))
	for i, c := range candidates {
		texts[i] = c.Content
	}
	rerankScores, err := embedClient.Rerank(ctx, query, texts)
	if err == nil && len(rerankScores) == len(candidates) {
		for i := range candidates {
			candidates[i].FusedScore = rerankScores[i]
		}
		sortByFusedScoreDesc(candidates)
	}
	// If rerank fails (e.g. reranker not loaded), fall back to hybrid fused score
	// order already returned by HybridSearch.

	if topK > 0 && len(candidates) > topK {
		candidates = candidates[:topK]
	}
	return candidates, nil
}

func buildRetrievalQuery(spec models.ProblemSpec) string {
	var b strings.Builder
	b.WriteString(spec.TargetKPI)
	if len(spec.TargetMetals) > 0 {
		b.WriteString(". Металлы: " + strings.Join(spec.TargetMetals, ", "))
	}
	if len(spec.LossHotspots) > 0 {
		b.WriteString(". Точки потерь: " + strings.Join(spec.LossHotspots, "; "))
	}
	if spec.Plant != "" {
		b.WriteString(". Фабрика: " + spec.Plant)
	}
	return b.String()
}

func sortByFusedScoreDesc(cs []models.RetrievedChunk) {
	for i := 1; i < len(cs); i++ {
		for j := i; j > 0 && cs[j].FusedScore > cs[j-1].FusedScore; j-- {
			cs[j], cs[j-1] = cs[j-1], cs[j]
		}
	}
}
