package hypothesisfactory

import (
	"context"
	"fmt"
	"strings"

	"hypothesis-factory/domain"
	"hypothesis-factory/externalApi"
	"hypothesis-factory/repositories"
)

// retrieve — гибридный (лексика+вектор) поиск, засеянный ProblemSpec, затем
// реранкинг bge-reranker-v2-m3 для стабилизации top-N перед (дорогими)
// вызовами claim extraction.
func retrieve(ctx context.Context, chunks *repositories.ChunkRepo, pyworker *externalApi.PyworkerClient, spec domain.ProblemSpec, domainFilter string, topK int) ([]domain.RetrievedChunk, error) {
	query := buildRetrievalQuery(spec)

	queryVec, err := pyworker.EmbedOne(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	candidates, err := chunks.HybridSearch(ctx, query, queryVec, domainFilter, 40)
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
	rerankScores, err := pyworker.Rerank(ctx, query, texts)
	if err == nil && len(rerankScores) == len(candidates) {
		for i := range candidates {
			candidates[i].FusedScore = rerankScores[i]
		}
		sortByFusedScoreDesc(candidates)
	}
	// Если реранкер недоступен — остаётся порядок из HybridSearch (fused lex+vector).

	if topK > 0 && len(candidates) > topK {
		candidates = candidates[:topK]
	}
	return candidates, nil
}

func buildRetrievalQuery(spec domain.ProblemSpec) string {
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

func sortByFusedScoreDesc(cs []domain.RetrievedChunk) {
	for i := 1; i < len(cs); i++ {
		for j := i; j > 0 && cs[j].FusedScore > cs[j-1].FusedScore; j-- {
			cs[j], cs[j-1] = cs[j-1], cs[j]
		}
	}
}
