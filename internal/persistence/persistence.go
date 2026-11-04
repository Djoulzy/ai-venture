// Package persistence stores validated snapshots independently of narration.
package persistence

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"ai-venture/internal/engine"
	"ai-venture/internal/locale"
	"ai-venture/internal/strictjson"
)

const Version = 1
const maxSaveBytes = 2 << 20

type SaveFile struct {
	Version      int          `json:"version"`
	ScenarioID   string       `json:"scenario_id"`
	ScenarioHash string       `json:"scenario_hash"`
	State        engine.State `json:"state"`
}

// Save writes beside the destination, syncs, closes, then atomically renames.
// An interrupted write cannot leave a half-written destination file.
func Save(path string, e *engine.Engine) error {
	data, err := json.MarshalIndent(SaveFile{Version, e.ScenarioID(), e.Fingerprint(), e.Snapshot()}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if len(data) > maxSaveBytes {
		return locale.Errorf("save file is too large")
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".ai-venture-save-*")
	if err != nil {
		return locale.Errorf("creating save file: %w", err)
	}
	temp := f.Name()
	defer os.Remove(temp)
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(temp, path); err != nil {
		return locale.Errorf("replacing save file: %w", err)
	}
	return nil
}

// Load validates the whole file before asking the engine to replace its state.
func Load(path string, e *engine.Engine) error {
	f, err := os.Open(path)
	if err != nil {
		return locale.Errorf("reading save file: %w", err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxSaveBytes+1))
	if err != nil {
		return err
	}
	if len(data) > maxSaveBytes {
		return locale.Errorf("save file is too large")
	}
	var save SaveFile
	if err := strictjson.Decode(data, &save); err != nil {
		return err
	}
	if save.Version != Version {
		return locale.Errorf("unsupported save version: %d", save.Version)
	}
	if save.ScenarioID != e.ScenarioID() || save.ScenarioHash != e.Fingerprint() {
		return locale.Errorf("save file does not match this version of the scenario")
	}
	return e.Restore(save.State)
}
