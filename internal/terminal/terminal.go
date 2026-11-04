// Package terminal implements the sequential terminal game loop.
package terminal

import (
	"bufio"
	"context"
	"io"
	"strings"
	"time"
	"unicode"

	"ai-venture/internal/engine"
	"ai-venture/internal/illustration"
	"ai-venture/internal/language"
	"ai-venture/internal/locale"
	"ai-venture/internal/persistence"
)

type Session struct {
	Engine       *engine.Engine
	Interpreter  language.Interpreter
	Narrator     language.Narrator        // nil selects canonical descriptions only
	Illustrator  illustration.Illustrator // nil disables location art
	Input        io.Reader
	Output       io.Writer
	Color        bool // resolved by NewPrinter; the zero value keeps output plain
	SavePath     string
	Timeout      time.Duration
	Title        string
	Introduction string
	artCache     map[string]cachedIllustration
}

const help = `Game commands (examples):
  look | examine key | take key | drop key
  go north | east | up | down
  use key on door | inventory
Local commands: help | save | load | quit
In LLM mode, describe your actions freely in English.
Prefix a command with / to use the local grammar without the LLM.
Use the IDs in square brackets to identify items unambiguously.`

const artHelp = "Illustration: art generates or shows the current sketch (LLM; no turn). /art only shows a cached sketch."

// safe prevents model or scenario text from emitting terminal control sequences.
func safe(s string) string {
	return strings.Map(func(r rune) rune {
		if (unicode.IsControl(r) && r != '\n' && r != '\t') || unicode.In(r, unicode.Cf) {
			return -1
		}
		return r
	}, s)
}

func (s *Session) print(format string, args ...any) {
	s.printStyle(StylePlain, format, args...)
}

func (s *Session) printStyle(style Style, format string, args ...any) {
	for i, arg := range args {
		if err, ok := arg.(error); ok {
			args[i] = s.Engine.Language().Error(err)
		}
	}
	Printer{Output: s.Output, Color: s.Color}.Println(style, s.Engine.Language().Text(format, args...))
}

func (s *Session) showView() {
	v := s.Engine.View()
	s.printStyle(StyleLocation, "\n— %s —", v.Name)
	s.print("%s", v.Description)
	items := []string{}
	for _, item := range v.Items {
		items = append(items, item.Name+" ["+item.ID+"]")
	}
	if len(items) == 0 {
		s.printStyle(StyleObject, "Items here: none.")
	} else {
		s.printStyle(StyleObject, "Items here: %s.", strings.Join(items, ", "))
	}
	exits := []string{}
	for _, exit := range v.Exits {
		label := exit.Direction + " → " + exit.Destination
		if exit.Blocked {
			label += s.Engine.Language().Text(" (blocked)")
		}
		exits = append(exits, label)
	}
	if len(exits) == 0 {
		s.printStyle(StyleExit, "Exits: none.")
	} else {
		s.printStyle(StyleExit, "Exits: %s.", strings.Join(exits, " ; "))
	}
	if v.Completed {
		s.printStyle(StyleSuccess, "Objective complete: the adventure is over. You can still explore or save.")
	}
}

func (s *Session) narrate(ctx context.Context, result engine.Result) {
	if s.Narrator == nil {
		return
	}
	narrationCtx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	text, err := s.Narrator.Narrate(narrationCtx, result)
	if err != nil {
		s.printStyle(StyleWarning, "[Narration unavailable; the engine facts still apply.]")
		return
	}
	s.printStyle(StyleNarration, "\nAtmosphere: %s", text)
}

