package world_test

import (
	"context"
	"strings"
	"testing"

	"ai-venture/internal/world"
	"ai-venture/scenarios"
)

func TestRejectInvalidScenarios(t *testing.T) {
	w, err := (scenarios.Demo{}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*world.Definition){
		"version":   func(w *world.Definition) { w.Version = 99 },
		"start":     func(w *world.Definition) { w.Start = "absent" },
		"exit":      func(w *world.Definition) { w.Locations["quay"].Exits["north"] = world.Exit{To: "absent"} },
		"condition": func(w *world.Definition) { w.Rules[0].Conditions[0].Flag = "absent" },
		"effect":    func(w *world.Definition) { w.Rules[0].Effects["absent"] = true },
		"item":      func(w *world.Definition) { delete(w.Items, "key") },
		"shared_id": func(w *world.Definition) { w.Items["quay"] = w.Items["key"] },
		"ambiguous": func(w *world.Definition) { rule := w.Rules[0]; rule.ID = "autre"; w.Rules = append(w.Rules, rule) },
		"disconnected": func(w *world.Definition) {
			w.Locations["ile"] = world.Location{Name: "Île", Description: "Une île isolée."}
		},
		"goal": func(w *world.Definition) { w.Goal = nil },
	} {
		t.Run(name, func(t *testing.T) {
			copy := world.Clone(w)
			mutate(&copy)
			if world.Validate(copy) == nil {
				t.Fatal("scénario invalide accepté")
			}
		})
	}
}

func TestStrictScenarioLoading(t *testing.T) {
	for _, text := range []string{`{"version":1,"version":1}`, `{"surprise":true}`, `null`, `{}`, `{} {}`} {
		if _, err := world.Load(strings.NewReader(text)); err == nil {
			t.Fatalf("JSON accepté : %s", text)
		}
	}
}
