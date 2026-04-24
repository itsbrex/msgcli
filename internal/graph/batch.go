package graph

import "encoding/json"

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
