package world_test

import (
	"context"
	"strings"
	"testing"

	"ai-venture/internal/world"
	"ai-venture/scenarios"
)

func TestDocumentDiscoveryDefinitionsAreValidated(t *testing.T) {
	original, err := (scenarios.Demo{}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*world.Definition){
		"id":        func(w *world.Definition) { w.Documents[0].ID = "bad id" },
		"duplicate": func(w *world.Definition) { w.Documents = append(w.Documents, w.Documents[0]) },
		"language":  func(w *world.Definition) { w.Documents[0].Language = "de" },
		"location":  func(w *world.Definition) { w.Documents[0].Location = "missing" },
		"item":      func(w *world.Definition) { w.Documents[0].Item = "missing" },
		"condition": func(w *world.Definition) {
			w.Documents[0].Conditions = []world.Condition{{Flag: "missing", Value: true}}
		},
		"empty": func(w *world.Definition) { w.Documents[0].Text = " " },
		"large": func(w *world.Definition) { w.Documents[0].Text = strings.Repeat("é", 2001) },
	} {
		t.Run(name, func(t *testing.T) {
			w := world.Clone(original)
			mutate(&w)
			if err := world.Validate(w); err == nil {
				t.Fatal("invalid document accepted")
			}
		})
	}
}
