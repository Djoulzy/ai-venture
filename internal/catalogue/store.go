package catalogue

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"ai-venture/internal/locale"
	_ "modernc.org/sqlite"
)

const applicationID = 0x41564354 // AVCT, never the narration cache's AVRG.

type Store struct {
	db       *sql.DB
	writable bool
}

func Open(ctx context.Context, path string, writable bool) (*Store, error) {
	if path == "" || path == ":memory:" {
		return nil, locale.Errorf("a catalogue database file is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if writable {
		file, err := os.OpenFile(absolute, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil && !os.IsExist(err) {
			return nil, err
		}
		if err == nil {
			if err := file.Close(); err != nil {
				return nil, err
			}
		}
	}
	u := url.URL{Scheme: "file", Path: absolute}
	if !writable {
		u.RawQuery = "mode=ro"
	}
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	s := &Store{db: db, writable: writable}
	if err := s.initialize(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) initialize(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, "PRAGMA busy_timeout=1000"); err != nil {
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
	if app == applicationID && version == Version {
		return tx.Commit()
	}
	if !s.writable || app != 0 || version != 0 {
		return locale.Errorf("database is not a supported AI Venture catalogue")
	}
	var count int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE name NOT LIKE 'sqlite_%'").Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return locale.Errorf("database is not a supported AI Venture catalogue")
	}
	for _, statement := range []string{
		`CREATE TABLE entries(id TEXT NOT NULL, language TEXT NOT NULL, kind TEXT NOT NULL, title TEXT NOT NULL, body TEXT NOT NULL, tags TEXT NOT NULL, metadata TEXT NOT NULL, images TEXT NOT NULL, PRIMARY KEY(id,language))`,
		`CREATE TABLE assets(id TEXT PRIMARY KEY, kind TEXT NOT NULL, sha256 TEXT NOT NULL, width INTEGER NOT NULL, height INTEGER NOT NULL, license TEXT NOT NULL, png BLOB NOT NULL)`,
		`CREATE VIRTUAL TABLE entries_fts USING fts5(id UNINDEXED, language UNINDEXED, title, body, tags, tokenize='unicode61 remove_diacritics 2')`,
		fmt.Sprintf("PRAGMA application_id=%d", applicationID), fmt.Sprintf("PRAGMA user_version=%d", Version),
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Import(ctx context.Context, batch Batch, replace bool) (Report, error) {
	if !s.writable {
		return Report{}, locale.Errorf("catalogue is read-only")
	}
	if len(batch.Entries)+len(batch.Assets) == 0 || len(batch.Entries) > MaxEntries || len(batch.Assets) > MaxAssets {
		return Report{}, locale.Errorf("catalogue import is too large")
	}
	seen := map[string]bool{}
	for _, e := range batch.Entries {
		if err := ValidateEntry(e); err != nil {
			return Report{}, err
		}
		key := e.ID + ":" + e.Language
		if seen[key] {
			return Report{}, locale.Errorf("duplicate catalogue entry: %s", key)
		}
		seen[key] = true
	}
	seen = map[string]bool{}
	var total int
	for _, asset := range batch.Assets {
		if seen[asset.ID] {
			return Report{}, locale.Errorf("invalid catalogue asset: %s", asset.ID)
		}
		seen[asset.ID] = true
		if err := validateAsset(asset); err != nil {
			return Report{}, err
		}
		total += len(asset.PNG)
		if total > 128<<20 {
			return Report{}, locale.Errorf("catalogue import is too large")
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Report{}, err
	}
	defer tx.Rollback()
	if replace {
		for _, table := range []string{"entries", "assets", "entries_fts"} {
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+table); err != nil {
				return Report{}, err
			}
		}
	}
	for _, a := range batch.Assets {
		_, err := tx.ExecContext(ctx, `INSERT INTO assets VALUES(?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET kind=excluded.kind,sha256=excluded.sha256,width=excluded.width,height=excluded.height,license=excluded.license,png=excluded.png`, a.ID, a.Kind, a.SHA256, a.Width, a.Height, a.License, a.PNG)
		if err != nil {
			return Report{}, err
		}
	}
	for _, e := range batch.Entries {
		for _, id := range e.Images {
			var exists int
			if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM assets WHERE id=?", id).Scan(&exists); err != nil {
				return Report{}, err
			}
			if exists == 0 {
				return Report{}, locale.Errorf("unknown catalogue asset: %s", id)
			}
		}
		tags, _ := json.Marshal(e.Tags)
		metadata, _ := json.Marshal(e.Metadata)
		images, _ := json.Marshal(e.Images)
		_, err := tx.ExecContext(ctx, `INSERT INTO entries VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(id,language) DO UPDATE SET kind=excluded.kind,title=excluded.title,body=excluded.body,tags=excluded.tags,metadata=excluded.metadata,images=excluded.images`, e.ID, e.Language, e.Kind, e.Title, e.Text, string(tags), string(metadata), string(images))
		if err != nil {
			return Report{}, err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM entries_fts WHERE id=? AND language=?", e.ID, e.Language); err != nil {
			return Report{}, err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO entries_fts VALUES(?,?,?,?,?)", e.ID, e.Language, e.Title, e.Text, strings.Join(e.Tags, " ")); err != nil {
			return Report{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Report{}, err
	}
	return Report{len(batch.Entries), len(batch.Assets)}, nil
}

func validateAsset(a Asset) error {
	if !identifier.MatchString(a.ID) || !validKind(a.Kind, true) || len(a.PNG) == 0 || len(a.PNG) > MaxImageBytes || !validDimensions(a.Width, a.Height) || (a.License != "" && !validText(a.License, 120, false)) {
		return locale.Errorf("invalid catalogue asset: %s", a.ID)
	}
	hash := sha256.Sum256(a.PNG)
	if hex.EncodeToString(hash[:]) != a.SHA256 {
		return locale.Errorf("catalogue asset checksum mismatch: %s", a.ID)
	}
	config, err := png.DecodeConfig(bytes.NewReader(a.PNG))
	if err != nil || config.Width != a.Width || config.Height != a.Height {
		return locale.Errorf("invalid catalogue asset: %s", a.ID)
	}
	return CheckImage(config)
}

func scanEntry(row interface{ Scan(...any) error }) (Entry, error) {
	var e Entry
	var tags, metadata, images string
	if err := row.Scan(&e.ID, &e.Language, &e.Kind, &e.Title, &e.Text, &tags, &metadata, &images); err != nil {
		return e, err
	}
	for _, field := range []struct {
		data   string
		target any
	}{{tags, &e.Tags}, {metadata, &e.Metadata}, {images, &e.Images}} {
		if err := json.Unmarshal([]byte(field.data), field.target); err != nil {
			return e, err
		}
	}
	return e, ValidateEntry(e)
}

func (s *Store) Entry(ctx context.Context, id, language string) (Entry, error) {
	if !identifier.MatchString(id) || (language != "fr" && language != "en") {
		return Entry{}, locale.Errorf("invalid catalogue entry: %s", id)
	}
	return scanEntry(s.db.QueryRowContext(ctx, "SELECT * FROM entries WHERE id=? AND language=?", id, language))
}

func (s *Store) Search(ctx context.Context, q Query) ([]Entry, error) {
	if (q.Language != "en" && q.Language != "fr") || (q.Kind != "" && !validKind(q.Kind, false)) || len(q.Text) > 4096 || q.Limit < 0 || q.Limit > 100 {
		return nil, locale.Errorf("invalid catalogue search")
	}
	if q.Limit == 0 {
		q.Limit = 20
	}
	statement := "SELECT e.* FROM entries e WHERE e.language=? AND (?='' OR e.kind=?)"
	args := []any{q.Language, q.Kind, q.Kind}
	if strings.TrimSpace(q.Text) != "" {
		terms := []string{}
		for _, term := range strings.FieldsFunc(strings.ToLower(q.Text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
			if len([]rune(term)) <= 64 {
				terms = append(terms, `"`+term+`"`)
			}
			if len(terms) == 32 {
				break
			}
		}
		if len(terms) == 0 {
			return []Entry{}, nil
		}
		statement = "SELECT e.* FROM entries_fts f JOIN entries e ON e.id=f.id AND e.language=f.language WHERE entries_fts MATCH ? AND e.language=? AND (?='' OR e.kind=?) ORDER BY bm25(entries_fts),e.id LIMIT ?"
		args = append([]any{strings.Join(terms, " OR ")}, args...)
	} else {
		statement += " ORDER BY e.id LIMIT ?"
	}
	args = append(args, q.Limit)
	rows, err := s.db.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Entry{}
	for rows.Next() {
		entry, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, entry)
	}
	return result, rows.Err()
}

func (s *Store) ReadAssets(ctx context.Context, ids []string) (map[string]Asset, error) {
	if len(ids) > 32 {
		return nil, locale.Errorf("too many visual layers")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	result := map[string]Asset{}
	total := 0
	for _, id := range ids {
		if _, ok := result[id]; ok {
			continue
		}
		var a Asset
		if err := tx.QueryRowContext(ctx, "SELECT * FROM assets WHERE id=? AND length(png)<=?", id, MaxImageBytes).Scan(&a.ID, &a.Kind, &a.SHA256, &a.Width, &a.Height, &a.License, &a.PNG); err != nil {
			return nil, locale.Errorf("reading catalogue asset %s: %w", id, err)
		}
		if err := validateAsset(a); err != nil {
			return nil, err
		}
		total += len(a.PNG)
		if total > 64<<20 {
			return nil, locale.Errorf("catalogue scene assets are too large")
		}
		result[id] = a
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Store) Assets(ctx context.Context) ([]Asset, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,kind,sha256,width,height,license FROM assets ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Asset{}
	for rows.Next() {
		var a Asset
		if err := rows.Scan(&a.ID, &a.Kind, &a.SHA256, &a.Width, &a.Height, &a.License); err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}
