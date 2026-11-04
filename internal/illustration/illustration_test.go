package illustration_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"ai-venture/internal/engine"
	"ai-venture/internal/illustration"
	"ai-venture/internal/inference"
	"ai-venture/internal/locale"
	"ai-venture/scenarios"
)

func encoded(lines []illustration.Line) []byte {
	data, _ := json.Marshal(illustration.Frame{Lines: lines})
	return data
}

func TestDimensionsASCIIAndStrictJSON(t *testing.T) {
	lines := make([]illustration.Line, 24)
	for i := range lines {
		lines[i] = illustration.Line{Text: strings.Repeat("#", 80), Color: illustration.Cyan}
	}
	frame, err := illustration.Decode(encoded(lines))
	if err != nil || len(frame.Lines) != 24 || len(frame.Lines[0].Text) != 80 {
		t.Fatalf("exact 80 x 24 must be accepted: %v", err)
	}
	for name, data := range map[string][]byte{
		"wide":           encoded([]illustration.Line{{Text: strings.Repeat("#", 81), Color: illustration.Default}}),
		"tall":           encoded(append(lines, lines[0])),
		"empty":          encoded(nil),
		"blank":          encoded([]illustration.Line{{Text: "   ", Color: illustration.Cyan}}),
		"unicode":        encoded([]illustration.Line{{Text: "╔═╗", Color: illustration.Cyan}}),
		"control":        encoded([]illustration.Line{{Text: "\x1b[2J", Color: illustration.Cyan}}),
		"tab":            encoded([]illustration.Line{{Text: "\t#", Color: illustration.Cyan}}),
		"newline":        encoded([]illustration.Line{{Text: "#\n#", Color: illustration.Cyan}}),
		"palette":        encoded([]illustration.Line{{Text: "#", Color: "\x1b[31m"}}),
		"missing_color":  []byte(`{"lines":[{"text":"#"}]}`),
		"missing_text":   []byte(`{"lines":[{"color":"cyan"}]}`),
		"missing_lines":  []byte(`{}`),
		"wrong_case":     []byte(`{"Lines":[{"text":"#","color":"cyan"}]}`),
		"wrong_row_case": []byte(`{"lines":[{"Text":"#","color":"cyan"}]}`),
		"duplicate":      []byte(`{"lines":[{"text":"#","text":"X","color":"cyan"}]}`),
		"effects":        []byte(`{"lines":[{"text":"#","color":"cyan"}],"effects":{"door_open":true}}`),
		"null":           []byte(`{"lines":null}`),
		"trailing":       []byte(`{"lines":[{"text":"#","color":"cyan"}]} {}`),
		"oversized":      []byte(strings.Repeat(" ", 16385)),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := illustration.Decode(data); err == nil {
				t.Fatal("invalid illustration accepted")
			}
		})
	}
	frame.Lines[0].Text = "  #  "
	copy := frame.Clone()
	copy.Lines[0].Text = "changed"
	if frame.Lines[0].Text != "  #  " {
		t.Fatal("frame clone shares mutable storage")
	}
}

type fakeClient struct {
	request inference.Request
	err     error
	calls   int
}

func (f *fakeClient) Complete(_ context.Context, request inference.Request) (string, error) {
	f.request = request
	f.calls++
	return `{"lines":[{"text":"   +---+","color":"cyan"},{"text":"   |   |","color":"default"}]}`, f.err
}

func TestLLMOnlyReceivesThePublicSetting(t *testing.T) {
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
			before := e.Snapshot()
			view := e.View()
			view.Inventory = []engine.VisibleItem{{ID: "private_inventory_sentinel"}}
			client := &fakeClient{}
			llm := illustration.LLM{Client: client, Language: lang}
			frame, err := llm.Illustrate(context.Background(), view)
			if err != nil || frame.Lines[0].Text != "   +---+" {
				t.Fatalf("%+v, %v", frame, err)
			}
			r := client.request
			if len(r.Messages) != 2 || r.MaxTokens != 2048 || !json.Valid(r.Schema) {
				t.Fatal("invalid inference request")
			}
			for _, secret := range []string{"private_inventory_sentinel", `"flags"`, `"rules"`, `"inventory"`, `"memory"`, `"documents"`} {
				if strings.Contains(r.Messages[1].Content, secret) {
					t.Fatalf("hidden input leaked: %s", secret)
				}
			}
			want := "Draw a recognizable"
			if lang == locale.French {
				want = "Dessine une illustration"
			}
			if !strings.Contains(r.Messages[0].Content, want) {
				t.Fatal("wrong prompt language")
			}
			if !reflect.DeepEqual(before, e.Snapshot()) {
				t.Fatal("illustration changed engine state")
			}
			signature := illustration.Signature(view)
			view.Inventory = nil
			if signature != illustration.Signature(view) {
				t.Fatal("inventory should not invalidate scenery")
			}
			view.Items = nil
			if signature == illustration.Signature(view) {
				t.Fatal("visible items must invalidate scenery")
			}
			cause := errors.New("offline")
			client.err = cause
			if _, err := llm.Illustrate(context.Background(), view); !errors.Is(err, cause) {
				t.Fatal("lost provider error")
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			calls := client.calls
			if _, err := llm.Illustrate(ctx, view); !errors.Is(err, context.Canceled) || client.calls != calls {
				t.Fatal("cancellation ignored")
			}
		})
	}
}
