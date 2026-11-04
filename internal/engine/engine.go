// Package engine owns game state and validates all proposed actions.
package engine

import (
	"math"
	"sort"
	"strings"

	"ai-venture/internal/locale"
	"ai-venture/internal/world"
)

type Action struct {
	Kind   string `json:"action"`
	Target string `json:"target"`
	Tool   string `json:"tool"`
}

func (a Action) Validate() error {
	if len(a.Target) > 64 || len(a.Tool) > 64 {
		return locale.Errorf("ID is too long")
	}
	switch a.Kind {
	case "move", "take", "drop":
		if a.Target == "" || a.Tool != "" {
			return locale.Errorf("target required, without a tool")
		}
	case "observe":
		if a.Tool != "" {
			return locale.Errorf("observation does not take a tool")
		}
	case "use":
		if a.Tool == "" {
			return locale.Errorf("tool required")
		}
	case "inventory", "unknown":
		if a.Tool != "" || a.Target != "" {
			return locale.Errorf("this action does not take parameters")
		}
	default:
		return locale.Errorf("unsupported action: %s", a.Kind)
	}
	return nil
}

type State struct {
	Location      string            `json:"location"`
	Turn          uint64            `json:"turn"`
	ItemLocations map[string]string `json:"item_locations"`
	Flags         map[string]bool   `json:"flags"`
}

func (s State) Clone() State {
	copy := State{Location: s.Location, Turn: s.Turn, ItemLocations: map[string]string{}, Flags: map[string]bool{}}
	for k, v := range s.ItemLocations {
		copy.ItemLocations[k] = v
	}
	for k, v := range s.Flags {
		copy.Flags[k] = v
	}
	return copy
}

type VisibleItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Portable    bool   `json:"portable"`
}

type VisibleExit struct {
	Direction   string `json:"direction"`
	Destination string `json:"destination"`
	Blocked     bool   `json:"blocked"`
}

