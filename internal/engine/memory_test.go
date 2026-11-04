package engine_test

import (
	"reflect"
	"testing"

	"ai-venture/internal/engine"
)

func hasMemory(e *engine.Engine, id string) bool {
	for _, entry := range e.Memory() {
		if entry.ID == id {
			return true
		}
	}
	return false
}

func TestKnowledgeIsDiscoveredAndRestoredWithTheBranch(t *testing.T) {
	e := game(t)
	if !hasMemory(e, "lore:island_history") || !hasMemory(e, "lore:copper_key_tag") {
		t.Fatal("initial lore unavailable")
	}
	for _, id := range []string{"lore:keeper_workshop", "lore:stair_inscription", "lore:lantern_log"} {
		if hasMemory(e, id) {
			t.Fatalf("secret leaked: %s", id)
		}
	}
	apply(t, e, "take", "key", "")
	checkpoint := e.Snapshot()
	apply(t, e, "move", "north", "")
	if hasMemory(e, "lore:stair_inscription") {
		t.Fatal("locked lore leaked")
	}
	denied(t, e, engine.Action{Kind: "move", Target: "up"})
	apply(t, e, "use", "door", "key")
	if !hasMemory(e, "lore:stair_inscription") || !hasMemory(e, "event:3") {
		t.Fatal("discovery/event missing")
	}
	apply(t, e, "move", "south", "")
	if !hasMemory(e, "lore:stair_inscription") {
		t.Fatal("learned lore forgotten")
	}
	if err := e.Restore(checkpoint); err != nil {
		t.Fatal(err)
	}
	if hasMemory(e, "lore:stair_inscription") || hasMemory(e, "event:3") {
		t.Fatal("future branch survived reload")
	}
	if !reflect.DeepEqual(checkpoint, e.Snapshot()) {
		t.Fatal("checkpoint changed")
	}
	copy := e.Snapshot()
	copy.Knowledge[0] = "injected"
	copy.Journal[0].Action.Kind = "teleport"
	if !reflect.DeepEqual(checkpoint, e.Snapshot()) {
		t.Fatal("memory alias escaped")
	}
	for _, bad := range []func(*engine.State){
		func(s *engine.State) { s.Knowledge = append(s.Knowledge, "unknown_document") },
		func(s *engine.State) { s.Journal[0].Turn = 100 },
		func(s *engine.State) { s.Journal[0].Action = engine.Action{Kind: "use", Tool: "key", Target: "door"} },
	} {
		s := checkpoint.Clone()
		bad(&s)
		if err := e.Restore(s); err == nil {
			t.Fatal("invalid memory accepted")
		}
		if !reflect.DeepEqual(checkpoint, e.Snapshot()) {
			t.Fatal("partial restore")
		}
	}
}

func TestJournalIsBoundedAndObservationsDoNotAppend(t *testing.T) {
	e := game(t)
	apply(t, e, "observe", "key", "")
	apply(t, e, "inventory", "", "")
	if len(e.Snapshot().Journal) != 0 {
		t.Fatal("read-only action journaled")
	}
	for i := 0; i < engine.MaxJournal; i++ {
		apply(t, e, "take", "key", "")
		apply(t, e, "drop", "key", "")
	}
	s := e.Snapshot()
	if len(s.Journal) != engine.MaxJournal || s.Journal[0].Turn != engine.MaxJournal+1 {
		t.Fatalf("invalid bounded journal: %+v", s.Journal)
	}
	if err := e.Restore(s); err != nil {
		t.Fatal(err)
	}
}
