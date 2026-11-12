// Package presentation carries immutable display events from the sequential
// game session to a frontend. It never grants access to a mutable engine.
package presentation

import (
	"context"
	"image"
	"io"
	"strings"
	"unicode"

	"ai-venture/internal/engine"
	"ai-venture/internal/illustration"
)

type Kind uint8

const (
	Message Kind = iota
	Title
	View
	Ready
	Scene
	Art
)

type Tone uint8

const (
	Plain Tone = iota
	Info
	Fact
	Narration
	Success
	Warning
	Error
	Command
)

type Event struct {
	Kind  Kind
	Tone  Tone
	Text  string
	View  engine.View
	Image image.Image
	Frame illustration.Frame
	Turn  uint64
}

type Observer func(Event)

// Clean removes terminal controls and bidi overrides; native rendering never
// interprets model/scenario/player text as markup or commands.
func Clean(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.In(r, unicode.Cf) || (unicode.IsControl(r) && r != '\n' && r != '\t') {
			return -1
		}
		return r
	}, text)
}

// Reader bridges explicit UI submissions to the existing command loop. It is
// cancellable even when no player input arrives, unlike an open terminal stdin.
type Reader struct {
	Context  context.Context
	Commands <-chan string
	pending  string
}

func (r *Reader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(r.pending) == 0 {
		select {
		case <-r.Context.Done():
			return 0, r.Context.Err()
		case command, ok := <-r.Commands:
			if !ok {
				return 0, io.EOF
			}
			r.pending = command + "\n"
		}
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}
