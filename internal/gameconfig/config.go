// Package gameconfig selects external game content. No adventure is compiled
// into the application: a JSON manifest supplies every content path.
package gameconfig

import (
	"ai-venture/internal/catalogue"
	"ai-venture/internal/locale"
	"ai-venture/internal/strictjson"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const Filename = "game.json"

type Definition struct {
	Version         int               `json:"version"`
	ID              string            `json:"id"`
	Scenarios       map[string]string `json:"scenarios"`
	CatalogueSource string            `json:"catalogue_source,omitempty"`
	Documents       []string          `json:"documents,omitempty"`
	root            string
}

var identifier = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

func validPath(path string) bool {
	return filepath.IsLocal(path) && filepath.Clean(path) != "." && !strings.ContainsAny(path, "\\:")
}

func Load(ctx context.Context, path string) (Definition, error) {
	if err := ctx.Err(); err != nil {
		return Definition{}, err
	}
	f, err := os.Open(path)
	if err != nil {
		return Definition{}, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 65537))
	if err != nil {
		return Definition{}, err
	}
	var d Definition
	if len(data) > 65536 {
		return d, locale.Errorf("game configuration is too large")
	}
	if err := strictjson.Decode(data, &d); err != nil {
		return d, err
	}
	if d.Version != 1 || !identifier.MatchString(d.ID) || len(d.Scenarios) < 1 || len(d.Scenarios) > 2 || len(d.Documents) > 32 {
		return Definition{}, locale.Errorf("invalid game configuration")
	}
	for lang, file := range d.Scenarios {
		if (lang != "en" && lang != "fr") || !validPath(file) {
			return Definition{}, locale.Errorf("invalid game content path or language")
		}
	}
	if d.CatalogueSource != "" && !validPath(d.CatalogueSource) {
		return Definition{}, locale.Errorf("invalid game content path or language")
	}
	for _, file := range d.Documents {
		if !validPath(file) {
			return Definition{}, locale.Errorf("invalid game content path or language")
		}
	}
	d.root, err = filepath.Abs(filepath.Dir(path))
	return d, err
}

// Resolve through a rooted filesystem to reject escapes through symlinks too.
func (d Definition) resolve(relative string) (string, error) {
	root, err := os.OpenRoot(d.root)
	if err != nil {
		return "", err
	}
	defer root.Close()
	f, err := root.Open(relative)
	if err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return filepath.Join(d.root, relative), nil
}

func (d Definition) Scenario(language locale.Language) (string, error) {
	path, ok := d.Scenarios[string(language)]
	if !ok {
		return "", locale.Errorf("game configuration has no scenario for language %s", language)
	}
	return d.resolve(path)
}

// PrepareCatalogue imports once into the user's data directory. An existing
// catalogue is reused; changing a source never silently replaces player data.
func (d Definition) PrepareCatalogue(ctx context.Context, dataDir string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if d.CatalogueSource == "" {
		return "", nil
	}
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return "", err
	}
	database := filepath.Join(dataDir, "catalogue-"+d.ID+".sqlite")
	if _, err := os.Stat(database); err == nil {
		return database, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	source, err := d.resolve(d.CatalogueSource)
	if err != nil {
		return "", err
	}
	batch, err := catalogue.Load(ctx, source)
	if err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dataDir, ".catalogue-*.sqlite")
	if err != nil {
		return "", err
	}
	temp := f.Name()
	defer os.Remove(temp)
	if err := f.Close(); err != nil {
		return "", err
	}
	store, err := catalogue.Open(ctx, temp, true)
	if err != nil {
		return "", err
	}
	_, importErr := store.Import(ctx, batch, false)
	if err := errors.Join(importErr, store.Close()); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := os.Link(temp, database); err != nil && !os.IsExist(err) {
		return "", err
	}
	return database, nil
}
