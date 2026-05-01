package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/skylarbpayne/msgcli/internal/graph"
)

// RegisterCalendarTools registers all calendar_* tools against the supplied registry.
func RegisterCalendarTools(r *Registry, cf ClientFactory) {
	r.Register(Tool{
		Info: ToolInfo{
			Name:        "calendar_list",
			Description: "List calendar events within an optional time range.",
			InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
  "account":{"type":"string","description":"Account alias (default: first configured)"},
  "start":{"type":"string","description":"RFC3339 start time (optional)"},
  "end":{"type":"string","description":"RFC3339 end time (optional)"},
  "limit":{"type":"integer","default":25,"minimum":1,"maximum":200}
}}`),
		},
		Handler: calendarListHandler(cf),
	})

	r.Register(Tool{
		Info: ToolInfo{
			Name:        "calendar_get",
			Description: "Get a single calendar event by ID.",
			InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
  "account":{"type":"string","description":"Account alias (default: first configured)"},
  "id":{"type":"string","description":"Event ID"}
},
"required":["id"]}`),
		},
		Handler: calendarGetHandler(cf),
	})

	r.Register(Tool{
		Info: ToolInfo{
			Name:        "calendar_create",
			Description: "Create a new calendar event.",
			InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
  "account":{"type":"string","description":"Account alias (default: first configured)"},
  "subject":{"type":"string"},
  "start":{"type":"string","description":"RFC3339 start time"},
  "end":{"type":"string","description":"RFC3339 end time"},
  "location":{"type":"string","description":"Location display name (optional)"},
  "body":{"type":"string","description":"Event description (optional)"},
  "attendees":{"type":"array","items":{"type":"string"},"description":"Attendee email addresses (optional)"}
},
"required":["subject","start","end"]}`),
		},
		Handler: calendarCreateHandler(cf),
	})

	r.Register(Tool{
		Info: ToolInfo{
			Name:        "calendar_update",
			Description: "Update an existing calendar event.",
			InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
  "account":{"type":"string","description":"Account alias (default: first configured)"},
  "id":{"type":"string","description":"Event ID"},
  "updates":{"type":"object","description":"Fields to update (Graph API field names)"}
},
"required":["id","updates"]}`),
		},
		Handler: calendarUpdateHandler(cf),
	})

	r.Register(Tool{
		Info: ToolInfo{
			Name:        "calendar_delete",
			Description: "Delete a calendar event.",
			InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
  "account":{"type":"string","description":"Account alias (default: first configured)"},
  "id":{"type":"string","description":"Event ID"}
},
"required":["id"]}`),
		},
		Handler: calendarDeleteHandler(cf),
	})

	r.Register(Tool{
		Info: ToolInfo{
			Name:        "calendar_respond",
			Description: `Respond to a calendar event invitation. response must be "accept", "decline", or "tentative".`,
			InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
  "account":{"type":"string","description":"Account alias (default: first configured)"},
  "id":{"type":"string","description":"Event ID"},
  "response":{"type":"string","enum":["accept","decline","tentative"]},
  "comment":{"type":"string","description":"Optional comment"},
  "sendResponse":{"type":"boolean","default":true}
},
"required":["id","response"]}`),
		},
		Handler: calendarRespondHandler(cf),
	})

	r.Register(Tool{
		Info: ToolInfo{
			Name:        "calendar_availability",
			Description: "Get free/busy schedule information for a set of email addresses.",
			InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
  "account":{"type":"string","description":"Account alias (default: first configured)"},
  "emails":{"type":"array","items":{"type":"string"},"description":"Email addresses to query"},
  "start":{"type":"string","description":"RFC3339 start time"},
  "end":{"type":"string","description":"RFC3339 end time"}
},
"required":["emails","start","end"]}`),
		},
		Handler: calendarAvailabilityHandler(cf),
	})
}

func calendarListHandler(cf ClientFactory) ToolHandler {
	return func(ctx context.Context, args json.RawMessage) (ToolCallResult, error) {
		var p struct {
			Account string `json:"account"`
			Start   string `json:"start"`
			End     string `json:"end"`
			Limit   int    `json:"limit"`
		}
		if err := unmarshalArgs(args, &p); err != nil {
			return ToolCallResult{}, err
		}
		if p.Limit == 0 {
			p.Limit = 25
		}
		var startPtr, endPtr *time.Time
		if p.Start != "" {
			t, err := time.Parse(time.RFC3339, p.Start)
			if err != nil {
				return ToolCallResult{}, fmt.Errorf("invalid start time: %w", err)
			}
			startPtr = &t
		}
		if p.End != "" {
			t, err := time.Parse(time.RFC3339, p.End)
			if err != nil {
				return ToolCallResult{}, fmt.Errorf("invalid end time: %w", err)
			}
			endPtr = &t
		}
		client, err := cf(p.Account)
		if err != nil {
			return ToolCallResult{}, err
		}
		result, err := client.ListEvents(ctx, "", startPtr, endPtr, &graph.QueryParams{Top: p.Limit})
		if err != nil {
			return ToolCallResult{}, err
		}
		return jsonResult(result.Value)
	}
}

func calendarGetHandler(cf ClientFactory) ToolHandler {
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
		event, err := client.GetEvent(ctx, p.ID)
		if err != nil {
			return ToolCallResult{}, err
		}
		return jsonResult(event)
	}
}

func calendarCreateHandler(cf ClientFactory) ToolHandler {
	return func(ctx context.Context, args json.RawMessage) (ToolCallResult, error) {
		var p struct {
			Account   string   `json:"account"`
			Subject   string   `json:"subject"`
			Start     string   `json:"start"`
			End       string   `json:"end"`
			Location  string   `json:"location"`
			Body      string   `json:"body"`
			Attendees []string `json:"attendees"`
		}
		if err := unmarshalArgs(args, &p); err != nil {
			return ToolCallResult{}, err
		}
		startTime, err := time.Parse(time.RFC3339, p.Start)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid start time: %w", err)
		}
		endTime, err := time.Parse(time.RFC3339, p.End)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid end time: %w", err)
		}
		event := &graph.Event{
			Subject: p.Subject,
			Start:   graph.NewDateTimeZone(startTime.UTC(), "UTC"),
			End:     graph.NewDateTimeZone(endTime.UTC(), "UTC"),
		}
		if p.Location != "" {
			event.Location = &graph.Location{DisplayName: p.Location}
		}
		if p.Body != "" {
			event.Body = &graph.ItemBody{ContentType: "text", Content: p.Body}
		}
		if len(p.Attendees) > 0 {
			attendees := make([]graph.Attendee, len(p.Attendees))
			for i, email := range p.Attendees {
				attendees[i] = graph.Attendee{
					EmailAddress: graph.EmailAddress{Address: email},
					Type:         "required",
				}
			}
			event.Attendees = attendees
		}
		client, err := cf(p.Account)
		if err != nil {
			return ToolCallResult{}, err
		}
		created, err := client.CreateEvent(ctx, event)
		if err != nil {
			return ToolCallResult{}, err
		}
		return jsonResult(created)
	}
}

func calendarUpdateHandler(cf ClientFactory) ToolHandler {
	return func(ctx context.Context, args json.RawMessage) (ToolCallResult, error) {
		var p struct {
			Account string                 `json:"account"`
			ID      string                 `json:"id"`
			Updates map[string]interface{} `json:"updates"`
		}
		if err := unmarshalArgs(args, &p); err != nil {
			return ToolCallResult{}, err
		}
		client, err := cf(p.Account)
		if err != nil {
			return ToolCallResult{}, err
		}
		updated, err := client.UpdateEvent(ctx, p.ID, p.Updates)
		if err != nil {
			return ToolCallResult{}, err
		}
		return jsonResult(updated)
	}
}

func calendarDeleteHandler(cf ClientFactory) ToolHandler {
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
		if err := client.DeleteEvent(ctx, p.ID); err != nil {
			return ToolCallResult{}, err
		}
		return okResult, nil
	}
}

func calendarRespondHandler(cf ClientFactory) ToolHandler {
	return func(ctx context.Context, args json.RawMessage) (ToolCallResult, error) {
		var p struct {
			Account      string `json:"account"`
			ID           string `json:"id"`
			Response     string `json:"response"`
			Comment      string `json:"comment"`
			SendResponse *bool  `json:"sendResponse"`
		}
		if err := unmarshalArgs(args, &p); err != nil {
			return ToolCallResult{}, err
		}
		sendResp := true
		if p.SendResponse != nil {
			sendResp = *p.SendResponse
		}
		client, err := cf(p.Account)
		if err != nil {
			return ToolCallResult{}, err
		}
		switch p.Response {
		case "accept":
			err = client.AcceptEvent(ctx, p.ID, p.Comment, sendResp)
		case "decline":
			err = client.DeclineEvent(ctx, p.ID, p.Comment, sendResp)
		case "tentative":
			err = client.TentativelyAcceptEvent(ctx, p.ID, p.Comment, sendResp)
		default:
			return ToolCallResult{}, fmt.Errorf("unknown response %q: must be accept, decline, or tentative", p.Response)
		}
		if err != nil {
			return ToolCallResult{}, err
		}
		return okResult, nil
	}
}

func calendarAvailabilityHandler(cf ClientFactory) ToolHandler {
	return func(ctx context.Context, args json.RawMessage) (ToolCallResult, error) {
		var p struct {
			Account string   `json:"account"`
			Emails  []string `json:"emails"`
			Start   string   `json:"start"`
			End     string   `json:"end"`
		}
		if err := unmarshalArgs(args, &p); err != nil {
			return ToolCallResult{}, err
		}
		startTime, err := time.Parse(time.RFC3339, p.Start)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid start time: %w", err)
		}
		endTime, err := time.Parse(time.RFC3339, p.End)
		if err != nil {
			return ToolCallResult{}, fmt.Errorf("invalid end time: %w", err)
		}
		client, err := cf(p.Account)
		if err != nil {
			return ToolCallResult{}, err
		}
		schedules, err := client.GetSchedule(ctx, p.Emails, startTime, endTime)
		if err != nil {
			return ToolCallResult{}, err
		}
		return jsonResult(schedules)
	}
}
