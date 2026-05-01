package mcp

import "encoding/json"

// unmarshalArgs decodes args into dst when args is non-empty.
// It is a no-op when args is nil or zero-length (no arguments provided).
func unmarshalArgs(args json.RawMessage, dst any) error {
	if len(args) == 0 {
		return nil
	}
	return json.Unmarshal(args, dst)
}

// textResult wraps a plain string in a ToolCallResult with a single text content item.
func textResult(text string) ToolCallResult {
	return ToolCallResult{Content: []ToolContent{{Type: "text", Text: text}}}
}

// jsonResult marshals v to JSON and returns it as a textResult.
func jsonResult(v any) (ToolCallResult, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return ToolCallResult{}, err
	}
	return textResult(string(b)), nil
}

// okResult is the canonical success response for write operations.
var okResult = textResult(`{"ok":true}`)
