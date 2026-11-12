package desktop

import (
	"strings"
	"unicode/utf8"

	"ai-venture/internal/engine"
	"ai-venture/internal/illustration"
	"ai-venture/internal/language"
	"ai-venture/internal/presentation"
)

type entry struct {
	tone presentation.Tone
	text string
}

type model struct {
	title   string
	view    engine.View
	turn    uint64
	ready   bool
	status  string
	entries []entry
	version uint64
	art     illustration.Frame
}

func (m *model) append(tone presentation.Tone, text string) {
	text = strings.TrimSpace(presentation.Clean(text))
	if text == "" {
		return
	}
	runes := []rune(text)
	if len(runes) > 8000 {
		text = string(runes[:8000]) + "…"
	}
	m.entries = append(m.entries, entry{tone, text})
	if len(m.entries) > 200 {
		m.entries = append([]entry(nil), m.entries[len(m.entries)-200:]...)
	}
	m.version++
}

func (m *model) apply(event presentation.Event) {
	switch event.Kind {
	case presentation.Title:
		m.title = presentation.Clean(event.Text)
	case presentation.Message:
		if event.Tone == presentation.Info {
			m.status = presentation.Clean(event.Text)
		} else {
			m.append(event.Tone, event.Text)
		}
	case presentation.View:
		m.view = event.View
		m.turn = event.Turn
		m.art = illustration.Frame{}
		m.append(presentation.Plain, event.View.Name+"\n"+event.View.Description)
	case presentation.Ready:
		m.ready = true
	case presentation.Art:
		if event.Frame.Validate() == nil {
			m.art = event.Frame.Clone()
		}
	}
}

// editBuffer is rune-aware, while enforcing the engine's byte limit.
type editBuffer struct {
	text   []rune
	cursor int
	all    bool
}

func (e *editBuffer) set(text string) { e.text = nil; e.cursor = 0; e.all = false; e.insert(text) }
func (e *editBuffer) String() string  { return string(e.text) }
func (e *editBuffer) insert(text string) {
	if text != "" && e.all {
		e.set("")
	}
	for _, r := range presentation.Clean(text) {
		if r == '\n' || r == '\r' || r == '\t' {
			r = ' '
		}
		if utf8.RuneLen(r)+len(e.String()) > language.MaxCommandBytes {
			break
		}
		e.text = append(e.text, 0)
		copy(e.text[e.cursor+1:], e.text[e.cursor:])
		e.text[e.cursor] = r
		e.cursor++
	}
}
func (e *editBuffer) backspace() {
	if e.all {
		e.set("")
		return
	}
	if e.cursor > 0 {
		e.text = append(e.text[:e.cursor-1], e.text[e.cursor:]...)
		e.cursor--
	}
}
func (e *editBuffer) delete() {
	if e.all {
		e.set("")
		return
	}
	if e.cursor < len(e.text) {
		e.text = append(e.text[:e.cursor], e.text[e.cursor+1:]...)
	}
}
