package cmd

import (
	"strings"
	"testing"
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
