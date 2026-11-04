package sqlite

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"ai-venture/internal/retrieval"
)

func store(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "memory ?#.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestFTS5ScopesAccentsAndLiteralQueries(t *testing.T) {
	s := store(t)
	ctx := context.Background()
	for _, q := range []retrieval.Query{
		{Scenario: "scenario-a", Language: "en", Text: "copper", Documents: []retrieval.Document{{ID: "key", Kind: "lore", Title: "Copper key", Text: "A copper key rests here."}}},
		{Scenario: "scenario-b", Language: "fr", Text: "cle", Documents: []retrieval.Document{{ID: "cle", Kind: "lore", Title: "Clé", Text: "Une clé de cuivre."}}},
	} {
		hits, err := s.Search(ctx, q)
		if err != nil || len(hits) != 1 || hits[0].ID != q.Documents[0].ID {
			t.Fatalf("%+v %v", hits, err)
		}
	}
	// Seed foreign scopes with the exact same term. They must not enter top-k.
	q := retrieval.Query{Scenario: "scenario-b", Language: "en", Text: "copper", Documents: []retrieval.Document{{ID: "other", Kind: "lore", Title: "Copper", Text: "copper copper copper"}}}
	if _, err := s.Search(ctx, q); err != nil {
		t.Fatal(err)
	}
	q.Scenario = "scenario-a"
	q.Documents = []retrieval.Document{{ID: "allowed", Kind: "lore", Text: "The copper lighthouse fittings."}}
	for _, text := range []string{"copper", `copper\" OR scope:* -- ; DROP TABLE rag_passages;`, "COPPER", "copper NEAR(((("} {
		q.Text = text
		hits, err := s.Search(ctx, q)
		if err != nil || len(hits) != 1 || hits[0].ID != "allowed" {
			t.Fatalf("%q: %+v %v", text, hits, err)
		}
	}
}

func TestReplacementRemovesFutureAndReopenedCacheIsNotAuthority(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.sqlite")
	s, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	q := retrieval.Query{Scenario: "same", Language: "en", Text: "ship", Documents: []retrieval.Document{{ID: "event:8", Kind: "event", Text: "The ship was rescued.", Turn: 8}}}
	if _, err = s.Search(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// A restored snapshot contains neither the future journal event nor its lore.
	q.Documents = nil
	hits, err := s.Search(context.Background(), q)
	if err != nil || len(hits) != 0 {
		t.Fatalf("future leak: %+v %v", hits, err)
	}
	var count int
	if err = s.db.QueryRow("SELECT count(*) FROM rag_passages WHERE scope='same'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("stale rows: %d %v", count, err)
	}
	q.Documents = []retrieval.Document{{ID: "event:8", Kind: "event", Text: "You dropped the copper key.", Turn: 8}}
	hits, err = s.Search(context.Background(), q)
	if err != nil || len(hits) != 0 {
		t.Fatalf("abandoned branch: %+v %v", hits, err)
	}
	q.Text = "copper"
	hits, err = s.Search(context.Background(), q)
	if err != nil || len(hits) != 1 || strings.Contains(hits[0].Text, "ship") {
		t.Fatalf("branch replacement: %+v %v", hits, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("database permissions: %v %v", info, err)
	}
}

func TestSearchBoundsCancellationAndNoMatch(t *testing.T) {
	s := store(t)
	q := retrieval.Query{Scenario: "bounded", Language: "en", Text: "lighthouse"}
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		q.Documents = append(q.Documents, retrieval.Document{ID: id, Kind: "lore", Title: strings.Repeat("é", 100), Text: strings.Repeat("lighthouse é ", 100)})
	}
	hits, err := s.Search(context.Background(), q)
	if err != nil || len(hits) != retrieval.MaxPassages {
		t.Fatalf("%+v %v", hits, err)
	}
	total := 0
	for _, hit := range hits {
		total += utf8.RuneCountInString(hit.Text) + utf8.RuneCountInString(hit.Title)
		if !utf8.ValidString(hit.Text) {
			t.Fatal("invalid UTF-8")
		}
	}
	if total > retrieval.MaxContextRunes {
		t.Fatal("context budget exceeded")
	}
	for _, term := range []string{"", "the le la", "unmatchedterm", `\"*():';--`} {
		q.Text = term
		hits, err = s.Search(context.Background(), q)
		if err != nil || len(hits) != 0 {
			t.Fatalf("%q: %+v %v", term, hits, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = s.Search(ctx, q); err == nil {
		t.Fatal("cancellation ignored")
	}
	q.Documents = append(q.Documents, q.Documents[0])
	if _, err = s.Search(context.Background(), q); err == nil {
		t.Fatal("duplicate ID accepted")
	}
}

func TestUnrelatedDatabaseIsNotModified(t *testing.T) {
	path := filepath.Join(t.TempDir(), "user.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("CREATE TABLE important(value TEXT)"); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	if s, err := Open(context.Background(), path); err == nil {
		_ = s.Close()
		t.Fatal("unrelated database accepted")
	}
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err = db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name='important'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("unrelated database changed: %d %v", count, err)
	}
}
