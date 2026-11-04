// Package terminal implements the sequential terminal game loop.
package terminal

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"

	"ai-venture/internal/engine"
	"ai-venture/internal/language"
	"ai-venture/internal/locale"
	"ai-venture/internal/persistence"
)

type Session struct {
	Engine       *engine.Engine
	Interpreter  language.Interpreter
	Narrator     language.Narrator // nil selects canonical descriptions only
	Input        io.Reader
	Output       io.Writer
	SavePath     string
	Timeout      time.Duration
	Title        string
	Introduction string
}

const help = `Game commands (examples):
  look | examine key | take key | drop key
  go north | east | up | down
  use key on door | inventory
Local commands: help | save | load | quit
In LLM mode, describe your actions freely in English.
Prefix a command with / to use the local grammar without the LLM.
Use the IDs in square brackets to identify items unambiguously.`

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
	for i, arg := range args {
		if err, ok := arg.(error); ok {
			args[i] = s.Engine.Language().Error(err)
		}
	}
	_, _ = fmt.Fprintln(s.Output, safe(s.Engine.Language().Text(format, args...)))
}

func (s *Session) showView() {
	v := s.Engine.View()
	s.print("\n— %s —", v.Name)
	s.print("%s", v.Description)
	items := []string{}
	for _, item := range v.Items {
		items = append(items, item.Name+" ["+item.ID+"]")
	}
	if len(items) == 0 {
		s.print("Items here: none.")
	} else {
		s.print("Items here: %s.", strings.Join(items, ", "))
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
		s.print("Exits: none.")
	} else {
		s.print("Exits: %s.", strings.Join(exits, " ; "))
	}
	if v.Completed {
		s.print("Objective complete: the adventure is over. You can still explore or save.")
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
		s.print("[Narration unavailable; the engine facts still apply.]")
		return
	}
	s.print("\nAtmosphere: %s", text)
}

func (s *Session) Run(ctx context.Context) error {
	if s.Engine == nil || s.Interpreter == nil || s.Input == nil || s.Output == nil || s.Timeout <= 0 || s.SavePath == "" {
		return locale.Errorf("session configuration is incomplete")
	}
	s.print("\n%s\n%s", s.Title, s.Introduction)
	s.print("Type help for commands. Save file: %s", s.SavePath)
	s.showView()
	s.narrate(ctx, engine.Result{Action: engine.Action{Kind: "observe"}, Message: s.Engine.View().Description, View: s.Engine.View()})
	reader := bufio.NewReader(s.Input)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, _ = fmt.Fprint(s.Output, "\n> ")
		line, err := readCommand(reader)
		if err == io.EOF {
			s.print("Goodbye.")
			return nil
		}
		if err != nil {
			return err
		}
		if len(line) > language.MaxCommandBytes {
			s.print("Command is too long (maximum %d bytes).", language.MaxCommandBytes)
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
			}
		}
		switch command {
		case "help", "?":
			s.print("%s", s.Engine.Language().Text(help))
			continue
		case "quit", "exit":
			s.print("Goodbye. Use save before quitting to keep your progress.")
			return nil
		case "save":
			if err := persistence.Save(s.SavePath, s.Engine); err != nil {
				s.print("Save failed: %v", err)
			} else {
				s.print("Game saved.")
			}
			continue
		case "load":
			if err := persistence.Load(s.SavePath, s.Engine); err != nil {
				s.print("Load failed: %v", err)
			} else {
				s.print("Game loaded.")
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
			s.print("Could not interpret command: %v\nNo action applied. Try /help or a local command such as /look.", err)
			continue
		}
		result, err := s.Engine.Apply(action)
		if err != nil {
			s.print("Action refused: %v", err)
			continue
		}
		s.print("Fact: %s", result.Message)
		if action.Kind == "inventory" {
			for _, item := range result.View.Inventory {
				s.print("  %s [%s]", item.Name, item.ID)
			}
		}
		if action.Kind == "move" || action.Kind == "take" || action.Kind == "drop" || action.Kind == "use" || (action.Kind == "observe" && (action.Target == "" || action.Target == result.View.Location)) {
			s.showView()
		}
		if !local {
			s.narrate(ctx, result)
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
