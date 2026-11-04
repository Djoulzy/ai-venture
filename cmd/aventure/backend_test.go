package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"ai-venture/internal/engine"
	"ai-venture/internal/language"
	"ai-venture/internal/terminal"
	"ai-venture/scenarios"
)

func TestMain(m *testing.M) {
	if provider := os.Getenv("AI_VENTURE_FAKE_INFERENCE"); provider != "" {
		runFakeService(provider)
		return
	}
	os.Exit(m.Run())
}

// This subprocess accepts the exact command-line/environment contract of each
// real server, so tests exercise production launch, HTTP, gameplay and cleanup.
func runFakeService(provider string) {
	address := os.Getenv("OLLAMA_HOST")
	model, gguf := "lfm3:1.7b", ""
	if provider == "llamacpp" {
		args := map[string]string{}
		for i := 1; i < len(os.Args)-1; i++ {
			if strings.HasPrefix(os.Args[i], "--") {
				args[os.Args[i]] = os.Args[i+1]
			}
		}
		if args["--model"] == "" || args["--alias"] == "" {
			os.Exit(3)
		}
		model, gguf = args["--alias"], args["--model"]
		address = net.JoinHostPort(args["--host"], args["--port"])
	} else if len(os.Args) != 2 || os.Args[1] != "serve" {
		os.Exit(4)
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(5)
	}
	server := &http.Server{Handler: fakeHandler(provider, model, gguf)}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() { <-signals; _ = server.Close() }()
	_ = server.Serve(listener)
}

func fakeHandler(provider, model, gguf string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []any{map[string]string{"name": model}}})
			return
		case "/health":
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
			return
		case "/v1/models":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]string{"id": model}}})
			return
		case "/props":
			_ = json.NewEncoder(w).Encode(map[string]string{"model_path": gguf})
			return
		}
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Messages) != 2 || body.Model != model {
			w.WriteHeader(400)
			return
		}
		var input struct {
			Command string `json:"command"`
		}
		_ = json.Unmarshal([]byte(body.Messages[1].Content), &input)
		content := `{"text":"The wind whistles through the mooring ropes."}`
		if input.Command != "" {
			action := engine.Action{Kind: "unknown"}
			switch input.Command {
			case "I pick up the key.":
				action = engine.Action{Kind: "take", Target: "key"}
			case "I head towards the lighthouse.":
				action = engine.Action{Kind: "move", Target: "north"}
			case "I unlock the door with the key.":
				action = engine.Action{Kind: "use", Target: "door", Tool: "key"}
			}
			data, _ := json.Marshal(action)
			content = string(data)
		}
		message := map[string]string{"role": "assistant", "content": content}
		if provider == "ollama" {
			_ = json.NewEncoder(w).Encode(map[string]any{"done": true, "message": message})
		} else {
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": message, "finish_reason": "stop"}}})
		}
	})
}

