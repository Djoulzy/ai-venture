package desktop

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"ai-venture/internal/engine"
	"ai-venture/internal/illustration"
	"ai-venture/internal/language"
	"ai-venture/internal/presentation"
)

func TestEditingFrenchCommandAndSelection(t *testing.T) {
	var e editBuffer
	e.insert("clé")
	e.cursor = 2
	e.backspace()
	e.insert("r")
	if e.String() != "cré" || e.cursor != 2 {
		t.Fatalf("rune editing: %q, cursor %d", e.String(), e.cursor)
	}
	e.delete()
	if e.String() != "cr" {
		t.Fatal(e.String())
	}
	e.all = true
	e.insert("prendre\nla\tclé\x00\u202e")
	if e.String() != "prendre la clé" {
		t.Fatal(e.String())
	}
	e.all = true
	e.backspace()
	if e.String() != "" || e.cursor != 0 || e.all {
		t.Fatal("selection not removed")
	}
	e.insert(strings.Repeat("é", language.MaxCommandBytes))
	if !utf8.ValidString(e.String()) || len(e.String()) != language.MaxCommandBytes {
		t.Fatal("command byte limit or UTF-8 boundary lost")
	}
}

func TestDisplayKeepsFullDescriptionAndBoundsJournal(t *testing.T) {
	var m model
	m.apply(presentation.Event{Kind: presentation.View, Turn: 7, View: engine.View{Name: "Le quai", Description: strings.Repeat("Une longue description. ", 100)}})
	if m.turn != 7 || !strings.Contains(m.entries[0].text, strings.TrimSpace(m.view.Description)) {
		t.Fatal("location facts lost outside the short scene caption")
	}
	m.apply(presentation.Event{Kind: presentation.Ready})
	if !m.ready {
		t.Fatal("input did not become ready")
	}
	m.apply(presentation.Event{Kind: presentation.Message, Tone: presentation.Info, Text: "préparation"})
	if m.status != "préparation" || len(m.entries) != 1 {
		t.Fatal("status duplicated into story")
	}
	for range 205 {
		m.append(presentation.Narration, strings.Repeat("é", 8100)+"\x1b\u202e")
	}
	if len(m.entries) != 200 || len([]rune(m.entries[199].text)) != 8001 || strings.ContainsAny(m.entries[0].text, "\x1b\u202e") {
		t.Fatal("journal bounds or filtering failed")
	}
	m.apply(presentation.Event{Kind: presentation.Art, Frame: illustration.Frame{}})
	if len(m.art.Lines) != 0 {
		t.Fatal("invalid art accepted")
	}
}

func TestWorkerCancellationAndOfflineRecovery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startupErr := errors.New("model unavailable")
	g := &window{parent: ctx, config: Config{Start: func(ctx context.Context, input io.Reader, observe presentation.Observer, offline bool) error {
		if !offline {
			return startupErr
		}
		observe(presentation.Event{Kind: presentation.Ready})
		_, err := io.ReadAll(input)
		return err
	}}}
	g.start(false)
	select {
	case err := <-g.done:
		if !errors.Is(err, startupErr) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("startup did not return")
	}
	g.start(true)
	select {
	case event := <-g.events:
		if event.Kind != presentation.Ready {
			t.Fatal(event)
		}
	case <-time.After(time.Second):
		t.Fatal("offline recovery did not start")
	}
	g.cancel()
	select {
	case err := <-g.done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("window close did not cancel the session")
	}
}
