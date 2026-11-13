package language_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"ai-venture/internal/engine"
	"ai-venture/internal/language"
	"ai-venture/internal/locale"
	"ai-venture/internal/world"
)

func TestReadingUsesSharedRulesWithAndWithoutInference(t *testing.T) {
	for _, tc := range []struct {
		lang     locale.Language
		name     string
		commands []string
	}{
		{locale.English, "the clay codex", []string{"read manuscript", "Read the clay codex.", "study the clay codex"}},
		{locale.French, "le codex d’argile", []string{"lire manuscript", "Lis le codex d'argile.", "lisez le codex d’argile", "étudier le codex d'argile", "étudie manuscript"}},
	} {
		t.Run(string(tc.lang), func(t *testing.T) {
			w := world.Definition{
				Version: 1, ID: "reading_fixture", Title: "Reading", Introduction: "Study the manuscript.", Start: "room",
				Locations: map[string]world.Location{"room": {Name: "Room", Description: "A room.", Exits: map[string]world.Exit{}}},
				Items:     map[string]world.Item{"manuscript": {Name: tc.name, Description: "An annotated text.", Portable: true, Location: "room"}},
				Flags:     map[string]bool{"understood": false},
				Rules: []world.UseRule{{
					ID: "study", Tool: "manuscript", Location: "room",
					Conditions: []world.Condition{{Flag: "understood", Value: false}}, Effects: map[string]bool{"understood": true},
					Message: "The words become clear.", BlockedMessage: "Already understood.",
				}},
				Goal: []world.Condition{{Flag: "understood", Value: true}},
			}
			for _, command := range tc.commands {
				e, err := engine.NewWithLanguage(w, tc.lang)
				if err != nil {
					t.Fatal(err)
				}
				fake := &fakeClient{content: `{"action":"use","target":"","tool":"manuscript"}`}
				local := language.Offline{Language: tc.lang}
				llm := language.LLM{Client: fake, Language: tc.lang}
				a, err := local.Interpret(context.Background(), command, e.View())
				if err != nil {
					t.Fatal(command, err)
				}
				b, err := llm.Interpret(context.Background(), command, e.View())
				if err != nil || a != b || a != (engine.Action{Kind: "use", Tool: "manuscript"}) {
					t.Fatal("different reading semantics", command, a, b, err)
				}
				assertModelCommand(t, fake, command)
				before := e.Snapshot()
				if _, err := e.Apply(a); err == nil || !reflect.DeepEqual(before, e.Snapshot()) {
					t.Fatal("reading bypassed possession")
				}
				if _, err := e.Apply(engine.Action{Kind: "take", Target: "manuscript"}); err != nil {
					t.Fatal(err)
				}
				if _, err := e.Apply(a); err != nil || !e.View().Completed {
					t.Fatal("reading did not apply authored effect", err)
				}
				before = e.Snapshot()
				if _, err := e.Apply(b); err == nil || !reflect.DeepEqual(before, e.Snapshot()) {
					t.Fatal("reading bypassed rule preconditions")
				}
			}
		})
	}
}

func TestReadingNeedsAnUnambiguousObjectAndPreservesFreeText(t *testing.T) {
	for _, tc := range []struct {
		lang         locale.Language
		verb, phrase string
	}{
		{locale.English, "read", "I decipher the marginal notes in the manuscript."},
		{locale.French, "lire", "Je déchiffre les annotations du manuscrit."},
	} {
		view := engine.View{Items: []engine.VisibleItem{{ID: "text_a", Name: "manuscript"}, {ID: "text_b", Name: "manuscript"}}}
		offline := language.Offline{Language: tc.lang}
		for _, command := range []string{tc.verb, tc.verb + " manuscript", tc.verb + " missing", tc.verb + " text_a and take text_b"} {
			if _, err := offline.Interpret(context.Background(), command, view); err == nil {
				t.Fatal("missing/ambiguous/multiple command accepted", command)
			}
		}
		fake := &fakeClient{content: `{"action":"unknown","target":"","tool":""}`}
		llm := language.LLM{Client: fake, Language: tc.lang}
		if action, err := llm.Interpret(context.Background(), tc.verb, view); err != nil || action.Kind != "unknown" {
			t.Fatal("model must decide whether an item can be inferred", action, err)
		}
		assertModelCommand(t, fake, tc.verb)
		view.Items = view.Items[:1]
		if action, err := offline.Interpret(context.Background(), tc.phrase, view); err == nil && action.Kind != "unknown" {
			t.Fatal("fixture should require free-form interpretation")
		}
		fake.content = `{"action":"use","target":"","tool":"text_a"}`
		if action, err := llm.Interpret(context.Background(), tc.phrase, view); err != nil || action != (engine.Action{Kind: "use", Tool: "text_a"}) {
			t.Fatal(err)
		}
		assertModelCommand(t, fake, tc.phrase)
		if !strings.Contains(fake.request.Messages[0].Content, `target=""`) {
			t.Fatal("reading prompt lost")
		}
		cancelled, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := llm.Interpret(cancelled, tc.verb+" text_a", view); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		assertModelCommand(t, fake, tc.phrase) // No further request after cancellation.
	}
}

func assertModelCommand(t *testing.T, fake *fakeClient, command string) {
	t.Helper()
	if len(fake.request.Messages) != 2 {
		t.Fatal("command bypassed inference", command)
	}
	var input struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(fake.request.Messages[1].Content), &input); err != nil || input.Command != command {
		t.Fatal("model did not receive unchanged command", input.Command, command, err)
	}
}
