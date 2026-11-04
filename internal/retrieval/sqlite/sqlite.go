// Package sqlite provides a rebuildable SQLite FTS5 cache of approved memories.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"ai-venture/internal/locale"
	"ai-venture/internal/retrieval"
	_ "modernc.org/sqlite"
)

const applicationID = 0x41565247 // AVRG
const schemaVersion = 1

type Store struct{ db *sql.DB }

func Open(ctx context.Context, path string) (*Store, error) {
	if path == "" {
		return nil, locale.Errorf("RAG database path is empty")
	}
	dsn := path
	if path != ":memory:" {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		f, err := os.OpenFile(absolute, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil && !os.IsExist(err) {
			return nil, err
		}
		if err == nil {
			if err := f.Close(); err != nil {
				return nil, err
			}
		}
		dsn = (&url.URL{Scheme: "file", Path: absolute}).String()
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	s := &Store{db: db}
	if err := s.initialize(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) initialize(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, "PRAGMA busy_timeout = 1000"); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var app, version int
	if err := tx.QueryRowContext(ctx, "PRAGMA application_id").Scan(&app); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if app == 0 && version == 0 {
		var tables int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE name NOT LIKE 'sqlite_%'").Scan(&tables); err != nil {
			return err
		}
		if tables != 0 {
			return locale.Errorf("database is not an AI Venture RAG cache")
		}
	} else if app != applicationID || version != schemaVersion {
		return locale.Errorf("unsupported RAG database format")
	}
	for _, statement := range []string{
		`CREATE VIRTUAL TABLE IF NOT EXISTS rag_passages USING fts5(
		 scope UNINDEXED, language UNINDEXED, id UNINDEXED, kind UNINDEXED, turn UNINDEXED,
		 title, body, tokenize = 'unicode61 remove_diacritics 2')`,
		fmt.Sprintf("PRAGMA application_id = %d", applicationID),
		fmt.Sprintf("PRAGMA user_version = %d", schemaVersion),
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Search(ctx context.Context, query retrieval.Query) ([]retrieval.Document, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if query.Scenario == "" || len(query.Scenario) > 128 || (query.Language != "en" && query.Language != "fr") || len(query.Documents) > retrieval.MaxDocuments {
		return nil, locale.Errorf("invalid RAG query scope or corpus")
	}
	seen := map[string]bool{}
	for _, doc := range query.Documents {
		if doc.ID == "" || len(doc.ID) > 128 || seen[doc.ID] || (doc.Kind != "lore" && doc.Kind != "event") ||
			strings.TrimSpace(doc.Text) == "" || utf8.RuneCountInString(doc.Text) > 2000 || utf8.RuneCountInString(doc.Title) > 120 {
			return nil, locale.Errorf("invalid RAG document: %s", doc.ID)
		}
		seen[doc.ID] = true
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	// Replace and search in one transaction, including when the new corpus is
	// empty. An abandoned future branch can never leak through an old index.
	if _, err := tx.ExecContext(ctx, "DELETE FROM rag_passages WHERE scope = ? AND language = ?", query.Scenario, query.Language); err != nil {
		return nil, err
	}
	insert, err := tx.PrepareContext(ctx, "INSERT INTO rag_passages(scope,language,id,kind,turn,title,body) VALUES(?,?,?,?,?,?,?)")
	if err != nil {
		return nil, err
	}
	defer insert.Close()
	for _, doc := range query.Documents {
		// Store uint64 turns as decimal text, avoiding SQLite's signed integer limit.
		if _, err := insert.ExecContext(ctx, query.Scenario, query.Language, doc.ID, doc.Kind, fmt.Sprint(doc.Turn), doc.Title, doc.Text); err != nil {
			return nil, err
		}
	}
	passages := []retrieval.Document{}
	match := expression(query.Text)
	if match != "" {
		rows, err := tx.QueryContext(ctx, `SELECT id,kind,title,body,turn FROM rag_passages
		 WHERE scope = ? AND language = ? AND rag_passages MATCH ?
		 ORDER BY bm25(rag_passages,0,0,0,0,0,2.0,1.0), id LIMIT ?`, query.Scenario, query.Language, match, retrieval.MaxPassages)
		if err != nil {
			return nil, err
		}
		remaining := retrieval.MaxContextRunes
		for rows.Next() {
			var doc retrieval.Document
			if err := rows.Scan(&doc.ID, &doc.Kind, &doc.Title, &doc.Text, &doc.Turn); err != nil {
				rows.Close()
				return nil, err
			}
			budget := min(500, remaining-utf8.RuneCountInString(doc.Title))
			if budget <= 0 {
				break
			}
			doc.Text = retrieval.Clip(doc.Text, budget)
			remaining -= utf8.RuneCountInString(doc.Text) + utf8.RuneCountInString(doc.Title)
			passages = append(passages, doc)
		}
		err = rows.Err()
		closeErr := rows.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return passages, nil
}

var stopWords = func() map[string]bool {
	words := strings.Fields("the a an and or to of in at on with you your is are it this that as for from into then le la les un une des de du au aux et ou vous votre vos est sont ce cette ces dans sur avec pour par puis en se sa son ses qui que")
	result := map[string]bool{}
	for _, word := range words {
		result[word] = true
	}
	return result
}()

// Build a bounded literal OR query, never execute player text as SQL or FTS syntax.
func expression(text string) string {
	terms := []string{}
	seen := map[string]bool{}
	for _, term := range strings.FieldsFunc(strings.ToLower(retrieval.Clip(text, 4096)), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
		if len([]rune(term)) < 2 || len([]rune(term)) > 64 || seen[term] || stopWords[term] {
			continue
		}
		seen[term] = true
		terms = append(terms, `"`+term+`"`)
		if len(terms) == 32 {
			break
		}
	}
	return strings.Join(terms, " OR ")
}
