package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"ai-venture/internal/catalogue"
)

func TestImportListSearchShowAndErrors(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "catalogue.sqlite")
	source := filepath.Join(dir, "entry.json")
	if err := os.WriteFile(source, []byte(`{"version":1,"entries":[{"id":"quai","kind":"location","language":"fr","title":"Le quai","text":"Le quai abrite les pêcheurs."}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"import", "-db", db, "-source", source, "-lang", "fr"},
		{"list", "-db", db, "-lang", "fr"},
		{"search", "-db", db, "-lang", "fr", "-query", "pecheurs"},
		{"show", "-db", db, "-lang", "fr", "-id", "quai"},
		{"assets", "-db", db},
	} {
		var output, diagnostics bytes.Buffer
		if _, err := run(context.Background(), args, &output, &diagnostics); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if !json.Valid(output.Bytes()) {
			t.Fatal(output.String())
		}
		if args[0] == "search" {
			var entries []catalogue.Entry
			if err := json.Unmarshal(output.Bytes(), &entries); err != nil || len(entries) != 1 {
				t.Fatal(output.String())
			}
		}
	}
	for _, args := range [][]string{{"unknown"}, {"import", "-db", db}, {"list", "-db", db, "-replace"}, {"search", "-db", db}, {"show", "-db", db}, {"list", "extra"}} {
		if _, err := run(context.Background(), args, &bytes.Buffer{}, &bytes.Buffer{}); err == nil {
			t.Fatalf("bad command accepted: %v", args)
		}
	}
}
