package scenarios_test

import (
	"bytes"
	"context"
	"image"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"ai-venture/internal/catalogue"
	"ai-venture/internal/engine"
	"ai-venture/internal/language"
	"ai-venture/internal/locale"
	"ai-venture/internal/persistence"
	"ai-venture/internal/presentation"
	"ai-venture/internal/terminal"
	"ai-venture/internal/visuals"
	"ai-venture/internal/world"
)

func TestProvidenceCanBePlayedAndSavedInBothLanguages(t *testing.T) {
	ctx := context.Background()
	batch, err := catalogue.Load(ctx, "../data/catalogue/providence")
	if err != nil {
		t.Fatal(err)
	}
	store, err := catalogue.Open(ctx, filepath.Join(t.TempDir(), "catalogue.sqlite"), true)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.Import(ctx, batch, false); err != nil {
		t.Fatal(err)
	}
	var previousID, previousHash, previousSave string
	games := t.TempDir()
	for _, lang := range []locale.Language{locale.French, locale.English} {
		t.Run(string(lang), func(t *testing.T) {
			w, err := (world.FileSource{Path: "providence-" + string(lang) + ".json"}).Load(ctx)
			if err != nil {
				t.Fatal(err)
			}
			w, err = store.Bind(ctx, w, lang)
			if err != nil {
				t.Fatal(err)
			}
			entry, err := store.Entry(ctx, "colonial_house", string(lang))
			if err != nil {
				t.Fatal(err)
			}
			if len(w.Locations) != 1 || w.Locations[w.Start].Description != entry.Text || len(w.Documents) != 1 || w.Documents[0].Text != entry.Text || w.Documents[0].Language != string(lang) {
				t.Fatal("scenario failed to preserve the selected catalogue entry")
			}
			e, err := engine.NewWithLanguage(w, lang)
			if err != nil {
				t.Fatal(err)
			}
			key, door := "old_key", "door"
			commands := "/examine old_key\n/take old_key\n/drop old_key\n/take old_key\n/use old_key on door\n/save\n/quit\n"
			if lang == locale.French {
				key, door = "cle_ancienne", "porte"
				commands = "/observer cle_ancienne\n/prendre cle_ancienne\n/deposer cle_ancienne\n/prendre cle_ancienne\n/utiliser cle_ancienne sur porte\n/sauver\n/quitter\n"
			}
			if e.View().Completed || len(e.View().Exits) != 0 || len(e.Memory()) != 1 || e.Memory()[0].Text != entry.Text {
				t.Fatal("unexpected initial state or discovery")
			}
			initial := e.Snapshot()
			for _, action := range []engine.Action{
				{Kind: "use", Tool: key, Target: door},
				{Kind: "take", Target: door},
				{Kind: "move", Target: "inside"},
			} {
				if _, err := e.Apply(action); err == nil || !reflect.DeepEqual(initial, e.Snapshot()) {
					t.Fatalf("invalid action mutated state: %+v", action)
				}
			}
			if previousSave != "" {
				if e.ScenarioID() == previousID || e.Fingerprint() == previousHash || persistence.Load(previousSave, e) == nil || !reflect.DeepEqual(initial, e.Snapshot()) {
					t.Fatal("language variants share a save or loading changed the state")
				}
			}
			folder := filepath.Join(games, string(lang))
			if err := os.MkdirAll(folder, 0700); err != nil {
				t.Fatal(err)
			}
			save := filepath.Join(folder, "save.json")
			scenes := 0
			var output bytes.Buffer
			s := terminal.Session{
				Engine: e, Interpreter: language.Offline{Language: lang}, Input: strings.NewReader(commands), Output: &output,
				SavePath: save, Timeout: 5 * time.Second, Title: w.Title, Introduction: w.Introduction,
				Visuals: visuals.Compositor{Assets: store, Directory: filepath.Join(folder, "visuals")},
				Observer: func(event presentation.Event) {
					if event.Kind == presentation.Scene {
						scenes++
						if event.Image.Bounds().Size() != image.Pt(640, 384) {
							t.Fatal("scene size changed")
						}
					}
				},
			}
			if err := s.Run(ctx); err != nil {
				t.Fatal(err)
			}
			if !e.View().Completed || e.Snapshot().Turn != 4 || len(e.View().Inventory) != 1 || e.View().Inventory[0].ID != key || scenes < 2 {
				t.Fatalf("unplayable scenario: state=%+v scenes=%d\n%s", e.Snapshot(), scenes, output.String())
			}
			final := e.Snapshot()
			if _, err := e.Apply(engine.Action{Kind: "use", Tool: key, Target: door}); err == nil || !reflect.DeepEqual(final, e.Snapshot()) {
				t.Fatal("completed rule could be applied twice")
			}
			reloaded, err := engine.NewWithLanguage(w, lang)
			if err != nil {
				t.Fatal(err)
			}
			if err := persistence.Load(save, reloaded); err != nil || !reflect.DeepEqual(final, reloaded.Snapshot()) {
				t.Fatalf("save/load lost progress: %v", err)
			}
			// Resume through the same command loop, not only the save decoder.
			s.Engine, s.Output = reloaded, io.Discard
			s.Input = strings.NewReader("/load\n/quit\n")
			if lang == locale.French {
				s.Input = strings.NewReader("/charger\n/quitter\n")
			}
			if err := s.Run(ctx); err != nil || !reflect.DeepEqual(final, reloaded.Snapshot()) {
				t.Fatalf("resume failed: %v", err)
			}
			previousID, previousHash, previousSave = e.ScenarioID(), e.Fingerprint(), save
		})
	}
}
