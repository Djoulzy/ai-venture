// Package world defines immutable, declarative adventure scenarios.
package world

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"regexp"
	"strings"

	"ai-venture/internal/locale"
	"ai-venture/internal/strictjson"
)

const FormatVersion = 1
const Inventory = "@inventory"
const MaxBytes = 2 << 20

type Definition struct {
	Version      int                 `json:"version"`
	ID           string              `json:"id"`
	Title        string              `json:"title"`
	Introduction string              `json:"introduction"`
	Start        string              `json:"start"`
	Locations    map[string]Location `json:"locations"`
	Items        map[string]Item     `json:"items"`
	Flags        map[string]bool     `json:"flags"`
	Rules        []UseRule           `json:"rules"`
	Goal         []Condition         `json:"goal"`
	Provenance   *Provenance         `json:"provenance,omitempty"`
}

type Provenance struct {
	Generator string `json:"generator"`
	Seed      int64  `json:"seed"`
}

type Location struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Exits       map[string]Exit `json:"exits"`
}

type Exit struct {
	To             string      `json:"to"`
	Conditions     []Condition `json:"conditions,omitempty"`
	BlockedMessage string      `json:"blocked_message,omitempty"`
}

type Item struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Portable    bool   `json:"portable"`
	Location    string `json:"location"`
}

type Condition struct {
	Flag  string `json:"flag"`
	Value bool   `json:"value"`
}

type UseRule struct {
	ID             string          `json:"id"`
	Tool           string          `json:"tool"`
	Target         string          `json:"target"`
	Location       string          `json:"location"`
	Conditions     []Condition     `json:"conditions,omitempty"`
	Effects        map[string]bool `json:"effects"`
	Message        string          `json:"message"`
	BlockedMessage string          `json:"blocked_message"`
}

// Source is the extension point for authored files and future seeded generators.
type Source interface {
	Load(context.Context) (Definition, error)
}

type FileSource struct{ Path string }

func (s FileSource) Load(ctx context.Context) (Definition, error) {
	if err := ctx.Err(); err != nil {
		return Definition{}, err
	}
	f, err := os.Open(s.Path)
	if err != nil {
		return Definition{}, err
	}
	defer f.Close()
	return Load(f)
}

func Load(r io.Reader) (Definition, error) {
	var w Definition
	b, err := io.ReadAll(io.LimitReader(r, MaxBytes+1))
	if err != nil {
		return w, err
	}
	if len(b) > MaxBytes {
		return w, locale.Errorf("scenario is too large")
	}
	if err := strictjson.Decode(b, &w); err != nil {
		return w, err
	}
	return w, Validate(w)
}

func Clone(w Definition) Definition {
	b, _ := json.Marshal(w)
	var copy Definition
	_ = json.Unmarshal(b, &copy)
	return copy
}

func Fingerprint(w Definition) string {
	b, _ := json.Marshal(w)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func Matches(flags map[string]bool, conditions []Condition) bool {
	for _, c := range conditions {
		v, ok := flags[c.Flag]
		if !ok || v != c.Value {
			return false
		}
	}
	return true
}

var identifier = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

func Validate(w Definition) error {
	if w.Version != FormatVersion {
		return locale.Errorf("unsupported scenario version: %d", w.Version)
	}
	if !identifier.MatchString(w.ID) || strings.TrimSpace(w.Title) == "" {
		return locale.Errorf("invalid scenario identity")
	}
	if _, ok := w.Locations[w.Start]; !ok {
		return locale.Errorf("unknown starting location")
	}
	checkConditions := func(cs []Condition) error {
		seen := map[string]bool{}
		for _, c := range cs {
			if _, ok := w.Flags[c.Flag]; !ok || seen[c.Flag] {
				return locale.Errorf("unknown or duplicate condition: %s", c.Flag)
			}
			seen[c.Flag] = true
		}
		return nil
	}
	for flag := range w.Flags {
		if !identifier.MatchString(flag) {
			return locale.Errorf("invalid flag: %s", flag)
		}
	}
	for id, loc := range w.Locations {
		if !identifier.MatchString(id) || strings.TrimSpace(loc.Name) == "" || strings.TrimSpace(loc.Description) == "" {
			return locale.Errorf("invalid location: %s", id)
		}
		for direction, exit := range loc.Exits {
			if !identifier.MatchString(direction) {
				return locale.Errorf("invalid direction: %s", direction)
			}
			if _, ok := w.Locations[exit.To]; !ok {
				return locale.Errorf("exit to an unknown location: %s", exit.To)
			}
			if err := checkConditions(exit.Conditions); err != nil {
				return err
			}
		}
	}
	for id, item := range w.Items {
		if !identifier.MatchString(id) || strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.Description) == "" {
			return locale.Errorf("invalid item: %s", id)
		}
		if _, exists := w.Locations[id]; exists {
			return locale.Errorf("ID shared by a location and an item: %s", id)
		}
		if _, ok := w.Locations[item.Location]; !ok {
			return locale.Errorf("unknown initial position: %s", id)
		}
	}
	ruleIDs, interactions := map[string]bool{}, map[string]bool{}
	for _, rule := range w.Rules {
		if !identifier.MatchString(rule.ID) || ruleIDs[rule.ID] {
			return locale.Errorf("invalid or duplicate rule: %s", rule.ID)
		}
		ruleIDs[rule.ID] = true
		key := rule.Location + "/" + rule.Tool + "/" + rule.Target
		if interactions[key] {
			return locale.Errorf("ambiguous interaction: %s", key)
		}
		interactions[key] = true
		if _, ok := w.Items[rule.Tool]; !ok {
			return locale.Errorf("unknown tool: %s", rule.Tool)
		}
		if _, ok := w.Items[rule.Target]; rule.Target != "" && !ok {
			return locale.Errorf("unknown target: %s", rule.Target)
		}
		if _, ok := w.Locations[rule.Location]; !ok {
			return locale.Errorf("unknown rule location")
		}
		if err := checkConditions(rule.Conditions); err != nil {
			return err
		}
		if len(rule.Effects) == 0 || strings.TrimSpace(rule.Message) == "" || strings.TrimSpace(rule.BlockedMessage) == "" {
			return locale.Errorf("incomplete rule: %s", rule.ID)
		}
		for flag := range rule.Effects {
			if _, ok := w.Flags[flag]; !ok {
				return locale.Errorf("effect on an unknown flag: %s", flag)
			}
		}
	}
	if len(w.Goal) == 0 {
		return locale.Errorf("missing objective")
	}
	if err := checkConditions(w.Goal); err != nil {
		return err
	}
	seen := map[string]bool{}
	queue := []string{w.Start}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if seen[id] {
			continue
		}
		seen[id] = true
		for _, exit := range w.Locations[id].Exits {
			queue = append(queue, exit.To)
		}
	}
	if len(seen) != len(w.Locations) {
		return locale.Errorf("a location cannot be reached from the starting location")
	}
	return nil
}
