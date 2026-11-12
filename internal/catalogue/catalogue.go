// Package catalogue owns permanent authoring material, separate from game state
// and the rebuildable narration index. Image bytes are imported into SQLite.
package catalogue

import (
	"context"
	"image"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"ai-venture/internal/locale"
)

const Version = 1
const MaxEntries = 2048
const MaxAssets = 128
const MaxImageBytes = 16 << 20
const MaxPixels = 8 * 1024 * 1024

var identifier = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

type Entry struct {
	ID       string            `json:"id"`
	Kind     string            `json:"kind"`
	Language string            `json:"language"`
	Title    string            `json:"title"`
	Text     string            `json:"text"`
	Tags     []string          `json:"tags,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
	Images   []string          `json:"images,omitempty"`
}

type AssetSource struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Path    string `json:"path"`
	License string `json:"license,omitempty"`
}

type Manifest struct {
	Version int           `json:"version"`
	Entries []Entry       `json:"entries,omitempty"`
	Assets  []AssetSource `json:"assets,omitempty"`
}

type Asset struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	SHA256  string `json:"sha256"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	License string `json:"license,omitempty"`
	PNG     []byte `json:"-"`
}

type Batch struct {
	Entries []Entry
	Assets  []Asset
}

type Report struct {
	Entries int `json:"entries"`
	Assets  int `json:"assets"`
}

type Query struct {
	Language string
	Kind     string
	Text     string
	Limit    int
}

type AssetReader interface {
	ReadAssets(context.Context, []string) (map[string]Asset, error)
}

func validKind(kind string, asset bool) bool {
	switch kind {
	case "location", "character", "monster", "item":
		return true
	case "situation":
		return !asset
	}
	return false
}

func validText(text string, max int, multiline bool) bool {
	if strings.TrimSpace(text) == "" || utf8.RuneCountInString(text) > max {
		return false
	}
	for _, r := range text {
		if unicode.In(r, unicode.Cf) || (unicode.IsControl(r) && !(multiline && (r == '\n' || r == '\t'))) {
			return false
		}
	}
	return true
}

func ValidateEntry(e Entry) error {
	if !identifier.MatchString(e.ID) || !validKind(e.Kind, false) || (e.Language != "en" && e.Language != "fr") ||
		!validText(e.Title, 120, false) || !validText(e.Text, 2000, true) || len(e.Tags) > 16 || len(e.Metadata) > 32 || len(e.Images) > 8 {
		return locale.Errorf("invalid catalogue entry: %s", e.ID)
	}
	seen := map[string]bool{}
	for _, tag := range e.Tags {
		if !validText(tag, 32, false) || seen[tag] {
			return locale.Errorf("invalid catalogue entry: %s", e.ID)
		}
		seen[tag] = true
	}
	for key, value := range e.Metadata {
		if !identifier.MatchString(key) || !validText(value, 256, false) {
			return locale.Errorf("invalid catalogue entry: %s", e.ID)
		}
	}
	seen = map[string]bool{}
	for _, id := range e.Images {
		if !identifier.MatchString(id) || seen[id] {
			return locale.Errorf("invalid catalogue entry: %s", e.ID)
		}
		seen[id] = true
	}
	return nil
}

func validDimensions(width, height int) bool {
	return width > 0 && height > 0 && width <= 4096 && height <= 4096 && int64(width)*int64(height) <= MaxPixels
}

func CheckImage(config image.Config) error {
	if !validDimensions(config.Width, config.Height) {
		return locale.Errorf("catalogue image exceeds size limits")
	}
	return nil
}
