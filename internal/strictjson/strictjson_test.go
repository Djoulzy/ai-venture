package strictjson_test

import (
	"strings"
	"testing"

	"ai-venture/internal/strictjson"
)

func TestAmbiguousDocumentsAreRejected(t *testing.T) {
	for _, text := range []string{`{"a":1,"a":2}`, `{"a":{"b":1,"b":2}}`, `{"a":null}`, `{} {}`, `[] true`, strings.Repeat("[", 66) + strings.Repeat("]", 66)} {
		var value any
		if err := strictjson.Decode([]byte(text), &value); err == nil {
			t.Fatalf("document ambigu accepté : %s", text)
		}
	}
	var out struct {
		Name string `json:"name"`
	}
	if err := strictjson.Decode([]byte(`{"unknown":"x"}`), &out); err == nil {
		t.Fatal("champ inconnu accepté")
	}
	if err := strictjson.Decode([]byte(`{"name":"Phare"}`), &out); err != nil {
		t.Fatal(err)
	}
}
