package externalApi

import "context"

// LLMClient — pluggable-интерфейс reasoning-LLM. Сегодня реализован Yandex GPT
// (Foundation Models API); прод-развёртывание может подменить его на
// self-hosted OpenAI-совместимый эндпоинт (vLLM/Ollama + Qwen3), не трогая
// services/hypothesisfactory.
type LLMClient interface {
	Complete(ctx context.Context, req CompleteRequest) (CompleteResponse, error)
}

type Message struct {
	Role    string // system | user | assistant
	Content string
}

type CompleteRequest struct {
	Messages    []Message
	Temperature float64
	MaxTokens   int
}

type CompleteResponse struct {
	Text string
}
