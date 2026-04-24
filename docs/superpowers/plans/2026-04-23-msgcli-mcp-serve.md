# msgcli mcp serve — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `msgcli mcp serve` — a stdio-transport Model Context Protocol server that exposes every read/write Graph operation msgcli already supports as an MCP tool. Agents (Claude Code, Claude Desktop, Cursor) call mail/calendar operations via MCP, reusing msgcli's keychain-backed auth with zero secret sprawl.

**Architecture:** A thin hand-rolled MCP server (JSON-RPC 2.0 over stdio, no third-party SDK to start) lives in `internal/mcp/`. A `toolRegistry` maps each tool name to a handler that calls the same `graph.Client` methods the CLI commands use. A separate `msgcli mcp install` helper writes the right JSON config entry into Claude Code / Claude Desktop settings. No new Graph scopes.

**Tech Stack:** Go 1.25, `encoding/json`, `bufio`, `os.Stdin`/`os.Stdout`, existing `graph.Client` + `auth.ResolveAccount`. MCP protocol version `2024-11-05` (stable at time of writing).

---

## File Structure

**Create:**
- `internal/mcp/protocol.go` — JSON-RPC 2.0 envelopes + MCP-specific message types (`initialize`, `tools/list`, `tools/call`, etc.).
- `internal/mcp/server.go` — stdio read/write loop, request dispatch, error mapping.
- `internal/mcp/registry.go` — `Tool` struct, `Registry` with `Register` + `Handle`.
- `internal/mcp/tools_mail.go` — mail_list, mail_get, mail_send, mail_reply, mail_move, mail_delete, mail_folders handlers.
- `internal/mcp/tools_calendar.go` — calendar_list, calendar_get, calendar_create, calendar_update, calendar_delete, calendar_respond, calendar_availability handlers.
- `internal/mcp/tools_auth.go` — auth_list, auth_status handlers (read-only, no interactive flows).
- `internal/mcp/server_test.go` — request/response round-trip tests.
- `internal/mcp/registry_test.go` — registry + per-tool dispatch tests.
- `internal/cmd/mcp.go` — `msgcli mcp serve` and `msgcli mcp install` subcommands.
- `internal/cmd/mcp_test.go` — install-helper tests.
- `docs/reference/mcp.md` — user-facing reference (tool catalog, client setup, security notes).

**Modify:**
- None (self-registering commands). If root_test breaks due to new commands, patch as needed.

---

## Task 1: JSON-RPC 2.0 envelope types + round-trip test

**Files:**
- Create: `internal/mcp/protocol.go`, `internal/mcp/protocol_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/mcp/protocol_test.go
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
	if !containsAll(string(data), []string{`"code":-32601`, `"message":"Method not found"`}) {
		t.Fatalf("missing error fields: %s", data)
	}
	if containsAll(string(data), []string{`"result":`}) {
		t.Fatalf("error response must omit result")
	}
}

func containsAll(s string, subs []string) bool {
	for _, sub := range subs {
		if !jsonContains(s, sub) {
			return false
		}
	}
	return true
}

func jsonContains(s, sub string) bool {
	return len(s) >= len(sub) && (indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/mcp/ -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Create protocol.go**

```go
// internal/mcp/protocol.go
package mcp

import "encoding/json"

const ProtocolVersion = "2024-11-05"

// Request is a JSON-RPC 2.0 request or notification.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

// RPCError is the error object carried in a Response.
type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Standard JSON-RPC error codes.
const (
	ErrParse          = -32700
	ErrInvalidRequest = -32600
	ErrMethodNotFound = -32601
	ErrInvalidParams  = -32602
	ErrInternal       = -32603
)

// ServerInfo is returned by initialize.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InitializeResult is the response to the initialize method.
type InitializeResult struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    map[string]interface{} `json:"capabilities"`
	ServerInfo      ServerInfo             `json:"serverInfo"`
}

// ToolListResult is the response to tools/list.
type ToolListResult struct {
	Tools []ToolInfo `json:"tools"`
}

