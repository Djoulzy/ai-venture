package terminal_test

import (
	"context"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ai-venture/internal/engine"
	"ai-venture/internal/language"
	"ai-venture/internal/locale"
	"ai-venture/internal/presentation"
	"ai-venture/internal/terminal"
	"ai-venture/internal/testfixtures"
)

type exportedScene string

func (p exportedScene) Render(context.Context, engine.View) (string, error) { return string(p), nil }

func TestStructuredFrontendActionsSaveLoadAndVisibleSnapshots(t *testing.T) {
	for _, lang := range []locale.Language{locale.English, locale.French} {
		t.Run(string(lang), func(t *testing.T) {
			ctx := context.Background()
			w, err := (testfixtures.Scenario{Language: lang}).Load(ctx)
			if err != nil {
				t.Fatal(err)
			}
			e, err := engine.NewWithLanguage(w, lang)
			if err != nil {
				t.Fatal(err)
			}
			commands := "/take key\n/save\n/drop key\n/load\n/quit\n"
			key := "key"
			if lang == locale.French {
				commands = "/prendre cle\n/sauver\n/deposer cle\n/charger\n/quitter\n"
				key = "cle"
			}
			var events []presentation.Event
			s := terminal.Session{Engine: e, Interpreter: language.Offline{Language: lang}, Input: strings.NewReader(commands), Output: io.Discard,
				SavePath: filepath.Join(t.TempDir(), "game.json"), Timeout: time.Second, Title: w.Title,
				Observer: func(event presentation.Event) { events = append(events, event) }}
			if err := s.Run(ctx); err != nil {
				t.Fatal(err)
			}
			var views []presentation.Event
			ready, facts := 0, 0
			for _, event := range events {
				if event.Kind == presentation.View {
					views = append(views, event)
				}
				if event.Kind == presentation.Ready {
					ready++
				}
				if event.Kind == presentation.Message && event.Tone == presentation.Fact {
					facts++
				}
			}
			if ready != 5 || facts != 2 || len(views) != 4 || views[0].Turn != 0 || views[3].Turn != 1 || len(views[3].View.Inventory) != 1 || views[3].View.Inventory[0].ID != key {
				t.Fatalf("frontend progression: ready=%d facts=%d views=%+v", ready, facts, views)
			}
			if len(views[0].View.Inventory) != 0 || len(views[0].View.Items) == 0 {
				t.Fatal("a later action mutated a previously published view")
			}
			views[3].View.Inventory[0].Name = "forged"
			if e.View().Inventory[0].Name == "forged" {
				t.Fatal("frontend received mutable engine data")
			}
		})
	}
}

func TestOnlyCompositorProducesSceneEvents(t *testing.T) {
	for _, size := range []image.Point{{640, 384}, {800, 600}} {
		path := filepath.Join(t.TempDir(), "scene.png")
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, image.NewNRGBA(image.Rect(0, 0, size.X, size.Y))); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
		e := game(t)
		s := terminal.Session{Engine: e, Interpreter: language.Offline{}, Input: strings.NewReader("/quit\n"), Output: io.Discard,
			SavePath: filepath.Join(t.TempDir(), "game.json"), Timeout: time.Second, Visuals: exportedScene(path)}
		scenes := 0
		s.Observer = func(event presentation.Event) {
			if event.Kind == presentation.Scene {
				scenes++
				if event.Image.Bounds().Size() != size {
					t.Fatal("unexpected image")
				}
			}
		}
		if err := s.Run(context.Background()); err != nil {
			t.Fatal(err)
		}
		want := 0
		if size == image.Pt(640, 384) {
			want = 1
		}
		if scenes != want {
			t.Fatalf("image size %v: %d scene events", size, scenes)
		}
		// Even a valid file path printed by a model is plain journal text.
		s.Visuals = nil
		s.Input = strings.NewReader("/quit\n")
		s.Narrator = language.LLM{Client: &scriptedClient{responses: []string{`{"text":"` + path + `"}`}}}
		scenes = 0
		if err := s.Run(context.Background()); err != nil || scenes != 0 {
			t.Fatalf("narration opened an image: err=%v scenes=%d", err, scenes)
		}
	}
}
