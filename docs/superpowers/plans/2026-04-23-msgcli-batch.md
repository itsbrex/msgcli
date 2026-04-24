# msgcli batch — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add JSON batching support to msgcli so multiple Graph requests can be issued in one HTTP round-trip (20 per call, with `dependsOn` chains), exposed both as a Go API (`graph.Client.Batch`) and as a user-facing `msgcli batch` subcommand that reads JSONL from stdin or a file.

**Architecture:** New `internal/graph/batch.go` owns the `$batch` client; it chunks request slices into 20-item groups, submits each via existing `Client.Post` semantics, and reassembles ordered responses. A new `internal/cmd/batch.go` Cobra command wires stdin/file input, parallel chunk execution, and error-propagation policies. No new scopes. All chunk boundaries respect `dependsOn` constraints (a request cannot depend on a request in another chunk).

**Tech Stack:** Go 1.25, `net/http`, `encoding/json`, existing `graph.Client` + `auth.GetValidToken`, Cobra, `net/http/httptest` for tests.

---

## File Structure

**Create:**
- `internal/graph/batch.go` — `BatchRequest`, `BatchResponse`, `Client.Batch(ctx, reqs []BatchRequest)`, chunk splitter, dependency validator.
- `internal/graph/batch_test.go` — unit tests with `httptest.Server`.
- `internal/cmd/batch.go` — `msgcli batch` Cobra command (reads JSONL or YAML file, prints JSON responses).
- `internal/cmd/batch_test.go` — command-level tests for input parsing and error exit codes.
- `docs/reference/batch.md` — user-facing reference (commands, examples, request/response shapes).

**Modify:**
- `internal/cmd/root.go` — register the new `batchCmd` via `init()` in the new file (no edit needed to `root.go` if the new file self-registers into `rootCmd`).

**Do not modify yet (future work, out of scope):** `mail_move.go`, `mail_delete.go` — internal refactors to use batch can follow in a separate plan once the API is stable.

---

## Task 1: Scaffold the batch package with request/response types

**Files:**
- Create: `internal/graph/batch.go`
- Test: `internal/graph/batch_test.go`

- [ ] **Step 1: Write the failing test for request/response marshaling**

```go
// internal/graph/batch_test.go
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/graph/ -run TestBatchRequestMarshal -v`
Expected: FAIL with "undefined: BatchPayload" / "undefined: BatchRequest".

- [ ] **Step 3: Create the types**

```go
// internal/graph/batch.go
package graph

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
	Requests []BatchRequest `json:"requests,omitempty"`
	Responses []BatchResponse `json:"responses,omitempty"`
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/graph/ -run TestBatchRequestMarshal -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/graph/batch.go internal/graph/batch_test.go
git commit -m "feat(graph): add BatchRequest/Response types

Scaffolds internal/graph/batch.go with JSON-batching envelope types.
Groundwork for Client.Batch (follow-up commits)."
```

---

## Task 2: Add chunking function (20 items per chunk, respect dependsOn)

**Files:**
- Modify: `internal/graph/batch.go`
- Test: `internal/graph/batch_test.go`

- [ ] **Step 1: Write the failing test for chunking**

```go
// append to internal/graph/batch_test.go
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
```

Add `"fmt"` to the test file imports.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/graph/ -run TestChunkBatch -v`
Expected: FAIL with "undefined: chunkBatch".

- [ ] **Step 3: Implement chunkBatch**

```go
// append to internal/graph/batch.go
import (
	"encoding/json"
	"fmt"
)

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
```

Note: replace the stub `import` in batch.go with the consolidated block above if needed.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/graph/ -run TestChunkBatch -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add internal/graph/batch.go internal/graph/batch_test.go
git commit -m "feat(graph): chunk batch requests with dep-safety check"
```

---

## Task 3: Implement Client.Batch against a fake Graph server

**Files:**
- Modify: `internal/graph/batch.go`, `internal/graph/client.go`
- Test: `internal/graph/batch_test.go`

- [ ] **Step 1: Expose a test seam on Client for base URL override**

Add a `baseURL` field to `Client` and a package-private setter for tests. This is needed so the test can point `Client.Batch` at `httptest.Server.URL` without mocking auth.

Replace `internal/graph/client.go` lines around `const baseURL` and `NewClient` with:

```go
const defaultBaseURL = "https://graph.microsoft.com/v1.0"

type Client struct {
	httpClient *http.Client
	account    string
	baseURL    string
	// tokenFn is a test seam so tests can bypass auth.GetValidToken.
	tokenFn func(ctx context.Context, account string) (string, error)
}

func NewClient(account string) *Client {
	return &Client{
		httpClient: &http.Client{},
		account:    account,
		baseURL:    defaultBaseURL,
	}
}

// newTestClient returns a Client configured for httptest. Package-private.
func newTestClient(baseURL string) *Client {
	return &Client{
		httpClient: &http.Client{},
		account:    "test",
		baseURL:    baseURL,
		tokenFn:    func(ctx context.Context, account string) (string, error) { return "test-token", nil },
	}
}

func (c *Client) token(ctx context.Context) (string, error) {
	if c.tokenFn != nil {
		return c.tokenFn(ctx, c.account)
	}
	return auth.GetValidToken(ctx, c.account)
}
```