// ToolInfo describes a tool surfaced to clients.
type ToolInfo struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// ToolCallParams is the params envelope for tools/call.
type ToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolCallResult is returned from tools/call.
type ToolCallResult struct {
	Content []ToolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// ToolContent is one element of a tool call's response content.
type ToolContent struct {
	Type string `json:"type"` // "text" or "resource"
	Text string `json:"text,omitempty"`
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/mcp/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/mcp/protocol.go internal/mcp/protocol_test.go
git commit -m "feat(mcp): JSON-RPC 2.0 + MCP protocol types"
```

---

## Task 2: Tool registry

**Files:**
- Create: `internal/mcp/registry.go`, `internal/mcp/registry_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/mcp/registry_test.go
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/mcp/ -run TestRegistryDispatch -v`
Expected: FAIL — "undefined: NewRegistry".

- [ ] **Step 3: Implement registry.go**

```go
// internal/mcp/registry.go
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

// ToolHandler executes a single tool call.
type ToolHandler func(ctx context.Context, args json.RawMessage) (ToolCallResult, error)

// Tool pairs metadata with its handler.
type Tool struct {
	Info    ToolInfo
	Handler ToolHandler
}

// Registry stores available tools for a server.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

func NewRegistry() *Registry { return &Registry{tools: map[string]Tool{}} }

// Register adds or replaces a tool. Panics on empty name (programmer error).
func (r *Registry) Register(t Tool) {
	if t.Info.Name == "" {
		panic("mcp: tool name is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Info.Name] = t
}

// List returns tool metadata sorted by name (stable for clients).
func (r *Registry) List() []ToolInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	infos := make([]ToolInfo, 0, len(r.tools))
	for _, t := range r.tools {
		infos = append(infos, t.Info)
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Name < infos[j].Name })
	return infos
}

// Call dispatches a tools/call request.
func (r *Registry) Call(ctx context.Context, name string, args json.RawMessage) (ToolCallResult, error) {
	r.mu.RLock()
	t, ok := r.tools[name]
	r.mu.RUnlock()
	if !ok {
		return ToolCallResult{}, fmt.Errorf("unknown tool: %s", name)
	}
	return t.Handler(ctx, args)
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/mcp/ -run TestRegistryDispatch -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/mcp/registry.go internal/mcp/registry_test.go
git commit -m "feat(mcp): tool registry with sorted listing"
```

---

## Task 3: Server — stdio read/write loop + initialize + tools/list + tools/call

**Files:**
- Create: `internal/mcp/server.go`, `internal/mcp/server_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/mcp/server_test.go
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
	// Re-marshal the result field to inspect it.
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/mcp/ -run TestServer -v`
Expected: FAIL — "undefined: NewServer".

- [ ] **Step 3: Implement server.go**

```go
// internal/mcp/server.go
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Server is a single-threaded JSON-RPC / MCP server over any io.Reader/Writer.
type Server struct {
	reg      *Registry
	in       io.Reader
	out      io.Writer
	info     ServerInfo
}

// NewServer creates a new server. Use os.Stdin/os.Stdout for stdio transport.
func NewServer(reg *Registry, in io.Reader, out io.Writer, info ServerInfo) *Server {
	return &Server{reg: reg, in: in, out: out, info: info}
}

// Run reads JSON-RPC messages (newline-delimited) until EOF, dispatching
// each one. Returns nil on clean EOF.
func (s *Server) Run(ctx context.Context) error {
	scanner := bufio.NewScanner(s.in)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		s.handleLine(ctx, line)
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

func (s *Server) handleLine(ctx context.Context, line []byte) {
	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		s.writeErr(nil, ErrParse, "parse error", err.Error())
		return
	}
	// Notifications (no id) get no response.
	isNotification := req.ID == nil

	switch req.Method {
	case "initialize":
		s.writeResult(req.ID, InitializeResult{
			ProtocolVersion: ProtocolVersion,
			Capabilities:    map[string]interface{}{"tools": map[string]interface{}{}},
			ServerInfo:      s.info,
		})
	case "notifications/initialized", "initialized":
		// no response required
	case "tools/list":
		s.writeResult(req.ID, ToolListResult{Tools: s.reg.List()})
	case "tools/call":
		var p ToolCallParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			s.writeErr(req.ID, ErrInvalidParams, "invalid params", err.Error())
			return
		}
		res, err := s.reg.Call(ctx, p.Name, p.Arguments)
		if err != nil {
			// Per MCP, tool errors go in-band as isError=true, not as RPC errors.
			s.writeResult(req.ID, ToolCallResult{
				IsError: true,
				Content: []ToolContent{{Type: "text", Text: err.Error()}},
			})
			return
		}
		s.writeResult(req.ID, res)
	case "ping":
		s.writeResult(req.ID, map[string]interface{}{})
	default:
		if !isNotification {
			s.writeErr(req.ID, ErrMethodNotFound, fmt.Sprintf("method not found: %s", req.Method), nil)
		}
	}
}

func (s *Server) writeResult(id interface{}, result interface{}) {
	resp := Response{JSONRPC: "2.0", ID: id, Result: result}
	data, _ := json.Marshal(resp)
	data = append(data, '\n')
	_, _ = s.out.Write(data)
}

func (s *Server) writeErr(id interface{}, code int, msg string, data interface{}) {
	resp := Response{JSONRPC: "2.0", ID: id, Error: &RPCError{Code: code, Message: msg, Data: data}}
	b, _ := json.Marshal(resp)
	b = append(b, '\n')
	_, _ = s.out.Write(b)
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/mcp/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/mcp/server.go internal/mcp/server_test.go
git commit -m "feat(mcp): stdio server with initialize/tools/list/tools/call"
```

---

## Task 4: Mail tool handlers (one representative tool, mail_list)

**Files:**
- Create: `internal/mcp/tools_mail.go`, append to `internal/mcp/registry_test.go`

- [ ] **Step 1: Write the failing test**

```go
// append to internal/mcp/registry_test.go
import (
	"net/http"
	"net/http/httptest"

	"github.com/skylarbpayne/msgcli/internal/graph"
)

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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/mcp/ -run TestMailListTool -v`
Expected: FAIL — "undefined: RegisterMailTools".

- [ ] **Step 3: Implement tools_mail.go**

```go
// internal/mcp/tools_mail.go
package mcp

import (
	"context"
	"encoding/json"

	"github.com/skylarbpayne/msgcli/internal/graph"
)

// ClientFactory returns a graph.Client for the given account alias
// (empty string = default / first configured).
type ClientFactory func(account string) (*graph.Client, error)

// RegisterMailTools registers all mail_* tools against the supplied registry.
// cf lets tests inject a fake client; production code uses a factory that
// wraps auth.ResolveAccount + graph.NewClient.
func RegisterMailTools(r *Registry, cf ClientFactory) {
	r.Register(Tool{
		Info: ToolInfo{
			Name:        "mail_list",
			Description: "List email messages from a folder (default: inbox, last 25).",
			InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
  "account":{"type":"string","description":"Account alias (default: first configured)"},
  "folder":{"type":"string","default":"inbox"},
  "limit":{"type":"integer","default":25,"minimum":1,"maximum":200},
  "query":{"type":"string","description":"KQL search query (optional)"}
}}`),
		},
		Handler: func(ctx context.Context, args json.RawMessage) (ToolCallResult, error) {
			var p struct {
				Account string `json:"account"`
				Folder  string `json:"folder"`
				Limit   int    `json:"limit"`
				Query   string `json:"query"`
			}
			if len(args) > 0 {
				if err := json.Unmarshal(args, &p); err != nil {
					return ToolCallResult{}, err
				}
			}
			if p.Folder == "" {
				p.Folder = "inbox"
			}
			if p.Limit == 0 {
				p.Limit = 25
			}
			client, err := cf(p.Account)
			if err != nil {
				return ToolCallResult{}, err
			}
			var result *graph.ListResponse[graph.Message]
			if p.Query != "" {
				result, err = client.SearchMessages(ctx, p.Query, p.Limit)
			} else {
				result, err = client.ListMessages(ctx, p.Folder, &graph.QueryParams{
					Top:     p.Limit,
					OrderBy: "receivedDateTime desc",
					Select:  []string{"id", "subject", "from", "receivedDateTime", "isRead", "hasAttachments", "bodyPreview"},
				})
			}
			if err != nil {
				return ToolCallResult{}, err
			}
			body, err := json.Marshal(result.Value)
			if err != nil {
				return ToolCallResult{}, err
			}
			return ToolCallResult{Content: []ToolContent{{Type: "text", Text: string(body)}}}, nil
		},
	})
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/mcp/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/mcp/tools_mail.go internal/mcp/registry_test.go
git commit -m "feat(mcp): mail_list tool handler + injection seam"
```

---

## Task 5: Remaining mail, calendar, and auth tool handlers

**Files:**
- Modify: `internal/mcp/tools_mail.go`
- Create: `internal/mcp/tools_calendar.go`, `internal/mcp/tools_auth.go`

- [ ] **Step 1: Add remaining mail tools to tools_mail.go**

Append handlers following the exact pattern from Task 4. Tool surface and argument schemas:

- **mail_get** — `{ account, id }` → `client.GetMessage(ctx, id)` → JSON.
- **mail_send** — `{ account, to:[], cc:[], subject, body, isHtml }` → `client.SendMail(...)` → `{ok:true}`.
- **mail_reply** — `{ account, id, comment, replyAll }` → `client.ReplyToMessage` or `client.ReplyAllToMessage` → `{ok:true}`.
- **mail_move** — `{ account, id, folder }` → `client.MoveMessage` → the returned Message as JSON.
- **mail_delete** — `{ account, id }` → `client.DeleteMessage` → `{ok:true}`.
- **mail_folders** — `{ account }` → `client.ListMailFolders` → folders as JSON.

Each handler uses the same `ClientFactory` injection pattern. For each new tool, add a companion test in `registry_test.go` following the `TestMailListTool` shape.

- [ ] **Step 2: Create tools_calendar.go**

Mirror the pattern for:
- **calendar_list** — `{ account, start, end, limit }` → existing `graph.Client` calendar methods.
- **calendar_get** — `{ account, id }`
- **calendar_create** — `{ account, subject, start, end, attendees, location, body }`
- **calendar_update** — `{ account, id, ...optional fields }`
- **calendar_delete** — `{ account, id, cancel, comment }`
- **calendar_respond** — `{ account, id, response: "accept|decline|tentative", comment }`
- **calendar_availability** — `{ account, emails:[], start, end }`

Add a `RegisterCalendarTools(r, cf)` that wires them all.

- [ ] **Step 3: Create tools_auth.go** (read-only; interactive flows excluded)

```go
// internal/mcp/tools_auth.go
package mcp

import (
	"context"
	"encoding/json"

	"github.com/skylarbpayne/msgcli/internal/auth"
)

func RegisterAuthTools(r *Registry) {
	r.Register(Tool{
		Info: ToolInfo{
			Name:        "auth_list",
			Description: "List configured accounts (aliases, email, flow).",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		},
		Handler: func(ctx context.Context, args json.RawMessage) (ToolCallResult, error) {
			accounts, err := auth.ListAccounts()
			if err != nil {
				return ToolCallResult{}, err
			}
			body, err := json.Marshal(accounts)
			if err != nil {
				return ToolCallResult{}, err
			}
			return ToolCallResult{Content: []ToolContent{{Type: "text", Text: string(body)}}}, nil
		},
	})
	r.Register(Tool{
		Info: ToolInfo{
			Name:        "auth_status",
			Description: "Show auth status / token validity for one or all accounts.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"account":{"type":"string"}}}`),
		},
		Handler: func(ctx context.Context, args json.RawMessage) (ToolCallResult, error) {
			var p struct {
				Account string `json:"account"`
			}
			_ = json.Unmarshal(args, &p)
			status, err := auth.Status(ctx, p.Account)
			if err != nil {
				return ToolCallResult{}, err
			}
			body, err := json.Marshal(status)
			if err != nil {
				return ToolCallResult{}, err
			}
			return ToolCallResult{Content: []ToolContent{{Type: "text", Text: string(body)}}}, nil
		},
	})
}
```

> **Note:** If `auth.ListAccounts()` and `auth.Status()` don't exist with those exact signatures, use whatever the existing auth package exposes (see `internal/cmd/auth_list.go` and `internal/cmd/auth_status.go`). Extract a thin read-only API in `internal/auth/` as needed — that refactor is in scope for this task.

- [ ] **Step 4: Run all tests**

Run: `go test ./internal/mcp/ -v`
Expected: every mail/calendar/auth tool test passes.

- [ ] **Step 5: Commit**

```bash
git add internal/mcp/tools_mail.go internal/mcp/tools_calendar.go internal/mcp/tools_auth.go internal/mcp/registry_test.go internal/auth/
git commit -m "feat(mcp): mail, calendar, and read-only auth tool handlers"
```

---

## Task 6: `msgcli mcp serve` Cobra command

**Files:**
- Create: `internal/cmd/mcp.go`

- [ ] **Step 1: Implement the command (no test needed here — server tested in mcp package)**

```go
// internal/cmd/mcp.go
package cmd

