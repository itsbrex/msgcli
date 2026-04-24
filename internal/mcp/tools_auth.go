package mcp

import (
	"context"
	"encoding/json"

	"github.com/skylarbpayne/msgcli/internal/auth"
)

// RegisterAuthTools registers all auth_* tools against the supplied registry.
// No ClientFactory is needed — auth tools operate on local auth state only.
func RegisterAuthTools(r *Registry) {

	// auth_list
	r.Register(Tool{
		Info: ToolInfo{
			Name:        "auth_list",
			Description: "List all configured accounts (alias, email, flow).",
			InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		},
		Handler: func(ctx context.Context, args json.RawMessage) (ToolCallResult, error) {
			accounts, err := auth.ListAccounts()
			if err != nil {
				return ToolCallResult{}, err
			}
			if accounts == nil {
				accounts = []auth.AccountInfo{}
			}
			body, err := json.Marshal(accounts)
			if err != nil {
				return ToolCallResult{}, err
			}
			return ToolCallResult{Content: []ToolContent{{Type: "text", Text: string(body)}}}, nil
		},
	})

	// auth_status
	r.Register(Tool{
		Info: ToolInfo{
			Name:        "auth_status",
			Description: "Show auth configuration and per-account token validity. Leave account empty to report all accounts.",
			InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
  "account":{"type":"string","description":"Account alias to report (empty = all)"}
}}`),
		},
		Handler: func(ctx context.Context, args json.RawMessage) (ToolCallResult, error) {
			var p struct {
				Account string `json:"account"`
			}
			if len(args) > 0 {
				if err := json.Unmarshal(args, &p); err != nil {
					return ToolCallResult{}, err
				}
			}
			report, err := auth.Status(ctx, p.Account)
			if err != nil {
				return ToolCallResult{}, err
			}
			body, err := json.Marshal(report)
			if err != nil {
				return ToolCallResult{}, err
			}
			return ToolCallResult{Content: []ToolContent{{Type: "text", Text: string(body)}}}, nil
		},
	})
}
