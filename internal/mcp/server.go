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
	reg  *Registry
	in   io.Reader
	out  io.Writer
	info ServerInfo
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