import (
	"context"
	"os"

	"github.com/skylarbpayne/msgcli/internal/auth"
	"github.com/skylarbpayne/msgcli/internal/graph"
	"github.com/skylarbpayne/msgcli/internal/mcp"
	"github.com/spf13/cobra"
)

// version is injected via ldflags in release builds; "dev" otherwise.
var version = "dev"

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Model Context Protocol integration",
}

var mcpServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run msgcli as an MCP server over stdio",
	Long: `Start a Model Context Protocol server on stdin/stdout.

Intended to be launched by an MCP client (Claude Code, Claude Desktop, etc.);
see 'msgcli mcp install' to configure a client automatically.`,
	RunE: runMCPServe,
}

func init() {
	mcpCmd.AddCommand(mcpServeCmd)
	rootCmd.AddCommand(mcpCmd)
}

func runMCPServe(cmd *cobra.Command, args []string) error {
	cf := func(account string) (*graph.Client, error) {
		resolved, err := auth.ResolveAccount(account)
		if err != nil {
			return nil, err
		}
		return graph.NewClient(resolved), nil
	}

	reg := mcp.NewRegistry()
	mcp.RegisterMailTools(reg, cf)
	mcp.RegisterCalendarTools(reg, cf)
	mcp.RegisterAuthTools(reg)

	server := mcp.NewServer(reg, os.Stdin, os.Stdout, mcp.ServerInfo{Name: "msgcli", Version: version})
	return server.Run(context.Background())
}
```

- [ ] **Step 2: Build**

Run: `go build ./...`
Expected: no errors.

- [ ] **Step 3: Smoke test (manual, required before commit)**

```bash
./bin/msgcli mcp serve <<'EOF'
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}
{"jsonrpc":"2.0","id":2,"method":"tools/list"}
EOF
```

Expected: two JSON-RPC responses, `tools/list` result contains at least `mail_list`, `calendar_list`, `auth_list`.

- [ ] **Step 4: Commit**

```bash
git add internal/cmd/mcp.go
git commit -m "feat(cmd): \`msgcli mcp serve\` stdio MCP server"
```

---

## Task 7: `msgcli mcp install` — write config for Claude Code

**Files:**
- Modify: `internal/cmd/mcp.go`
- Create: `internal/cmd/mcp_install.go`, `internal/cmd/mcp_install_test.go`

- [ ] **Step 1: Write the failing test for config-merge logic**

```go
// internal/cmd/mcp_install_test.go
package cmd

