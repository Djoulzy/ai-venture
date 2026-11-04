package terminal

import (
	"fmt"
	"io"
	"os"

	"ai-venture/internal/locale"
	"github.com/mattn/go-isatty"
)

type ColorMode string

const (
	ColorAuto   ColorMode = "auto"
	ColorAlways ColorMode = "always"
	ColorNever  ColorMode = "never"
)

func ParseColorMode(value string) (ColorMode, error) {
	switch ColorMode(value) {
	case ColorAuto, ColorAlways, ColorNever:
		return ColorMode(value), nil
	default:
		return ColorAuto, locale.Errorf("invalid color mode %q: use auto, always or never", value)
	}
}

type Style uint8

const (
	StylePlain Style = iota
	StyleTitle
	StyleLocation
	StyleObject
	StyleExit
	StyleFact
	StyleNarration
	StyleSuccess
	StyleWarning
	StyleError
	StyleInfo
	StylePrompt
	StyleArtBlue
	StyleArtRed
)

var styleCodes = [...]string{
	StylePlain:     "",
	StyleTitle:     "1;35",
	StyleLocation:  "1;36",
	StyleObject:    "33",
	StyleExit:      "36",
	StyleFact:      "32",
	StyleNarration: "35",
	StyleSuccess:   "1;32",
	StyleWarning:   "33",
	StyleError:     "1;31",
	StyleInfo:      "90",
	StylePrompt:    "1;36",
	StyleArtBlue:   "34",
	StyleArtRed:    "31",
}

// Printer adds only application-owned SGR sequences, after sanitizing all text.
// Each styled message resets its attributes before returning control to input.
type Printer struct {
	Output io.Writer
	Color  bool
}

func NewPrinter(output io.Writer, mode ColorMode) Printer {
	color := mode == ColorAlways
	if (mode == ColorAuto || mode == "") && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb" {
		if file, ok := output.(interface{ Fd() uintptr }); ok {
			color = isatty.IsTerminal(file.Fd()) || isatty.IsCygwinTerminal(file.Fd())
		}
	}
	return Printer{Output: output, Color: color}
}

func (p Printer) text(style Style, message string) string {
	message = safe(message)
	if !p.Color || int(style) >= len(styleCodes) || styleCodes[style] == "" || message == "" {
		return message
	}
	return "\x1b[" + styleCodes[style] + "m" + message + "\x1b[0m"
}

func (p Printer) Print(style Style, message string) {
	_, _ = fmt.Fprint(p.Output, p.text(style, message))
}

func (p Printer) Println(style Style, message string) {
	_, _ = fmt.Fprintln(p.Output, p.text(style, message))
}
