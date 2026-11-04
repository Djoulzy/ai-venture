package persistence_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"ai-venture/internal/engine"
	"ai-venture/internal/persistence"
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

func TestRoundTripAndOverwrite(t *testing.T) {
	e := game(t)
	path := filepath.Join(t.TempDir(), "partie.json")
	for _, a := range []engine.Action{{Kind: "take", Target: "key"}, {Kind: "move", Target: "north"}, {Kind: "use", Tool: "key", Target: "door"}, {Kind: "drop", Target: "key"}} {
		if _, err := e.Apply(a); err != nil {
			t.Fatal(err)
		}
		if err := persistence.Save(path, e); err != nil {
			t.Fatal(err)
		}
		loaded := game(t)
		if err := persistence.Load(path, loaded); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(e.Snapshot(), loaded.Snapshot()) {
			t.Fatal("état perdu au chargement")
		}
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("fichier temporaire restant : %v %v", entries, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("permissions inattendues : %v", info.Mode())
	}
}

func TestInvalidSaveNeverChangesGame(t *testing.T) {
	e := game(t)
	before := e.Snapshot()
	path := filepath.Join(t.TempDir(), "partie.json")
	for name, mutate := range map[string]func(*persistence.SaveFile){
		"version":      func(s *persistence.SaveFile) { s.Version = 99 },
		"scenario":     func(s *persistence.SaveFile) { s.ScenarioID = "autre" },
		"hash":         func(s *persistence.SaveFile) { s.ScenarioHash = "ancienne_version" },
		"position":     func(s *persistence.SaveFile) { s.State.Location = "absent" },
		"fixed_item":   func(s *persistence.SaveFile) { s.State.ItemLocations["door"] = "@inventory" },
		"missing_item": func(s *persistence.SaveFile) { delete(s.State.ItemLocations, "key") },
		"flag":         func(s *persistence.SaveFile) { s.State.Flags["invente"] = true },
	} {
		t.Run(name, func(t *testing.T) {
			s := persistence.SaveFile{Version: persistence.Version, ScenarioID: e.ScenarioID(), ScenarioHash: e.Fingerprint(), State: e.Snapshot()}
			mutate(&s)
			data, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			if err := persistence.Load(path, e); err == nil {
				t.Fatal("sauvegarde invalide acceptée")
			}
			if !reflect.DeepEqual(before, e.Snapshot()) {
				t.Fatal("mutation lors d'un chargement invalide")
			}
		})
	}
	for _, data := range []string{`{"version":1,"version":1}`, `{"surprise":true}`, `null`, `{"version":`, `{} {}`} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if err := persistence.Load(path, e); err == nil {
			t.Fatal("JSON invalide accepté")
		}
		if !reflect.DeepEqual(before, e.Snapshot()) {
			t.Fatal("mutation après JSON invalide")
		}
	}
	if err := persistence.Load(path+".absent", e); err == nil {
		t.Fatal("fichier absent accepté")
	}
}

func TestScenarioFingerprintIncludesRules(t *testing.T) {
	e := game(t)
	path := filepath.Join(t.TempDir(), "partie.json")
	if err := persistence.Save(path, e); err != nil {
		t.Fatal(err)
	}
	w, err := (scenarios.Demo{}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	w.Rules[0].Effects["beacon_lit"] = true
	other, err := engine.New(w)
	if err != nil {
		t.Fatal(err)
	}
	if err := persistence.Load(path, other); err == nil {
		t.Fatal("scénario modifié accepté")
	}
}
