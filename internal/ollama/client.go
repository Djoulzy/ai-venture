// Package ollama implements inference.Client using the local Ollama chat API.
package ollama

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

func New(baseURL, model string, timeout time.Duration) (*Client, error) {
	if strings.TrimSpace(model) == "" || timeout <= 0 {
		return nil, locale.Errorf("Ollama model and timeout required")
	}
	client, err := httpjson.New(baseURL, timeout)
	if err != nil {
		return nil, err
	}
	return &Client{model: model, http: client}, nil
}

// Ready checks service availability and the requested model without generating text.
// false, nil means that an existing service is still becoming ready.
func (c *Client) Ready(ctx context.Context) (bool, error) {
	status, data, err := c.http.Do(ctx, http.MethodGet, "/api/tags", nil)
	if err != nil {
		return false, err
	}
	if status == http.StatusServiceUnavailable {
		return false, nil
	}
	if status != http.StatusOK {
		return false, locale.Errorf("Ollama check: HTTP %d", status)
	}
	var result struct {
		Models *[]struct {
			Name  string `json:"name"`
			Model string `json:"model"`
		} `json:"models"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return false, locale.Errorf("invalid Ollama model list: %w", err)
	}
	if result.Models == nil {
		return false, locale.Errorf("this service does not provide an Ollama model list")
	}
	for _, model := range *result.Models {
		if modelName(model.Name) == modelName(c.model) || modelName(model.Model) == modelName(c.model) {
			return true, nil
		}
	}
	return false, locale.Errorf("Ollama model %q is missing; run ollama pull %s, then restart the game", c.model, c.model)
}

func modelName(name string) string {
	if name != "" && !strings.Contains(name[strings.LastIndex(name, "/")+1:], ":") {
		return name + ":latest"
	}
	return name
}

func (c *Client) Complete(ctx context.Context, request inference.Request) (string, error) {
	if !json.Valid(request.Schema) || request.MaxTokens <= 0 {
		return "", locale.Errorf("invalid inference request")
	}
	body := struct {
		Model    string              `json:"model"`
		Messages []inference.Message `json:"messages"`
		Format   json.RawMessage     `json:"format"`
		Stream   bool                `json:"stream"`
		Think    bool                `json:"think"`
		Options  map[string]any      `json:"options"`
	}{Model: c.model, Messages: request.Messages, Format: request.Schema,
		Options: map[string]any{"temperature": request.Temperature, "num_predict": request.MaxTokens}}
	status, data, err := c.http.Do(ctx, http.MethodPost, "/api/chat", body)
	if err != nil {
		return "", locale.Errorf("Ollama unavailable: %w", err)
	}
	var result struct {
		Message    inference.Message `json:"message"`
		Done       bool              `json:"done"`
		DoneReason string            `json:"done_reason"`
		Error      string            `json:"error"`
	}
	if status != http.StatusOK {
		// Do not echo an untrusted server body into the terminal.
		return "", locale.Errorf("Ollama HTTP %d (check the service and model %s)", status, c.model)
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", locale.Errorf("invalid Ollama response envelope: %w", err)
	}
	if result.Error != "" {
		return "", locale.Errorf("Ollama reported a generation error")
	}
	if !result.Done || result.DoneReason == "length" || result.Message.Role != "assistant" || strings.TrimSpace(result.Message.Content) == "" {
		return "", locale.Errorf("Ollama response is empty, truncated or incomplete")
	}
	return result.Message.Content, nil
}
