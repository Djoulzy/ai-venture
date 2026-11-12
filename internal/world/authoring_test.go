package world_test

import (
	"context"
	"testing"

	"ai-venture/internal/world"
	"ai-venture/scenarios"
)

func TestAuthoringReferencesAndVisualBounds(t *testing.T) {
	base, err := (scenarios.Demo{}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for name, layer := range map[string]world.VisualLayer{
		"valid":            {Asset: "background", Location: "quay", Width: 100, Height: 100},
		"unknown_location": {Asset: "background", Location: "missing", Width: 100, Height: 100},
		"unknown_item":     {Asset: "background", Location: "quay", Item: "missing", Width: 100, Height: 100},
		"overflow":         {Asset: "background", Location: "quay", X: 1, Width: 100, Height: 100},
		"negative":         {Asset: "background", Location: "quay", X: -1, Width: 100, Height: 100},
		"zero":             {Asset: "background", Location: "quay", Height: 100},
		"flag":             {Asset: "background", Location: "quay", Width: 100, Height: 100, Conditions: []world.Condition{{Flag: "missing"}}},
	} {
		t.Run(name, func(t *testing.T) {
			w := world.Clone(base)
			w.Visuals = []world.VisualLayer{layer}
			err := world.Validate(w)
			if (err == nil) != (name == "valid") {
				t.Fatalf("unexpected validation result: %v", err)
			}
		})
	}
	w := world.Clone(base)
	w.CatalogueDocuments = []world.DocumentRef{{ID: base.Documents[0].ID, Entry: "keeper"}}
	if err := world.Validate(w); err == nil {
		t.Fatal("duplicate regular/catalogue document ID accepted")
	}
	w = world.Clone(base)
	for range 33 {
		w.Visuals = append(w.Visuals, world.VisualLayer{Asset: "background", Location: "quay", Width: 100, Height: 100})
	}
	if err := world.Validate(w); err == nil {
		t.Fatal("too many layers accepted")
	}
}
