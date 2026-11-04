package language

import (
	"context"
	"time"
	"unicode/utf8"

	"ai-venture/internal/engine"
	"ai-venture/internal/retrieval"
)

// RAGNarrator augments narration only. Action interpretation remains limited to
// the visible world. The index never receives the raw scenario or private flags.
type RAGNarrator struct {
	LLM     LLM
	Engine  *engine.Engine
	Search  retrieval.Searcher
	OnError func(error)
}

func (n RAGNarrator) Narrate(ctx context.Context, result engine.Result) (string, error) {
	corpus := []retrieval.Document{}
	approved := map[string]retrieval.Document{}
	for _, entry := range n.Engine.Memory() {
		doc := retrieval.Document{ID: entry.ID, Kind: entry.Kind, Title: retrieval.Clip(entry.Title, 120), Text: retrieval.Clip(entry.Text, 2000), Turn: entry.Turn}
		corpus = append(corpus, doc)
		approved[doc.ID] = doc
	}
	query := retrieval.Query{Scenario: n.Engine.Fingerprint(), Language: string(n.Engine.Language()), Text: result.Message + " " + result.View.Name, Documents: corpus}
	searchCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	passages, err := n.Search.Search(searchCtx, query)
	cancel()
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if n.OnError != nil {
			n.OnError(err)
		}
		return n.LLM.Narrate(ctx, result)
	}
	// Rehydrate by ID from the engine's approved copy: even a modified index or
	// replacement search backend cannot introduce new text, metadata or secrets.
	memory := []retrieval.Document{}
	seen := map[string]bool{}
	remaining := retrieval.MaxContextRunes
	for _, hit := range passages {
		doc, ok := approved[hit.ID]
		if !ok || seen[hit.ID] {
			continue
		}
		seen[hit.ID] = true
		budget := min(500, remaining-utf8.RuneCountInString(doc.Title))
		if budget <= 0 {
			break
		}
		doc.Text = retrieval.Clip(doc.Text, budget)
		remaining -= utf8.RuneCountInString(doc.Title) + utf8.RuneCountInString(doc.Text)
		memory = append(memory, doc)
		if len(memory) == retrieval.MaxPassages {
			break
		}
	}
	return n.LLM.narrate(ctx, result, memory)
}
