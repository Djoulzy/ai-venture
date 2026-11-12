package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"ai-venture/internal/catalogue"
	"ai-venture/internal/locale"
)

func bundleResources(executable string) string {
	macos := filepath.Dir(executable)
	contents := filepath.Dir(macos)
	if filepath.Base(macos) != "MacOS" || filepath.Base(contents) != "Contents" || !strings.HasSuffix(filepath.Dir(contents), ".app") {
		return ""
	}
	return filepath.Join(contents, "Resources")
}

// A Finder launch can play immediately without a terminal, Go or a model. The
// shipped Providence catalogue is imported once into user data, never written
// inside the .app. Its database is separate from the former lighthouse demo.
func prepareBundle(ctx context.Context, resources, dataDir string, language locale.Language) (string, string, error) {
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return "", "", err
	}
	database := filepath.Join(dataDir, "catalogue-providence.sqlite")
	if _, err := os.Stat(database); os.IsNotExist(err) {
		batch, err := catalogue.Load(ctx, filepath.Join(resources, "data/catalogue/providence"))
		if err != nil {
			return "", "", err
		}
		file, err := os.CreateTemp(dataDir, ".catalogue-*.sqlite")
		if err != nil {
			return "", "", err
		}
		temp := file.Name()
		defer os.Remove(temp)
		if err := file.Close(); err != nil {
			return "", "", err
		}
		store, err := catalogue.Open(ctx, temp, true)
		if err != nil {
			return "", "", err
		}
		_, importErr := store.Import(ctx, batch, false)
		closeErr := store.Close()
		if importErr != nil {
			return "", "", importErr
		}
		if closeErr != nil {
			return "", "", closeErr
		}
		if err := os.Rename(temp, database); err != nil {
			return "", "", err
		}
	} else if err != nil {
		return "", "", err
	}
	return filepath.Join(resources, "scenarios/providence-"+string(language)+".json"), database, nil
}
