// Package retrieval defines a provider-independent search over engine-approved facts.
package retrieval

import "context"

const MaxDocuments = 640
const MaxPassages = 3
const MaxContextRunes = 1600

type Document struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"` // lore or event; events describe the past
	Title string `json:"title"`
	Text  string `json:"text"`
	Turn  uint64 `json:"turn,omitempty"`
}

type Query struct {
	Scenario  string // fingerprint, not only the scenario name
	Language  string
	Text      string
	Documents []Document // complete authorized corpus for this snapshot
}

// Search must replace its cached corpus with Documents before querying. Cached
// rows are never authority: new games and restored branches supply a new corpus.
// Implementations must bound returned context and keep scenario/language scopes separate.
type Searcher interface {
	Search(context.Context, Query) ([]Document, error)
}

func Clip(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	if limit < 2 {
		return ""
	}
	return string(runes[:limit-1]) + "…"
}
