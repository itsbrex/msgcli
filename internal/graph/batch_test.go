package graph

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestBatchRequestMarshal(t *testing.T) {
	reqs := BatchPayload{
		Requests: []BatchRequest{
			{ID: "1", Method: "GET", URL: "/me"},
			{ID: "2", Method: "POST", URL: "/me/sendMail", Body: map[string]string{"a": "b"}, Headers: map[string]string{"Content-Type": "application/json"}, DependsOn: []string{"1"}},
		},
	}

	data, err := json.Marshal(reqs)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	arr, ok := got["requests"].([]interface{})
	if !ok || len(arr) != 2 {
		t.Fatalf("expected 2 requests, got: %v", got)
	}
	first := arr[0].(map[string]interface{})
	if first["id"] != "1" || first["method"] != "GET" || first["url"] != "/me" {
		t.Fatalf("unexpected first request: %v", first)
	}
	second := arr[1].(map[string]interface{})
	deps, ok := second["dependsOn"].([]interface{})
	if !ok || len(deps) != 1 || deps[0] != "1" {
		t.Fatalf("expected dependsOn=[1], got: %v", second["dependsOn"])
	}
}

func TestChunkBatch(t *testing.T) {
	reqs := make([]BatchRequest, 45)
	for i := range reqs {
		reqs[i] = BatchRequest{ID: fmt.Sprintf("%d", i+1), Method: "GET", URL: "/me"}
	}
	chunks, err := chunkBatch(reqs, 20)
	if err != nil {
		t.Fatalf("chunkBatch error: %v", err)
	}
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	if len(chunks[0]) != 20 || len(chunks[1]) != 20 || len(chunks[2]) != 5 {
		t.Fatalf("unexpected chunk sizes: %d/%d/%d", len(chunks[0]), len(chunks[1]), len(chunks[2]))
	}
}

func TestChunkBatchCrossChunkDependencyRejected(t *testing.T) {
	reqs := make([]BatchRequest, 21)
	for i := range reqs {
		reqs[i] = BatchRequest{ID: fmt.Sprintf("%d", i+1), Method: "GET", URL: "/me"}
	}
	// Request 21 depends on request 1 — across the 20-item boundary.
	reqs[20].DependsOn = []string{"1"}
	if _, err := chunkBatch(reqs, 20); err == nil {
		t.Fatalf("expected cross-chunk dependency error, got nil")
	}
}
