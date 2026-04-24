package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/skylarbpayne/msgcli/internal/graph"
)

func TestParseJSONLRequests(t *testing.T) {
	input := `{"id":"1","method":"GET","url":"/me"}
{"id":"2","method":"POST","url":"/me/sendMail","body":{"subject":"hi"}}
`
	reqs, err := parseJSONLRequests(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parseJSONLRequests error: %v", err)
	}
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(reqs))
	}
	if reqs[0].ID != "1" || reqs[0].Method != "GET" || reqs[0].URL != "/me" {
		t.Fatalf("unexpected first request: %+v", reqs[0])
	}
	if reqs[1].Method != "POST" || reqs[1].Body == nil {
		t.Fatalf("unexpected second request: %+v", reqs[1])
	}
}

func TestParseJSONLRequestsEmptyLinesIgnored(t *testing.T) {
	input := "\n" +
		`{"id":"1","method":"GET","url":"/me"}` + "\n" +
		"\n" +
		`{"id":"2","method":"GET","url":"/me/messages"}` + "\n"
	reqs, err := parseJSONLRequests(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parseJSONLRequests error: %v", err)
	}
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests after ignoring blank lines, got %d", len(reqs))
	}
}

func TestParseJSONLRequestsRejectsDuplicateIDs(t *testing.T) {
	input := `{"id":"1","method":"GET","url":"/me"}
{"id":"2","method":"GET","url":"/me/messages"}
{"id":"1","method":"GET","url":"/me/events"}
`
	_, err := parseJSONLRequests(strings.NewReader(input))
	if err == nil {
		t.Fatalf("expected error for duplicate id=1, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate") || !strings.Contains(err.Error(), "1") {
		t.Fatalf("expected error to mention duplicate id 1, got: %v", err)
	}
}

func TestRunBatchAgainstFakeServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in graph.BatchPayload
		_ = json.NewDecoder(r.Body).Decode(&in)
		out := graph.BatchPayload{}
		for _, req := range in.Requests {
			out.Responses = append(out.Responses, graph.BatchResponse{
				ID: req.ID, Status: 200, Body: json.RawMessage(`{"ok":true}`),
			})
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	defer srv.Close()

	client := graph.NewTestClient(srv.URL)
	input := `{"id":"1","method":"GET","url":"/me"}` + "\n" +
		`{"id":"2","method":"GET","url":"/me/messages"}` + "\n"
	reqs, err := parseJSONLRequests(bytes.NewBufferString(input))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	resps, err := client.Batch(context.Background(), reqs)
	if err != nil {
		t.Fatalf("batch error: %v", err)
	}
	if len(resps) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(resps))
	}
}
