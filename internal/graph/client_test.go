package graph

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type testPayload struct {
	Name string `json:"name"`
	N    int    `json:"n"`
}

func TestClient_Get(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/me" {
			t.Errorf("path = %s, want /me", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer test-token")
		}
		_ = json.NewEncoder(w).Encode(testPayload{Name: "alice", N: 42})
	}))
	defer server.Close()

	c := NewTestClient(server.URL)
	var got testPayload
	if err := c.Get(context.Background(), "/me", &got); err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got.Name != "alice" || got.N != 42 {
		t.Errorf("got %+v, want {alice 42}", got)
	}
}

func TestClient_Post_MarshalsBody(t *testing.T) {
	var received testPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &received); err != nil {
			t.Fatalf("unmarshal request body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"name":"echo","n":7}`))
	}))
	defer server.Close()

	c := NewTestClient(server.URL)
	in := testPayload{Name: "bob", N: 1}
	var out testPayload
	if err := c.Post(context.Background(), "/items", in, &out); err != nil {
		t.Fatalf("Post returned error: %v", err)
	}
	if received != in {
		t.Errorf("server received %+v, want %+v", received, in)
	}
	if out.Name != "echo" || out.N != 7 {
		t.Errorf("response = %+v, want {echo 7}", out)
	}
}

func TestClient_Patch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("method = %s, want PATCH", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := NewTestClient(server.URL)
	if err := c.Patch(context.Background(), "/items/1", map[string]string{"x": "y"}, nil); err != nil {
		t.Fatalf("Patch returned error: %v", err)
	}
}

func TestClient_Delete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	c := NewTestClient(server.URL)
	if err := c.Delete(context.Background(), "/items/1"); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
}

func TestClient_GraphAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"BadArg","message":"thing is wrong"}}`))
	}))
	defer server.Close()

	c := NewTestClient(server.URL)
	err := c.Get(context.Background(), "/x", nil)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "BadArg") || !strings.Contains(err.Error(), "thing is wrong") {
		t.Errorf("error = %v, want BadArg/thing is wrong", err)
	}
}

func TestClient_HTTPError_NoGraphBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("plain text failure"))
	}))
	defer server.Close()

	c := NewTestClient(server.URL)
	err := c.Get(context.Background(), "/x", nil)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "plain text failure") {
		t.Errorf("error = %v, want HTTP 500 with body", err)
	}
}

func TestClient_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	c := NewTestClient(server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := c.Get(ctx, "/slow", nil)
	if err == nil {
		t.Fatal("want error from cancelled context, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled in chain", err)
	}
}

func TestQueryParams_ToQuery(t *testing.T) {
	tests := []struct {
		name    string
		q       QueryParams
		wantHas []string
		wantEq  string
	}{
		{
			name:   "empty",
			q:      QueryParams{},
			wantEq: "",
		},
		{
			name:    "select and top",
			q:       QueryParams{Select: []string{"id", "subject"}, Top: 25},
			wantHas: []string{"%24select=id%2Csubject", "%24top=25"},
		},
		{
			name:    "search wraps in quotes",
			q:       QueryParams{Search: "urgent"},
			wantHas: []string{"%24search=%22urgent%22"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.q.ToQuery()
			if tt.wantEq != "" || len(tt.wantHas) == 0 {
				if got != tt.wantEq {
					t.Errorf("ToQuery() = %q, want %q", got, tt.wantEq)
				}
				return
			}
			if !strings.HasPrefix(got, "?") {
				t.Errorf("ToQuery() = %q, want leading ?", got)
			}
			for _, sub := range tt.wantHas {
				if !strings.Contains(got, sub) {
					t.Errorf("ToQuery() = %q, missing %q", got, sub)
				}
			}
		})
	}
}
