// Package locale holds player language choices and translates game messages.
// The zero value selects English. No package-level mutable language is used.
package locale

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

type Language string

const (
	English Language = "en"
	French  Language = "fr"
)

func Parse(value string) (Language, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "en":
		return English, nil
	case "fr":
		return French, nil
	default:
		return "", fmt.Errorf("unsupported language %q: use en or fr / langue inconnue : utilisez en ou fr", value)
	}
}

//go:embed fr.json
var frenchJSON []byte

var french = func() map[string]string {
	var messages map[string]string
	if err := json.Unmarshal(frenchJSON, &messages); err != nil {
		panic(err)
	}
	return messages
}()

// Text translates a format string before interpolation, never user/model data.
func (l Language) Text(format string, args ...any) string {
	if l == French {
		if translated, ok := french[format]; ok {
			format = translated
		}
	}
	if len(args) == 0 {
		return format
	}
	return fmt.Sprintf(format, args...)
}

type messageError struct {
	format string
	args   []any
	cause  error
}

func (e *messageError) Error() string { return e.cause.Error() }
func (e *messageError) Unwrap() error { return e.cause }

// Errorf preserves Go's error wrapping while retaining a translatable message.
func Errorf(format string, args ...any) error {
	return &messageError{format: format, args: args, cause: fmt.Errorf(format, args...)}
}

// Error localizes our messages; external diagnostics remain intact.
func (l Language) Error(err error) string {
	if err == nil {
		return ""
	}
	if message, ok := err.(*messageError); ok {
		args := append([]any(nil), message.args...)
		for i, arg := range args {
			if cause, ok := arg.(error); ok {
				args[i] = l.Error(cause)
			}
		}
		// fmt.Sprintf does not support %w; translate the original format first.
		format := strings.ReplaceAll(l.Text(message.format), "%w", "%v")
		return fmt.Sprintf(format, args...)
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		parts := []string{}
		for _, cause := range joined.Unwrap() {
			parts = append(parts, l.Error(cause))
		}
		return strings.Join(parts, "\n")
	}
	return err.Error()
}
