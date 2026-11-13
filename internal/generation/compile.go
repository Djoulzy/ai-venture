package generation

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"ai-venture/internal/catalogue"
	"ai-venture/internal/engine"
	"ai-venture/internal/locale"
	"ai-venture/internal/world"
)

func compile(ctx context.Context, p Plan, entries []catalogue.Entry, store *catalogue.Store, lang locale.Language, seed int64) (Result, error) {
	if !validText(p.Title, 120) || !validText(p.Introduction, 1200) || len(p.Locations) < 1 || len(p.Locations) > 4 || len(p.Entities) < 1 || len(p.Entities) > 12 || len(p.Steps) < 1 || len(p.Steps) > 6 {
		return Result{}, invalid(lang, "invalid plan size or text")
	}
	known := map[string]catalogue.Entry{}
	for _, e := range entries {
		known[e.ID] = e
	}
	identity, _ := json.Marshal(struct {
		Plan     Plan
		Language locale.Language
		Seed     int64
	}{p, lang, seed})
	hash := sha256.Sum256(identity)
	w := world.Definition{Version: world.FormatVersion, ID: fmt.Sprintf("generated_%s_%x", lang, hash[:10]), Title: p.Title, Introduction: p.Introduction, Start: p.Locations[0], Locations: map[string]world.Location{}, Items: map[string]world.Item{}, Flags: map[string]bool{}, Rules: []world.UseRule{}, Goal: []world.Condition{}, Provenance: &world.Provenance{Generator: "catalogue-planner-v1", Seed: seed}}
	forward, backward := "forward", "back"
	if lang == locale.French {
		forward, backward = "avant", "arriere"
	}
	used := []catalogue.Entry{}
	for i, id := range p.Locations {
		e, ok := known[id]
		if !ok || e.Kind != "location" {
			return Result{}, invalid(lang, "unknown location reference")
		}
		if _, ok := w.Locations[id]; ok {
			return Result{}, invalid(lang, "duplicate location reference")
		}
		exits := map[string]world.Exit{}
		if i > 0 {
			exits[backward] = world.Exit{To: p.Locations[i-1]}
		}
		if i+1 < len(p.Locations) {
			exits[forward] = world.Exit{To: p.Locations[i+1]}
		}
		w.Locations[id] = world.Location{Name: e.Title, Description: e.Text, Exits: exits}
		used = append(used, e)
	}
	counts := map[string]int{}
	for _, placement := range p.Entities {
		e, ok := known[placement.Entry]
		if !ok || !isEntity(e.Kind) {
			return Result{}, invalid(lang, "unknown entity reference")
		}
		if _, ok := w.Locations[placement.Location]; !ok {
			return Result{}, invalid(lang, "entity outside selected locations")
		}
		if _, ok := w.Items[e.ID]; ok {
			return Result{}, invalid(lang, "duplicate entity reference")
		}
		counts[placement.Location]++
		if counts[placement.Location] > 4 {
			return Result{}, invalid(lang, "too many entities in one location")
		}
		w.Items[e.ID] = world.Item{Name: e.Title, Description: e.Text, Portable: e.Kind == "item", Location: placement.Location}
		used = append(used, e)
	}
	interactions := map[[3]string]int{}
	for i, s := range p.Steps {
		key := [3]string{s.Location, s.Tool, s.Target}
		if previous, exists := interactions[key]; exists {
			return Result{}, locale.Errorf("steps %d and %d repeat the same location/tool/target; merge them into a single use and remove the duplicate", previous+1, i+1)
		}
		interactions[key] = i
		tool, ok := w.Items[s.Tool]
		if !ok || !tool.Portable {
			return Result{}, invalid(lang, "step tool must be a selected portable item")
		}
		if _, ok := w.Locations[s.Location]; !ok {
			return Result{}, invalid(lang, "step outside selected locations")
		}
		if s.Target != "" {
			target, ok := w.Items[s.Target]
			if s.Tool == s.Target {
				return Result{}, locale.Errorf("step %d targets its own tool; use an empty target to read or activate an item", i+1)
			}
			if !ok {
				return Result{}, locale.Errorf("step %d targets an entity absent from entities", i+1)
			}
			if !target.Portable && target.Location != s.Location {
				return Result{}, locale.Errorf("step %d must occur at its fixed target location: %s", i+1, target.Location)
			}
		}
		if !validText(s.Message, 600) {
			return Result{}, locale.Errorf("step %d needs a nonempty message of at most 600 characters without control characters", i+1)
		}
		flag := stepFlag(i)
		w.Flags[flag] = false
		conditions := []world.Condition{{Flag: flag, Value: false}}
		if i > 0 {
			conditions = append(conditions, world.Condition{Flag: stepFlag(i - 1), Value: true})
		}
		w.Rules = append(w.Rules, world.UseRule{ID: fmt.Sprintf("step_%d", i+1), Tool: s.Tool, Target: s.Target, Location: s.Location, Conditions: conditions, Effects: map[string]bool{flag: true}, Message: s.Message, BlockedMessage: lang.Text("Complete the previous step first; this interaction can only succeed once.")})
	}
	w.Goal = []world.Condition{{Flag: stepFlag(len(p.Steps) - 1), Value: true}}
	if p.Situation != "" {
		e, ok := known[p.Situation]
		if !ok || e.Kind != "situation" {
			return Result{}, invalid(lang, "unknown situation reference")
		}
		used = append(used, e)
	}
	assetIDs := []string{}
	for _, e := range used {
		if e.Kind != "situation" && len(e.Images) > 0 {
			assetIDs = append(assetIDs, e.Images[0])
		}
	}
	read, cancel := context.WithTimeout(ctx, 3*time.Second)
	assets, err := store.ReadAssets(read, assetIDs)
	cancel()
	if err != nil {
		return Result{}, err
	}
	positions := map[string]int{}
	for i, e := range used {
		ref := world.DocumentRef{ID: fmt.Sprintf("catalogue_%d", i), Entry: e.ID}
		switch e.Kind {
		case "location":
			ref.Location = e.ID
		case "situation":
			ref.Location = w.Start
		default:
			ref.Location = w.Items[e.ID].Location
			ref.Item = e.ID
		}
		w.CatalogueDocuments = append(w.CatalogueDocuments, ref)
		if e.Kind == "situation" || len(e.Images) == 0 {
			continue
		}
		a := assets[e.Images[0]]
		if a.Kind != e.Kind {
			return Result{}, invalid(lang, "catalogue image kind does not match its entry")
		}
		layer := world.VisualLayer{Asset: a.ID, Location: ref.Location, Width: 100, Height: 100}
		if e.Kind != "location" {
			width, height := 20, int(math.Round(20.0*640/384*float64(a.Height)/float64(a.Width)))
			if height > 55 {
				height = 55
				width = int(math.Round(55.0 * 384 / 640 * float64(a.Width) / float64(a.Height)))
			}
			layer.Item = e.ID
			layer.Width = max(1, width)
			layer.Height = max(1, height)
			layer.X = 3 + positions[ref.Location]*24
			layer.Y = 95 - layer.Height
			layer.Z = 10 + positions[ref.Location]
			positions[ref.Location]++
		}
		w.Visuals = append(w.Visuals, layer)
	}
	if err := world.Validate(w); err != nil {
		return Result{}, err
	}
	bind, cancel := context.WithTimeout(ctx, 3*time.Second)
	w, err = store.Bind(bind, w, lang)
	cancel()
	if err != nil {
		return Result{}, err
	}
	e, err := engine.NewWithLanguage(w, lang)
	if err != nil {
		return Result{}, err
	}
	if e.View().Completed {
		return Result{}, invalid(lang, "objective is already complete")
	}
	solution := []engine.Action{}
	apply := func(a engine.Action) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := e.Apply(a); err != nil {
			return err
		}
		solution = append(solution, a)
		return nil
	}
	at := 0
	move := func(to int) error {
		for at != to {
			direction := forward
			next := at + 1
			if at > to {
				direction = backward
				next = at - 1
			}
			if err := apply(engine.Action{Kind: "move", Target: direction}); err != nil {
				return err
			}
			at = next
		}
		return nil
	}
	// Construct and replay a witness using the real rules. Nothing mutates the
	// returned scenario or a player session. No solver path is sent to narration.
	for i, id := range p.Locations {
		if err := move(i); err != nil {
			return Result{}, err
		}
		for _, placement := range p.Entities {
			if placement.Location == id && w.Items[placement.Entry].Portable {
				if err := apply(engine.Action{Kind: "take", Target: placement.Entry}); err != nil {
					return Result{}, err
				}
			}
		}
	}
	for _, s := range p.Steps {
		for i, id := range p.Locations {
			if id == s.Location {
				if err := move(i); err != nil {
					return Result{}, err
				}
				break
			}
		}
		if err := apply(engine.Action{Kind: "use", Tool: s.Tool, Target: s.Target}); err != nil {
			return Result{}, err
		}
	}
	if !e.View().Completed {
		return Result{}, invalid(lang, "objective cannot be completed")
	}
	return Result{World: w, Solution: solution}, nil
}
