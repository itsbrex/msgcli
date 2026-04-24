package mcp

import (
	"context"
	"encoding/json"
	"testing"
)

func TestRegistryDispatch(t *testing.T) {
	r := NewRegistry()
	r.Register(Tool{
		Info: ToolInfo{
			Name:        "echo",
			Description: "echoes input",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"msg":{"type":"string"}},"required":["msg"]}`),
		},
		Handler: func(ctx context.Context, args json.RawMessage) (ToolCallResult, error) {
			var p struct {
				Msg string `json:"msg"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return ToolCallResult{}, err
			}
			return ToolCallResult{Content: []ToolContent{{Type: "text", Text: p.Msg}}}, nil
		},
	})

	list := r.List()
	if len(list) != 1 || list[0].Name != "echo" {
		t.Fatalf("unexpected list: %+v", list)
	}

	res, err := r.Call(context.Background(), "echo", json.RawMessage(`{"msg":"hi"}`))
	if err != nil {
		t.Fatalf("call error: %v", err)
	}
	if len(res.Content) != 1 || res.Content[0].Text != "hi" {
		t.Fatalf("unexpected result: %+v", res)
	}

	if _, err := r.Call(context.Background(), "nonexistent", nil); err == nil {
		t.Fatalf("expected error for unknown tool")
	}
}
