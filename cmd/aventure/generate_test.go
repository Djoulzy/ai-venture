package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"ai-venture/internal/catalogue"
	"ai-venture/internal/engine"
	"ai-venture/internal/generation"
	"ai-venture/internal/locale"
	"ai-venture/internal/terminal"
	"ai-venture/internal/world"
)

func TestGenerateScenarioThroughBothHTTPProviders(t *testing.T) {
	for _, provider := range []string{"ollama", "llamacpp"} {
		for _, lang := range []locale.Language{locale.French, locale.English} {
			t.Run(provider+"/"+string(lang), func(t *testing.T) {
				ctx := context.Background()
				dir := t.TempDir()
				database := filepath.Join(dir, "catalogue.sqlite")
				s, err := catalogue.Open(ctx, database, true)
				if err != nil {
					t.Fatal(err)
				}
				_, err = s.Import(ctx, catalogue.Batch{Entries: []catalogue.Entry{
					{ID: "library", Kind: "location", Language: string(lang), Title: "Library", Text: "A quiet room."},
					{ID: "book", Kind: "item", Language: string(lang), Title: "Book", Text: "A book of rites."},
				}}, false)
				if err != nil {
					t.Fatal(err)
				}
				if err := s.Close(); err != nil {
					t.Fatal(err)
				}
				p := generation.Plan{Title: "The rite", Introduction: "Read the book.", Locations: []string{"library"}, Entities: []generation.Placement{{Entry: "book", Location: "library"}}, Steps: []generation.Step{{Tool: "book", Target: "", Location: "library", Message: "The rite is complete."}}, Situation: ""}
				if lang == locale.French {
					p.Title, p.Introduction, p.Steps[0].Message = "Le rite", "Lisez le livre.", "Le rite est accompli."
				}
				raw, _ := json.Marshal(p)
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != "/api/chat" && r.URL.Path != "/v1/chat/completions" {
						fakeHandler(provider, "test-model", "").ServeHTTP(w, r)
						return
					}
					calls.Add(1)
					var request map[string]json.RawMessage
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
						w.WriteHeader(400)
						return
					}
					field := "format"
					if provider == "llamacpp" {
						field = "response_format"
					}
					if !strings.Contains(string(request[field]), "library") {
						t.Error("missing catalogue-constrained JSON schema")
					}
					w.Header().Set("Content-Type", "application/json")
					message := map[string]string{"role": "assistant", "content": string(raw)}
					if provider == "ollama" {
						_ = json.NewEncoder(w).Encode(map[string]any{"done": true, "message": message})
					} else {
						_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": message, "finish_reason": "stop"}}})
					}
				}))
				defer server.Close()
				cfg := testConfig(t, provider, server.URL)
				cfg.model, cfg.gguf, cfg.autoStart = "test-model", "", false
				var output bytes.Buffer
				path := filepath.Join(dir, "new.json")
				if err := generateScenario(ctx, lang, "book", path, database, 42, cfg, terminal.NewPrinter(&output, terminal.ColorNever)); err != nil {
					t.Fatal(err)
				}
				if calls.Load() != 1 {
					t.Fatalf("calls: %d", calls.Load())
				}
				w, err := (world.FileSource{Path: path}).Load(ctx)
				if err != nil {
					t.Fatal(err)
				}
				e, err := engine.NewWithLanguage(w, lang)
				if err != nil {
					t.Fatal(err)
				}
				for _, a := range []engine.Action{{Kind: "take", Target: "book"}, {Kind: "use", Tool: "book"}} {
					if _, err := e.Apply(a); err != nil {
						t.Fatal(err)
					}
				}
				if !e.View().Completed || w.Provenance.Seed != 42 {
					t.Fatal("published quest not playable")
				}
				response, err := http.Get(server.URL + "/health")
				if err != nil {
					t.Fatal("existing inference service stopped", err)
				}
				response.Body.Close()
				if _, err := os.Stat(path); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
