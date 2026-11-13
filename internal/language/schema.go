package language

import (
	"encoding/json"
	"sort"

	"ai-venture/internal/engine"
)

type actionProperty struct {
	Type      string   `json:"type"`
	Enum      []string `json:"enum,omitempty"`
	MaxLength int      `json:"maxLength,omitempty"`
}

type actionObjectSchema struct {
	Type                 string   `json:"type"`
	AdditionalProperties bool     `json:"additionalProperties"`
	Required             []string `json:"required"`
	Properties           struct {
		Action actionProperty `json:"action"`
		Tool   actionProperty `json:"tool"`
		Target actionProperty `json:"target"`
	} `json:"properties"`
}

// visibleActionSchema narrows the shared schema to public references and the
// parameter shapes enforced by Action.Validate. Blocked exits and uncarried
// items remain proposals: engine.Apply alone enforces rule preconditions.
func visibleActionSchema(view engine.View) (json.RawMessage, error) {
	var base actionObjectSchema
	if err := json.Unmarshal([]byte(actionSchema), &base); err != nil {
		return nil, err
	}
	items, directions := map[string]bool{}, map[string]bool{}
	for _, exit := range view.Exits {
		directions[exit.Direction] = true
	}
	for _, entries := range [][]engine.VisibleItem{view.Items, view.Inventory} {
		for _, item := range entries {
			items[item.ID] = true
		}
	}
	itemIDs, exitNames := sortedReferences(items), sortedReferences(directions)
	observations := append([]string{""}, itemIDs...)
	if view.Location != "" {
		observations = append(observations, view.Location)
	}
	var schema struct {
		AnyOf []actionObjectSchema `json:"anyOf"`
	}
	for _, kind := range base.Properties.Action.Enum {
		branch := base
		branch.Properties.Action.Enum = []string{kind}
		branch.Properties.Tool.Enum = []string{""}
		branch.Properties.Target.Enum = []string{""}
		switch kind {
		case "observe":
			branch.Properties.Target.Enum = observations
		case "move":
			if len(exitNames) == 0 {
				continue
			}
			branch.Properties.Target.Enum = exitNames
		case "take", "drop", "use":
			if len(itemIDs) == 0 {
				continue
			}
			if kind == "use" {
				branch.Properties.Tool.Enum = itemIDs
				branch.Properties.Target.Enum = append([]string{""}, itemIDs...)
			} else {
				branch.Properties.Target.Enum = itemIDs
			}
		}
		schema.AnyOf = append(schema.AnyOf, branch)
	}
	return json.Marshal(schema)
}

func sortedReferences(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
