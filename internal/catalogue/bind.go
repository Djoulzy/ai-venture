package catalogue

import (
	"context"

	"ai-venture/internal/locale"
	"ai-venture/internal/world"
)

// Bind copies selected-language entries into ordinary discoverable documents.
// Only Engine.Memory() will subsequently decide what the narrator may retrieve.
func (s *Store) Bind(ctx context.Context, w world.Definition, language locale.Language) (world.Definition, error) {
	if err := world.Validate(w); err != nil {
		return world.Definition{}, err
	}
	result := world.Clone(w)
	seen := map[string]bool{}
	for _, layer := range result.Visuals {
		if seen[layer.Asset] {
			continue
		}
		seen[layer.Asset] = true
		var count int
		if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM assets WHERE id=?", layer.Asset).Scan(&count); err != nil {
			return world.Definition{}, err
		}
		if count == 0 {
			return world.Definition{}, locale.Errorf("unknown catalogue asset: %s", layer.Asset)
		}
	}
	for _, ref := range result.CatalogueDocuments {
		entry, err := s.Entry(ctx, ref.Entry, string(language))
		if err != nil {
			return world.Definition{}, locale.Errorf("reading catalogue entry %s: %w", ref.Entry, err)
		}
		result.Documents = append(result.Documents, world.Document{ID: ref.ID, Language: entry.Language, Title: entry.Title, Text: entry.Text, Location: ref.Location, Item: ref.Item, Conditions: ref.Conditions})
	}
	result.CatalogueDocuments = nil
	return result, world.Validate(result)
}
