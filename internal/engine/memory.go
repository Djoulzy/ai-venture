package engine

import (
	"fmt"
	"sort"

	"ai-venture/internal/locale"
	"ai-venture/internal/world"
)

const MaxJournal = 128

// Event contains only engine-validated actions, never generated prose.
type Event struct {
	Turn     uint64 `json:"turn"`
	Location string `json:"location"`
	Action   Action `json:"action"`
}

// MemoryEntry is a public fact, with no discovery conditions or secret flags.
type MemoryEntry struct {
	ID    string
	Kind  string
	Title string
	Text  string
	Turn  uint64
}

func (e *Engine) discover(s *State) {
	known := map[string]bool{}
	for _, id := range s.Knowledge {
		known[id] = true
	}
	for _, doc := range e.definition.Documents {
		if known[doc.ID] || (doc.Location != "" && doc.Location != s.Location) || !world.Matches(s.Flags, doc.Conditions) {
			continue
		}
		if doc.Item != "" && s.ItemLocations[doc.Item] != s.Location && s.ItemLocations[doc.Item] != world.Inventory {
			continue
		}
		s.Knowledge = append(s.Knowledge, doc.ID)
	}
	sort.Strings(s.Knowledge)
}

// Memory returns copies of learned lore in the selected language and historical
// engine facts. Only this approved corpus may be sent to the retrieval backend.
func (e *Engine) Memory() []MemoryEntry {
	entries := []MemoryEntry{}
	known := map[string]bool{}
	for _, id := range e.state.Knowledge {
		known[id] = true
	}
	for _, doc := range e.definition.Documents {
		if known[doc.ID] && doc.Language == string(e.language) {
			entries = append(entries, MemoryEntry{ID: "lore:" + doc.ID, Kind: "lore", Title: doc.Title, Text: doc.Text})
		}
	}
	for _, event := range e.state.Journal {
		text := ""
		switch event.Action.Kind {
		case "move":
			text = e.language.Text("You arrive at %s.", e.definition.Locations[event.Location].Name)
		case "take":
			text = e.language.Text("You take %s.", e.definition.Items[event.Action.Target].Name)
		case "drop":
			text = e.language.Text("You drop %s.", e.definition.Items[event.Action.Target].Name)
		case "use":
			for _, rule := range e.definition.Rules {
				if rule.Tool == event.Action.Tool && rule.Target == event.Action.Target && rule.Location == event.Location {
					text = rule.Message
					break
				}
			}
		}
		entries = append(entries, MemoryEntry{ID: fmt.Sprintf("event:%d", event.Turn), Kind: "event", Title: e.language.Text("Turn %d", event.Turn), Text: text, Turn: event.Turn})
	}
	return entries
}

func validateMemory(w world.Definition, s State) error {
	ids := map[string]bool{}
	for _, doc := range w.Documents {
		ids[doc.ID] = true
	}
	seen := map[string]bool{}
	for _, id := range s.Knowledge {
		if !ids[id] || seen[id] {
			return locale.Errorf("invalid discovered document: %s", id)
		}
		seen[id] = true
	}
	if len(s.Journal) > MaxJournal {
		return locale.Errorf("game journal is too large")
	}
	var previous uint64
	for _, event := range s.Journal {
		if event.Turn <= previous || event.Turn > s.Turn {
			return locale.Errorf("invalid journal turn")
		}
		previous = event.Turn
		if _, ok := w.Locations[event.Location]; !ok {
			return locale.Errorf("invalid journal location")
		}
		if err := event.Action.Validate(); err != nil {
			return err
		}
		switch event.Action.Kind {
		case "move":
			found := false
			for _, loc := range w.Locations {
				if exit, ok := loc.Exits[event.Action.Target]; ok && exit.To == event.Location {
					found = true
				}
			}
			if !found {
				return locale.Errorf("invalid journal movement")
			}
		case "take", "drop":
			item, ok := w.Items[event.Action.Target]
			if !ok || !item.Portable {
				return locale.Errorf("invalid journal item")
			}
		case "use":
			found := false
			for _, rule := range w.Rules {
				if rule.Tool == event.Action.Tool && rule.Target == event.Action.Target && rule.Location == event.Location {
					found = true
				}
			}
			if !found {
				return locale.Errorf("invalid journal interaction")
			}
		default:
			return locale.Errorf("invalid journal action")
		}
	}
	return nil
}
