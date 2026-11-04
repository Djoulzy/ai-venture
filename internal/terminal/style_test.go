package terminal_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"ai-venture/internal/engine"
	"ai-venture/internal/language"
	"ai-venture/internal/locale"
	"ai-venture/internal/terminal"
	"ai-venture/scenarios"
)

var sgr = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func TestColorModesRespectRedirectionAndExplicitChoice(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	var out bytes.Buffer
	if terminal.NewPrinter(&out, terminal.ColorAuto).Color {
		t.Fatal("redirected output must stay plain in auto mode")
	}
	file, err := os.CreateTemp(t.TempDir(), "output")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if terminal.NewPrinter(file, terminal.ColorAuto).Color {
		t.Fatal("a regular file is not a terminal")
	}
	for _, mode := range []terminal.ColorMode{terminal.ColorAuto, terminal.ColorAlways, terminal.ColorNever} {
		parsed, err := terminal.ParseColorMode(string(mode))
		if err != nil || parsed != mode {
			t.Fatalf("mode %q: %q, %v", mode, parsed, err)
		}
	}
	if _, err := terminal.ParseColorMode("sometimes"); err == nil || !strings.Contains(locale.French.Error(err), "mode de couleur invalide") {
		t.Fatal("invalid color option must return a localized error")
	}
	t.Setenv("NO_COLOR", "1")
	t.Setenv("TERM", "dumb")
	if terminal.NewPrinter(&out, terminal.ColorAuto).Color || terminal.NewPrinter(&out, terminal.ColorNever).Color {
		t.Fatal("disabled colors were enabled")
	}
	printer := terminal.NewPrinter(&out, terminal.ColorAlways)
	printer.Println(terminal.StyleError, "error")
	if !printer.Color || !strings.Contains(out.String(), "\x1b[") || !strings.HasSuffix(out.String(), "\x1b[0m\n") {
		t.Fatal("always must force colors and reset their attributes")
	}
}

type colorTestNarrator struct{}

func (colorTestNarrator) Narrate(context.Context, engine.Result) (string, error) {
	return "\x1b[2JSea mist.\u202e", nil
}

func TestColoredSessionsPreserveTextStateAndSanitization(t *testing.T) {
	for _, lang := range []locale.Language{locale.English, locale.French} {
		t.Run(string(lang), func(t *testing.T) {
			w, err := (scenarios.Demo{Language: lang}).Load(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			commands := "take key\nnorth\nup\nsave\nload\nquit\n"
			if lang == locale.French {
				commands = "prendre cle\nnord\nhaut\nsauver\ncharger\nquitter\n"
			}
			savePath := filepath.Join(t.TempDir(), "save.json")
			var plain string
			var state engine.State
			for _, color := range []bool{false, true} {
				e, err := engine.NewWithLanguage(w, lang)
				if err != nil {
					t.Fatal(err)
				}
				var out bytes.Buffer
				s := session(t, e, commands, &out)
				s.Title, s.Introduction = "\x1b[31mTitle", w.Introduction
				s.Color, s.SavePath = color, savePath
				s.Interpreter = language.Offline{Language: lang}
				s.Narrator = colorTestNarrator{}
				if err := s.Run(context.Background()); err != nil {
					t.Fatal(err)
				}
				text := out.String()
				stripped := sgr.ReplaceAllString(text, "")
				if strings.ContainsAny(stripped, "\x1b\u202e") {
					t.Fatal("scenario or model controls escaped sanitization")
				}
				if !color {
					if strings.Contains(text, "\x1b") {
						t.Fatal("plain mode emitted ANSI")
					}
					plain, state = text, e.Snapshot()
					continue
				}
				if text == stripped || stripped != plain || !reflect.DeepEqual(state, e.Snapshot()) {
					t.Fatal("colors must change only presentation")
				}
				// Reset before reading player input, so typed text keeps the terminal's style.
				if !strings.Contains(text, "> \x1b[0m") {
					t.Fatal("prompt did not reset its attributes")
				}
			}
		})
	}
}
