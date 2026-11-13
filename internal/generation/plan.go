// Package generation creates validated authored worlds from a catalogue-backed
// LLM plan. It operates before gameplay and never reads a player's memory cache.
package generation

import (
	"encoding/json"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"

	"ai-venture/internal/catalogue"
	"ai-venture/internal/locale"
	"ai-venture/internal/strictjson"
)

type Placement struct {
	Entry    string `json:"entry"`
	Location string `json:"location"`
}

type Step struct {
	Tool     string `json:"tool"`
	Target   string `json:"target"`
	Location string `json:"location"`
	Message  string `json:"message"`
}

// Locations form a bidirectional route. Steps are ordered, engine-enforced
// interactions; every ID refers to a retrieved catalogue entry.
type Plan struct {
	Title        string      `json:"title"`
	Introduction string      `json:"introduction"`
	Locations    []string    `json:"locations"`
	Entities     []Placement `json:"entities"`
	Steps        []Step      `json:"steps"`
	Situation    string      `json:"situation"`
}

func decode(data string) (Plan, error) {
	var p Plan
	if len(data) > 32768 {
		return p, locale.Errorf("generation proposal is too large")
	}
	if err := strictjson.Decode([]byte(data), &p); err != nil {
		return p, err
	}
	// All fields, even empty strings/arrays, are required at every level.
	var supplied, complete any
	_ = json.Unmarshal([]byte(data), &supplied)
	b, _ := json.Marshal(p)
	_ = json.Unmarshal(b, &complete)
	if !reflect.DeepEqual(supplied, complete) {
		return p, locale.Errorf("generation proposal has missing fields")
	}
	return p, nil
}

func validText(s string, limit int) bool {
	if strings.TrimSpace(s) == "" || utf8.RuneCountInString(s) > limit {
		return false
	}
	for _, r := range s {
		if unicode.In(r, unicode.Cf) || (unicode.IsControl(r) && r != '\n' && r != '\t') {
			return false
		}
	}
	return true
}

func schema(entries []catalogue.Entry, fixed *Plan) json.RawMessage {
	str := func(n int) map[string]any { return map[string]any{"type": "string", "minLength": 1, "maxLength": n} }
	obj := func(properties map[string]any, required ...string) map[string]any {
		return map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": required}
	}
	arr := func(item any, min, max int) map[string]any {
		return map[string]any{"type": "array", "items": item, "minItems": min, "maxItems": max}
	}
	places, entities, tools, situations := []string{}, []string{}, []string{}, []string{""}
	for _, e := range entries {
		switch e.Kind {
		case "location":
			places = append(places, e.ID)
		case "item":
			entities = append(entities, e.ID)
			tools = append(tools, e.ID)
		case "monster", "character":
			entities = append(entities, e.ID)
		case "situation":
			situations = append(situations, e.ID)
		}
	}
	choice := func(values []string) map[string]any { return map[string]any{"type": "string", "enum": values} }
	if fixed != nil {
		places = fixed.Locations
		entities, tools = []string{}, []string{}
		for _, placed := range fixed.Entities {
			entities = append(entities, placed.Entry)
			for _, e := range entries {
				if e.ID == placed.Entry && e.Kind == "item" {
					tools = append(tools, e.ID)
				}
			}
		}
	}
	// Encode the tool/target relation as well as the ID vocabulary. Small local
	// models otherwise tend to use a book on itself instead of reading it.
	alternatives := []any{}
	for _, tool := range tools {
		targets := []string{""}
		for _, entity := range entities {
			if entity != tool {
				targets = append(targets, entity)
			}
		}
		if fixed == nil {
			alternatives = append(alternatives, obj(map[string]any{"tool": choice([]string{tool}), "target": choice(targets), "location": choice(places), "message": str(600)}, "tool", "target", "location", "message"))
			continue
		}
		// Once placements are valid, fix them during repair and encode the
		// location of each fixed target. The engine still revalidates everything.
		for _, target := range targets {
			locations := places
			for _, e := range entries {
				if e.ID == target && (e.Kind == "monster" || e.Kind == "character") {
					for _, placed := range fixed.Entities {
						if placed.Entry == target {
							locations = []string{placed.Location}
						}
					}
				}
			}
			alternatives = append(alternatives, obj(map[string]any{"tool": choice([]string{tool}), "target": choice([]string{target}), "location": choice(locations), "message": str(600)}, "tool", "target", "location", "message"))
		}
	}
	var step any = map[string]any{"anyOf": alternatives}
	if len(alternatives) == 1 {
		step = alternatives[0]
	}
	properties := map[string]any{
		"title": str(120), "introduction": str(1200), "situation": choice(situations),
		"locations": arr(choice(places), 1, min(4, len(places))),
		"entities":  arr(obj(map[string]any{"entry": choice(entities), "location": choice(places)}, "entry", "location"), 1, min(12, len(entities))),
		"steps":     arr(step, 1, 6),
	}
	if fixed != nil {
		properties["locations"] = map[string]any{"const": fixed.Locations}
		properties["entities"] = map[string]any{"const": fixed.Entities}
	}
	b, _ := json.Marshal(obj(properties, "title", "introduction", "locations", "entities", "steps", "situation"))
	return b
}

// This only selects a stricter repair schema. compile remains the authority and
// checks all rules again, including when a provider ignores response schemas.
func usableLayout(p Plan, entries []catalogue.Entry) bool {
	if len(p.Locations) < 1 || len(p.Locations) > 4 || len(p.Entities) < 1 || len(p.Entities) > 12 {
		return false
	}
	kinds := map[string]string{}
	for _, e := range entries {
		kinds[e.ID] = e.Kind
	}
	locations, entities, counts := map[string]bool{}, map[string]bool{}, map[string]int{}
	for _, id := range p.Locations {
		if kinds[id] != "location" || locations[id] {
			return false
		}
		locations[id] = true
	}
	hasTool := false
	for _, e := range p.Entities {
		if !isEntity(kinds[e.Entry]) || entities[e.Entry] || !locations[e.Location] {
			return false
		}
		entities[e.Entry] = true
		counts[e.Location]++
		if counts[e.Location] > 4 {
			return false
		}
		hasTool = hasTool || kinds[e.Entry] == "item"
	}
	return hasTool
}
