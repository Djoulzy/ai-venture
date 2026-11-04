// Package strictjson rejects ambiguous JSON at trust boundaries.
package strictjson

import (
	"bytes"
	"encoding/json"
	"io"

	"ai-venture/internal/locale"
)

// Decode rejects duplicate keys, unknown struct fields, nulls and trailing values.
// Prototype formats deliberately use explicit empty strings/collections instead of null.
func Decode(data []byte, dst any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if err := value(d, 0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return locale.Errorf("JSON: unexpected or invalid trailing content")
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return locale.Errorf("JSON: %w", err)
	}
	return nil
}

func value(d *json.Decoder, depth int) error {
	if depth > 64 {
		return locale.Errorf("JSON nesting is too deep")
	}
	t, err := d.Token()
	if err != nil {
		return err
	}
	if t == nil {
		return locale.Errorf("JSON: null is not allowed")
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return locale.Errorf("JSON: invalid or duplicate key %q", key)
			}
			seen[name] = true
			if err := value(d, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := value(d, depth+1); err != nil {
				return err
			}
		}
	default:
		return locale.Errorf("JSON: unexpected delimiter")
	}
	_, err = d.Token()
	return err
}
