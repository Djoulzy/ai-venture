package language_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"ai-venture/internal/engine"
	"ai-venture/internal/inference"
	"ai-venture/internal/language"
	"ai-venture/internal/locale"
	"ai-venture/scenarios"
)

type fakeClient struct {
	content string
	err     error
	request inference.Request
}

func (f *fakeClient) Complete(_ context.Context, r inference.Request) (string, error) {
	f.request = r
	return f.content, f.err
}

func TestActionJSONValidation(t *testing.T) {
	valid := `{"action":"take","target":"key","tool":""}`
	a, err := language.DecodeAction([]byte(valid))
	if err != nil || a.Kind != "take" || a.Target != "key" {
		t.Fatalf("%+v %v", a, err)
	}
	for _, text := range []string{
		`{"action":"take","target":"key"}`,
		`{"action":"take","target":"key","tool":null}`,
		`{"action":"take","target":"key","tool":"","effects":{"door_open":true}}`,
		`{"action":"take","action":"move","target":"key","tool":""}`,
		`{"action":"teleport","target":"lantern_room","tool":""}`,
		`{"action":"take","target":42,"tool":""}`,
		`{"Action":"take","target":"key","tool":""}`,
		`{"action":"inventory","target":"key","tool":""}`,
		`{"action":"use","target":"door","tool":""}`,
		`{"action":"take","target":"","tool":""}`,
		valid + ` {}`, "```json\n" + valid + "\n```", `[]`, `null`,
		`{"action":"take","target":"` + strings.Repeat("a", 65) + `","tool":""}`,
	} {
		if _, err := language.DecodeAction([]byte(text)); err == nil {
			t.Fatalf("proposition acceptée : %s", text)
		}
	}
}

func TestInterpreterOnlySeesVisibleWorld(t *testing.T) {
	w, err := (scenarios.Demo{}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	e, err := engine.New(w)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeClient{content: `{"action":"take","target":"key","tool":""}`}
	l := language.LLM{Client: f}
	before := e.Snapshot()
	a, err := l.Interpret(context.Background(), "I pick up the small key.", e.View())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, e.Snapshot()) {
		t.Fatal("l'interprétation a modifié l'état")
	}
	if !json.Valid(f.request.Schema) || f.request.Temperature != 0 {
		t.Fatal("schéma ou température incorrect")
	}
	var input struct {
		Command string      `json:"command"`
		View    engine.View `json:"view"`
	}
	if err := json.Unmarshal([]byte(f.request.Messages[1].Content), &input); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.request.Messages[0].Content, "English") || input.Command != "I pick up the small key." || !reflect.DeepEqual(input.View, e.View()) {
		t.Fatal("interpreter did not receive the English command and current view")
	}
	for _, hidden := range []string{"beacon_lit", "unlock_door", "battery", "effects"} {
		if strings.Contains(f.request.Messages[1].Content, hidden) {
			t.Fatalf("information secrète transmise : %s", hidden)
		}
	}
	if _, err := e.Apply(a); err != nil {
		t.Fatal(err)
	}
	f.content = `{"action":"move","target":"lantern_room","tool":""}`
	a, err = l.Interpret(context.Background(), "Teleport me to the top.", e.View())
	if err != nil {
		t.Fatal(err)
	}
	before = e.Snapshot()
	if _, err := e.Apply(a); err == nil {
		t.Fatal("téléportation acceptée")
	}
	if !reflect.DeepEqual(before, e.Snapshot()) {
		t.Fatal("mutation après refus")
	}
}

func TestNarrationValidationAndProviderErrors(t *testing.T) {
	f := &fakeClient{content: `{"text":"Rain lashes the windows."}`}
	l := language.LLM{Client: f}
	if text, err := l.Narrate(context.Background(), engine.Result{}); err != nil || text != "Rain lashes the windows." {
		t.Fatalf("%q %v", text, err)
	}
	if !strings.Contains(f.request.Messages[0].Content, "Write in English") {
		t.Fatal("narrator was not instructed to write in English")
	}
	for _, content := range []string{`{}`, `{"text":""}`, `{"text":null}`, `{"Text":"prose"}`, `{"text":"prose","effects":{}}`, `{"text":"a","text":"b"}`, `{"text":"\u001b[2J"}`, `{"text":"` + strings.Repeat("é", 801) + `"}`} {
		f.content = content
		if _, err := l.Narrate(context.Background(), engine.Result{}); err == nil {
			t.Fatal("narration invalide acceptée")
		}
	}
	f.err = errors.New("service arrêté")
	if _, err := l.Interpret(context.Background(), "look", engine.View{}); err == nil {
		t.Fatal("erreur d'inférence masquée")
	}
	if _, err := l.Narrate(context.Background(), engine.Result{}); err == nil {
		t.Fatal("erreur narrative masquée")
	}
}

