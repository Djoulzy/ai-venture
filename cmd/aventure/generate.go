package main

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"

	"ai-venture/internal/catalogue"
	"ai-venture/internal/generation"
	"ai-venture/internal/locale"
	"ai-venture/internal/terminal"
)

func generateScenario(ctx context.Context, lang locale.Language, pitch, output, database string, seed int64, cfg backendConfig, printer terminal.Printer) (runErr error) {
	// Validate the destination directory before starting an expensive model.
	info, err := os.Stat(filepath.Dir(output))
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return locale.Errorf("scenario output directory is invalid")
	}
	store, err := catalogue.Open(ctx, database, false)
	if err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, store.Close()) }()
	if seed == 0 {
		var b [8]byte
		if _, err := rand.Read(b[:]); err != nil {
			return err
		}
		seed = int64(binary.LittleEndian.Uint64(b[:]) & 0x7fffffffffffffff)
	}
	printer.Println(terminal.StyleInfo, lang.Text("Generating a scenario with %s via %s...", cfg.model, cfg.provider))
	client, service, err := startBackend(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, service.Close()) }()
	g := generation.Generator{Client: client, Catalogue: store, Language: lang, Timeout: cfg.timeout, Attempts: 3}
	result, err := g.Generate(ctx, pitch, seed)
	if err != nil {
		return err
	}
	if err := generation.Publish(ctx, output, result.World); err != nil {
		return err
	}
	printer.Println(terminal.StyleSuccess, lang.Text("Scenario created: %s (%d locations, %d entities; solution verified).", output, len(result.World.Locations), len(result.World.Items)))
	return nil
}
