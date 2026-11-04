package llamacpp_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
				Type       string          `json:"type"`
				Schema     json.RawMessage `json:"schema"`
				JSONSchema struct {
					Name   string         `json:"name"`
					Strict bool           `json:"strict"`
					Schema map[string]any `json:"schema"`
				} `json:"json_schema"`
			} `json:"response_format"`
			ChatTemplateKwargs map[string]bool `json:"chat_template_kwargs"`
			ReasoningEffort    string          `json:"reasoning_effort"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request.Model != "lfm3:1.7b" || request.Stream == nil || *request.Stream || request.MaxTokens != 160 || len(request.Messages) != 1 || request.ResponseFormat.Type != "json_schema" || request.ResponseFormat.JSONSchema.Schema["type"] != "object" || request.ReasoningEffort != "none" {
			t.Errorf("corps incorrect : %+v", request)
		}
		if len(request.ResponseFormat.Schema) != 0 || request.ResponseFormat.JSONSchema.Name != "game_response" || !request.ResponseFormat.JSONSchema.Strict {
			t.Error("schema must be nested under response_format.json_schema, not silently ignored")
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

func TestVerifyModelFile(t *testing.T) {
	want := filepath.Join(t.TempDir(), "llama.gguf")
	if err := os.WriteFile(want, []byte("test fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "alias.gguf")
	if err := os.Symlink(want, link); err != nil {
		t.Fatal(err)
	}
	props := func(path string) string {
		body, _ := json.Marshal(map[string]string{"model_path": path})
		return string(body)
	}
	for _, tc := range []struct {
		name, body string
		status     int
		wantError  bool
	}{
		{"matching", props(want), 200, false},
		{"symlink", props(link), 200, false},
		{"different_directory", props(filepath.Join(t.TempDir(), "llama.gguf")), 200, true},
		{"different_model", props(filepath.Join(filepath.Dir(want), "lfm3.gguf")), 200, true},
		{"relative_path", props("llama.gguf"), 200, true},
		{"missing_path", `{}`, 200, true},
		{"malformed", `invalid`, 200, true},
		{"unsupported", `{}`, 404, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Path != "/props" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			c, err := llamacpp.New(server.URL, "same-alias", time.Second)
			if err != nil {
				t.Fatal(err)
			}
			if err := c.VerifyModelFile(context.Background(), want); (err != nil) != tc.wantError {
				t.Fatalf("verification error: %v", err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := c.VerifyModelFile(ctx, want); err == nil {
				t.Fatal("cancellation ignored")
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
