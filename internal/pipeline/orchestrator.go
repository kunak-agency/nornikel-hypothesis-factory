package pipeline

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"hypothesis-factory/internal/embed"
	"hypothesis-factory/internal/llm"
	"hypothesis-factory/internal/models"
	"hypothesis-factory/internal/store"
)

type Orchestrator struct {
	llmClient   llm.Client
	embedClient *embed.Client
	store       *store.Store
	domain      string
	topK        int
}

func NewOrchestrator(llmClient llm.Client, embedClient *embed.Client, s *store.Store, domain string, topK int) *Orchestrator {
	if domain == "" {
		domain = "flotation"
	}
	if topK <= 0 {
		topK = 15
	}
	return &Orchestrator{llmClient: llmClient, embedClient: embedClient, store: s, domain: domain, topK: topK}
}

type RunResult struct {
	RunID       uuid.UUID
	ProblemSpec models.ProblemSpec
	Hypotheses  []models.Hypothesis
}

// Run executes the full pipeline for one user request: rawText is free-text
// problem description and/or a serialized tailings-report summary.
func (o *Orchestrator) Run(ctx context.Context, rawText string, rawInput map[string]any) (RunResult, error) {
	spec, err := BuildProblemSpec(ctx, o.llmClient, rawText)
	if err != nil {
		return RunResult{}, fmt.Errorf("build problem spec: %w", err)
	}

	runID, err := o.store.CreateRun(ctx, spec, rawInput)
	if err != nil {
		return RunResult{}, fmt.Errorf("create run: %w", err)
	}

	_ = o.store.UpdateRunStatus(ctx, runID, "retrieving")
	chunks, err := Retrieve(ctx, o.store, o.embedClient, spec, o.domain, o.topK)
	if err != nil {
		_ = o.store.UpdateRunStatus(ctx, runID, "failed")
		return RunResult{}, fmt.Errorf("retrieve: %w", err)
	}
	if len(chunks) == 0 {
		_ = o.store.UpdateRunStatus(ctx, runID, "failed")
		return RunResult{}, fmt.Errorf("no relevant chunks found in knowledge base for domain %q — ingest documents first", o.domain)
	}

	_ = o.store.UpdateRunStatus(ctx, runID, "extracting")
	claims, err := ExtractClaims(ctx, o.llmClient, chunks)
	if err != nil {
		_ = o.store.UpdateRunStatus(ctx, runID, "failed")
		return RunResult{}, fmt.Errorf("extract claims: %w", err)
	}
	for _, c := range claims {
		if _, err := o.store.InsertClaim(ctx, c); err != nil {
			return RunResult{}, fmt.Errorf("persist claim: %w", err)
		}
	}

	_ = o.store.UpdateRunStatus(ctx, runID, "generating")
	hyps, err := GenerateHypotheses(ctx, o.llmClient, spec, claims)
	if err != nil {
		_ = o.store.UpdateRunStatus(ctx, runID, "failed")
		return RunResult{}, fmt.Errorf("generate hypotheses: %w", err)
	}
	for i := range hyps {
		hyps[i].RunID = runID
	}

	_ = o.store.UpdateRunStatus(ctx, runID, "critiquing")
	claimByID := make(map[string]models.Claim, len(claims))
	for _, c := range claims {
		claimByID[c.ID.String()] = c
	}
	hyps, err = Critique(ctx, o.llmClient, spec, claimByID, hyps)
	if err != nil {
		_ = o.store.UpdateRunStatus(ctx, runID, "failed")
		return RunResult{}, fmt.Errorf("critique: %w", err)
	}

	for _, h := range hyps {
		if _, err := o.store.InsertHypothesis(ctx, h); err != nil {
			return RunResult{}, fmt.Errorf("persist hypothesis: %w", err)
		}
	}

	_ = o.store.UpdateRunStatus(ctx, runID, "done")
	return RunResult{RunID: runID, ProblemSpec: spec, Hypotheses: hyps}, nil
}
