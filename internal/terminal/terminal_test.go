package terminal_test

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"ai-venture/internal/engine"
	"ai-venture/internal/inference"
	"ai-venture/internal/language"
	"ai-venture/internal/locale"
	"ai-venture/internal/persistence"
	"ai-venture/internal/terminal"
	"ai-venture/scenarios"
)

func game(t *testing.T) *engine.Engine {
	t.Helper()
	w, err := (scenarios.Demo{}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	e, err := engine.New(w)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func session(t *testing.T, e *engine.Engine, commands string, out *bytes.Buffer) terminal.Session {
	t.Helper()
	return terminal.Session{Engine: e, Interpreter: language.Offline{}, Input: strings.NewReader(commands), Output: out, SavePath: filepath.Join(t.TempDir(), "partie.json"), Timeout: time.Second}
}

func TestPlayableSessionSaveLoadAndVictory(t *testing.T) {
	e := game(t)
	var out bytes.Buffer
	s := session(t, e, strings.Join([]string{
		"help", "take key", "save", "drop key", "load", "inventory",
		"east", "take battery", "west", "north", "up", // locked
		"use key on door", "up", "use battery on beacon", "save", "quit",
	}, "\n"), &out)
	if err := s.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !e.View().Completed {
		t.Fatal("la partie n'a pas pu être terminée")
	}
	loaded := game(t)
	if err := persistence.Load(s.SavePath, loaded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(e.Snapshot(), loaded.Snapshot()) {
		t.Fatal("progression perdue")
	}
	for _, text := range []string{"Game saved", "Game loaded", "Action refused", "Objective complete", "[key]"} {
		if !strings.Contains(out.String(), text) {
			t.Fatalf("sortie absente : %s\n%s", text, out.String())
		}
	}
}

type scriptedClient struct {
	responses []string
	calls     int
}

func (c *scriptedClient) Complete(_ context.Context, r inference.Request) (string, error) {
	c.calls++
	if len(c.responses) == 0 {
		return "", errors.New("service indisponible")
	}
	result := c.responses[0]
	c.responses = c.responses[1:]
	return result, nil
}

func TestLLMProposalsAndNarrationFailure(t *testing.T) {
	e := game(t)
	var out bytes.Buffer
	fake := &scriptedClient{responses: []string{
		`{"text":"The mooring ropes creak."}`,                              // initial narration
		`{"action":"take","target":"key","tool":""}`,                       // valid proposal
		`{"text":"The key is in your hand.","effects":{"door_open":true}}`, // invalid narration
		`{"action":"move","target":"lantern_room","tool":""}`,              // valid shape, impossible move
		`{"action":"take","target":"battery","tool":"","flags":{}}`,        // invalid proposal
	}}
	s := session(t, e, "I pick up the key.\nTeleport me to the top.\nInvent an item.\n/save\n/inventory\n/quit\n", &out)
	s.Interpreter = language.LLM{Client: fake}
	s.Narrator = language.LLM{Client: fake}
	if err := s.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	state := e.Snapshot()
	if state.Location != "quay" || state.Turn != 1 || state.ItemLocations["key"] != "@inventory" || state.Flags["door_open"] {
		t.Fatalf("état incorrect : %+v", state)
	}
	if fake.calls != 5 {
		t.Fatalf("les commandes locales ont appelé le LLM : %d appels", fake.calls)
	}
	for _, text := range []string{"Narration unavailable", "Action refused", "No action applied", "Game saved"} {
		if !strings.Contains(out.String(), text) {
			t.Fatalf("sortie absente : %s", text)
		}
	}
}

func TestUnavailableLLMAllowsLocalRecovery(t *testing.T) {
	e := game(t)
	var out bytes.Buffer
	fake := &scriptedClient{}
	s := session(t, e, "take key\n/take key\n/quit\n", &out)
	s.Interpreter = language.LLM{Client: fake}
	if err := s.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if e.Snapshot().Turn != 1 || fake.calls != 1 {
		t.Fatal("reprise locale impossible")
	}
}

func TestOversizedInputCanRecoverAndEOFIsClean(t *testing.T) {
	e := game(t)
	var out bytes.Buffer
	s := session(t, e, strings.Repeat("a", 10000)+"\ntake key", &out)
	if err := s.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if e.Snapshot().Turn != 1 || !strings.Contains(out.String(), "Command is too long") {
		t.Fatal("reprise après ligne trop longue impossible")
	}
}

func TestInputBoundaryAndHelpAlias(t *testing.T) {
	e := game(t)
	var out bytes.Buffer
	command := "take key" + strings.Repeat(" ", language.MaxCommandBytes-len("take key"))
	s := session(t, e, "?\n"+command+"\r\nquit\n", &out)
	if err := s.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if e.Snapshot().Turn != 1 || !strings.Contains(out.String(), "Game commands") {
		t.Fatal("limite de saisie ou help incorrecte")
	}
}

func TestTerminalStripsEscapeSequences(t *testing.T) {
	e := game(t)
	var out bytes.Buffer
	s := session(t, e, "quit\n", &out)
	s.Title = "\x1b[2JUn titre\u202e"
	if err := s.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(out.String(), "\x1b\u202e") {
		t.Fatal("séquence de contrôle affichée")
	}
}

func TestFrenchSessionSaveLoadAndVictory(t *testing.T) {
	w, err := (scenarios.Demo{Language: locale.French}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	e, err := engine.NewWithLanguage(w, locale.French)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	s := session(t, e, strings.Join([]string{
		"/aide", "/prendre la clé de cuivre", "/sauver", "/déposer cle", "/charger", "/inventaire",
		"/est", "/prendre batterie", "/ouest", "/nord", "/haut",
		"/ouvre porte avec cle", "/haut", "/utiliser batterie sur balise", "/sauver", "/quitter",
	}, "\n"), &out)
	// Slash commands must use the session's selected language, bypassing the LLM.
	fake := &scriptedClient{}
	s.Interpreter = language.LLM{Client: fake, Language: locale.French}
	if err := s.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fake.calls != 0 || !e.View().Completed || e.Snapshot().Turn != 8 {
		t.Fatalf("invalid French session: %+v\n%s", e.Snapshot(), out.String())
	}
	for _, text := range []string{"Commandes du jeu", "Partie sauvegardée", "Partie chargée", "Objectif accompli", "[cle]", "Action refusée : La porte de cuivre est verrouillée"} {
		if !strings.Contains(out.String(), text) {
			t.Fatalf("missing %q in %s", text, out.String())
		}
	}
	loaded, err := engine.NewWithLanguage(w, locale.French)
	if err != nil {
		t.Fatal(err)
	}
	if err := persistence.Load(s.SavePath, loaded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(e.Snapshot(), loaded.Snapshot()) {
		t.Fatal("French save lost state")
	}
	english := game(t)
	before := english.Snapshot()
	if err := persistence.Load(s.SavePath, english); err == nil {
		t.Fatal("French save accepted by English scenario")
	}
	if !reflect.DeepEqual(before, english.Snapshot()) {
		t.Fatal("incompatible save changed English state")
	}
}

func TestFrenchInferenceFailureCanRecoverLocally(t *testing.T) {
	w, err := (scenarios.Demo{Language: locale.French}).Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	e, err := engine.NewWithLanguage(w, locale.French)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	s := session(t, e, "prends cle\n/prendre cle\n/quitter\n", &out)
	fake := &scriptedClient{}
	s.Interpreter = language.LLM{Client: fake, Language: locale.French}
	if err := s.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fake.calls != 1 || e.Snapshot().Turn != 1 {
		t.Fatal("French local recovery failed")
	}
	if !strings.Contains(out.String(), "Aucune action appliquée. Essayez /aide") || strings.Contains(out.String(), "No action applied") {
		t.Fatal(out.String())
	}
}

func TestExactDirectionsBypassLLMInBothLanguages(t *testing.T) {
	for _, entry := range []struct {
		language locale.Language
		commands string
		location string
	}{
		{locale.English, "east\n/quit\n", "workshop"},
		{locale.French, "est\n/quitter\n", "atelier"},
	} {
		t.Run(string(entry.language), func(t *testing.T) {
			w, err := (scenarios.Demo{Language: entry.language}).Load(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			e, err := engine.NewWithLanguage(w, entry.language)
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			s := session(t, e, entry.commands, &out)
			fake := &scriptedClient{}
			s.Interpreter = language.LLM{Client: fake, Language: entry.language}
			if err := s.Run(context.Background()); err != nil {
				t.Fatal(err)
			}
			if fake.calls != 0 || e.Snapshot().Location != entry.location || e.Snapshot().Turn != 1 {
				t.Fatalf("native direction failed: %+v", e.Snapshot())
			}
		})
	}
}
