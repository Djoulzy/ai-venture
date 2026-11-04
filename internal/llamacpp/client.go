// Package llamacpp implements inference.Client through a local llama-server.
package llamacpp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"ai-venture/internal/inference"
	"ai-venture/internal/inference/httpjson"
	"ai-venture/internal/locale"
)

type Client struct {
	model string
	http  *httpjson.Client
}

var _ inference.Client = (*Client)(nil)

func New(address, model string, timeout time.Duration) (*Client, error) {
	if strings.TrimSpace(model) == "" {
		return nil, locale.Errorf("llama.cpp model name required")
	}
	client, err := httpjson.New(address, timeout)
	if err != nil {
		return nil, err
	}
	return &Client{model: model, http: client}, nil
}

// Ready waits for model loading and verifies the alias on reused servers too.
func (c *Client) Ready(ctx context.Context) (bool, error) {
	status, data, err := c.http.Do(ctx, http.MethodGet, "/health", nil)
	if err != nil {
		return false, err
	}
	if status == http.StatusServiceUnavailable {
		return false, nil
	}
	if status != http.StatusOK {
		return false, locale.Errorf("llama-server check: HTTP %d", status)
	}
	var health struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(data, &health); err != nil || health.Status != "ok" {
		return false, locale.Errorf("invalid llama-server /health response")
	}
	status, data, err = c.http.Do(ctx, http.MethodGet, "/v1/models", nil)
	if err != nil {
		return false, err
	}
	if status == http.StatusServiceUnavailable {
		return false, nil
	}
	if status != http.StatusOK {
		return false, locale.Errorf("llama-server model list: HTTP %d", status)
	}
	var models struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &models); err != nil {
		return false, locale.Errorf("invalid llama-server model list: %w", err)
	}
	for _, model := range models.Data {
		if model.ID == c.model {
			return true, nil
		}
	}
	return false, locale.Errorf("llama.cpp server does not offer model %q; check -model or the server --alias", c.model)
}

func (c *Client) Complete(ctx context.Context, request inference.Request) (string, error) {
	if !json.Valid(request.Schema) || request.MaxTokens <= 0 {
		return "", locale.Errorf("invalid inference request")
	}
	body := struct {
		Model          string              `json:"model"`
		Messages       []inference.Message `json:"messages"`
		Stream         bool                `json:"stream"`
		Temperature    float64             `json:"temperature"`
		MaxTokens      int                 `json:"max_tokens"`
		ResponseFormat struct {
			Type   string          `json:"type"`
			Schema json.RawMessage `json:"schema"`
		} `json:"response_format"`
		ChatTemplateKwargs map[string]bool `json:"chat_template_kwargs"`
		ReasoningEffort    string          `json:"reasoning_effort"`
	}{Model: c.model, Messages: request.Messages, Temperature: request.Temperature, MaxTokens: request.MaxTokens,
		ChatTemplateKwargs: map[string]bool{"enable_thinking": false}, ReasoningEffort: "none"}
	// llama-server documents its schema directly inside response_format.
	body.ResponseFormat.Type = "json_schema"
	body.ResponseFormat.Schema = request.Schema
	status, data, err := c.http.Do(ctx, http.MethodPost, "/v1/chat/completions", body)
	if err != nil {
		return "", locale.Errorf("llama-server unavailable: %w", err)
	}
	if status != http.StatusOK {
		return "", locale.Errorf("llama-server HTTP %d (check the model and server)", status)
	}
	var response struct {
		Choices []struct {
			Message      inference.Message `json:"message"`
			FinishReason string            `json:"finish_reason"`
		} `json:"choices"`
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return "", locale.Errorf("invalid llama-server response envelope: %w", err)
	}
	if len(response.Error) > 0 && string(response.Error) != "null" {
		return "", locale.Errorf("llama-server reported a generation error")
	}
	if len(response.Choices) != 1 {
		return "", locale.Errorf("llama-server must return exactly one response")
	}
	choice := response.Choices[0]
	if choice.FinishReason != "stop" || choice.Message.Role != "assistant" || strings.TrimSpace(choice.Message.Content) == "" {
		return "", locale.Errorf("llama-server response is empty, truncated or incomplete")
	}
	return choice.Message.Content, nil
}
