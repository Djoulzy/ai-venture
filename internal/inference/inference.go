// Package inference is the provider-independent language model boundary.
package inference

import (
	"context"
	"encoding/json"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Request struct {
	Messages    []Message
	Schema      json.RawMessage
	Temperature float64
	MaxTokens   int
}

// Client returns untrusted generated content; callers must validate it.
type Client interface {
	Complete(context.Context, Request) (string, error)
}
