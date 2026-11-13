package catalogue_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-venture/internal/catalogue"
	"ai-venture/internal/engine"
	"ai-venture/internal/locale"
	ragstore "ai-venture/internal/retrieval/sqlite"
	"ai-venture/internal/testfixtures"
	"ai-venture/internal/world"
)

func source(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 128})
	f, err := os.Create(filepath.Join(dir, "sprite.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	writeManifest(t, dir, catalogue.Manifest{Version: 1, Entries: []catalogue.Entry{
		{ID: "keeper", Kind: "character", Language: "fr", Title: "Le gardien", Text: "Le gardien répare le phare.", Tags: []string{"réparation"}, Images: []string{"keeper_image"}},
		{ID: "keeper", Kind: "character", Language: "en", Title: "The keeper", Text: "The keeper repairs the lighthouse.", Images: []string{"keeper_image"}},
	}, Assets: []catalogue.AssetSource{{ID: "keeper_image", Kind: "character", Path: "sprite.png"}}})
	return dir
}

func writeManifest(t *testing.T, dir string, m catalogue.Manifest) {
	t.Helper()
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "catalogue.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestEmptyCategoryCanCoexistWithContent(t *testing.T) {
	dir := source(t)
	if err := os.WriteFile(filepath.Join(dir, "empty.json"), []byte(`{"version":1,"entries":[],"assets":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	batch, err := catalogue.Load(context.Background(), dir)
	if err != nil || len(batch.Entries) != 2 || len(batch.Assets) != 1 {
		t.Fatalf("empty category blocked content: %v", err)
	}
	empty := t.TempDir()
	if err := os.WriteFile(filepath.Join(empty, "empty.json"), []byte(`{"version":1,"entries":[],"assets":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := catalogue.Load(context.Background(), empty); err == nil {
		t.Fatal("entirely empty import accepted")
	}
}

func open(t *testing.T, path string, writable bool) *catalogue.Store {
	t.Helper()
	s, err := catalogue.Open(context.Background(), path, writable)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestImportPersistsImagesLanguagesAndLiteralSearch(t *testing.T) {
	ctx := context.Background()
	dir := source(t)
	batch, err := catalogue.Load(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "catalogue ?#.sqlite")
	s := open(t, path, true)
	report, err := s.Import(ctx, batch, false)
	if err != nil || report.Entries != 2 || report.Assets != 1 {
		t.Fatalf("%+v %v", report, err)
	}
	if _, err := s.Import(ctx, batch, false); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	s = open(t, path, false)
	for _, q := range []catalogue.Query{{Language: "fr", Text: "reparation"}, {Language: "en", Text: "lighthouse"}} {
		entries, err := s.Search(ctx, q)
		if err != nil || len(entries) != 1 || entries[0].Language != q.Language {
			t.Fatalf("%+v %v", entries, err)
		}
	}
	for _, text := range []string{`" OR *`, `'); DROP TABLE entries; --`, "NEAR(a b)", "###"} {
		if _, err := s.Search(ctx, catalogue.Query{Language: "fr", Text: text}); err != nil {
			t.Fatal(err)
		}
	}
	assets, err := s.ReadAssets(ctx, []string{"keeper_image", "keeper_image"})
	if err != nil || len(assets) != 1 {
		t.Fatalf("%+v %v", assets, err)
	}
	img, err := png.Decode(bytes.NewReader(assets["keeper_image"].PNG))
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, alpha := img.At(0, 0).RGBA()
	if alpha == 0 || alpha == 65535 {
		t.Fatal("PNG transparency lost")
	}
	if _, err := s.Import(ctx, batch, false); err == nil {
		t.Fatal("read-only catalogue accepted import")
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("database permissions changed")
	}
}

func TestFailedImportsAreAtomicIncludingReplace(t *testing.T) {
	ctx := context.Background()
	batch, err := catalogue.Load(ctx, source(t))
	if err != nil {
		t.Fatal(err)
	}
	s := open(t, filepath.Join(t.TempDir(), "catalogue.sqlite"), true)
	if _, err := s.Import(ctx, batch, false); err != nil {
		t.Fatal(err)
	}
	for _, replace := range []bool{false, true} {
		bad := catalogue.Batch{Entries: []catalogue.Entry{
			{ID: "first", Kind: "location", Language: "fr", Title: "Premier", Text: "Une fiche valable."},
			{ID: "second", Kind: "location", Language: "fr", Title: "Deuxième", Text: "Une autre fiche.", Images: []string{"missing"}},
		}}
		if _, err := s.Import(ctx, bad, replace); err == nil {
			t.Fatal("missing image accepted")
		}
		if _, err := s.Entry(ctx, "first", "fr"); err == nil {
			t.Fatal("partial entry survived rollback")
		}
		if _, err := s.Entry(ctx, "keeper", "fr"); err != nil {
			t.Fatal("failed replacement deleted existing content")
		}
	}
	if _, err := s.Import(ctx, catalogue.Batch{Entries: batch.Entries}, true); err == nil {
		t.Fatal("replacement referenced assets from deleted catalogue")
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := s.Import(ctx, batch, false); err == nil {
		t.Fatal("cancelled import accepted")
	}
}

func TestSourceBoundariesAndDatabaseOwnership(t *testing.T) {
	ctx := context.Background()
	for name, mutate := range map[string]func(string){
		"unknown_json": func(dir string) {
			_ = os.WriteFile(filepath.Join(dir, "bad.json"), []byte(`{"version":1,"entries":[],"unknown":1}`), 0600)
		},
		"duplicate": func(dir string) {
			data, _ := os.ReadFile(filepath.Join(dir, "catalogue.json"))
			_ = os.WriteFile(filepath.Join(dir, "duplicate.json"), data, 0600)
		},
		"image_not_valid": func(dir string) { _ = os.WriteFile(filepath.Join(dir, "sprite.png"), []byte("not an image"), 0600) },
		"escaping_symlink": func(dir string) {
			_ = os.Remove(filepath.Join(dir, "sprite.png"))
			outside := filepath.Join(t.TempDir(), "outside.png")
			_ = os.WriteFile(outside, []byte("outside"), 0600)
			if err := os.Symlink(outside, filepath.Join(dir, "sprite.png")); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := source(t)
			mutate(dir)
			if _, err := catalogue.Load(ctx, dir); err == nil {
				t.Fatal("invalid source accepted")
			}
		})
	}
	path := filepath.Join(t.TempDir(), "rag.sqlite")
	rag, err := ragstore.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer rag.Close()
	if _, err := catalogue.Open(ctx, path, true); err == nil {
		t.Fatal("RAG database accepted as permanent catalogue")
	}
	missing := filepath.Join(t.TempDir(), "missing.sqlite")
	if _, err := catalogue.Open(ctx, missing, false); err == nil {
		t.Fatal("missing read-only catalogue accepted")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("read-only open created a file")
	}
}

func TestJPEGAndGIFAreNormalizedAndImageDimensionsAreBounded(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.White)
	for _, format := range []string{"jpeg", "gif"} {
		t.Run(format, func(t *testing.T) {
			dir := source(t)
			var data bytes.Buffer
			var err error
			if format == "jpeg" {
				err = jpeg.Encode(&data, img, nil)
			} else {
				err = gif.Encode(&data, img, nil)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "sprite.png"), data.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			batch, err := catalogue.Load(context.Background(), dir)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.HasPrefix(batch.Assets[0].PNG, []byte("\x89PNG\r\n\x1a\n")) {
				t.Fatal("source image was not normalized")
			}
		})
	}
	for _, config := range []image.Config{{Width: 4097, Height: 1}, {Width: 4096, Height: 4096}, {Width: 0, Height: 10}} {
		if err := catalogue.CheckImage(config); err == nil {
			t.Fatal("oversized image accepted")
		}
	}
}

func TestBindingsPreserveDiscoveryAndRejectUnresolvedEngine(t *testing.T) {
	ctx := context.Background()
	batch, err := catalogue.Load(ctx, source(t))
	if err != nil {
		t.Fatal(err)
	}
	s := open(t, filepath.Join(t.TempDir(), "catalogue.sqlite"), true)
	if _, err := s.Import(ctx, batch, false); err != nil {
		t.Fatal(err)
	}
	w, err := (testfixtures.Scenario{}).Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	w.CatalogueDocuments = []world.DocumentRef{{ID: "keeper_entry", Entry: "keeper", Location: "workshop"}}
	if _, err := engine.New(w); err == nil {
		t.Fatal("unresolved references accepted")
	}
	bound, err := s.Bind(ctx, w, locale.English)
	if err != nil {
		t.Fatal(err)
	}
	if len(w.CatalogueDocuments) != 1 || len(bound.CatalogueDocuments) != 0 {
		t.Fatal("binding mutated its input or did not resolve")
	}
	e, err := engine.New(bound)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range e.Memory() {
		if entry.ID == "lore:keeper_entry" {
			t.Fatal("undiscovered catalogue lore leaked")
		}
	}
	if _, err := e.Apply(engine.Action{Kind: "move", Target: "east"}); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range e.Memory() {
		if entry.ID == "lore:keeper_entry" {
			found = true
			if !strings.Contains(entry.Text, "lighthouse") {
				t.Fatal("wrong language bound")
			}
		}
	}
	if !found {
		t.Fatal("catalogue lore not discovered")
	}
	if _, err := s.Bind(ctx, w, locale.Language("de")); err == nil {
		t.Fatal("missing language accepted")
	}
}
