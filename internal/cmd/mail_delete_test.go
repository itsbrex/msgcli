package cmd

import (
	"testing"
)

func TestBuildDeleteBatch(t *testing.T) {
	reqs := buildDeleteBatch([]string{"a", "b", "c"})
	if len(reqs) != 3 {
		t.Fatalf("expected 3 requests, got %d", len(reqs))
	}
	for i, r := range reqs {
		if r.Method != "DELETE" {
			t.Errorf("req[%d]: expected DELETE, got %q", i, r.Method)
		}
	}
	if reqs[0].URL != "/me/messages/a" || reqs[2].URL != "/me/messages/c" {
		t.Fatalf("unexpected URLs: %+v", reqs)
	}
	// IDs must be unique per Client.Batch.
	seen := map[string]bool{}
	for _, r := range reqs {
		if seen[r.ID] {
			t.Fatalf("duplicate batch id: %q", r.ID)
		}
		seen[r.ID] = true
	}
}

func TestBuildDeleteBatchEmpty(t *testing.T) {
	reqs := buildDeleteBatch(nil)
	if len(reqs) != 0 {
		t.Fatalf("expected empty, got %d", len(reqs))
	}
}
