package language_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"ai-venture/internal/engine"
	"ai-venture/internal/language"
	"ai-venture/internal/locale"
)

func TestInferenceReferencesFollowVisibleView(t *testing.T) {
	for _, lang := range []locale.Language{locale.French, locale.English} {
		t.Run(string(lang), func(t *testing.T) {
			fake := &fakeClient{content: `{"action":"inventory","target":"","tool":""}`}
			llm := language.LLM{Client: fake, Language: lang}
			views := []engine.View{
				{Location: "room", Exits: []engine.VisibleExit{{Direction: "down", Destination: "Secret chamber", Blocked: true}}, Items: []engine.VisibleItem{{ID: "book"}}, Inventory: []engine.VisibleItem{{ID: "key"}}},
				{Location: "other", Items: []engine.VisibleItem{}, Inventory: []engine.VisibleItem{{ID: "key"}}},
				{},
			}
			items := [][]string{{"book", "key"}, {"key"}, nil}
			observations := [][]string{{"", "book", "key", "room"}, {"", "key", "other"}, {""}}
			for i, view := range views {
				before, err := json.Marshal(view)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := llm.Interpret(context.Background(), "inventory", view); err != nil {
					t.Fatal(err)
				}
				var schema struct {
					AnyOf []struct {
						Properties map[string]struct {
							Enum []string `json:"enum"`
						} `json:"properties"`
					} `json:"anyOf"`
				}
				if err := json.Unmarshal(fake.request.Schema, &schema); err != nil {
					t.Fatal(err)
				}
				seen := map[string]bool{}
				for _, branch := range schema.AnyOf {
					kinds := branch.Properties["action"].Enum
					if len(kinds) != 1 || seen[kinds[0]] {
						t.Fatal("ambiguous action branch", kinds)
					}
					kind := kinds[0]
					seen[kind] = true
					wantTargets, wantTools := []string{""}, []string{""}
					switch kind {
					case "observe":
						wantTargets = observations[i]
					case "move":
						wantTargets = []string{"down"}
					case "take", "drop":
						wantTargets = items[i]
					case "use":
						wantTools = items[i]
						wantTargets = append([]string{""}, items[i]...)
					case "inventory", "unknown":
					default:
						t.Fatal("unexpected action", kind)
					}
					if !reflect.DeepEqual(branch.Properties["target"].Enum, wantTargets) || !reflect.DeepEqual(branch.Properties["tool"].Enum, wantTools) {
						t.Fatalf("wrong parameters for %s: %s", kind, fake.request.Schema)
					}
					// Every permitted combination also satisfies the Go shape validator.
					for _, target := range wantTargets {
						for _, tool := range wantTools {
							if err := (engine.Action{Kind: kind, Target: target, Tool: tool}).Validate(); err != nil {
								t.Fatalf("schema diverges from Go validation: %v", err)
							}
						}
					}
				}
				wantKinds := map[string]bool{"observe": true, "inventory": true, "unknown": true}
				if len(items[i]) > 0 {
					wantKinds["take"], wantKinds["drop"], wantKinds["use"] = true, true, true
				}
				if i == 0 {
					wantKinds["move"] = true // Blocked exits remain proposals, validated by Go.
				}
				if !reflect.DeepEqual(seen, wantKinds) {
					t.Fatal("wrong visible actions", seen, wantKinds)
				}
				after, err := json.Marshal(view)
				if err != nil || string(before) != string(after) {
					t.Fatal("schema construction mutated the view", err)
				}
			}
		})
	}
}
