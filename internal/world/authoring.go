package world

import "ai-venture/internal/locale"

// DocumentRef is resolved from the permanent catalogue before Engine creation.
// Discovery gates remain authored scenario rules, never catalogue/LLM decisions.
type DocumentRef struct {
	ID         string      `json:"id"`
	Entry      string      `json:"entry"`
	Location   string      `json:"location,omitempty"`
	Item       string      `json:"item,omitempty"`
	Conditions []Condition `json:"conditions,omitempty"`
}

// Coordinates are percentages of a fixed canvas. Source image alpha is retained.
// Item-gated layers disappear when the item is no longer present in the room.
type VisualLayer struct {
	Asset      string      `json:"asset"`
	Location   string      `json:"location"`
	Item       string      `json:"item,omitempty"`
	Conditions []Condition `json:"conditions,omitempty"`
	X          int         `json:"x"`
	Y          int         `json:"y"`
	Width      int         `json:"width"`
	Height     int         `json:"height"`
	Z          int         `json:"z"`
}

func validateAuthoring(w Definition, documentIDs map[string]bool, checkConditions func([]Condition) error) error {
	checkGates := func(location, item string, conditions []Condition) error {
		if _, ok := w.Locations[location]; location != "" && !ok {
			return locale.Errorf("unknown authoring location: %s", location)
		}
		if _, ok := w.Items[item]; item != "" && !ok {
			return locale.Errorf("unknown authoring item: %s", item)
		}
		return checkConditions(conditions)
	}
	for _, ref := range w.CatalogueDocuments {
		if !identifier.MatchString(ref.ID) || !identifier.MatchString(ref.Entry) || documentIDs[ref.ID] {
			return locale.Errorf("invalid catalogue document reference: %s", ref.ID)
		}
		documentIDs[ref.ID] = true
		if err := checkGates(ref.Location, ref.Item, ref.Conditions); err != nil {
			return err
		}
	}
	if len(w.Visuals) > 512 {
		return locale.Errorf("too many visual layers")
	}
	counts := map[string]int{}
	for _, layer := range w.Visuals {
		counts[layer.Location]++
		if !identifier.MatchString(layer.Asset) || layer.Location == "" || counts[layer.Location] > 32 || layer.X < 0 || layer.Y < 0 || layer.Width < 1 || layer.Height < 1 || layer.Width > 100 || layer.Height > 100 || layer.X > 100-layer.Width || layer.Y > 100-layer.Height || layer.Z < -1000 || layer.Z > 1000 {
			return locale.Errorf("invalid visual layer: %s", layer.Asset)
		}
		if err := checkGates(layer.Location, layer.Item, layer.Conditions); err != nil {
			return err
		}
	}
	return nil
}
