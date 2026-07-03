package externalApi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const yandexCompletionURL = "https://llm.api.cloud.yandex.net/foundationModels/v1/completion"

// YandexClient — клиент Yandex Cloud Foundation Models completion API.
type YandexClient struct {
	APIKey   string
	FolderID string
	ModelURI string
	HTTP     *http.Client
}

func NewYandexClient(apiKey, folderID, modelURI string) *YandexClient {
	return &YandexClient{
		APIKey:   apiKey,
		FolderID: folderID,
		ModelURI: modelURI,
		HTTP:     &http.Client{Timeout: 90 * time.Second},
	}
}

type yandexRequest struct {
	ModelURI          string              `json:"modelUri"`
	CompletionOptions yandexCompletionOpt `json:"completionOptions"`
	Messages          []yandexMessage     `json:"messages"`
}

type yandexCompletionOpt struct {
	Stream      bool    `json:"stream"`
	Temperature float64 `json:"temperature"`
	MaxTokens   string  `json:"maxTokens"`
}

type yandexMessage struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

type yandexResponse struct {
	Result struct {
		Alternatives []struct {
			Message struct {
				Role string `json:"role"`
				Text string `json:"text"`
			} `json:"message"`
			Status string `json:"status"`
		} `json:"alternatives"`
	} `json:"result"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (c *YandexClient) Complete(ctx context.Context, req CompleteRequest) (CompleteResponse, error) {
	msgs := make([]yandexMessage, 0, len(req.Messages))
	for _, m := range req.Messages {
		msgs = append(msgs, yandexMessage{Role: m.Role, Text: m.Content})
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4000
	}

	body := yandexRequest{
		ModelURI: c.ModelURI,
		CompletionOptions: yandexCompletionOpt{
			Stream:      false,
			Temperature: req.Temperature,
			MaxTokens:   fmt.Sprintf("%d", maxTokens),
		},
		Messages: msgs,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return CompleteResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, yandexCompletionURL, bytes.NewReader(payload))
	if err != nil {
		return CompleteResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Api-Key "+c.APIKey)
	httpReq.Header.Set("x-folder-id", c.FolderID)

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return CompleteResponse{}, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return CompleteResponse{}, err
	}

	if resp.StatusCode != http.StatusOK {
		return CompleteResponse{}, fmt.Errorf("yandex gpt: status %d: %s", resp.StatusCode, string(raw))
	}

	var parsed yandexResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return CompleteResponse{}, fmt.Errorf("yandex gpt: decode response: %w (raw=%s)", err, string(raw))
	}
	if parsed.Error != nil {
		return CompleteResponse{}, fmt.Errorf("yandex gpt: api error: %s", parsed.Error.Message)
	}
	if len(parsed.Result.Alternatives) == 0 {
		return CompleteResponse{}, fmt.Errorf("yandex gpt: empty alternatives (raw=%s)", string(raw))
	}

	return CompleteResponse{Text: parsed.Result.Alternatives[0].Message.Text}, nil
}
