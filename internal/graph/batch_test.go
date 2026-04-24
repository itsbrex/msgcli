package graph

import (
	"encoding/json"
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
