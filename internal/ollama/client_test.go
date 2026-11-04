package ollama_test

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
	"ai-venture/internal/ollama"
)

func request() inference.Request {
	return inference.Request{Messages: []inference.Message{{Role: "user", Content: "look"}}, Schema: json.RawMessage(`{"type":"object"}`), MaxTokens: 160}
}

func TestChatProtocol(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/chat" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("requête incorrecte")
		}
		var body struct {
			Model    string              `json:"model"`
			Stream   *bool               `json:"stream"`
			Think    *bool               `json:"think"`
			Format   map[string]any      `json:"format"`
			Messages []inference.Message `json:"messages"`
			Options  map[string]any      `json:"options"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Model != "lfm3:1.7b" || body.Stream == nil || *body.Stream || body.Think == nil || *body.Think || body.Format["type"] != "object" || body.Options["num_predict"] != float64(160) || len(body.Messages) != 1 {
			t.Errorf("corps incorrect : %+v", body)
		}
		_, _ = io.WriteString(w, `{"done":true,"done_reason":"stop","message":{"role":"assistant","content":"{\"action\":\"observe\",\"target\":\"\",\"tool\":\"\"}"},"eval_count":20}`)
	}))
	defer server.Close()
	c, err := ollama.New(server.URL, "lfm3:1.7b", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	content, err := c.Complete(context.Background(), request())
	if err != nil || !strings.Contains(content, `"action":"observe"`) {
		t.Fatalf("%s %v", content, err)
	}
}

func TestHTTPFailures(t *testing.T) {
	for name, body := range map[string]string{
		"malformed":  `not json`,
		"empty":      `{"done":true,"message":{"role":"assistant","content":""}}`,
		"incomplete": `{"done":false,"message":{"role":"assistant","content":"{}"}}`,
		"truncated":  `{"done":true,"done_reason":"length","message":{"role":"assistant","content":"{}"}}`,
		"wrong_role": `{"done":true,"message":{"role":"user","content":"{}"}}`,
		"error":      `{"done":true,"error":"failure","message":{"role":"assistant","content":"{}"}}`,
		"oversize":   strings.Repeat("x", (1<<20)+1),
		"http404":    "404",
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if name == "http404" {
					w.WriteHeader(http.StatusNotFound)
				}
				_, _ = io.WriteString(w, body)
			}))
			defer server.Close()
			c, err := ollama.New(server.URL, "lfm3:1.7b", time.Second)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := c.Complete(context.Background(), request()); err == nil {
				t.Fatal("réponse invalide acceptée")
			}
		})
	}
}

func TestCancellationAndTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Consume the request so the server observes client disconnection.
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	}))
	defer server.Close()
	c, err := ollama.New(server.URL, "lfm3:1.7b", 30*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Complete(context.Background(), request()); err == nil {
		t.Fatal("délai ignoré")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Complete(ctx, request()); err == nil {
		t.Fatal("annulation ignorée")
	}
}

func TestConfigurationValidation(t *testing.T) {
	for _, address := range []string{"", "://", "file:///tmp/ollama", "http://user:pass@localhost:11434", "http://localhost:11434?x=1"} {
		if _, err := ollama.New(address, "model", time.Second); err == nil {
			t.Fatal("URL invalide acceptée")
		}
	}
}

func TestReadyChecksModelWithoutGenerating(t *testing.T) {
	for _, scenario := range []struct {
		name, body, model string
		status            int
		ready, wantError  bool
	}{
		{"present", `{"models":[{"name":"lfm3:1.7b"}]}`, "lfm3:1.7b", 200, true, false},
		{"latest", `{"models":[{"model":"lfm3:latest"}]}`, "lfm3", 200, true, false},
		{"missing", `{"models":[]}`, "lfm3:1.7b", 200, false, true},
		{"wrong_model", `{"models":[{"name":"other"}]}`, "lfm3:1.7b", 200, false, true},
		{"wrong_service", `{}`, "lfm3", 200, false, true},
		{"null", `{"models":null}`, "lfm3", 200, false, true},
		{"malformed", "bad", "lfm3", 200, false, true},
		{"loading", "", "lfm3", 503, false, false},
		{"http_error", "", "lfm3", 404, false, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Path != "/api/tags" {
					t.Errorf("sonde inattendue : %s %s", r.Method, r.URL.Path)
				}
				w.WriteHeader(scenario.status)
				_, _ = io.WriteString(w, scenario.body)
			}))
			defer server.Close()
			c, err := ollama.New(server.URL, scenario.model, time.Second)
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
