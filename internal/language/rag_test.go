package language_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"ai-venture/internal/engine"
	"ai-venture/internal/language"
	"ai-venture/internal/locale"
	"ai-venture/internal/persistence"
	"ai-venture/internal/retrieval"
	ragstore "ai-venture/internal/retrieval/sqlite"
	"ai-venture/internal/world"
	"ai-venture/scenarios"
)

type recordingSearch struct {
	query retrieval.Query
	hits  []retrieval.Document
	err   error
}

func (s *recordingSearch) Search(_ context.Context, q retrieval.Query) ([]retrieval.Document, error) {
	s.query = q
	return s.hits, s.err
}

func TestRAGOnlyUsesEngineApprovedText(t *testing.T) {
	w, err := (scenarios.Demo{}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	e, err := engine.New(w)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeClient{content: `{"text":"You stand on the quay."}`}
	search := &recordingSearch{hits: []retrieval.Document{
		{ID: "lore:island_history", Text: "INJECTED TEXT", Kind: "event", Turn: 999},
		{ID: "lore:stair_inscription", Text: "SECRET"},
		{ID: "lore:island_history", Text: "DUPLICATE"},
	}}
	n := language.RAGNarrator{LLM: language.LLM{Client: f}, Engine: e, Search: search}
	before := e.Snapshot()
	if _, err := n.Narrate(context.Background(), engine.Result{View: e.View(), Message: e.View().Description}); err != nil {
		t.Fatal(err)
	}
	if search.query.Scenario != e.Fingerprint() || search.query.Language != "en" {
		t.Fatal("scope missing")
	}
	for _, doc := range search.query.Documents {
		if doc.ID == "lore:stair_inscription" || doc.ID == "lore:keeper_workshop" {
			t.Fatal("undiscovered text sent to retrieval")
		}
	}
	var payload struct {
		Memory []retrieval.Document `json:"memory"`
	}
	if err := json.Unmarshal([]byte(f.request.Messages[1].Content), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Memory) != 1 || payload.Memory[0].Kind != "lore" || payload.Memory[0].Text != w.Documents[0].Text {
		t.Fatalf("index became authority: %+v", payload.Memory)
	}
	for _, secret := range []string{"INJECTED", "SECRET", "DUPLICATE", "door_open", "conditions"} {
		if strings.Contains(f.request.Messages[1].Content, secret) {
			t.Fatalf("leaked %s", secret)
		}
	}
	if !reflect.DeepEqual(before, e.Snapshot()) {
		t.Fatal("narration changed state")
	}
}

func TestRAGSelectsFrenchAndRecoversFromSearchFailure(t *testing.T) {
	w, err := (scenarios.Demo{Language: locale.French}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	w.Documents = append(w.Documents, world.Document{ID: "english_only", Language: "en", Title: "ENGLISH ONLY", Text: "ENGLISH ONLY"})
	e, err := engine.NewWithLanguage(w, locale.French)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeClient{content: `{"text":"Vous êtes sur le quai."}`}
	search := &recordingSearch{err: errors.New("index unavailable")}
	notified := false
	n := language.RAGNarrator{LLM: language.LLM{Client: f, Language: locale.French}, Engine: e, Search: search, OnError: func(error) { notified = true }}
	if _, err := n.Narrate(context.Background(), engine.Result{View: e.View(), Message: e.View().Description}); err != nil {
		t.Fatal(err)
	}
	if !notified || search.query.Language != "fr" || strings.Contains(f.request.Messages[1].Content, `"memory"`) {
		t.Fatal("search failure did not fall back")
	}
	for _, doc := range search.query.Documents {
		if doc.ID == "lore:english_only" {
			t.Fatal("wrong language indexed")
		}
	}
	if !strings.Contains(f.request.Messages[0].Content, "Écris en français") {
		t.Fatal("wrong narration language")
	}
}

func TestRAGSaveReloadDropsFutureKnowledgeAndJournal(t *testing.T) {
	w, err := (scenarios.Demo{}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	e, err := engine.New(w)
	if err != nil {
		t.Fatal(err)
	}
	store, err := ragstore.Open(context.Background(), filepath.Join(t.TempDir(), "rag.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	f := &fakeClient{content: `{"text":"You wait."}`}
	n := language.RAGNarrator{LLM: language.LLM{Client: f}, Engine: e, Search: store}
	apply := func(kind, target, tool string) {
		t.Helper()
		r, err := e.Apply(engine.Action{Kind: kind, Target: target, Tool: tool})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = n.Narrate(context.Background(), r); err != nil {
			t.Fatal(err)
		}
	}
	apply("take", "key", "")
	path := filepath.Join(t.TempDir(), "save.json")
	if err := persistence.Save(path, e); err != nil {
		t.Fatal(err)
	}
	apply("move", "north", "")
	apply("use", "door", "key")
	if err := persistence.Load(path, e); err != nil {
		t.Fatal(err)
	}
	apply("move", "east", "")
	var payload struct {
		Memory []retrieval.Document `json:"memory"`
	}
	if err := json.Unmarshal([]byte(f.request.Messages[1].Content), &payload); err != nil {
		t.Fatal(err)
	}
	for _, doc := range payload.Memory {
		if doc.ID == "lore:stair_inscription" || doc.ID == "event:3" || strings.Contains(doc.Text, "door opens") || strings.Contains(doc.Text, "lighthouse hall") {
			t.Fatalf("future branch leaked: %+v", doc)
		}
	}
	if e.Snapshot().Flags["door_open"] || e.Snapshot().Turn != 2 {
		t.Fatal("reload mutated the wrong branch")
	}
}