Then replace in the `request` method:

```go
token, err := c.token(ctx)
...
reqURL := c.baseURL + path
```

(Remove the old `baseURL` const and the direct `auth.GetValidToken` call from `request`.)

- [ ] **Step 2: Write the failing test for Client.Batch**

```go
// append to internal/graph/batch_test.go
func TestClientBatchRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/$batch" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("missing bearer: %q", got)
		}
		var in BatchPayload
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Fatalf("decode: %v", err)
		}
		// Echo each request as a response with status 200.
		resp := BatchPayload{}
		for _, req := range in.Requests {
			resp.Responses = append(resp.Responses, BatchResponse{
				ID: req.ID, Status: 200,
				Body: json.RawMessage(fmt.Sprintf(`{"echoed":%q}`, req.URL)),
			})
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
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
```

Add `"context"`, `"net/http"`, and `"net/http/httptest"` to test imports.

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/graph/ -run TestClientBatchRoundTrip -v`
Expected: FAIL with "undefined: (*Client).Batch".

- [ ] **Step 4: Implement Client.Batch**

```go
// append to internal/graph/batch.go
import (
	"bytes"
	"context"
	"io"
	"net/http"
	"sort"
)

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
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/graph/ -v`
Expected: all graph tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/graph/client.go internal/graph/batch.go internal/graph/batch_test.go
git commit -m "feat(graph): implement Client.Batch against \$batch endpoint

Adds Client.Batch(ctx, requests) with per-chunk submission (20/call),
input-order preservation, and httptest coverage.

Introduces baseURL + tokenFn test seams on Client so fakes can bypass
the keychain-backed auth path in tests."
```

---

## Task 4: Surface per-sub-request 429 / error handling

**Files:**
- Modify: `internal/graph/batch.go`
- Test: `internal/graph/batch_test.go`

- [ ] **Step 1: Write the failing test**

```go
// append to internal/graph/batch_test.go
func TestClientBatchPartialFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(BatchPayload{
			Responses: []BatchResponse{
				{ID: "1", Status: 200, Body: json.RawMessage(`{"ok":true}`)},
				{ID: "2", Status: 429, Headers: map[string]string{"Retry-After": "3"}, Body: json.RawMessage(`{"error":{"code":"tooManyRequests"}}`)},
				{ID: "3", Status: 500, Body: json.RawMessage(`{"error":{"code":"internal"}}`)},
			},
		})
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/graph/ -run TestClientBatchPartialFailure -v`
Expected: FAIL (methods undefined).

- [ ] **Step 3: Add helper methods**

```go
// append to internal/graph/batch.go
import "strconv"

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
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/graph/ -run TestClientBatchPartialFailure -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/graph/batch.go internal/graph/batch_test.go
git commit -m "feat(graph): batch response helpers (OK, Throttled, RetryAfterSeconds)"
```

---

## Task 5: Cobra command — `msgcli batch` reading JSONL from stdin

**Files:**
- Create: `internal/cmd/batch.go`
- Test: `internal/cmd/batch_test.go`

- [ ] **Step 1: Write the failing test for JSONL parsing**

```go
// internal/cmd/batch_test.go
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/cmd/ -run TestParseJSONLRequests -v`
Expected: FAIL — "undefined: parseJSONLRequests".

- [ ] **Step 3: Create the command file with the parser**

```go
// internal/cmd/batch.go
package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/skylarbpayne/msgcli/internal/auth"
	"github.com/skylarbpayne/msgcli/internal/graph"
	"github.com/spf13/cobra"
)

var (
	batchInputFile string
)

var batchCmd = &cobra.Command{
	Use:   "batch",
	Short: "Execute a batch of Graph API requests in one call",
	Long: `Execute multiple Graph API requests as a single $batch call (up to 20 per chunk).

Reads JSONL (one BatchRequest per line) from stdin or --file.
Each request must have: id, method, url. Optional: body, headers, dependsOn.

Output is a JSON array of responses in the same order as input.`,
	RunE: runBatch,
}

func init() {
	batchCmd.Flags().StringVarP(&batchInputFile, "file", "f", "", "Read JSONL requests from a file (default: stdin)")
	rootCmd.AddCommand(batchCmd)
}

func parseJSONLRequests(r io.Reader) ([]graph.BatchRequest, error) {
	var out []graph.BatchRequest
	s := bufio.NewScanner(r)
	// Expand scanner buffer for large request bodies.
	s.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNum := 0
	for s.Scan() {
		lineNum++
		line := strings.TrimSpace(s.Text())
		if line == "" {
			continue
		}
		var req graph.BatchRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNum, err)
		}
		if req.ID == "" || req.Method == "" || req.URL == "" {
			return nil, fmt.Errorf("line %d: id, method, and url are required", lineNum)
		}
		out = append(out, req)
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func runBatch(cmd *cobra.Command, args []string) error {
	var src io.Reader = os.Stdin
	if batchInputFile != "" {
		f, err := os.Open(batchInputFile)
		if err != nil {
			return err
		}
		defer f.Close()
		src = f
	}

	reqs, err := parseJSONLRequests(src)
	if err != nil {
		return err
	}
	if len(reqs) == 0 {
		return fmt.Errorf("no requests found on input")
	}

	account, err := auth.ResolveAccount(GetAccountFlag())
	if err != nil {
		return err
	}
	client := graph.NewClient(account)

	responses, err := client.Batch(context.Background(), reqs)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(responses)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cmd/ -run TestParseJSONLRequests -v`