func TestOfflineEnglishCommands(t *testing.T) {
	view := engine.View{
		Items: []engine.VisibleItem{{ID: "key", Name: "the copper key"}, {ID: "door", Name: "the copper door"}},
		Exits: []engine.VisibleExit{{Direction: "north"}},
	}
	for command, want := range map[string]engine.Action{
		"Take the copper key.":                  {Kind: "take", Target: "key"},
		"  PICK   UP the copper key!  ":         {Kind: "take", Target: "key"},
		"drop key":                              {Kind: "drop", Target: "key"},
		"examine the copper door":               {Kind: "observe", Target: "door"},
		"look at the copper key":                {Kind: "observe", Target: "key"},
		"look":                                  {Kind: "observe"},
		"look around me":                        {Kind: "observe"},
		"go to the north":                       {Kind: "move", Target: "north"},
		"go north":                              {Kind: "move", Target: "north"},
		"north":                                 {Kind: "move", Target: "north"},
		"inventory":                             {Kind: "inventory"},
		"use the copper key on the copper door": {Kind: "use", Tool: "key", Target: "door"},
		"unlock door with key":                  {Kind: "use", Tool: "key", Target: "door"},
		"use key":                               {Kind: "use", Tool: "key"},
		"pick a lock":                           {Kind: "unknown"},
	} {
		got, err := (language.Offline{}).Interpret(context.Background(), command, view)
		if err != nil || got != want {
			t.Fatalf("%q : %+v %v, attendu %+v", command, got, err, want)
		}
	}
	if _, err := (language.Offline{}).Interpret(context.Background(), "take ghost", view); err == nil {
		t.Fatal("objet absent reconnu")
	}
	for _, command := range []string{"open door", "take key and go north", "use key on ghost"} {
		if _, err := (language.Offline{}).Interpret(context.Background(), command, view); err == nil {
			t.Fatalf("incomplete or unresolvable command accepted: %s", command)
		}
	}
	view.Inventory = []engine.VisibleItem{{ID: "spare_key", Name: "a copper key"}}
	if _, err := (language.Offline{}).Interpret(context.Background(), "take the copper key", view); err == nil {
		t.Fatal("ambiguous item name accepted")
	}
}

func FuzzActionJSON(f *testing.F) {
	f.Add([]byte(`{"action":"observe","target":"","tool":""}`))
	f.Add([]byte(`{"action":"use","tool":null}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		a, err := language.DecodeAction(data)
		if err == nil && a.Validate() != nil {
			t.Fatal("action non conforme acceptée")
		}
	})
}

func TestFrenchGrammarAndPrompts(t *testing.T) {
	w, err := (scenarios.Demo{Language: locale.French}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	e, err := engine.NewWithLanguage(w, locale.French)
	if err != nil {
		t.Fatal(err)
	}
	offline := language.Offline{Language: locale.French}
	for command, want := range map[string]engine.Action{
		"Prendre la clé de cuivre.": {Kind: "take", Target: "cle"},
		"regarder autour de moi":    {Kind: "observe"},
		"examiner la clé de cuivre": {Kind: "observe", Target: "cle"},
		"aller au nord":             {Kind: "move", Target: "nord"},
		"nord":                      {Kind: "move", Target: "nord"},
		"inventaire":                {Kind: "inventory"},
	} {
		got, err := offline.Interpret(context.Background(), command, e.View())
		if err != nil || got != want {
			t.Fatalf("%q: %+v %v", command, got, err)
		}
	}
	fake := &fakeClient{content: `{"action":"take","target":"cle","tool":""}`}
	llm := language.LLM{Client: fake, Language: locale.French}
	a, err := llm.Interpret(context.Background(), "Je ramasse la clé de cuivre.", e.View())
	if err != nil || a.Target != "cle" {
		t.Fatalf("%+v %v", a, err)
	}
	if !strings.Contains(fake.request.Messages[0].Content, "commandes françaises") || !strings.Contains(fake.request.Messages[1].Content, "clé de cuivre") {
		t.Fatal("French interpreter context missing")
	}
	result, err := e.Apply(a)
	if err != nil {
		t.Fatal(err)
	}
	if result.Message != "Vous prenez la clé de cuivre." {
		t.Fatal(result.Message)
	}
	fake.content = `{"text":"Vous ramassez la clé de cuivre."}`
	if _, err := llm.Narrate(context.Background(), result); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fake.request.Messages[0].Content, "Écris en français") || !strings.Contains(fake.request.Messages[1].Content, result.Message) {
		t.Fatal("French narration context missing")
	}
}