func testConfig(t *testing.T, provider, address string) backendConfig {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	gguf := filepath.Join(t.TempDir(), "model.gguf")
	if err := os.WriteFile(gguf, []byte("GGUF test fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	return backendConfig{provider: provider, model: "lfm3:1.7b", ollamaURL: address, llamaURL: address,
		ollamaBinary: binary, llamaBinary: binary, gguf: gguf, llamaContext: 4096, autoStart: true, timeout: time.Second, startupTimeout: 10 * time.Second}
}

func TestBackendLaunchThroughGameplayAndShutdown(t *testing.T) {
	t.Setenv("OLLAMA_MODEL", "")
	for _, provider := range []string{"ollama", "llamacpp"} {
		t.Run(provider, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			address := "http://" + listener.Addr().String()
			_ = listener.Close()
			t.Setenv("AI_VENTURE_FAKE_INFERENCE", provider)
			cfg := testConfig(t, provider, address)
			cfg.model, err = resolveModel(provider, "", cfg.gguf)
			if err != nil {
				t.Fatal(err)
			}
			client, service, err := startBackend(context.Background(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = service.Close() })
			if !service.Owned() {
				t.Fatal("service non lancé")
			}
			world, err := (scenarios.Demo{}).Load(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			e, err := engine.New(world)
			if err != nil {
				t.Fatal(err)
			}
			llm := language.LLM{Client: client}
			var output bytes.Buffer
			session := terminal.Session{Engine: e, Interpreter: llm, Narrator: llm,
				Input:  strings.NewReader("I pick up the key.\nI head towards the lighthouse.\nI unlock the door with the key.\nsave\nquit\n"),
				Output: &output, SavePath: filepath.Join(t.TempDir(), "save.json"), Timeout: time.Second}
			if err := session.Run(context.Background()); err != nil {
				t.Fatal(err)
			}
			if e.Snapshot().Turn != 3 || !e.Snapshot().Flags["door_open"] || !strings.Contains(output.String(), "Atmosphere:") {
				t.Fatalf("partie invalide : %s", output.String())
			}
			if err := service.Close(); err != nil {
				t.Fatal(err)
			}
			conn, err := net.DialTimeout("tcp", strings.TrimPrefix(address, "http://"), 100*time.Millisecond)
			if err == nil {
				_ = conn.Close()
				t.Fatal("service encore en écoute après fermeture")
			}
		})
	}
}

func TestBackendPreservesExistingService(t *testing.T) {
	for _, provider := range []string{"ollama", "llamacpp"} {
		t.Run(provider, func(t *testing.T) {
			server := httptest.NewServer(fakeHandler(provider, "lfm3:1.7b", ""))
			defer server.Close()
			cfg := testConfig(t, provider, server.URL)
			// A reused service requires neither a local executable nor a GGUF file.
			cfg.ollamaBinary, cfg.llamaBinary, cfg.gguf = "/absent", "/absent", ""
			_, service, err := startBackend(context.Background(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			if service.Owned() {
				t.Fatal("service existant déclaré possédé")
			}
			if err := service.Close(); err != nil {
				t.Fatal(err)
			}
			response, err := http.Get(server.URL + "/health")
			if err != nil {
				t.Fatal("service existant arrêté")
			}
			_ = response.Body.Close()
		})
	}
}

func TestModelDefaultsFollowProviderAndGGUF(t *testing.T) {
	t.Setenv("OLLAMA_MODEL", "ollama-only")
	for _, tc := range []struct {
		provider, model, gguf, want string
		wantError                   bool
	}{
		{"ollama", "", "", "ollama-only", false},
		{"ollama", "custom", "", "custom", false},
		{"llamacpp", "", "./models/llama-3.2-3b-instruct-q4_k_m.gguf", "llama-3.2-3b-instruct-q4_k_m.gguf", false},
		{"llamacpp", "", "./models/LFM3-1.7B-Q8_0.gguf", "LFM3-1.7B-Q8_0.gguf", false},
		{"llamacpp", "explicit-alias", "./models/llama.gguf", "explicit-alias", false},
		{"llamacpp", "server-alias", "", "server-alias", false},
		{"llamacpp", "", "", "", true},
	} {
		got, err := resolveModel(tc.provider, tc.model, tc.gguf)
		if got != tc.want || (err != nil) != tc.wantError {
			t.Fatalf("%+v: got %q, %v", tc, got, err)
		}
	}
	t.Setenv("OLLAMA_MODEL", "")
	if got, err := resolveModel("ollama", "", ""); err != nil || got != "lfm3:1.7b" {
		t.Fatalf("Ollama default: %q, %v", got, err)
	}
}

func TestBackendChecksGGUFOnExistingServerWithSameAlias(t *testing.T) {
	for _, matching := range []bool{true, false} {
		t.Run(fmt.Sprint(matching), func(t *testing.T) {
			cfg := testConfig(t, "llamacpp", "")
			reported := cfg.gguf
			if !matching {
				// Same alias AND filename, but a different model file.
				reported = filepath.Join(t.TempDir(), filepath.Base(cfg.gguf))
			}
			server := httptest.NewServer(fakeHandler("llamacpp", cfg.model, reported))
			defer server.Close()
			cfg.llamaURL, cfg.llamaBinary = server.URL, "/must-not-start"
			client, service, err := startBackend(context.Background(), cfg)
			if matching {
				if err != nil || service.Owned() {
					t.Fatalf("matching server was not reused: %v", err)
				}
				if err := service.Close(); err != nil {
					t.Fatal(err)
				}
			} else if err == nil || client != nil || service != nil || !strings.Contains(err.Error(), "loaded GGUF") {
				t.Fatalf("different GGUF accepted or wrong failure: %v", err)
			}
			response, err := http.Get(server.URL + "/health")
			if err != nil {
				t.Fatal("existing server was stopped")
			}
			_ = response.Body.Close()
		})
	}
}
