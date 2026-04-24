package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestServerInitializeAndListTools(t *testing.T) {
	reg := NewRegistry()
	reg.Register(Tool{
		Info: ToolInfo{Name: "mail_list", Description: "list mail", InputSchema: json.RawMessage(`{"type":"object"}`)},
		Handler: func(ctx context.Context, args json.RawMessage) (ToolCallResult, error) {
			return ToolCallResult{Content: []ToolContent{{Type: "text", Text: "[]"}}}, nil
		},
	})

	input := strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n" +
			`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"mail_list","arguments":{}}}` + "\n",
	)
	var out bytes.Buffer
	s := NewServer(reg, input, &out, ServerInfo{Name: "msgcli", Version: "test"})
	if err := s.Run(context.Background()); err != nil && err != io.EOF {
		t.Fatalf("Run error: %v", err)
	}

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 responses, got %d: %q", len(lines), out.String())
	}

	var init Response
	_ = json.Unmarshal([]byte(lines[0]), &init)
	if init.Error != nil {
		t.Fatalf("initialize returned error: %+v", init.Error)
	}

	var list Response
	_ = json.Unmarshal([]byte(lines[1]), &list)
	if list.Error != nil {
		t.Fatalf("tools/list error: %+v", list.Error)
	}
	listBody, _ := json.Marshal(list.Result)
	if !bytes.Contains(listBody, []byte(`"mail_list"`)) {
		t.Fatalf("expected mail_list in list result: %s", listBody)
	}

	var call Response
	_ = json.Unmarshal([]byte(lines[2]), &call)
	if call.Error != nil {
		t.Fatalf("tools/call error: %+v", call.Error)
	}
}

func TestServerUnknownMethodReturnsError(t *testing.T) {
	reg := NewRegistry()
	input := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"not/a/method"}` + "\n")
	var out bytes.Buffer
	s := NewServer(reg, input, &out, ServerInfo{Name: "msgcli", Version: "test"})
	_ = s.Run(context.Background())

	var resp Response
	_ = json.Unmarshal(bytes.TrimSpace(out.Bytes()), &resp)
	if resp.Error == nil || resp.Error.Code != ErrMethodNotFound {
		t.Fatalf("expected method-not-found, got: %+v", resp)
	}
}

func TestServerToolsCallNullParamsReturnsInvalidParams(t *testing.T) {
	reg := NewRegistry()
	input := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":null}` + "\n")
	var out bytes.Buffer
	s := NewServer(reg, input, &out, ServerInfo{Name: "msgcli", Version: "test"})
	_ = s.Run(context.Background())

	var resp Response
	_ = json.Unmarshal(bytes.TrimSpace(out.Bytes()), &resp)
	if resp.Error == nil || resp.Error.Code != ErrInvalidParams {
		t.Fatalf("expected ErrInvalidParams, got: %+v", resp)
	}
}

func TestServerToolsCallEmptyNameReturnsInvalidParams(t *testing.T) {
	reg := NewRegistry()
	input := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{}}` + "\n")
	var out bytes.Buffer
	s := NewServer(reg, input, &out, ServerInfo{Name: "msgcli", Version: "test"})
	_ = s.Run(context.Background())

	var resp Response
	_ = json.Unmarshal(bytes.TrimSpace(out.Bytes()), &resp)
	if resp.Error == nil || resp.Error.Code != ErrInvalidParams {
		t.Fatalf("expected ErrInvalidParams, got: %+v", resp)
	}
}
