package language

import (
	"context"
	"strings"

	"ai-venture/internal/engine"
	"ai-venture/internal/locale"
)

// Offline is a small grammar in the selected language, also used for recovery.
type Offline struct{ Language locale.Language }

func Normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "?" {
		return s
	}
	s = strings.NewReplacer("é", "e", "è", "e", "ê", "e", "ë", "e", "à", "a", "â", "a", "ä", "a", "î", "i", "ï", "i", "ô", "o", "ö", "o", "ù", "u", "û", "u", "ü", "u", "ç", "c", "œ", "oe", "’", "'").Replace(s)
	return strings.Join(strings.Fields(strings.Trim(s, ".!? ")), " ")
}

func (o Offline) noun(s string) string {
	prefixes := []string{"the ", "an ", "a "}
	if o.Language == locale.French {
		prefixes = []string{"de la ", "de l'", "du ", "des ", "la ", "le ", "les ", "une ", "un ", "l'"}
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(s, prefix))
		}
	}
	return s
}

func (o Offline) resolve(text string, view engine.View) (string, error) {
	text = o.noun(strings.TrimSpace(text))
	matches := map[string]bool{}
	for _, items := range [][]engine.VisibleItem{view.Items, view.Inventory} {
		for _, item := range items {
			if text == Normalize(item.ID) || text == o.noun(Normalize(item.Name)) {
				matches[item.ID] = true
			}
		}
	}
	if len(matches) == 1 {
		for id := range matches {
			return id, nil
		}
	}
	return "", locale.Errorf("unknown or ambiguous item: use a displayed ID")
}

func (o Offline) Interpret(ctx context.Context, command string, view engine.View) (engine.Action, error) {
	if err := ctx.Err(); err != nil {
		return engine.Action{}, err
	}
	if len(command) > MaxCommandBytes {
		return engine.Action{}, locale.Errorf("command is too long")
	}
	command = Normalize(command)
	if o.Language == locale.French {
		command = frenchCommand(command)
	}
	if command == "inventory" || command == "i" {
		return engine.Action{Kind: "inventory"}, nil
	}
	for _, exit := range view.Exits {
		if command == exit.Direction {
			return engine.Action{Kind: "move", Target: exit.Direction}, nil
		}
	}
	verb, rest, _ := strings.Cut(command, " ")
	a := engine.Action{}
	switch verb {
	case "look", "observe", "examine", "inspect":
		a.Kind = "observe"
		if rest == "" || rest == "around" || rest == "around me" || rest == "room" {
			return a, nil
		}
		rest = strings.TrimPrefix(rest, "at ")
	case "go", "move", "walk":
		rest = strings.TrimPrefix(rest, "to ")
		rest = strings.TrimPrefix(rest, "the ")
		return engine.Action{Kind: "move", Target: rest}, nil
	case "take", "get", "grab":
		a.Kind = "take"
	case "pick":
		if !strings.HasPrefix(rest, "up ") {
			return engine.Action{Kind: "unknown"}, nil
		}
		rest = strings.TrimPrefix(rest, "up ")
		a.Kind = "take"
	case "drop":
		a.Kind = "drop"
	case "use":
		tool, target, found := strings.Cut(rest, " on ")
		toolID, err := o.resolve(tool, view)
		if err != nil {
			return a, err
		}
		a = engine.Action{Kind: "use", Tool: toolID}
		if !found {
			return a, nil
		}
		a.Target, err = o.resolve(target, view)
		return a, err
	case "open", "unlock", "light":
		target, tool, found := strings.Cut(rest, " with ")
		if !found {
			return a, locale.Errorf("specify a tool: use <tool> on <target>")
		}
		toolID, err := o.resolve(tool, view)
		if err != nil {
			return a, err
		}
		targetID, err := o.resolve(target, view)
		return engine.Action{Kind: "use", Tool: toolID, Target: targetID}, err
	default:
		return engine.Action{Kind: "unknown"}, nil
	}
	id, err := o.resolve(rest, view)
	a.Target = id
	return a, err
}

// Translate grammar tokens only; item names and scenario directions stay intact.
func frenchCommand(command string) string {
	verb, rest, _ := strings.Cut(command, " ")
	switch verb {
	case "inventaire":
		if rest == "" {
			return "inventory"
		}
	case "regarder", "regarde", "observer", "observe", "examiner", "examine":
		verb = "look"
		if rest == "autour" || rest == "autour de moi" || rest == "lieu" {
			rest = "around"
		}
	case "aller", "va", "vais":
		verb = "go"
		for _, prefix := range []string{"vers le ", "vers ", "au ", "a l'"} {
			rest = strings.TrimPrefix(rest, prefix)
		}
	case "prendre", "prends", "ramasser", "ramasse":
		verb = "take"
	case "deposer", "depose", "poser", "pose":
		verb = "drop"
	case "utiliser", "utilise":
		verb, rest = "use", strings.Replace(rest, " sur ", " on ", 1)
	case "ouvrir", "ouvre", "allumer", "allume":
		verb, rest = "open", strings.Replace(rest, " avec ", " with ", 1)
	}
	return strings.TrimSpace(verb + " " + rest)
}
