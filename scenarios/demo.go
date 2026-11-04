// Package scenarios provides the embedded demonstration adventure.
package scenarios

import (
	"bytes"
	"context"
	_ "embed"

	"ai-venture/internal/locale"
	"ai-venture/internal/world"
)

//go:embed lighthouse.json
var demo []byte

//go:embed phare.json
var frenchDemo []byte

type Demo struct{ Language locale.Language }

func (source Demo) Load(ctx context.Context) (world.Definition, error) {
	if err := ctx.Err(); err != nil {
		return world.Definition{}, err
	}
	selected, err := locale.Parse(string(source.Language))
	if err != nil {
		return world.Definition{}, err
	}
	data := demo
	if selected == locale.French {
		data = frenchDemo
	}
	return world.Load(bytes.NewReader(data))
}
