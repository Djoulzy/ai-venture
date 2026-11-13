package gameconfig

import (
	"ai-venture/internal/catalogue"
	"ai-venture/internal/engine"
	"ai-venture/internal/locale"
	"ai-venture/internal/world"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func project(t *testing.T, id string) (string, Definition) {
	t.Helper()
	root := t.TempDir()
	manifest := Definition{Version: 1, ID: id, Scenarios: map[string]string{}, CatalogueSource: "lore"}
	entries := []catalogue.Entry{}
	for _, lang := range []string{"fr", "en"} {
		path := "content/" + lang + ".json"
		manifest.Scenarios[lang] = path
		w := world.Definition{Version: 1, ID: id + "_" + lang, Title: id + " " + lang, Introduction: "Explore.", Start: "room", Locations: map[string]world.Location{"room": {Name: id, Description: lang, Exits: map[string]world.Exit{}}}, Items: map[string]world.Item{}, Flags: map[string]bool{"done": false}, Rules: []world.UseRule{}, Goal: []world.Condition{{Flag: "done", Value: true}}, CatalogueDocuments: []world.DocumentRef{{ID: "history", Entry: "history", Location: "room"}}}
		write(t, filepath.Join(root, path), w)
		entries = append(entries, catalogue.Entry{ID: "history", Kind: "location", Language: lang, Title: id, Text: id + " history " + lang})
	}
	write(t, filepath.Join(root, "lore", "entries.json"), catalogue.Manifest{Version: 1, Entries: entries})
	path := filepath.Join(root, Filename)
	write(t, path, manifest)
	d, err := Load(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return path, d
}

func TestProjectsAreInterchangeableAndKeepSeparateData(t *testing.T) {
	ctx := context.Background()
	data := t.TempDir()
	for _, id := range []string{"alpha", "beta"} {
		path, d := project(t, id)
		for _, lang := range []locale.Language{locale.French, locale.English} {
			scenario, err := d.Scenario(lang)
			if err != nil {
				t.Fatal(err)
			}
			w, err := (world.FileSource{Path: scenario}).Load(ctx)
			if err != nil {
				t.Fatal(err)
			}
			database, err := d.PrepareCatalogue(ctx, data)
			if err != nil {
				t.Fatal(err)
			}
			if filepath.Base(database) != "catalogue-"+id+".sqlite" {
				t.Fatal(database)
			}
			store, err := catalogue.Open(ctx, database, false)
			if err != nil {
				t.Fatal(err)
			}
			w, err = store.Bind(ctx, w, lang)
			store.Close()
			if err != nil {
				t.Fatal(err)
			}
			e, err := engine.NewWithLanguage(w, lang)
			if err != nil {
				t.Fatal(err)
			}
			if e.View().Name != id || e.Memory()[0].Text != id+" history "+string(lang) {
				t.Fatal("wrong project loaded")
			}
		}
		// Reuse existing data even if catalogue sources are temporarily unavailable.
		if err := os.RemoveAll(filepath.Join(filepath.Dir(path), "lore")); err != nil {
			t.Fatal(err)
		}
		if _, err := d.PrepareCatalogue(ctx, data); err != nil {
			t.Fatal(err)
		}
	}
	files, err := os.ReadDir(data)
	if err != nil || len(files) != 2 {
		t.Fatal("project data collision", err)
	}
}

func TestInvalidConfigAndCancelledImport(t *testing.T) {
	path, d := project(t, "valid")
	valid, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{`null`, `{} {}`, `{"version":1,"version":1}`, `{"version":1,"id":"x","scenarios":{"en":"../escape"}}`, `{"version":1,"id":"x","scenarios":{"de":"a.json"}}`, `{"version":1,"id":"../x","scenarios":{"en":"a.json"}}`, `{"version":1,"id":"x","scenarios":{"en":"/tmp/a.json"}}`, `{"version":1,"id":"x","scenarios":{"en":"a.json"},"extra":true}`} {
		if err := os.WriteFile(path, []byte(bad), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(context.Background(), path); err == nil {
			t.Fatal("invalid config accepted", bad)
		}
	}
	if err := os.WriteFile(path, valid, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dir := t.TempDir()
	if _, err := d.PrepareCatalogue(ctx, dir); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	files, _ := os.ReadDir(dir)
	if len(files) != 0 {
		t.Fatal("cancelled import wrote files")
	}
	if _, err := Load(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	delete(d.Scenarios, "fr")
	if _, err := d.Scenario(locale.French); err == nil {
		t.Fatal("missing language silently replaced")
	}
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(filepath.Dir(path), "linked.json")); err != nil {
		t.Fatal(err)
	}
	d.Scenarios["en"] = "linked.json"
	if _, err := d.Scenario(locale.English); err == nil {
		t.Fatal("symlink escaped content root")
	}
}
