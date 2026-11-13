package catalogue

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"ai-venture/internal/locale"
	"ai-venture/internal/strictjson"
)

// Load validates the complete source before a database is opened for writing.
// Sources are one manifest or a directory of manifests, with local image paths.
func Load(ctx context.Context, source string) (Batch, error) {
	absolute, err := filepath.Abs(source)
	if err != nil {
		return Batch{}, err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return Batch{}, err
	}
	folder := absolute
	files := []string{}
	if !info.IsDir() {
		folder = filepath.Dir(absolute)
		files = append(files, filepath.Base(absolute))
	} else {
		err = filepath.WalkDir(folder, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			rel, err := filepath.Rel(folder, path)
			if err != nil {
				return err
			}
			if d.IsDir() {
				if rel != "." && (strings.HasPrefix(d.Name(), ".") || strings.Count(rel, string(filepath.Separator)) >= 8) {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.EqualFold(filepath.Ext(path), ".json") {
				files = append(files, rel)
				if len(files) > 128 {
					return locale.Errorf("too many catalogue source files")
				}
			}
			return nil
		})
		if err != nil {
			return Batch{}, err
		}
	}
	if len(files) == 0 {
		return Batch{}, locale.Errorf("no catalogue manifests found")
	}
	root, err := os.OpenRoot(folder)
	if err != nil {
		return Batch{}, err
	}
	defer root.Close()
	batch := Batch{}
	entries, assets := map[string]bool{}, map[string]bool{}
	var imageBytes int
	for _, name := range files {
		if err := ctx.Err(); err != nil {
			return Batch{}, err
		}
		data, err := readLocal(root, name, 1<<20)
		if err != nil {
			return Batch{}, err
		}
		var manifest Manifest
		if err := strictjson.Decode(data, &manifest); err != nil {
			return Batch{}, err
		}
		if manifest.Version != Version {
			return Batch{}, locale.Errorf("invalid catalogue manifest: %s", name)
		}
		if len(batch.Entries)+len(manifest.Entries) > MaxEntries || len(batch.Assets)+len(manifest.Assets) > MaxAssets {
			return Batch{}, locale.Errorf("catalogue import is too large")
		}
		for _, entry := range manifest.Entries {
			if err := ValidateEntry(entry); err != nil {
				return Batch{}, err
			}
			key := entry.ID + ":" + entry.Language
			if entries[key] {
				return Batch{}, locale.Errorf("duplicate catalogue entry: %s", key)
			}
			entries[key] = true
			batch.Entries = append(batch.Entries, entry)
		}
		for _, source := range manifest.Assets {
			if !identifier.MatchString(source.ID) || !validKind(source.Kind, true) || source.Path == "" || filepath.IsAbs(source.Path) || assets[source.ID] || (source.License != "" && !validText(source.License, 120, false)) {
				return Batch{}, locale.Errorf("invalid catalogue asset: %s", source.ID)
			}
			assets[source.ID] = true
			path := filepath.Join(filepath.Dir(name), filepath.FromSlash(source.Path))
			data, err := readLocal(root, path, MaxImageBytes)
			if err != nil {
				return Batch{}, err
			}
			config, _, err := image.DecodeConfig(bytes.NewReader(data))
			if err != nil {
				return Batch{}, locale.Errorf("invalid catalogue image: %s: %w", source.ID, err)
			}
			if err := CheckImage(config); err != nil {
				return Batch{}, err
			}
			img, _, err := image.Decode(bytes.NewReader(data))
			if err != nil {
				return Batch{}, err
			}
			var normalized bytes.Buffer
			if err := png.Encode(&normalized, img); err != nil {
				return Batch{}, err
			}
			blob := normalized.Bytes()
			imageBytes += len(blob)
			if len(blob) > MaxImageBytes || imageBytes > 128<<20 {
				return Batch{}, locale.Errorf("catalogue import is too large")
			}
			hash := sha256.Sum256(blob)
			batch.Assets = append(batch.Assets, Asset{ID: source.ID, Kind: source.Kind, SHA256: hex.EncodeToString(hash[:]), Width: config.Width, Height: config.Height, License: source.License, PNG: blob})
		}
	}
	if len(batch.Entries)+len(batch.Assets) == 0 {
		return Batch{}, locale.Errorf("catalogue contains no entries or images")
	}
	return batch, ctx.Err()
}

func readLocal(root *os.Root, path string, limit int64) ([]byte, error) {
	f, err := root.Open(path) // Rejects escapes through .. and symlinks.
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, locale.Errorf("catalogue source is not a bounded regular file: %s", path)
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, locale.Errorf("catalogue source is too large")
	}
	return data, nil
}