Expected: PASS (both tests).

- [ ] **Step 5: Build to confirm wiring**

Run: `go build ./...`
Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add internal/cmd/batch.go internal/cmd/batch_test.go
git commit -m "feat(cmd): add \`msgcli batch\` reading JSONL from stdin or --file"
```

---

## Task 6: Command-level test — end-to-end via fake server

**Files:**
- Modify: `internal/cmd/batch_test.go`

- [ ] **Step 1: Write the failing test**

```go
// append to internal/cmd/batch_test.go
import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/skylarbpayne/msgcli/internal/graph"
)

func TestRunBatchAgainstFakeServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in graph.BatchPayload
		_ = json.NewDecoder(r.Body).Decode(&in)
		out := graph.BatchPayload{}
		for _, req := range in.Requests {
			out.Responses = append(out.Responses, graph.BatchResponse{
				ID: req.ID, Status: 200, Body: json.RawMessage(`{"ok":true}`),
			})
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	defer srv.Close()

	client := graph.NewTestClient(srv.URL) // exported test helper — add in next step
	input := `{"id":"1","method":"GET","url":"/me"}` + "\n" +
		`{"id":"2","method":"GET","url":"/me/messages"}` + "\n"
	reqs, err := parseJSONLRequests(bytes.NewBufferString(input))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	resps, err := client.Batch(nil, reqs)
	if err != nil {
		t.Fatalf("batch error: %v", err)
	}
	if len(resps) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(resps))
	}
}
```

- [ ] **Step 2: Export the test helper from the graph package**

```go
// append to internal/graph/batch.go
// NewTestClient returns a Client wired to a custom base URL and a stub token.
// Intended for tests in other packages; do not use in production code paths.
func NewTestClient(baseURL string) *Client { return newTestClient(baseURL) }
```

- [ ] **Step 3: Run the test to verify it passes**

Run: `go test ./internal/cmd/ -run TestRunBatchAgainstFakeServer -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/cmd/batch_test.go internal/graph/batch.go
git commit -m "test(batch): end-to-end command test via httptest fake"
```

---

## Task 7: Docs + example

**Files:**
- Create: `docs/reference/batch.md`
- Modify: `README.md` (add `msgcli batch` row under Commands)

- [ ] **Step 1: Write the reference doc**

Create `docs/reference/batch.md` with: purpose, JSONL schema (id/method/url/body/headers/dependsOn), three examples (simple GET fanout, send-and-flag with `dependsOn`, reading from file), troubleshooting (per-sub 429, cross-chunk dependencies rejected).

- [ ] **Step 2: Add row to README Commands table**

Edit `README.md`; under the Mail section, add a new section "Batch" with a single row:

```markdown
### Batch

| Command | Description |
|---------|-------------|
| `msgcli batch [--file <path>]` | Run multiple Graph requests in one $batch call (JSONL in, JSON out) |
```

- [ ] **Step 3: Smoke-run against real account (manual)**

This step is **manual** — not covered by automated tests. Gate the automated portion of this plan as complete once Task 6 passes.

```bash
./bin/msgcli auth status -a <alias> -o json | head
echo '{"id":"1","method":"GET","url":"/me"}' | ./bin/msgcli batch -a <alias>
```

Expected: JSON array with one response where `status: 200` and `body.id` equals the account's Graph user ID.

- [ ] **Step 4: Commit**

```bash
git add docs/reference/batch.md README.md
git commit -m "docs(batch): reference + README pointer"
```

---

## Self-Review

- **Spec coverage:** Roadmap calls for BatchRequest/Response types (Task 1), chunking + dependsOn safety (Task 2), Client.Batch implementation (Task 3), 429/partial-failure handling (Task 4), CLI surface reading JSONL (Tasks 5-6), docs (Task 7). All covered.
- **Placeholder scan:** No "TBD" / "add appropriate error handling" strings. Every code block is real Go.
- **Type consistency:** `BatchRequest`, `BatchResponse`, `BatchPayload`, `chunkBatch`, `Client.Batch`, `newTestClient` / `NewTestClient` used consistently across tasks.
- **Known follow-ups (not in scope here):** refactor `mail delete` / `mail move` to consume `Client.Batch` internally; expose Retry-After retry loop for whole-call 429; YAML input mode. These are separate plans.