import (
	"encoding/json"
	"testing"
)

func TestMergeClaudeCodeMCPConfig(t *testing.T) {
	existing := `{"mcpServers":{"other":{"command":"foo","args":[]}}}`
	merged, err := mergeClaudeCodeMCPConfig([]byte(existing), "/usr/local/bin/msgcli")
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	var got map[string]map[string]map[string]interface{}
	if err := json.Unmarshal(merged, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	servers := got["mcpServers"]
	if _, ok := servers["other"]; !ok {
		t.Fatalf("existing entry lost")
	}
	msg, ok := servers["msgcli"]
	if !ok {
		t.Fatalf("msgcli entry missing")
	}
	if msg["command"] != "/usr/local/bin/msgcli" {
		t.Fatalf("unexpected command: %v", msg["command"])
	}
}

func TestMergeEmptyConfigInitializesStructure(t *testing.T) {
	merged, err := mergeClaudeCodeMCPConfig([]byte(""), "/bin/msgcli")
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	var got map[string]map[string]map[string]interface{}
	if err := json.Unmarshal(merged, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := got["mcpServers"]["msgcli"]; !ok {
		t.Fatalf("msgcli entry missing")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/cmd/ -run TestMergeClaudeCodeMCPConfig -v`
Expected: FAIL — "undefined: mergeClaudeCodeMCPConfig".

- [ ] **Step 3: Implement install command + merge**

```go
// internal/cmd/mcp_install.go
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var mcpInstallClient string

var mcpInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install msgcli into an MCP client's config",
	Long:  `Write an mcpServers.msgcli entry into a client's config file. Currently supports --client claude-code.`,
	RunE:  runMCPInstall,
}

func init() {
	mcpInstallCmd.Flags().StringVar(&mcpInstallClient, "client", "claude-code", "Target client: claude-code")
	mcpCmd.AddCommand(mcpInstallCmd)
}

func runMCPInstall(cmd *cobra.Command, args []string) error {
	if mcpInstallClient != "claude-code" {
		return fmt.Errorf("unsupported client %q (supported: claude-code)", mcpInstallClient)
	}
	bin, err := exec.LookPath("msgcli")
	if err != nil {
		// Fall back to absolute path of the running binary.
		if exe, e2 := os.Executable(); e2 == nil {
			bin = exe
		} else {
			return fmt.Errorf("cannot locate msgcli binary: %w", err)
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, ".claude", "settings.json")
	existing, _ := os.ReadFile(path) // treat missing as empty
	merged, err := mergeClaudeCodeMCPConfig(existing, bin)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, merged, 0o600); err != nil {
		return err
	}
	Infof("Installed msgcli MCP server into %s", path)
	return nil
}

// mergeClaudeCodeMCPConfig adds/updates an mcpServers.msgcli entry without
// clobbering existing keys. Accepts empty input (returns a fresh config).
func mergeClaudeCodeMCPConfig(existing []byte, bin string) ([]byte, error) {
	root := map[string]interface{}{}
	if len(existing) > 0 {
		if err := json.Unmarshal(existing, &root); err != nil {
			return nil, fmt.Errorf("existing config is not valid JSON: %w", err)
		}
	}
	servers, _ := root["mcpServers"].(map[string]interface{})
	if servers == nil {
		servers = map[string]interface{}{}
	}
	servers["msgcli"] = map[string]interface{}{
		"command": bin,
		"args":    []string{"mcp", "serve"},
	}
	root["mcpServers"] = servers
	return json.MarshalIndent(root, "", "  ")
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/cmd/ -run TestMerge -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add internal/cmd/mcp_install.go internal/cmd/mcp_install_test.go
git commit -m "feat(mcp): \`msgcli mcp install --client claude-code\` helper"
```

---

## Task 8: Docs + README

**Files:**
- Create: `docs/reference/mcp.md`
- Modify: `README.md`

- [ ] **Step 1: Write `docs/reference/mcp.md`**

Sections:
1. **What is it** — short explanation of MCP + why msgcli exposes it.
2. **Quick setup** — `msgcli mcp install --client claude-code` + verification in Claude Code.
3. **Tool catalog** — table of every tool name, description, and argument schema.
4. **Security notes** — tokens stay in OS keychain; account arg controls which keychain entry is used; no tokens ever leave the local machine.
5. **Troubleshooting** — how to tail stderr; how to run `msgcli mcp serve` manually with a JSONL script to reproduce.

- [ ] **Step 2: Add README row**

Under a new "MCP" section:

```markdown
### MCP Server

| Command | Description |
|---------|-------------|
| `msgcli mcp serve` | Run as an MCP server over stdio (launched by MCP clients) |
| `msgcli mcp install --client claude-code` | Register msgcli into Claude Code's MCP config |
```

- [ ] **Step 3: Commit**

```bash
git add docs/reference/mcp.md README.md
git commit -m "docs(mcp): reference + README pointer"
```

---

## Self-Review

- **Spec coverage:** protocol types (Task 1), registry (Task 2), server loop with initialize/list/call (Task 3), tool handlers for mail (Tasks 4–5), calendar + auth (Task 5), CLI wiring (Task 6), install helper (Task 7), docs (Task 8). All covered.
- **Placeholder scan:** One explicit "Note" in Task 5 step 3 acknowledges that the exact auth API may differ and directs the implementer to the existing `auth_list.go` / `auth_status.go` cmd files. This is not a TBD — it's a concrete instruction. All code blocks are complete Go.
- **Type consistency:** `Request`, `Response`, `RPCError`, `ToolInfo`, `ToolCallResult`, `Tool`, `Registry`, `ClientFactory`, `ServerInfo` used consistently across tasks.
- **Known follow-ups (out of scope):** SSE transport; resources/prompts MCP surfaces (only `tools/*` implemented here); streaming `tools/call` responses. Each could be a separate plan if needed.
