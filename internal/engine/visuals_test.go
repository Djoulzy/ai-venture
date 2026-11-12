package engine_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"ai-venture/internal/engine"
	"ai-venture/internal/world"
	"ai-venture/scenarios"
)

func TestVisualLayersFollowValidatedStateAndAreNeverInLLMPayload(t *testing.T) {
	w, err := (scenarios.Demo{}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	w.Visuals = []world.VisualLayer{
		{Asset: "background", Location: "quay", Width: 100, Height: 100},
		{Asset: "key_sprite", Location: "quay", Item: "key", Width: 10, Height: 10},
		{Asset: "hidden_monster", Location: "quay", Width: 10, Height: 10, Conditions: []world.Condition{{Flag: "door_open", Value: true}}},
	}
	e, err := engine.New(w)
	if err != nil {
		t.Fatal(err)
	}
	before := e.Snapshot()
	view := e.View()
	if len(view.Visuals) != 2 {
		t.Fatal(view.Visuals)
	}
	data, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "sprite") || strings.Contains(string(data), "visual") || strings.Contains(string(data), "hidden_monster") {
		t.Fatal("visual asset IDs leaked to inference")
	}
	view.Visuals[0].Asset = "changed"
	if e.View().Visuals[0].Asset != "background" {
		t.Fatal("view mutation changed scenario")
	}
	if _, err := e.Apply(engine.Action{Kind: "take", Target: "key"}); err != nil {
		t.Fatal(err)
	}
	if len(e.View().Visuals) != 1 {
		t.Fatal("carried key still rendered")
	}
	if err := e.Restore(before); err != nil {
		t.Fatal(err)
	}
	if len(e.View().Visuals) != 2 {
		t.Fatal("restored scene did not include key")
	}
	for _, a := range []engine.Action{{Kind: "take", Target: "key"}, {Kind: "move", Target: "north"}, {Kind: "use", Tool: "key", Target: "door"}, {Kind: "move", Target: "south"}} {
		if _, err := e.Apply(a); err != nil {
			t.Fatal(err)
		}
	}
	if got := e.View().Visuals; len(got) != 2 || got[1].Asset != "hidden_monster" {
		t.Fatal(got)
	}
}
