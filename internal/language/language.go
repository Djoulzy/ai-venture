// Package language translates between player prose and typed engine messages.
package language

import (
	"context"
	"encoding/json"
	"strings"
	"unicode"
	"unicode/utf8"

	"ai-venture/internal/engine"
	"ai-venture/internal/inference"
	"ai-venture/internal/locale"
	"ai-venture/internal/retrieval"
	"ai-venture/internal/strictjson"
)

const MaxCommandBytes = 2048

type Interpreter interface {
	Interpret(context.Context, string, engine.View) (engine.Action, error)
}

type Narrator interface {
	Narrate(context.Context, engine.Result) (string, error)
}

// These schemas describe the entire accepted output, not engine mutations.
const actionSchema = `{"type":"object","additionalProperties":false,"required":["action","target","tool"],"properties":{"action":{"type":"string","enum":["observe","move","take","drop","use","inventory","unknown"]},"target":{"type":"string","maxLength":64},"tool":{"type":"string","maxLength":64}}}`
const narrationSchema = `{"type":"object","additionalProperties":false,"required":["text"],"properties":{"text":{"type":"string","minLength":1,"maxLength":800}}}`

type LLM struct {
	Client   inference.Client
	Language locale.Language
}

func (l LLM) Interpret(ctx context.Context, command string, view engine.View) (engine.Action, error) {
	if strings.TrimSpace(command) == "" || len(command) > MaxCommandBytes {
		return engine.Action{}, locale.Errorf("command is empty or too long")
	}
	payload, err := json.Marshal(struct {
		Command string      `json:"command"`
		View    engine.View `json:"view"`
	}{command, view})
	if err != nil {
		return engine.Action{}, err
	}
	content, err := l.Client.Complete(ctx, inference.Request{
		Messages: []inference.Message{{Role: "system", Content: commandPrompt(l.Language)}, {Role: "user", Content: string(payload)}},
		Schema:   json.RawMessage(actionSchema), Temperature: 0, MaxTokens: 160,
	})
	if err != nil {
		return engine.Action{}, err
	}
	return DecodeAction([]byte(content))
}

// DecodeAction enforces the schema again, independently of model/provider claims.
func DecodeAction(data []byte) (engine.Action, error) {
	var action engine.Action
	if len(data) > 4096 {
		return action, locale.Errorf("proposal is too large")
	}
	if err := strictjson.Decode(data, &action); err != nil {
		return action, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return action, err
	}
	if len(fields) != 3 || fields["action"] == nil || fields["target"] == nil || fields["tool"] == nil {
		return action, locale.Errorf("proposal must contain exactly action, target and tool")
	}
	return action, action.Validate()
}

func (l LLM) Narrate(ctx context.Context, result engine.Result) (string, error) {
	return l.narrate(ctx, result, nil)
}

func (l LLM) narrate(ctx context.Context, result engine.Result, memory []retrieval.Document) (string, error) {
	maxTokens := 160
	if len(memory) > 0 {
		// Leave room for one contextual detail and the JSON envelope, especially
		// in French. The existing 800-character validation still applies.
		maxTokens = 256
	}
	payload, err := json.Marshal(struct {
		engine.Result
		Memory []retrieval.Document `json:"memory,omitempty"`
	}{result, memory})
	if err != nil {
		return "", err
	}
	content, err := l.Client.Complete(ctx, inference.Request{
		Messages: []inference.Message{
			{Role: "system", Content: narratorPrompt(l.Language)},
			{Role: "user", Content: string(payload)},
		}, Schema: json.RawMessage(narrationSchema), Temperature: 0.2, MaxTokens: maxTokens,
	})
	if err != nil {
		return "", err
	}
	if len(content) > 8192 {
		return "", locale.Errorf("narration is too large")
	}
	var out struct {
		Text string `json:"text"`
	}
	if err := strictjson.Decode([]byte(content), &out); err != nil {
		return "", err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(content), &fields); err != nil {
		return "", err
	}
	if len(fields) != 1 || fields["text"] == nil || strings.TrimSpace(out.Text) == "" || utf8.RuneCountInString(out.Text) > 800 {
		return "", locale.Errorf("narration is empty or invalid")
	}
	for _, r := range out.Text {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return "", locale.Errorf("control character in narration")
		}
	}
	return strings.TrimSpace(out.Text), nil
}
