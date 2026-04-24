package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/skylarbpayne/msgcli/internal/graph"
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

func TestMailGetTool(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"m1","subject":"hello"}`))
	}))
	defer srv.Close()
	client := graph.NewTestClient(srv.URL)
	reg := NewRegistry()
	RegisterMailTools(reg, func(_ string) (*graph.Client, error) { return client, nil })
	res, err := reg.Call(context.Background(), "mail_get", json.RawMessage(`{"id":"m1"}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !bytes.Contains([]byte(res.Content[0].Text), []byte(`"subject":"hello"`)) {
		t.Fatalf("unexpected: %q", res.Content[0].Text)
	}
}

func TestMailDeleteTool(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204)
	}))
	defer srv.Close()
	client := graph.NewTestClient(srv.URL)
	reg := NewRegistry()
	RegisterMailTools(reg, func(_ string) (*graph.Client, error) { return client, nil })
	res, err := reg.Call(context.Background(), "mail_delete", json.RawMessage(`{"id":"m1"}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !bytes.Contains([]byte(res.Content[0].Text), []byte(`"ok":true`)) {
		t.Fatalf("unexpected: %q", res.Content[0].Text)
	}
}

func TestCalendarCreateTool(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"e1","subject":"Sync"}`))
	}))
	defer srv.Close()
	client := graph.NewTestClient(srv.URL)
	reg := NewRegistry()
	RegisterCalendarTools(reg, func(_ string) (*graph.Client, error) { return client, nil })
	res, err := reg.Call(context.Background(), "calendar_create", json.RawMessage(`{"subject":"Sync","start":"2026-05-01T10:00:00Z","end":"2026-05-01T10:30:00Z"}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !bytes.Contains([]byte(res.Content[0].Text), []byte(`"subject":"Sync"`)) {
		t.Fatalf("unexpected: %q", res.Content[0].Text)
	}
}

func TestCalendarRespondToolUnknownResponseRejected(t *testing.T) {
	reg := NewRegistry()
	RegisterCalendarTools(reg, func(_ string) (*graph.Client, error) { return nil, nil })
	_, err := reg.Call(context.Background(), "calendar_respond", json.RawMessage(`{"id":"e1","response":"bogus"}`))
	if err == nil {
		t.Fatalf("expected error for unknown response")
	}
}

func TestAuthListTool(t *testing.T) {
	reg := NewRegistry()
	RegisterAuthTools(reg)
	res, err := reg.Call(context.Background(), "auth_list", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	// auth_list may return [] if no accounts, but should always return text content.
	if len(res.Content) == 0 || res.Content[0].Type != "text" {
		t.Fatalf("unexpected: %+v", res)
	}
}

func TestMailListTool(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"value":[{"id":"m1","subject":"hello"}]}`))
	}))
	defer srv.Close()

	client := graph.NewTestClient(srv.URL)
	reg := NewRegistry()
	RegisterMailTools(reg, func(_ string) (*graph.Client, error) { return client, nil })

	res, err := reg.Call(context.Background(), "mail_list", json.RawMessage(`{"limit":5}`))
	if err != nil {
		t.Fatalf("call error: %v", err)
	}
	if len(res.Content) == 0 || res.Content[0].Type != "text" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if !bytes.Contains([]byte(res.Content[0].Text), []byte(`"subject":"hello"`)) {
		t.Fatalf("expected subject in text: %q", res.Content[0].Text)
	}
}
