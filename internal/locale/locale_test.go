package locale

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestLanguageSelection(t *testing.T) {
	for input, want := range map[string]Language{"": English, "en": English, "EN": English, " fr ": French} {
		got, err := Parse(input)
		if err != nil || got != want {
			t.Fatalf("%q: %q, %v", input, got, err)
		}
	}
	if _, err := Parse("de"); err == nil {
		t.Fatal("unsupported language accepted")
	}
}

func TestLocalizedErrorsPreserveCausesAndData(t *testing.T) {
	cause := &os.PathError{Op: "open", Path: "player%file", Err: os.ErrNotExist}
	err := Errorf("reading save file: %w", cause)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatal("error identity lost")
	}
	var pathError *os.PathError
	if !errors.As(err, &pathError) || pathError != cause {
		t.Fatal("typed cause lost")
	}
	if got := French.Error(err); got != "lecture de sauvegarde : "+cause.Error() {
		t.Fatal(got)
	}
	if got := English.Error(err); got != err.Error() {
		t.Fatal(got)
	}
	joined := errors.Join(Errorf("save file is too large"), err)
	if got := French.Error(joined); !strings.Contains(got, "sauvegarde trop volumineuse\nlecture de sauvegarde :") {
		t.Fatal(got)
	}
	// Player content must not be interpreted as a translation key or a format.
	if got := French.Text("Fact: %s", "Game saved. %s"); got != "Fait : Game saved. %s" {
		t.Fatal(got)
	}
	if got := (Language("")).Text("Game saved."); got != "Game saved." {
		t.Fatal(got)
	}
}
