package hooks

import (
	"bytes"
	"encoding/json"
	"fmt"
)

func cloneOutput(output interface{}) interface{} {
	switch typed := output.(type) {
	case *ToolExecuteOutput:
		return &ToolExecuteOutput{Args: cloneRawMessage(typed.Args)}
	default:
		return output
	}
}

func extractUpdatedArgs(local interface{}, original interface{}) json.RawMessage {
	localOut, ok := local.(*ToolExecuteOutput)
	if !ok {
		return nil
	}
	originalOut, ok := original.(*ToolExecuteOutput)
	if !ok {
		if len(localOut.Args) == 0 {
			return nil
		}
		return cloneRawMessage(localOut.Args)
	}
	if bytes.Equal(bytes.TrimSpace(localOut.Args), bytes.TrimSpace(originalOut.Args)) {
		return nil
	}
	return cloneRawMessage(localOut.Args)
}

func applyUpdatedArgs(output interface{}, updated json.RawMessage) error {
	if len(updated) == 0 {
		return nil
	}
	out, ok := output.(*ToolExecuteOutput)
	if !ok {
		return fmt.Errorf("unsupported output type %T", output)
	}
	out.Args = cloneRawMessage(updated)
	return nil
}

func mergeJSONObjects(updates []json.RawMessage) (json.RawMessage, error) {
	if len(updates) == 0 {
		return nil, nil
	}
	if len(updates) == 1 {
		return cloneRawMessage(updates[0]), nil
	}

	merged := map[string]interface{}{}
	for _, update := range updates {
		if len(update) == 0 {
			continue
		}
		var obj map[string]interface{}
		if err := json.Unmarshal(update, &obj); err != nil {
			return nil, fmt.Errorf("merge updated input: %w", err)
		}
		if obj == nil {
			return nil, fmt.Errorf("merge updated input: non-object update")
		}
		for key, value := range obj {
			merged[key] = value
		}
	}
	return json.Marshal(merged)
}

func cloneRawMessage(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	clone := make([]byte, len(raw))
	copy(clone, raw)
	return clone
}
