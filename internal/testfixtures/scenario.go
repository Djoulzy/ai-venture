// Package testfixtures provides data only to tests, never to the game executable.
package testfixtures

import (
	"ai-venture/internal/locale"
	"ai-venture/internal/world"
	"bytes"
	"context"
	"embed"
)

//go:embed testdata/*.json
var fixtures embed.FS

type Scenario struct{ Language locale.Language }

func (s Scenario) Load(ctx context.Context) (world.Definition, error) {
	if err := ctx.Err(); err != nil {
		return world.Definition{}, err
	}
	lang, err := locale.Parse(string(s.Language))
	if err != nil {
		return world.Definition{}, err
	}
	data, err := fixtures.ReadFile("testdata/engine-" + string(lang) + ".json")
	if err != nil {
		return world.Definition{}, err
	}
	return world.Load(bytes.NewReader(data))
}
