package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
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

func TestServerRunReturnsOnContextCancel(t *testing.T) {
	// A reader that blocks forever (simulates stdin with no input).
	reader, writer := io.Pipe()
	defer writer.Close()

	reg := NewRegistry()
	var out bytes.Buffer
	s := NewServer(reg, reader, &out, ServerInfo{Name: "msgcli", Version: "test"})

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Run(ctx)
	}()

	// Let Run start and block on the reader.
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("Run did not return within 500ms of context cancel")
	}
}

func TestServerRunReturnsOnCleanEOF(t *testing.T) {
	// A reader that emits one request then closes (simulates client closing stdin).
	input := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n")
	reg := NewRegistry()
	var out bytes.Buffer
	s := NewServer(reg, input, &out, ServerInfo{Name: "msgcli", Version: "test"})

	done := make(chan error, 1)
	go func() { done <- s.Run(context.Background()) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected nil on EOF, got: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("Run did not return on EOF within 500ms")
	}
	// The one request should have been handled.
	if !bytes.Contains(out.Bytes(), []byte(`"jsonrpc":"2.0"`)) {
		t.Fatalf("expected response written, got: %q", out.String())
	}
}