type View struct {
	Location    string        `json:"location"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Exits       []VisibleExit `json:"exits"`
	Items       []VisibleItem `json:"items"`
	Inventory   []VisibleItem `json:"inventory"`
	Completed   bool          `json:"completed"`
}

type Result struct {
	Action  Action `json:"action"`
	Message string `json:"message"`
	View    View   `json:"view"`
}

type Engine struct {
	definition  world.Definition
	state       State
	fingerprint string
	language    locale.Language
}

func New(w world.Definition) (*Engine, error) {
	return NewWithLanguage(w, locale.English)
}

func NewWithLanguage(w world.Definition, language locale.Language) (*Engine, error) {
	selected, err := locale.Parse(string(language))
	if err != nil {
		return nil, err
	}
	if err := world.Validate(w); err != nil {
		return nil, err
	}
	w = world.Clone(w)
	s := State{Location: w.Start, ItemLocations: map[string]string{}, Flags: map[string]bool{}}
	for id, item := range w.Items {
		s.ItemLocations[id] = item.Location
	}
	for flag, value := range w.Flags {
		s.Flags[flag] = value
	}
	return &Engine{definition: w, state: s, fingerprint: world.Fingerprint(w), language: selected}, nil
}

func (e *Engine) Language() locale.Language { return e.language }

func (e *Engine) ScenarioID() string  { return e.definition.ID }
func (e *Engine) Fingerprint() string { return e.fingerprint }
func (e *Engine) Snapshot() State     { return e.state.Clone() }

func (e *Engine) Restore(s State) error {
	if err := ValidateState(e.definition, s); err != nil {
		return err
	}
	e.state = s.Clone()
	return nil
}

func ValidateState(w world.Definition, s State) error {
	if _, ok := w.Locations[s.Location]; !ok {
		return locale.Errorf("unknown current location")
	}
	if s.Turn == math.MaxUint64 {
		return locale.Errorf("turn count is out of range")
	}
	if len(s.ItemLocations) != len(w.Items) {
		return locale.Errorf("inconsistent item list")
	}
	for id, item := range w.Items {
		position, ok := s.ItemLocations[id]
		if !ok {
			return locale.Errorf("missing item: %s", id)
		}
		if _, ok := w.Locations[position]; position != world.Inventory && !ok {
			return locale.Errorf("invalid position: %s", id)
		}
		if !item.Portable && position != item.Location {
			return locale.Errorf("fixed item moved: %s", id)
		}
	}
	if len(s.Flags) != len(w.Flags) {
		return locale.Errorf("inconsistent flag list")
	}
	for flag := range w.Flags {
		if _, ok := s.Flags[flag]; !ok {
			return locale.Errorf("missing flag: %s", flag)
		}
	}
	return nil
}

func (e *Engine) accessible(id string) bool {
	position, ok := e.state.ItemLocations[id]
	return ok && (position == e.state.Location || position == world.Inventory)
}

func (e *Engine) Apply(a Action) (Result, error) {
	if err := a.Validate(); err != nil {
		return Result{}, err
	}
	next := e.state.Clone()
	message := ""
	mutates := false
	switch a.Kind {
	case "unknown":
		return Result{}, locale.Errorf("command not understood; try 'help'")
	case "observe":
		if a.Target == "" || a.Target == e.state.Location {
			message = e.definition.Locations[e.state.Location].Description
		} else {
			if !e.accessible(a.Target) {
				return Result{}, locale.Errorf("that item is not visible here")
			}
			message = e.definition.Items[a.Target].Description
		}
	case "inventory":
		names := []string{}
		for _, item := range e.View().Inventory {
			names = append(names, item.Name)
		}
		message = e.language.Text("Your inventory is empty.")
		if len(names) > 0 {
			message = e.language.Text("You are carrying: %s.", strings.Join(names, ", "))
		}
	case "move":
		exit, ok := e.definition.Locations[e.state.Location].Exits[a.Target]
		if !ok {
			return Result{}, locale.Errorf("there is no exit in that direction")
		}
		if !world.Matches(e.state.Flags, exit.Conditions) {
			message = exit.BlockedMessage
			if message == "" {
				message = e.language.Text("This passage is blocked.")
			}
			return Result{}, locale.Errorf("%s", message)
		}
		next.Location = exit.To
		message = e.language.Text("You arrive at %s.", e.definition.Locations[exit.To].Name)
		mutates = true
	case "take":
		item, ok := e.definition.Items[a.Target]
		if !ok || e.state.ItemLocations[a.Target] != e.state.Location {
			return Result{}, locale.Errorf("that item is not available here")
		}
		if !item.Portable {
			return Result{}, locale.Errorf("that item is fixed in place")
		}
		next.ItemLocations[a.Target] = world.Inventory
		message = e.language.Text("You take %s.", item.Name)
		mutates = true
	case "drop":
		if e.state.ItemLocations[a.Target] != world.Inventory {
			return Result{}, locale.Errorf("you are not carrying that item")
		}
		next.ItemLocations[a.Target] = e.state.Location
		message = e.language.Text("You drop %s.", e.definition.Items[a.Target].Name)
		mutates = true
	case "use":
		tool, ok := e.definition.Items[a.Tool]
		if !ok || !e.accessible(a.Tool) {
			return Result{}, locale.Errorf("that tool is not accessible")
		}
		if tool.Portable && e.state.ItemLocations[a.Tool] != world.Inventory {
			return Result{}, locale.Errorf("take that item first")
		}
		if a.Target != "" && !e.accessible(a.Target) {
			return Result{}, locale.Errorf("that target is not accessible")
		}
		var match *world.UseRule
		for i := range e.definition.Rules {
			rule := &e.definition.Rules[i]
			if rule.Tool == a.Tool && rule.Target == a.Target && rule.Location == e.state.Location {
				match = rule
				break
			}
		}
		if match == nil {
			return Result{}, locale.Errorf("that use has no defined effect here")
		}
		if !world.Matches(e.state.Flags, match.Conditions) {
			return Result{}, locale.Errorf("%s", match.BlockedMessage)
		}
		for flag, value := range match.Effects {
			next.Flags[flag] = value
		}
		message = match.Message
		mutates = true
	}
	if mutates {
		next.Turn++
	}
	if err := ValidateState(e.definition, next); err != nil {
		return Result{}, err
	}
	e.state = next
	return Result{Action: a, Message: message, View: e.View()}, nil
}

func (e *Engine) View() View {
	loc := e.definition.Locations[e.state.Location]
	v := View{Location: e.state.Location, Name: loc.Name, Description: loc.Description,
		Items: []VisibleItem{}, Inventory: []VisibleItem{}, Exits: []VisibleExit{}, Completed: world.Matches(e.state.Flags, e.definition.Goal)}
	for direction, exit := range loc.Exits {
		v.Exits = append(v.Exits, VisibleExit{direction, e.definition.Locations[exit.To].Name, !world.Matches(e.state.Flags, exit.Conditions)})
	}
	for id, item := range e.definition.Items {
		visible := VisibleItem{id, item.Name, item.Description, item.Portable}
		switch e.state.ItemLocations[id] {
		case e.state.Location:
			v.Items = append(v.Items, visible)
		case world.Inventory:
			v.Inventory = append(v.Inventory, visible)
		}
	}
	sort.Slice(v.Items, func(i, j int) bool { return v.Items[i].ID < v.Items[j].ID })
	sort.Slice(v.Inventory, func(i, j int) bool { return v.Inventory[i].ID < v.Inventory[j].ID })
	sort.Slice(v.Exits, func(i, j int) bool { return v.Exits[i].Direction < v.Exits[j].Direction })
	return v
}
