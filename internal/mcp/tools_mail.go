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

	// mail_get
	r.Register(Tool{
		Info: ToolInfo{
			Name:        "mail_get",
			Description: "Get a single email message by ID.",
			InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
  "account":{"type":"string","description":"Account alias (default: first configured)"},
  "id":{"type":"string","description":"Message ID"}
},
"required":["id"]}`),
		},
		Handler: func(ctx context.Context, args json.RawMessage) (ToolCallResult, error) {
			var p struct {
				Account string `json:"account"`
				ID      string `json:"id"`
			}
			if len(args) > 0 {
				if err := json.Unmarshal(args, &p); err != nil {
					return ToolCallResult{}, err
				}
			}
			client, err := cf(p.Account)
			if err != nil {
				return ToolCallResult{}, err
			}
			msg, err := client.GetMessage(ctx, p.ID)
			if err != nil {
				return ToolCallResult{}, err
			}
			body, err := json.Marshal(msg)
			if err != nil {
				return ToolCallResult{}, err
			}
			return ToolCallResult{Content: []ToolContent{{Type: "text", Text: string(body)}}}, nil
		},
	})

	// mail_send
	r.Register(Tool{
		Info: ToolInfo{
			Name:        "mail_send",
			Description: "Send an email message.",
			InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
  "account":{"type":"string","description":"Account alias (default: first configured)"},
  "to":{"type":"array","items":{"type":"string"},"description":"Recipients"},
  "cc":{"type":"array","items":{"type":"string"},"description":"CC recipients"},
  "subject":{"type":"string"},
  "body":{"type":"string"},
  "isHtml":{"type":"boolean","default":false}
},
"required":["to","subject","body"]}`),
		},
		Handler: func(ctx context.Context, args json.RawMessage) (ToolCallResult, error) {
			var p struct {
				Account string   `json:"account"`
				To      []string `json:"to"`
				CC      []string `json:"cc"`
				Subject string   `json:"subject"`
				Body    string   `json:"body"`
				IsHTML  bool     `json:"isHtml"`
			}
			if len(args) > 0 {
				if err := json.Unmarshal(args, &p); err != nil {
					return ToolCallResult{}, err
				}
			}
			if p.CC == nil {
				p.CC = []string{}
			}
			client, err := cf(p.Account)
			if err != nil {
				return ToolCallResult{}, err
			}
			if err := client.SendMail(ctx, p.To, p.CC, p.Subject, p.Body, p.IsHTML); err != nil {
				return ToolCallResult{}, err
			}
			return ToolCallResult{Content: []ToolContent{{Type: "text", Text: `{"ok":true}`}}}, nil
		},
	})

	// mail_reply
	r.Register(Tool{
		Info: ToolInfo{
			Name:        "mail_reply",
			Description: "Reply to an email message.",
			InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
  "account":{"type":"string","description":"Account alias (default: first configured)"},
  "id":{"type":"string","description":"Message ID"},
  "comment":{"type":"string","description":"Reply text"},
  "replyAll":{"type":"boolean","default":false}
},
"required":["id"]}`),
		},
		Handler: func(ctx context.Context, args json.RawMessage) (ToolCallResult, error) {
			var p struct {
				Account  string `json:"account"`
				ID       string `json:"id"`
				Comment  string `json:"comment"`
				ReplyAll bool   `json:"replyAll"`
			}
			if len(args) > 0 {
				if err := json.Unmarshal(args, &p); err != nil {
					return ToolCallResult{}, err
				}
			}
			client, err := cf(p.Account)
			if err != nil {
				return ToolCallResult{}, err
			}
			if p.ReplyAll {
				err = client.ReplyAllToMessage(ctx, p.ID, p.Comment)
			} else {
				err = client.ReplyToMessage(ctx, p.ID, p.Comment)
			}
			if err != nil {
				return ToolCallResult{}, err
			}
			return ToolCallResult{Content: []ToolContent{{Type: "text", Text: `{"ok":true}`}}}, nil
		},
	})

	// mail_move
	r.Register(Tool{
		Info: ToolInfo{
			Name:        "mail_move",
			Description: "Move an email message to a different folder.",
			InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
  "account":{"type":"string","description":"Account alias (default: first configured)"},
  "id":{"type":"string","description":"Message ID"},
  "folder":{"type":"string","description":"Destination folder ID or well-known name"}
},
"required":["id","folder"]}`),
		},
		Handler: func(ctx context.Context, args json.RawMessage) (ToolCallResult, error) {
			var p struct {
				Account string `json:"account"`
				ID      string `json:"id"`
				Folder  string `json:"folder"`
			}
			if len(args) > 0 {
				if err := json.Unmarshal(args, &p); err != nil {
					return ToolCallResult{}, err
				}
			}
			client, err := cf(p.Account)
			if err != nil {
				return ToolCallResult{}, err
			}
			msg, err := client.MoveMessage(ctx, p.ID, p.Folder)
			if err != nil {
				return ToolCallResult{}, err
			}
			body, err := json.Marshal(msg)
			if err != nil {
				return ToolCallResult{}, err
			}
			return ToolCallResult{Content: []ToolContent{{Type: "text", Text: string(body)}}}, nil
		},
	})

	// mail_delete
	r.Register(Tool{
		Info: ToolInfo{
			Name:        "mail_delete",
			Description: "Delete an email message.",
			InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
  "account":{"type":"string","description":"Account alias (default: first configured)"},
  "id":{"type":"string","description":"Message ID"}
},
"required":["id"]}`),
		},
		Handler: func(ctx context.Context, args json.RawMessage) (ToolCallResult, error) {
			var p struct {
				Account string `json:"account"`
				ID      string `json:"id"`
			}
			if len(args) > 0 {
				if err := json.Unmarshal(args, &p); err != nil {
					return ToolCallResult{}, err
				}
			}
			client, err := cf(p.Account)
			if err != nil {
				return ToolCallResult{}, err
			}
			if err := client.DeleteMessage(ctx, p.ID); err != nil {
				return ToolCallResult{}, err
			}
			return ToolCallResult{Content: []ToolContent{{Type: "text", Text: `{"ok":true}`}}}, nil
		},
	})

	// mail_folders
	r.Register(Tool{
		Info: ToolInfo{
			Name:        "mail_folders",
			Description: "List mail folders.",
			InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
  "account":{"type":"string","description":"Account alias (default: first configured)"}
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
			client, err := cf(p.Account)
			if err != nil {
				return ToolCallResult{}, err
			}
			result, err := client.ListMailFolders(ctx)
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
