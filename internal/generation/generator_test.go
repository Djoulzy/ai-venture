package generation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"ai-venture/internal/catalogue"
	"ai-venture/internal/engine"
	"ai-venture/internal/inference"
	"ai-venture/internal/locale"
	"ai-venture/internal/persistence"
	"ai-venture/internal/world"
)

type fake struct {
	responses []string
	requests  []inference.Request
	complete  func(context.Context) (string, error)
}

func (f *fake) Complete(ctx context.Context, r inference.Request) (string, error) {
	f.requests = append(f.requests, r)
	if f.complete != nil {
		return f.complete(ctx)
	}
	index := min(len(f.requests)-1, len(f.responses)-1)
	return f.responses[index], nil
}
func fixture(t *testing.T) *catalogue.Store {
	t.Helper()
	ctx := context.Background()
	s, err := catalogue.Open(ctx, filepath.Join(t.TempDir(), "catalogue.sqlite"), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	batch := catalogue.Batch{}
	for _, lang := range []string{"en", "fr"} {
		for _, entry := range []struct{ id, kind string }{{"house", "location"}, {"library", "location"}, {"book", "item"}, {"beast", "monster"}, {"storm", "situation"}} {
			images := []string{}
			if entry.kind != "situation" {
				images = append(images, "asset_"+entry.id)
			}
			batch.Entries = append(batch.Entries, catalogue.Entry{ID: entry.id, Kind: entry.kind, Language: lang, Title: lang + " " + entry.id, Text: lang + " " + strings.Repeat("lore ", 150), Images: images, Metadata: map[string]string{"private": "NOT_FOR_PROMPT"}})
		}
	}
	for _, e := range batch.Entries[:5] {
		if len(e.Images) == 0 {
			continue
		}
		var b bytes.Buffer
		_ = png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 2, 2)))
		hash := sha256.Sum256(b.Bytes())
		batch.Assets = append(batch.Assets, catalogue.Asset{ID: e.Images[0], Kind: e.Kind, Width: 2, Height: 2, SHA256: hex.EncodeToString(hash[:]), PNG: b.Bytes()})
	}
	if _, err := s.Import(ctx, batch, false); err != nil {
		t.Fatal(err)
	}
	return s
}
func proposal() Plan {
	return Plan{Title: "Test", Introduction: "Read the book, then use it to appease the beast.", Locations: []string{"house", "library"}, Entities: []Placement{{"book", "house"}, {"beast", "library"}}, Steps: []Step{{"book", "", "house", "The book reveals a rite."}, {"book", "beast", "library", "The beast accepts the rite."}}, Situation: "storm"}
}
func encode(p Plan) string { b, _ := json.Marshal(p); return string(b) }

