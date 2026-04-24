package mcp

import (
	"encoding/json"
	"testing"
)

func TestRequestDecode(t *testing.T) {
	raw := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`
	var req Request
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if req.JSONRPC != "2.0" || req.Method != "initialize" {
		t.Fatalf("unexpected: %+v", req)
	}
	id, ok := req.ID.(float64)
	if !ok || id != 1 {
		t.Fatalf("expected id=1, got %v (%T)", req.ID, req.ID)
	}
}

func TestResponseEncode(t *testing.T) {
	resp := Response{JSONRPC: "2.0", ID: 1, Result: map[string]string{"ok": "yes"}}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var out map[string]interface{}
	_ = json.Unmarshal(data, &out)
	if out["jsonrpc"] != "2.0" {
		t.Fatalf("missing jsonrpc")
	}
	if _, ok := out["error"]; ok {
		t.Fatalf("result response should omit error")
	}
}

func TestErrorResponseEncode(t *testing.T) {
	resp := Response{JSONRPC: "2.0", ID: 1, Error: &RPCError{Code: -32601, Message: "Method not found"}}
	data, _ := json.Marshal(resp)
	s := string(data)
	if !containsAll(s, []string{`"code":-32601`, `"message":"Method not found"`}) {
		t.Fatalf("missing error fields: %s", s)
	}
	if containsSubstr(s, `"result":`) {
		t.Fatalf("error response must omit result")
	}
}

func containsAll(s string, subs []string) bool {
	for _, sub := range subs {
		if !containsSubstr(s, sub) {
			return false
		}
	}
	return true
}

func containsSubstr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
