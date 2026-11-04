package llamacpp_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ai-venture/internal/inference"
	"ai-venture/internal/llamacpp"
)

func TestChatProtocol(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/chat/completions" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("requête incorrecte")
		}
		var request struct {
			Model          string              `json:"model"`
			Stream         *bool               `json:"stream"`
			Messages       []inference.Message `json:"messages"`
			MaxTokens      int                 `json:"max_tokens"`
			ResponseFormat struct {
				Type   string         `json:"type"`
				Schema map[string]any `json:"schema"`
			} `json:"response_format"`
			ChatTemplateKwargs map[string]bool `json:"chat_template_kwargs"`
			ReasoningEffort    string          `json:"reasoning_effort"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request.Model != "lfm3:1.7b" || request.Stream == nil || *request.Stream || request.MaxTokens != 160 || len(request.Messages) != 1 || request.ResponseFormat.Type != "json_schema" || request.ResponseFormat.Schema["type"] != "object" || request.ReasoningEffort != "none" {
			t.Errorf("corps incorrect : %+v", request)
		}
		if think, ok := request.ChatTemplateKwargs["enable_thinking"]; !ok || think {
			t.Error("thinking non désactivé")
		}
		_, _ = io.WriteString(w, `{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"{\"action\":\"observe\",\"target\":\"\",\"tool\":\"\"}"}}]}`)
	}))
	defer server.Close()
	c, err := llamacpp.New(server.URL, "lfm3:1.7b", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	content, err := c.Complete(context.Background(), request())
	if err != nil || !strings.Contains(content, `"action":"observe"`) {
		t.Fatalf("%s %v", content, err)
	}
}

func request() inference.Request {
	return inference.Request{Messages: []inference.Message{{Role: "user", Content: "look"}}, Schema: json.RawMessage(`{"type":"object"}`), MaxTokens: 160}
}

func TestReadyAndModelIdentity(t *testing.T) {
	for _, scenario := range []struct {
		name, health, models string
		status               int
		ready, wantError     bool
	}{
		{"ready", `{"status":"ok"}`, `{"data":[{"id":"lfm3:1.7b"}]}`, 200, true, false},
		{"loading", `{"error":{"message":"Loading model"}}`, "", 503, false, false},
		{"wrong_model", `{"status":"ok"}`, `{"data":[{"id":"other"}]}`, 200, false, true},
		{"wrong_service", `{"status":"ready"}`, "", 200, false, true},
		{"malformed", "not json", "", 200, false, true},
		{"http_error", "", "", 401, false, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Error("la sonde ne doit pas générer de texte")
				}
				if r.URL.Path == "/health" {
					w.WriteHeader(scenario.status)
					_, _ = io.WriteString(w, scenario.health)
					return
				}
				if r.URL.Path != "/v1/models" {
					t.Errorf("sonde inattendue : %s", r.URL.Path)
				}
				_, _ = io.WriteString(w, scenario.models)
			}))
			defer server.Close()
			c, err := llamacpp.New(server.URL, "lfm3:1.7b", time.Second)
			if err != nil {
				t.Fatal(err)
			}
			ready, err := c.Ready(context.Background())
			if ready != scenario.ready || (err != nil) != scenario.wantError {
				t.Fatalf("ready=%v err=%v", ready, err)
			}
		})
	}
}

func TestBadResponsesAndTimeout(t *testing.T) {
	for _, body := range []string{
		`{}`, `not json`,
		`{"choices":[{"finish_reason":"length","message":{"role":"assistant","content":"{}"}}]}`,
		`{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":""}}]}`,
		`{"choices":[{"finish_reason":"stop","message":{"role":"user","content":"{}"}}]}`,
		`{"choices":[{},{}]}`, `{"error":{"message":"oops"}}`, strings.Repeat("x", (1<<20)+1),
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) }))
		c, err := llamacpp.New(server.URL, "model", time.Second)
		if err != nil {
			t.Fatal(err)
		}
		_, err = c.Complete(context.Background(), request())
		server.Close()
		if err == nil {
			t.Fatal("réponse invalide acceptée")
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	}))
	defer server.Close()
	c, err := llamacpp.New(server.URL, "model", 30*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Complete(context.Background(), request()); err == nil {
		t.Fatal("délai ignoré")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Ready(ctx); err == nil {
		t.Fatal("annulation ignorée")
	}
}
