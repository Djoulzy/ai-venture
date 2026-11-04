package engine_test

import (
	"context"
	"reflect"
	"testing"

	"ai-venture/internal/engine"
	"ai-venture/internal/world"
	"ai-venture/scenarios"
)

func game(t *testing.T) *engine.Engine {
	t.Helper()
	w, err := (scenarios.Demo{}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	e, err := engine.New(w)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func apply(t *testing.T, e *engine.Engine, kind, target, tool string) engine.Result {
	t.Helper()
	r, err := e.Apply(engine.Action{Kind: kind, Target: target, Tool: tool})
	if err != nil {
		t.Fatalf("%s %s %s: %v", kind, target, tool, err)
	}
	return r
}

func denied(t *testing.T, e *engine.Engine, a engine.Action) {
	t.Helper()
	before := e.Snapshot()
	if _, err := e.Apply(a); err == nil {
		t.Fatalf("action acceptée : %+v", a)
	}
	if !reflect.DeepEqual(before, e.Snapshot()) {
		t.Fatal("un refus a modifié l'état")
	}
}

func TestMovementAndPreconditions(t *testing.T) {
	e := game(t)
	denied(t, e, engine.Action{Kind: "move", Target: "lantern_room"})
	denied(t, e, engine.Action{Kind: "take", Target: "battery"})
	apply(t, e, "take", "key", "")
	apply(t, e, "move", "north", "")
	denied(t, e, engine.Action{Kind: "move", Target: "up"})
	denied(t, e, engine.Action{Kind: "take", Target: "door"})
	apply(t, e, "use", "door", "key")
	denied(t, e, engine.Action{Kind: "use", Target: "door", Tool: "key"})
	apply(t, e, "move", "up", "")
	if e.Snapshot().Location != "lantern_room" {
		t.Fatal("déplacement manquant")
	}
	denied(t, e, engine.Action{Kind: "observe", Target: "door"})
}

func TestInventoryAndOwnership(t *testing.T) {
	e := game(t)
	apply(t, e, "take", "key", "")
	denied(t, e, engine.Action{Kind: "take", Target: "key"})
	apply(t, e, "move", "north", "")
	apply(t, e, "drop", "key", "")
	if e.Snapshot().ItemLocations["key"] != "hall" {
		t.Fatal("dépôt dans le mauvais lieu")
	}
	denied(t, e, engine.Action{Kind: "drop", Target: "key"})
	denied(t, e, engine.Action{Kind: "use", Target: "door", Tool: "key"})
	apply(t, e, "take", "key", "")
	apply(t, e, "use", "door", "key")
	if len(e.View().Inventory) != 1 {
		t.Fatal("inventory incohérent")
	}
}

func TestDemoCanBeCompleted(t *testing.T) {
	e := game(t)
	for _, a := range []engine.Action{
		{Kind: "take", Target: "key"}, {Kind: "move", Target: "east"},
		{Kind: "take", Target: "battery"}, {Kind: "move", Target: "west"},
		{Kind: "move", Target: "north"}, {Kind: "use", Tool: "key", Target: "door"},
		{Kind: "move", Target: "up"}, {Kind: "use", Tool: "battery", Target: "beacon"},
	} {
		if _, err := e.Apply(a); err != nil {
			t.Fatal(err)
		}
	}
	if !e.View().Completed || e.Snapshot().Turn != 8 {
		t.Fatal("objectif non atteint")
	}
	denied(t, e, engine.Action{Kind: "use", Tool: "battery", Target: "beacon"})
}

func TestInvalidActionsCannotMutate(t *testing.T) {
	e := game(t)
	for _, a := range []engine.Action{
		{Kind: "teleport", Target: "lantern_room"}, {Kind: "take"},
		{Kind: "inventory", Target: "key"}, {Kind: "observe", Tool: "key"},
		{Kind: "move", Target: "east", Tool: "key"}, {Kind: "use", Target: "door"},
		{Kind: "use", Tool: "ghost"}, {Kind: "unknown"},
		{Kind: "drop", Target: "ghost"}, {Kind: "observe", Target: "ghost"},
	} {
		denied(t, e, a)
	}
}

func TestReadOnlyViewsAndSnapshots(t *testing.T) {
	e := game(t)
	before := e.Snapshot()
	apply(t, e, "observe", "", "")
	apply(t, e, "inventory", "", "")
	copy := e.Snapshot()
	copy.Flags["door_open"] = true
	copy.ItemLocations["key"] = world.Inventory
	v := e.View()
	v.Items[0].Name = "altéré"
	v.Exits[0].Direction = "teleport"
	if !reflect.DeepEqual(before, e.Snapshot()) || e.View().Items[0].Name == "altéré" {
		t.Fatal("référence mutable exposée")
	}
}

func TestEngineOwnsScenarioCopy(t *testing.T) {
	w, err := (scenarios.Demo{}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	e, err := engine.New(w)
	if err != nil {
		t.Fatal(err)
	}
	w.Flags["door_open"] = true
	w.Locations["quay"].Exits["north"] = world.Exit{To: "lantern_room"}
	w.Rules[0].Effects["beacon_lit"] = true
	apply(t, e, "take", "key", "")
	apply(t, e, "move", "north", "")
	if e.Snapshot().Location != "hall" {
		t.Fatal("scénario partagé avec l'appelant")
	}
	denied(t, e, engine.Action{Kind: "move", Target: "up"})
	apply(t, e, "use", "door", "key")
	if e.View().Completed {
		t.Fatal("effets partagés avec l'appelant")
	}
}

func TestRestoreValidationIsAtomic(t *testing.T) {
	e := game(t)
	before := e.Snapshot()
	mutations := []func(*engine.State){
		func(s *engine.State) { s.Location = "ailleurs" },
		func(s *engine.State) { s.ItemLocations["door"] = world.Inventory },
		func(s *engine.State) { s.ItemLocations["key"] = "ailleurs" },
		func(s *engine.State) { delete(s.ItemLocations, "key") },
		func(s *engine.State) { s.Flags["invente"] = true },
		func(s *engine.State) { delete(s.Flags, "door_open") },
	}
	for _, mutate := range mutations {
		state := before.Clone()
		mutate(&state)
		if err := e.Restore(state); err == nil {
			t.Fatal("état invalide accepté")
		}
		if !reflect.DeepEqual(before, e.Snapshot()) {
			t.Fatal("restauration partielle")
		}
	}
	valid := before.Clone()
	if err := e.Restore(valid); err != nil {
		t.Fatal(err)
	}
	valid.Flags["door_open"] = true
	if e.Snapshot().Flags["door_open"] {
		t.Fatal("restauration par référence")
	}
}