func (s *Session) Run(ctx context.Context) error {
	if s.Engine == nil || s.Interpreter == nil || s.Input == nil || s.Output == nil || s.Timeout <= 0 || s.SavePath == "" {
		return locale.Errorf("session configuration is incomplete")
	}
	s.printStyle(StyleTitle, "\n%s", s.Title)
	s.print("%s", s.Introduction)
	s.printStyle(StyleInfo, "Type help for commands. Save file: %s", s.SavePath)
	s.showView()
	s.illustrate(ctx, true)
	s.narrate(ctx, engine.Result{Action: engine.Action{Kind: "observe"}, Message: s.Engine.View().Description, View: s.Engine.View()})
	reader := bufio.NewReader(s.Input)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		Printer{Output: s.Output, Color: s.Color}.Print(StylePrompt, "\n> ")
		line, err := readCommand(reader)
		if err == io.EOF {
			s.printStyle(StyleInfo, "Goodbye.")
			return nil
		}
		if err != nil {
			return err
		}
		if len(line) > language.MaxCommandBytes {
			s.printStyle(StyleError, "Command is too long (maximum %d bytes).", language.MaxCommandBytes)
			continue
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		local := strings.HasPrefix(line, "/")
		line = strings.TrimSpace(strings.TrimPrefix(line, "/"))
		command := language.Normalize(line)
		if s.Engine.Language() == locale.French {
			switch command {
			case "aide":
				command = "help"
			case "sauver", "sauvegarder":
				command = "save"
			case "charger":
				command = "load"
			case "quitter":
				command = "quit"
			case "dessin":
				command = "art"
			}
		}
		switch command {
		case "help", "?":
			s.print("%s", s.Engine.Language().Text(help))
			s.print("%s", s.Engine.Language().Text(artHelp))
			continue
		case "art":
			if s.Illustrator == nil {
				s.printStyle(StyleInfo, "Illustrations are disabled; use -art in LLM mode.")
			} else {
				s.illustrate(ctx, !local)
			}
			continue
		case "quit", "exit":
			s.printStyle(StyleInfo, "Goodbye. Use save before quitting to keep your progress.")
			return nil
		case "save":
			if err := persistence.Save(s.SavePath, s.Engine); err != nil {
				s.printStyle(StyleError, "Save failed: %v", err)
			} else {
				s.printStyle(StyleSuccess, "Game saved.")
			}
			continue
		case "load":
			if err := persistence.Load(s.SavePath, s.Engine); err != nil {
				s.printStyle(StyleError, "Load failed: %v", err)
			} else {
				s.printStyle(StyleSuccess, "Game loaded.")
				s.artCache = nil
				s.showView()
			}
			continue
		}
		interpreter := s.Interpreter
		view := s.Engine.View()
		// Exact exit names are native shortcuts, including the French "est".
		// Natural-language movement still uses the selected interpreter.
		direction := false
		for _, exit := range view.Exits {
			if language.Normalize(line) == exit.Direction {
				direction = true
				break
			}
		}
		if local || direction {
			interpreter = language.Offline{Language: s.Engine.Language()}
		}
		actionCtx, cancel := context.WithTimeout(ctx, s.Timeout)
		action, err := interpreter.Interpret(actionCtx, line, view)
		cancel()
		if err != nil {
			s.printStyle(StyleError, "Could not interpret command: %v\nNo action applied. Try /help or a local command such as /look.", err)
			continue
		}
		result, err := s.Engine.Apply(action)
		if err != nil {
			s.printStyle(StyleWarning, "Action refused: %v", err)
			continue
		}
		s.printStyle(StyleFact, "Fact: %s", result.Message)
		if action.Kind == "inventory" {
			for _, item := range result.View.Inventory {
				s.printStyle(StyleObject, "  %s [%s]", item.Name, item.ID)
			}
		}
		if action.Kind == "move" || action.Kind == "take" || action.Kind == "drop" || action.Kind == "use" || (action.Kind == "observe" && (action.Target == "" || action.Target == result.View.Location)) {
			s.showView()
		}
		if !local {
			if action.Kind == "move" || (action.Kind == "observe" && (action.Target == "" || action.Target == result.View.Location)) {
				s.illustrate(ctx, true)
			}
			s.narrate(ctx, result)
		} else if action.Kind == "move" || (action.Kind == "observe" && (action.Target == "" || action.Target == result.View.Location)) {
			s.illustrate(ctx, false)
		}
	}
}

// readCommand drains oversized lines without storing them, then resumes normally.
func readCommand(reader *bufio.Reader) (string, error) {
	var result strings.Builder
	for {
		part, err := reader.ReadSlice('\n')
		if err == nil {
			part = part[:len(part)-1]
			if len(part) > 0 && part[len(part)-1] == '\r' {
				part = part[:len(part)-1]
			}
		}
		if result.Len() <= language.MaxCommandBytes {
			remaining := language.MaxCommandBytes + 1 - result.Len()
			if len(part) > remaining {
				part = part[:remaining]
			}
			result.Write(part)
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		if err == io.EOF && result.Len() > 0 {
			return result.String(), nil
		}
		return result.String(), err
	}
}
