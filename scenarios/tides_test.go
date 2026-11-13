package scenarios_test

import (
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"ai-venture/internal/catalogue"
	"ai-venture/internal/engine"
	"ai-venture/internal/language"
	"ai-venture/internal/locale"
	"ai-venture/internal/persistence"
	"ai-venture/internal/visuals"
	"ai-venture/internal/world"
)

func TestTidesQuestInBothLanguages(t *testing.T) {
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
	games := t.TempDir()
	var previousSave string
	for _, lang := range []locale.Language{locale.French, locale.English} {
		t.Run(string(lang), func(t *testing.T) {
			w, err := (world.FileSource{Path: "providence-tides-" + string(lang) + ".json"}).Load(ctx)
			if err != nil {
				t.Fatal(err)
			}
			w, err = store.Bind(ctx, w, lang)
			if err != nil {
				t.Fatal(err)
			}
			e, err := engine.NewWithLanguage(w, lang)
			if err != nil {
				t.Fatal(err)
			}
			if e.View().Completed || len(e.Memory()) != 1 || len(w.Locations) != 4 {
				t.Fatal("unexpected start")
			}
			if previousSave != "" {
				before := e.Snapshot()
				if persistence.Load(previousSave, e) == nil || !reflect.DeepEqual(before, e.Snapshot()) {
					t.Fatal("cross-language save accepted")
				}
			}
			north, south, west, east, down := "north", "south", "west", "east", "down"
			if lang == locale.French {
				north, south, west, east, down = "nord", "sud", "ouest", "est", "bas"
			}
			apply := func(a engine.Action) {
				t.Helper()
				if _, err := e.Apply(a); err != nil {
					t.Fatal(err)
				}
			}
			refuse := func(a engine.Action) {
				t.Helper()
				before := e.Snapshot()
				if _, err := e.Apply(a); err == nil || !reflect.DeepEqual(before, e.Snapshot()) {
					t.Fatalf("refusal failed: %+v", a)
				}
			}
			move := func(d string) { t.Helper(); apply(engine.Action{Kind: "move", Target: d}) }
			read := func() {
				t.Helper()
				command := "read book_necronomicon"
				if lang == locale.French {
					command = "lire le Necronomicon"
				}
				a, err := (language.Offline{Language: lang}).Interpret(ctx, command, e.View())
				if err != nil {
					t.Fatal(err)
				}
				apply(a)
			}
			use := engine.Action{Kind: "use", Tool: "book_necronomicon"}
			move(north)
			refuse(engine.Action{Kind: "move", Target: down})
			move(south)
			move(west)
			refuse(use)
			apply(engine.Action{Kind: "take", Target: "book_necronomicon"})
			move(east)
			move(north)
			refuse(use) // Having the book is insufficient without reading it first.
			refuse(engine.Action{Kind: "move", Target: down})
			move(south)
			move(west)
			apply(engine.Action{Kind: "drop", Target: "book_necronomicon"})
			refuse(use)
			apply(engine.Action{Kind: "take", Target: "book_necronomicon"})
			read()
			refuse(use)
			move(east)
			move(north)
			refuse(engine.Action{Kind: "move", Target: down})
			read()
			move(down)
			refuse(engine.Action{Kind: "take", Target: "monster_profond"})
			finalUse := engine.Action{Kind: "use", Tool: "book_necronomicon", Target: "monster_profond"}
			apply(finalUse)
			refuse(finalUse)
			if !e.View().Completed || len(e.View().Inventory) != 1 || len(e.View().Items) != 1 {
				t.Fatal("ending changed entity ownership")
			}
			found := false
			for _, doc := range w.Documents {
				if doc.Language != string(lang) {
					t.Fatal("wrong lore language")
				}
			}
			for _, doc := range e.Memory() {
				if doc.ID == "lore:chapter_release_witness" {
					found = true
				}
			}
			if !found {
				t.Fatal("ending lore not discovered")
			}
			path, err := (visuals.Compositor{Assets: store, Directory: t.TempDir()}).Render(ctx, e.View())
			if err != nil {
				t.Fatal(err)
			}
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			img, err := png.Decode(f)
			f.Close()
			if err != nil || img.Bounds().Size() != image.Pt(640, 384) {
				t.Fatal("invalid ending scene", err)
			}
			save := filepath.Join(games, "save-"+string(lang)+".json")
			if err := persistence.Save(save, e); err != nil {
				t.Fatal(err)
			}
			fresh, err := engine.NewWithLanguage(w, lang)
			if err != nil {
				t.Fatal(err)
			}
			if err := persistence.Load(save, fresh); err != nil || !reflect.DeepEqual(e.Snapshot(), fresh.Snapshot()) {
				t.Fatal("save/load mismatch", err)
			}
			previousSave = save
		})
	}
}
