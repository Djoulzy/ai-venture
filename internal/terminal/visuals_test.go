package terminal_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"ai-venture/internal/engine"
)

type sceneRenderer struct {
	calls int
	fail  bool
}

func (r *sceneRenderer) Render(ctx context.Context, view engine.View) (string, error) {
	r.calls++
	if _, ok := ctx.Deadline(); !ok {
		panic("no render deadline")
	}
	if r.fail {
		return "", errors.New("unavailable")
	}
	return "/tmp/scene.png", nil
}

func TestSceneExportWorksForLocalCommandsAndFailureDoesNotMutateRules(t *testing.T) {
	for _, fail := range []bool{false, true} {
		e := game(t)
		var output bytes.Buffer
		s := session(t, e, "/take key\n/quit\n", &output)
		r := &sceneRenderer{fail: fail}
		s.Visuals = r
		if err := s.Run(context.Background()); err != nil {
			t.Fatal(err)
		}
		if r.calls != 2 || e.Snapshot().Turn != 1 {
			t.Fatal("local scene composition or state progression failed")
		}
		want := "Scene image: /tmp/scene.png"
		if fail {
			want = "Scene image unavailable"
		}
		if !strings.Contains(output.String(), want) {
			t.Fatal(output.String())
		}
	}
}
