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
