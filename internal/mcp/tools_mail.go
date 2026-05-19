package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/skylarbpayne/msgcli/internal/graph"
)

// detectAttachmentContentType returns a MIME type for the file, preferring the
// extension and falling back to content sniffing.
func detectAttachmentContentType(path string, data []byte) string {
	if ct := mime.TypeByExtension(filepath.Ext(path)); ct != "" {
		return strings.SplitN(ct, ";", 2)[0]
	}
	return http.DetectContentType(data)
}

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
		Handler: mailListHandler(cf),
	})

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
		Handler: mailGetHandler(cf),
	})

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
		Handler: mailSendHandler(cf),
	})

	r.Register(Tool{
		Info: ToolInfo{
			Name:        "mail_draft_create",
			Description: "Create a draft email message, optionally with attachments. Attachments may be provided as local file paths or inline base64 content. Each attachment must be under 3MB.",
			InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
  "account":{"type":"string","description":"Account alias (default: first configured)"},
  "to":{"type":"array","items":{"type":"string"}},
  "cc":{"type":"array","items":{"type":"string"}},
  "bcc":{"type":"array","items":{"type":"string"}},
  "subject":{"type":"string"},
  "body":{"type":"string"},
  "isHtml":{"type":"boolean","default":false},
  "attachments":{
    "type":"array",
    "description":"Files to attach. Each item must have either path OR contentBase64.",
    "items":{
      "type":"object",
      "properties":{
        "path":{"type":"string","description":"Local file path to read (mutually exclusive with contentBase64)"},
        "name":{"type":"string","description":"Attachment filename (defaults to path basename)"},
        "contentType":{"type":"string","description":"MIME type (auto-detected if omitted)"},
        "contentBase64":{"type":"string","description":"Base64-encoded file content (mutually exclusive with path)"}
      }
    }
  }
}}`),
		},
		Handler: mailDraftCreateHandler(cf),
	})

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
		Handler: mailReplyHandler(cf),
	})

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
		Handler: mailMoveHandler(cf),
	})

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
		Handler: mailDeleteHandler(cf),
	})

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
		Handler: mailFoldersHandler(cf),
	})
}

func mailListHandler(cf ClientFactory) ToolHandler {
	return func(ctx context.Context, args json.RawMessage) (ToolCallResult, error) {
		var p struct {
			Account string `json:"account"`
			Folder  string `json:"folder"`
			Limit   int    `json:"limit"`
			Query   string `json:"query"`
		}
		if err := unmarshalArgs(args, &p); err != nil {
			return ToolCallResult{}, err
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
		return jsonResult(result.Value)
	}
}

func mailGetHandler(cf ClientFactory) ToolHandler {
	return func(ctx context.Context, args json.RawMessage) (ToolCallResult, error) {
		var p struct {
			Account string `json:"account"`
			ID      string `json:"id"`
		}
		if err := unmarshalArgs(args, &p); err != nil {
			return ToolCallResult{}, err
		}
		client, err := cf(p.Account)
		if err != nil {
			return ToolCallResult{}, err
		}
		msg, err := client.GetMessage(ctx, p.ID)
		if err != nil {
			return ToolCallResult{}, err
		}
		return jsonResult(msg)
	}
}

func mailSendHandler(cf ClientFactory) ToolHandler {
	return func(ctx context.Context, args json.RawMessage) (ToolCallResult, error) {
		var p struct {
			Account string   `json:"account"`
			To      []string `json:"to"`
			CC      []string `json:"cc"`
			Subject string   `json:"subject"`
			Body    string   `json:"body"`
			IsHTML  bool     `json:"isHtml"`
		}
		if err := unmarshalArgs(args, &p); err != nil {
			return ToolCallResult{}, err
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
		return okResult, nil
	}
}

func mailDraftCreateHandler(cf ClientFactory) ToolHandler {
	return func(ctx context.Context, args json.RawMessage) (ToolCallResult, error) {
		var p struct {
			Account     string   `json:"account"`
			To          []string `json:"to"`
			CC          []string `json:"cc"`
			BCC         []string `json:"bcc"`
			Subject     string   `json:"subject"`
			Body        string   `json:"body"`
			IsHTML      bool     `json:"isHtml"`
			Attachments []struct {
				Path          string `json:"path"`
				Name          string `json:"name"`
				ContentType   string `json:"contentType"`
				ContentBase64 string `json:"contentBase64"`
			} `json:"attachments"`
		}
		if err := unmarshalArgs(args, &p); err != nil {
			return ToolCallResult{}, err
		}
		client, err := cf(p.Account)
		if err != nil {
			return ToolCallResult{}, err
		}
		draft, err := client.CreateDraft(ctx, p.To, p.CC, p.BCC, p.Subject, p.Body, p.IsHTML)
		if err != nil {
			return ToolCallResult{}, err
		}
		for i, a := range p.Attachments {
			var data []byte
			name := a.Name
			ctype := a.ContentType
			switch {
			case a.Path != "" && a.ContentBase64 != "":
				return ToolCallResult{}, fmt.Errorf("attachment %d: specify path OR contentBase64, not both", i)
			case a.Path != "":
				b, err := os.ReadFile(a.Path)
				if err != nil {
					return ToolCallResult{}, fmt.Errorf("attachment %d: read %q: %w", i, a.Path, err)
				}
				data = b
				if name == "" {
					name = filepath.Base(a.Path)
				}
				if ctype == "" {
					ctype = detectAttachmentContentType(a.Path, data)
				}
			case a.ContentBase64 != "":
				b, err := base64.StdEncoding.DecodeString(a.ContentBase64)
				if err != nil {
					return ToolCallResult{}, fmt.Errorf("attachment %d: invalid base64: %w", i, err)
				}
				data = b
				if name == "" {
					return ToolCallResult{}, fmt.Errorf("attachment %d: name is required when using contentBase64", i)
				}
				if ctype == "" {
					ctype = http.DetectContentType(data)
				}
			default:
				return ToolCallResult{}, fmt.Errorf("attachment %d: must specify either path or contentBase64", i)
			}
			if err := client.AddAttachment(ctx, draft.ID, name, ctype, data); err != nil {
				return ToolCallResult{}, err
			}
		}
		return jsonResult(draft)
	}
}

func mailReplyHandler(cf ClientFactory) ToolHandler {
	return func(ctx context.Context, args json.RawMessage) (ToolCallResult, error) {
		var p struct {
			Account  string `json:"account"`
			ID       string `json:"id"`
			Comment  string `json:"comment"`
			ReplyAll bool   `json:"replyAll"`
		}
		if err := unmarshalArgs(args, &p); err != nil {
			return ToolCallResult{}, err
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
		return okResult, nil
	}
}

func mailMoveHandler(cf ClientFactory) ToolHandler {
	return func(ctx context.Context, args json.RawMessage) (ToolCallResult, error) {
		var p struct {
			Account string `json:"account"`
			ID      string `json:"id"`
			Folder  string `json:"folder"`
		}
		if err := unmarshalArgs(args, &p); err != nil {
			return ToolCallResult{}, err
		}
		client, err := cf(p.Account)
		if err != nil {
			return ToolCallResult{}, err
		}
		msg, err := client.MoveMessage(ctx, p.ID, p.Folder)
		if err != nil {
			return ToolCallResult{}, err
		}
		return jsonResult(msg)
	}
}

func mailDeleteHandler(cf ClientFactory) ToolHandler {
	return func(ctx context.Context, args json.RawMessage) (ToolCallResult, error) {
		var p struct {
			Account string `json:"account"`
			ID      string `json:"id"`
		}
		if err := unmarshalArgs(args, &p); err != nil {
			return ToolCallResult{}, err
		}
		client, err := cf(p.Account)
		if err != nil {
			return ToolCallResult{}, err
		}
		if err := client.DeleteMessage(ctx, p.ID); err != nil {
			return ToolCallResult{}, err
		}
		return okResult, nil
	}
}

func mailFoldersHandler(cf ClientFactory) ToolHandler {
	return func(ctx context.Context, args json.RawMessage) (ToolCallResult, error) {
		var p struct {
			Account string `json:"account"`
		}
		if err := unmarshalArgs(args, &p); err != nil {
			return ToolCallResult{}, err
		}
		client, err := cf(p.Account)
		if err != nil {
			return ToolCallResult{}, err
		}
		result, err := client.ListMailFolders(ctx)
		if err != nil {
			return ToolCallResult{}, err
		}
		return jsonResult(result.Value)
	}
}
