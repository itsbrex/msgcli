package graph

import (
	"encoding/json"
	"fmt"
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
