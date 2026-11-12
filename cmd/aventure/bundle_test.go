package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"ai-venture/internal/catalogue"
	"ai-venture/internal/locale"
	"ai-venture/internal/world"
)

func TestBundleDetection(t *testing.T) {
	for _, executable := range []string{"/tmp/bin/aventure", "/tmp/foo/Contents/MacOS/aventure"} {
		if bundleResources(executable) != "" {
			t.Fatal("ordinary binary treated as an app")
		}
	}
	if got := bundleResources("/tmp/AI Venture.app/Contents/MacOS/aventure"); got != "/tmp/AI Venture.app/Contents/Resources" {
		t.Fatal(got)
	}
}

func TestBundleImportsProvidenceOnceAndResolvesBothLanguages(t *testing.T) {
	resources, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	data := t.TempDir()
	// A previous demo database is unrelated and must not be read or replaced.
	old := filepath.Join(data, "catalogue-demo.sqlite")
	if err := os.WriteFile(old, []byte("previous catalogue"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, lang := range []locale.Language{locale.English, locale.French} {
		scenario, database, err := prepareBundle(context.Background(), resources, data, lang)
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Base(database) != "catalogue-providence.sqlite" || filepath.Base(scenario) != "providence-"+string(lang)+".json" {
			t.Fatal("bundle did not select Providence")
		}
		store, err := catalogue.Open(context.Background(), database, false)
		if err != nil {
			t.Fatal(err)
		}
		w, err := (world.FileSource{Path: scenario}).Load(context.Background())
		if err == nil {
			_, err = store.Bind(context.Background(), w, lang)
		}
		_ = store.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	if data, err := os.ReadFile(old); err != nil || string(data) != "previous catalogue" {
		t.Fatal("previous catalogue changed")
	}
	// The user's persistent catalogue is not overwritten by a later app launch,
	// even when the original resources have moved.
	_, database, err := prepareBundle(context.Background(), "missing-resources", data, locale.English)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(database); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	newData := t.TempDir()
	if _, _, err := prepareBundle(ctx, resources, newData, locale.English); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled import: %v", err)
	}
	files, _ := os.ReadDir(newData)
	if len(files) != 0 {
		t.Fatal("cancelled import published partial data")
	}
}
