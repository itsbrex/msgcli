package graph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBatchRequestMarshal(t *testing.T) {
	reqs := batchPayload{
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

func TestClientBatchRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/$batch" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("missing bearer: %q", got)
		}
		var in batchPayload
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Fatalf("decode: %v", err)
		}
		// Echo each request as a response with status 200.
		resp := batchPayload{}
		for _, req := range in.Requests {
			resp.Responses = append(resp.Responses, BatchResponse{
				ID: req.ID, Status: 200,
				Body: json.RawMessage(fmt.Sprintf(`{"echoed":%q}`, req.URL)),
			})
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := NewTestClient(srv.URL)
	reqs := []BatchRequest{
		{ID: "1", Method: "GET", URL: "/me"},
		{ID: "2", Method: "GET", URL: "/me/messages"},
	}
	responses, err := c.Batch(context.Background(), reqs)
	if err != nil {
		t.Fatalf("Batch error: %v", err)
	}
	if len(responses) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(responses))
	}
	if responses[0].ID != "1" || responses[1].ID != "2" {
		t.Fatalf("unexpected response ordering: %+v", responses)
	}
	if responses[0].Status != 200 {
		t.Fatalf("expected status 200, got %d", responses[0].Status)
	}
}

func TestClientBatchPartialFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(batchPayload{
			Responses: []BatchResponse{
				{ID: "1", Status: 200, Body: json.RawMessage(`{"ok":true}`)},
				{ID: "2", Status: 429, Headers: map[string]string{"Retry-After": "3"}, Body: json.RawMessage(`{"error":{"code":"tooManyRequests"}}`)},
				{ID: "3", Status: 500, Body: json.RawMessage(`{"error":{"code":"internal"}}`)},
			},
		})
	}))
	defer srv.Close()

	c := NewTestClient(srv.URL)
	responses, err := c.Batch(context.Background(), []BatchRequest{
		{ID: "1", Method: "GET", URL: "/me"},
		{ID: "2", Method: "GET", URL: "/me/messages"},
		{ID: "3", Method: "GET", URL: "/me/events"},
	})
	if err != nil {
		t.Fatalf("Batch returned wrapper error: %v", err)
	}
	if !responses[0].OK() {
		t.Errorf("expected response 1 OK")
	}
	if responses[1].OK() || !responses[1].Throttled() {
		t.Errorf("expected response 2 throttled")
	}
	if responses[1].RetryAfterSeconds() != 3 {
		t.Errorf("expected Retry-After 3s, got %d", responses[1].RetryAfterSeconds())
	}
	if responses[2].OK() {
		t.Errorf("expected response 3 not OK")
	}
}

func TestClientBatchRetriesOn429(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(429)
			_, _ = w.Write([]byte(`{"error":{"code":"tooManyRequests"}}`))
			return
		}
		_ = json.NewEncoder(w).Encode(batchPayload{
			Responses: []BatchResponse{
				{ID: "1", Status: 200, Body: json.RawMessage(`{"ok":true}`)},
			},
		})
	}))
	defer srv.Close()

	c := NewTestClient(srv.URL)
	responses, err := c.Batch(context.Background(), []BatchRequest{
		{ID: "1", Method: "GET", URL: "/me"},
	})
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls (429 + success), got %d", calls)
	}
	if len(responses) != 1 || responses[0].Status != 200 {
		t.Fatalf("unexpected responses after retry: %+v", responses)
	}
}

func TestClientBatchGivesUpAfterMaxRetriesOn429(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(429)
		_, _ = w.Write([]byte(`{"error":{"code":"tooManyRequests"}}`))
	}))
	defer srv.Close()

	c := NewTestClient(srv.URL)
	_, err := c.Batch(context.Background(), []BatchRequest{
		{ID: "1", Method: "GET", URL: "/me"},
	})
	if err == nil {
		t.Fatalf("expected error after exhausting retries, got nil")
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls (1 original + 2 retries), got %d", calls)
	}
	if !strings.Contains(err.Error(), "429") {
		t.Fatalf("expected error to mention 429: %v", err)
	}
}

func TestClientBatchRespectsContextDuringRetryWait(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Retry-After", "10") // longer than we'll wait
		w.WriteHeader(429)
		_, _ = w.Write([]byte(`{"error":{"code":"tooManyRequests"}}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	c := NewTestClient(srv.URL)
	_, err := c.Batch(ctx, []BatchRequest{{ID: "1", Method: "GET", URL: "/me"}})
	if err == nil {
		t.Fatalf("expected context error, got nil")
	}
	if calls != 1 {
		t.Fatalf("expected 1 call before context cancelled, got %d", calls)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got: %v", err)
	}
}
