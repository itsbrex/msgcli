package cmd

import (
	"testing"
)

func TestBuildMoveBatch(t *testing.T) {
	reqs := buildMoveBatch([]string{"x", "y"}, "archive")
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(reqs))
	}
	if reqs[0].Method != "POST" || reqs[0].URL != "/me/messages/x/move" {
		t.Fatalf("unexpected first request: %+v", reqs[0])
	}
	body, ok := reqs[0].Body.(map[string]string)
	if !ok || body["destinationId"] != "archive" {
		t.Fatalf("unexpected body: %+v", reqs[0].Body)
	}
	if reqs[0].Headers["Content-Type"] != "application/json" {
		t.Fatalf("missing Content-Type header: %+v", reqs[0].Headers)
	}
	// Unique batch IDs.
	seen := map[string]bool{}
	for _, r := range reqs {
		if seen[r.ID] {
			t.Fatalf("duplicate batch id: %q", r.ID)
		}
		seen[r.ID] = true
	}
}
