package terminal

import (
	"context"
	"image/png"
	"os"

	"ai-venture/internal/presentation"
)

// Paths come exclusively from the Go compositor, never from generated text.
func (s *Session) publishScene(ctx context.Context, path string) {
	if ctx.Err() != nil {
		return
	}
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	config, err := png.DecodeConfig(f)
	if err != nil || config.Width != 640 || config.Height != 384 {
		return
	}
	if _, err := f.Seek(0, 0); err != nil {
		return
	}
	img, err := png.Decode(f)
	if err != nil {
		return
	}
	s.Observer(presentation.Event{Kind: presentation.Scene, Image: img})
}
