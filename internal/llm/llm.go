// Package llm defines a pluggable reasoning-LLM interface.
// Today's backend is Yandex GPT (Foundation Models API); a production
// deployment can swap in a self-hosted OpenAI-compatible endpoint
// (vLLM/Ollama serving Qwen3) without touching pipeline code.
package llm

import "context"

// Message is a single chat turn.
type Message struct {
	Role    string // system | user | assistant
	Content string
}

// CompleteRequest is a structured-output completion request.
type CompleteRequest struct {
	Messages []Message
	// JSONSchema, if set, asks the backend to constrain output to this schema.
	// Not all backends enforce it server-side; callers must still validate/parse.
	JSONSchema map[string]any
	Temperature float64
	MaxTokens   int
}

type CompleteResponse struct {
	Text string
}

// Client is the reasoning-LLM abstraction used by the pipeline.
type Client interface {
	Complete(ctx context.Context, req CompleteRequest) (CompleteResponse, error)
}
