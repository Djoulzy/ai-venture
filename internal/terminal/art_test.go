package terminal_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"ai-venture/internal/engine"
	"ai-venture/internal/illustration"
	"ai-venture/internal/language"
	"ai-venture/internal/locale"
	"ai-venture/internal/terminal"
	"ai-venture/scenarios"
)

type fakeIllustrator struct {
	calls   int
	err     error
	invalid bool
}

func (f *fakeIllustrator) Illustrate(ctx context.Context, _ engine.View) (illustration.Frame, error) {
	f.calls++
	if _, ok := ctx.Deadline(); !ok {
		panic("illustration call has no deadline")
	}
	frame := illustration.Frame{Lines: []illustration.Line{{Text: "  +--+", Color: illustration.Cyan}}}
	if f.invalid {
		frame.Lines[0].Text = "\x1b[2J"
	}
	return frame, f.err
}

func TestArtCacheLocalCommandsAndRestore(t *testing.T) {
	for _, lang := range []locale.Language{locale.English, locale.French} {
		t.Run(string(lang), func(t *testing.T) {
			w, err := (scenarios.Demo{Language: lang}).Load(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			e, err := engine.NewWithLanguage(w, lang)
			if err != nil {
				t.Fatal(err)
			}
			commands := "art\neast\nwest\n/take key\n/art\nart\nsave\neast\nload\n/art\nart\nquit\n"
			if lang == locale.French {
				commands = "dessin\nest\nouest\n/prendre cle\n/dessin\ndessin\nsauver\nest\ncharger\n/dessin\ndessin\nquitter\n"
			}
			var out bytes.Buffer
			s := session(t, e, commands, &out)
			s.Interpreter = language.Offline{Language: lang}
			fake := &fakeIllustrator{}
			s.Illustrator = fake
			if err := s.Run(context.Background()); err != nil {
				t.Fatal(err)
			}
			// Initial quay, workshop, quay with key removed, quay after load.
			if fake.calls != 4 || e.Snapshot().Turn != 3 {
				t.Fatalf("%d calls, state %+v\n%s", fake.calls, e.Snapshot(), out.String())
			}
			if strings.Count(out.String(), "  +--+") != 7 {
				t.Fatalf("cached drawing missing or stale: %s", out.String())
			}
		})
	}
}

func TestArtFailureLeavesTheGamePlayable(t *testing.T) {
	for _, fake := range []*fakeIllustrator{{err: errors.New("unavailable")}, {invalid: true}} {
		e := game(t)
		var out bytes.Buffer
		s := session(t, e, "/take key\n/quit\n", &out)
		s.Illustrator = fake
		if err := s.Run(context.Background()); err != nil {
			t.Fatal(err)
		}
		if e.Snapshot().Turn != 1 || !strings.Contains(out.String(), "Illustration unavailable") || strings.Contains(out.String(), "\x1b") {
			t.Fatal(out.String())
		}
	}
}

func TestDrawRespects80By24WithAndWithoutANSI(t *testing.T) {
	frame := illustration.Frame{}
	for i := 0; i < 24; i++ {
		frame.Lines = append(frame.Lines, illustration.Line{Text: strings.Repeat("#", 80), Color: illustration.Cyan})
	}
	for _, color := range []bool{false, true} {
		var out bytes.Buffer
		printer := terminal.Printer{Output: &out, Color: color}
		if err := printer.Draw(frame); err != nil {
			t.Fatal(err)
		}
		plain := sgr.ReplaceAllString(out.String(), "")
		lines := strings.Split(strings.TrimSuffix(plain, "\n"), "\n")
		if len(lines) != 24 {
			t.Fatal("frame height changed")
		}
		for _, line := range lines {
			if len(line) != 80 {
				t.Fatal("frame width changed")
			}
		}
		if color && !strings.HasSuffix(out.String(), "\x1b[0m\n") {
			t.Fatal("attributes were not reset")
		}
		out.Reset()
		bad := frame.Clone()
		bad.Lines[23].Text = "\x1b[2J"
		if err := printer.Draw(bad); err == nil || out.Len() != 0 {
			t.Fatal("invalid frame was partially displayed")
		}
	}
}
