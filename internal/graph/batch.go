package graph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
)

// BatchRequest is one sub-request inside a $batch call.
// See https://learn.microsoft.com/en-us/graph/json-batching
type BatchRequest struct {
	ID        string            `json:"id"`
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	Body      interface{}       `json:"body,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	DependsOn []string          `json:"dependsOn,omitempty"`
}

// BatchResponse is one sub-response from a $batch call.
type BatchResponse struct {
	ID      string            `json:"id"`
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    json.RawMessage   `json:"body,omitempty"`
}

// BatchPayload is the JSON envelope Graph expects/returns.
type BatchPayload struct {
	Requests  []BatchRequest  `json:"requests,omitempty"`
	Responses []BatchResponse `json:"responses,omitempty"`
}

const batchChunkSize = 20

// Batch sends one or more Graph requests in a single $batch call.
// Responses are returned in the same order as the input requests.
func (c *Client) Batch(ctx context.Context, reqs []BatchRequest) ([]BatchResponse, error) {
	if len(reqs) == 0 {
		return nil, nil
	}
	chunks, err := chunkBatch(reqs, batchChunkSize)
	if err != nil {
		return nil, err
	}

	token, err := c.token(ctx)
	if err != nil {
		return nil, fmt.Errorf("auth error: %w", err)
	}

	var all []BatchResponse
	for _, chunk := range chunks {
		payload, err := json.Marshal(BatchPayload{Requests: chunk})
		if err != nil {
			return nil, err
		}
		httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/$batch", bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Authorization", "Bearer "+token)
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("batch HTTP %d: %s", resp.StatusCode, string(body))
		}
		var out BatchPayload
		if err := json.Unmarshal(body, &out); err != nil {
			return nil, fmt.Errorf("batch parse: %w", err)
		}
		all = append(all, out.Responses...)
	}

	// Sort responses by input order. Graph does not guarantee response order,
	// and we preserve the caller's ordering by ID.
	orderByID := make(map[string]int, len(reqs))
	for i, r := range reqs {
		orderByID[r.ID] = i
	}
	sort.SliceStable(all, func(i, j int) bool {
		return orderByID[all[i].ID] < orderByID[all[j].ID]
	})
	return all, nil
}

// chunkBatch splits reqs into groups of at most size. It returns an error if
// any request's dependsOn references an ID that is not in the same chunk
// (Graph's $batch requires dependencies to be within one batch call).
func chunkBatch(reqs []BatchRequest, size int) ([][]BatchRequest, error) {
	if size <= 0 {
		return nil, fmt.Errorf("chunk size must be > 0")
	}
	var chunks [][]BatchRequest
	for i := 0; i < len(reqs); i += size {
		end := i + size
		if end > len(reqs) {
			end = len(reqs)
		}
		chunk := reqs[i:end]

		ids := make(map[string]bool, len(chunk))
		for _, r := range chunk {
			ids[r.ID] = true
		}
		for _, r := range chunk {
			for _, dep := range r.DependsOn {
				if !ids[dep] {
					return nil, fmt.Errorf("request %q depends on %q which is not in the same chunk (move them together or split the call)", r.ID, dep)
				}
			}
		}
		chunks = append(chunks, chunk)
	}
	return chunks, nil
}

// OK reports whether the sub-response had a 2xx status.
func (r BatchResponse) OK() bool { return r.Status >= 200 && r.Status < 300 }

// Throttled reports whether Graph throttled this sub-request.
func (r BatchResponse) Throttled() bool { return r.Status == 429 }

// RetryAfterSeconds returns the Retry-After header in seconds, or 0 if absent.
func (r BatchResponse) RetryAfterSeconds() int {
	if v := r.Headers["Retry-After"]; v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return 0
}