func TestGenerateRetrievesCompilesAndProvesBothLanguages(t *testing.T) {
	s := fixture(t)
	for _, lang := range []locale.Language{locale.French, locale.English} {
		t.Run(string(lang), func(t *testing.T) {
			p := proposal()
			if lang == locale.French {
				p.Title = "Le rite"
				p.Introduction = "Lisez le livre puis apaisez la créature avec son rituel."
				p.Steps[0].Message = "Le livre révèle un rite."
				p.Steps[1].Message = "La créature accepte le rite."
			}
			f := &fake{responses: []string{encode(p)}}
			g := Generator{Client: f, Catalogue: s, Language: lang, Timeout: time.Second, Attempts: 2}
			r, err := g.Generate(context.Background(), "book beast", 42)
			if err != nil {
				t.Fatal(err)
			}
			request := f.requests[0]
			payload := request.Messages[1].Content
			for _, forbidden := range []string{"NOT_FOR_PROMPT", "asset_", "png", "metadata", "solution"} {
				if strings.Contains(payload, forbidden) {
					t.Fatalf("unexpected inference data: %s", forbidden)
				}
			}
			var input struct {
				Catalogue []candidate `json:"catalogue"`
			}
			if err := json.Unmarshal([]byte(payload), &input); err != nil {
				t.Fatal(err)
			}
			if len(input.Catalogue) > 12 || len(input.Catalogue) == 0 {
				t.Fatal("retrieval budget")
			}
			for _, entry := range input.Catalogue {
				if !strings.HasPrefix(entry.Title, string(lang)) || len([]rune(entry.Text)) > 600 {
					t.Fatal("language or text budget")
				}
			}
			if lang == locale.French && !strings.Contains(request.Messages[0].Content, "française") {
				t.Fatal("wrong prompt language")
			}
			if !json.Valid(request.Schema) {
				t.Fatal("invalid schema")
			}
			if err := world.Validate(r.World); err != nil {
				t.Fatal(err)
			}
			if len(r.World.CatalogueDocuments) != 0 || r.World.Provenance.Seed != 42 {
				t.Fatal("unresolved references or missing provenance")
			}
			e, _ := engine.NewWithLanguage(r.World, lang)
			if e.View().Completed || len(e.Memory()) != 3 {
				t.Fatal("wrong initial discoveries")
			}
			e.Apply(engine.Action{Kind: "take", Target: "book"})
			direction := "forward"
			if lang == locale.French {
				direction = "avant"
			}
			e.Apply(engine.Action{Kind: "move", Target: direction})
			before := e.Snapshot()
			if _, err := e.Apply(engine.Action{Kind: "use", Tool: "book", Target: "beast"}); err == nil || !reflect.DeepEqual(before, e.Snapshot()) {
				t.Fatal("ordered prerequisite bypassed")
			}
			e, _ = engine.NewWithLanguage(r.World, lang)
			for _, action := range r.Solution {
				if _, err := e.Apply(action); err != nil {
					t.Fatal(err)
				}
			}
			if !e.View().Completed {
				t.Fatal("no winning witness")
			}
			path := filepath.Join(t.TempDir(), "scenario.json")
			if err := Publish(context.Background(), path, r.World); err != nil {
				t.Fatal(err)
			}
			loaded, err := (world.FileSource{Path: path}).Load(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			restored, _ := engine.NewWithLanguage(loaded, lang)
			save := filepath.Join(t.TempDir(), "save.json")
			if err := persistence.Save(save, e); err != nil {
				t.Fatal(err)
			}
			if err := persistence.Load(save, restored); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(e.Snapshot(), restored.Snapshot()) {
				t.Fatal("persistence changed generated state")
			}
			if err := Publish(context.Background(), path, world.Definition{}); err == nil {
				t.Fatal("invalid world published")
			}
			if err := Publish(context.Background(), path, r.World); !errors.Is(err, os.ErrExist) {
				t.Fatalf("existing file overwritten: %v", err)
			}
			cancelled, cancel := context.WithCancel(context.Background())
			cancel()
			if err := Publish(cancelled, filepath.Join(t.TempDir(), "new.json"), r.World); !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		})
	}
}

func TestRejectsInvalidPlansAndRepairsWithBoundedCalls(t *testing.T) {
	s := fixture(t)
	for name, change := range map[string]func(*Plan){
		"invented":       func(p *Plan) { p.Locations[0] = "invented" },
		"wrong_kind":     func(p *Plan) { p.Locations[0] = "book" },
		"remote_target":  func(p *Plan) { p.Steps[1].Location = "house" },
		"monster_tool":   func(p *Plan) { p.Steps[1].Tool = "beast" },
		"self_target":    func(p *Plan) { p.Steps[0].Target = "book" },
		"duplicate_rule": func(p *Plan) { p.Steps[1] = p.Steps[0] },
		"duplicate_item": func(p *Plan) { p.Entities = append(p.Entities, p.Entities[0]) },
		"control":        func(p *Plan) { p.Title = "bad\x1b" },
		"no_goal":        func(p *Plan) { p.Steps = []Step{} },
	} {
		t.Run(name, func(t *testing.T) {
			p := proposal()
			change(&p)
			f := &fake{responses: []string{encode(p)}}
			g := Generator{Client: f, Catalogue: s, Timeout: time.Second, Attempts: 2}
			if _, err := g.Generate(context.Background(), "book", 1); err == nil || len(f.requests) != 2 {
				t.Fatalf("invalid accepted or retries unbounded: %v", err)
			}
		})
	}
	valid := encode(proposal())
	for _, bad := range []string{strings.Replace(valid, `"situation":"storm",`, "", 1), strings.Replace(valid, `"situation":"storm"`, `"situation":null`, 1), strings.Replace(valid, `"title":"Test"`, `"title":"Test","title":"Again"`, 1), valid + `{}`, strings.Replace(valid, `"title":"Test"`, `"title":"Test","path":"/tmp/model-output"`, 1), strings.Repeat("x", 32769)} {
		// Required-field removal also covers an explicitly empty field.
		if bad == valid {
			bad = strings.Replace(valid, `,"situation":"storm"`, "", 1)
		}
		f := &fake{responses: []string{bad, valid}}
		g := Generator{Client: f, Catalogue: s, Language: locale.French, Timeout: time.Second, Attempts: 2}
		if _, err := g.Generate(context.Background(), "book", 1); err != nil || len(f.requests) != 2 {
			t.Fatalf("repair failed: %v", err)
		}
		if len(f.requests[1].Messages) != 3 {
			t.Fatal("validation feedback absent")
		}
	}
}

func TestRepairSchemaRestrictsInteractionsToValidatedPlacements(t *testing.T) {
	s := fixture(t)
	p := proposal()
	p.Steps[1].Location = "house"
	f := &fake{responses: []string{encode(p), encode(proposal())}}
	g := Generator{Client: f, Catalogue: s, Language: locale.English, Timeout: time.Second, Attempts: 3}
	if _, err := g.Generate(context.Background(), "book", 42); err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Properties struct {
			Entities struct {
				Const []Placement `json:"const"`
			} `json:"entities"`
			Steps struct {
				Items struct {
					AnyOf []struct {
						Properties map[string]struct {
							Enum []string `json:"enum"`
						} `json:"properties"`
					} `json:"anyOf"`
				} `json:"items"`
			} `json:"steps"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(f.requests[1].Schema, &schema); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(schema.Properties.Entities.Const, p.Entities) {
		t.Fatal("placements not preserved for repair")
	}
	found := false
	for _, alternative := range schema.Properties.Steps.Items.AnyOf {
		tool, target, location := alternative.Properties["tool"].Enum, alternative.Properties["target"].Enum, alternative.Properties["location"].Enum
		if !reflect.DeepEqual(tool, []string{"book"}) || reflect.DeepEqual(target, tool) {
			t.Fatal("invalid tool or self-target allowed by schema")
		}
		if reflect.DeepEqual(target, []string{"beast"}) {
			found = true
			if !reflect.DeepEqual(location, []string{"library"}) {
				t.Fatal("remote interaction allowed by schema")
			}
		}
	}
	if !found {
		t.Fatal("fixed target interaction absent")
	}
}

func TestCancellationAndInsufficientCatalogue(t *testing.T) {
	s := fixture(t)
	f := &fake{complete: func(ctx context.Context) (string, error) { <-ctx.Done(); return "", ctx.Err() }}
	g := Generator{Client: f, Catalogue: s, Timeout: time.Millisecond, Attempts: 3}
	if _, err := g.Generate(context.Background(), "book", 1); !errors.Is(err, context.DeadlineExceeded) || len(f.requests) != 1 {
		t.Fatalf("timeout not propagated: %v", err)
	}
	empty, err := catalogue.Open(context.Background(), filepath.Join(t.TempDir(), "empty.sqlite"), true)
	if err != nil {
		t.Fatal(err)
	}
	defer empty.Close()
	g.Catalogue = empty
	if _, err := g.Generate(context.Background(), "book", 1); err == nil || len(f.requests) != 1 {
		t.Fatal("empty catalogue reached inference")
	}
}
