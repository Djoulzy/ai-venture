package terminal

import (
	"context"

	"ai-venture/internal/illustration"
)

type cachedIllustration struct {
	signature [32]byte
	frame     illustration.Frame
}

// Draw validates the complete frame before emitting even its first row.
func (p Printer) Draw(frame illustration.Frame) error {
	if err := frame.Validate(); err != nil {
		return err
	}
	styles := map[illustration.Color]Style{
		illustration.Default: StylePlain, illustration.Cyan: StyleExit,
		illustration.Yellow: StyleObject, illustration.Green: StyleFact,
		illustration.Blue: StyleArtBlue, illustration.Magenta: StyleNarration,
		illustration.Red: StyleArtRed, illustration.Gray: StyleInfo,
	}
	for _, line := range frame.Lines {
		p.Println(styles[line.Color], line.Text)
	}
	return nil
}

func (s *Session) illustrate(ctx context.Context, generate bool) {
	if s.Illustrator == nil || ctx.Err() != nil {
		return
	}
	view := s.Engine.View()
	signature := illustration.Signature(view)
	cached, ok := s.artCache[view.Location]
	if !ok || cached.signature != signature {
		// Slash commands can show a current cached drawing, but never call a model.
		if !generate {
			return
		}
		s.printStyle(StyleInfo, "[Drawing the location...]")
		artCtx, cancel := context.WithTimeout(ctx, s.Timeout)
		frame, err := s.Illustrator.Illustrate(artCtx, view)
		cancel()
		if err == nil {
			err = frame.Validate()
		}
		if err != nil {
			if ctx.Err() == nil {
				s.printStyle(StyleWarning, "[Illustration unavailable; continuing with the location description.]")
			}
			return
		}
		// At most one frame per location, with a bounded per-session cache.
		if s.artCache == nil || len(s.artCache) >= 64 {
			s.artCache = map[string]cachedIllustration{}
		}
		cached = cachedIllustration{signature, frame.Clone()}
		s.artCache[view.Location] = cached
	}
	_ = (Printer{Output: s.Output, Color: s.Color}).Draw(cached.frame)
}
