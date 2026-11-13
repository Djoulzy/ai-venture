package generation

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"ai-venture/internal/locale"
	"ai-venture/internal/world"
)

// Publish writes only to the user's CLI path, atomically and without replacement.
// The LLM cannot choose a destination or overwrite an existing scenario/save.
func Publish(ctx context.Context, path string, w world.Definition) error {
	if err := world.Validate(w); err != nil {
		return err
	}
	data, err := json.MarshalIndent(w, "", "  ")
	if err != nil {
		return err
	}
	if len(data)+1 > world.MaxBytes {
		return locale.Errorf("scenario is too large")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".scenario-*.json")
	if err != nil {
		return err
	}
	temp := f.Name()
	defer os.Remove(temp)
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Link(temp, path); err != nil {
		return locale.Errorf("publishing generated scenario: %w", err)
	}
	return nil
}
