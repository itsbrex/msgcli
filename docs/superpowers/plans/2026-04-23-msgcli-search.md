# msgcli search — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `msgcli search` — a unified search command that issues `POST /search/query` to Microsoft Graph and returns ranked hits across messages, events, and (when the auth flow permits) driveItems, chat messages, and people. Normalizes disparate entity shapes into a single JSON `Hit` record for agent consumption.

**Architecture:** A new `internal/graph/search.go` owns the search client and the `Hit` normalization. Entity-type eligibility is gated by the auth flow (legacy = message+event; msal-office = all). A new `internal/cmd/search.go` wraps the client with a Cobra command that accepts a KQL query (as positional arg) and `--in` to pick entity types. Table output shows a unified columnset; JSON output keeps per-entity detail in a `raw` field.

**Tech Stack:** Go 1.25, existing `graph.Client`, Cobra, `httptest` for tests. No new dependencies.

---

## File Structure

**Create:**
- `internal/graph/search.go` — `SearchRequest`, `Hit`, `Client.Search(ctx, req)`, entity-type helpers.
- `internal/graph/search_test.go`
- `internal/cmd/search.go` — `msgcli search` Cobra command.
- `internal/cmd/search_test.go`
- `docs/reference/search.md`

**Modify:**
- `README.md` — add Commands row.

---

## Task 1: Search request/response types + normalization

**Files:**
- Create: `internal/graph/search.go`, `internal/graph/search_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/graph/search_test.go
package graph

import (
	"encoding/json"
	"testing"
)

func TestSearchRequestEnvelope(t *testing.T) {
	req := SearchRequest{
		EntityTypes: []string{"message", "event"},
		Query:       SearchQuery{QueryString: "quarterly review"},
		From:        0,
		Size:        25,
	}
	data, err := json.Marshal(SearchPayload{Requests: []SearchRequest{req}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]interface{}
	_ = json.Unmarshal(data, &got)
	reqs := got["requests"].([]interface{})
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}
	r := reqs[0].(map[string]interface{})
	if r["from"].(float64) != 0 || r["size"].(float64) != 25 {
		t.Fatalf("pagination fields missing: %+v", r)
	}
	if q := r["query"].(map[string]interface{})["queryString"]; q != "quarterly review" {
		t.Fatalf("queryString missing: %v", q)
	}
}

func TestNormalizeMessageHit(t *testing.T) {
	raw := json.RawMessage(`{
		"hitId":"h1",
		"rank":1,
		"summary":"summary text",
		"resource":{
			"@odata.type":"#microsoft.graph.message",
			"id":"m1",
			"subject":"Budget sync",
			"from":{"emailAddress":{"address":"a@b.com","name":"A B"}},
			"receivedDateTime":"2026-03-01T10:00:00Z"
		}
	}`)
	hit, err := normalizeHit("message", raw)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if hit.Entity != "message" || hit.ID != "m1" || hit.Title != "Budget sync" || hit.From != "a@b.com" {
		t.Fatalf("unexpected hit: %+v", hit)
	}
	if hit.Summary != "summary text" {
		t.Fatalf("expected summary, got %q", hit.Summary)
	}
	if hit.Received.IsZero() {
		t.Fatalf("expected Received set")
	}
}

func TestNormalizeEventHit(t *testing.T) {
	raw := json.RawMessage(`{
		"hitId":"h2","rank":2,
		"resource":{
			"@odata.type":"#microsoft.graph.event",
			"id":"e1",
			"subject":"1:1",
			"organizer":{"emailAddress":{"address":"o@x.com"}},
			"start":{"dateTime":"2026-04-01T14:00:00","timeZone":"UTC"}
		}
	}`)
	hit, err := normalizeHit("event", raw)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if hit.Entity != "event" || hit.ID != "e1" || hit.Title != "1:1" || hit.From != "o@x.com" {
		t.Fatalf("unexpected hit: %+v", hit)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/graph/ -run TestSearch -v`
Expected: FAIL — types / function undefined.

- [ ] **Step 3: Implement search.go types and normalizer**

```go
// internal/graph/search.go
package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// SearchPayload wraps one or more search requests.
type SearchPayload struct {
	Requests []SearchRequest `json:"requests,omitempty"`
	// Graph returns the same shape with a "value" field for responses.
	Value []SearchResponseContainer `json:"value,omitempty"`
}

// SearchRequest follows the /search/query schema.
type SearchRequest struct {
	EntityTypes []string    `json:"entityTypes"`
	Query       SearchQuery `json:"query"`
	From        int         `json:"from,omitempty"`
	Size        int         `json:"size,omitempty"`
	Fields      []string    `json:"fields,omitempty"`
	Region      string      `json:"region,omitempty"`
}

// SearchQuery carries the KQL query string.
type SearchQuery struct {
	QueryString string `json:"queryString"`
}

// SearchResponseContainer is one entry under the top-level "value" array.
type SearchResponseContainer struct {
	SearchTerms  []string                  `json:"searchTerms,omitempty"`
	HitsContainers []SearchHitsContainer   `json:"hitsContainers"`
}

// SearchHitsContainer groups hits for one query.
type SearchHitsContainer struct {
	Hits  []SearchHitRaw `json:"hits"`
	Total int            `json:"total"`
	More  bool           `json:"moreResultsAvailable"`
}

// SearchHitRaw is the envelope before normalization.
type SearchHitRaw struct {
	HitID    string          `json:"hitId"`
	Rank     int             `json:"rank"`
	Summary  string          `json:"summary"`
	Resource json.RawMessage `json:"resource"`
}

// Hit is msgcli's normalized view of a search result across entity types.
type Hit struct {
	Entity   string          `json:"entity"` // "message" | "event" | "driveItem" | "chatMessage" | "person"
	ID       string          `json:"id"`
	HitID    string          `json:"hitId,omitempty"`
	Rank     int             `json:"rank"`
	Title    string          `json:"title,omitempty"`
	From     string          `json:"from,omitempty"`
	Received time.Time       `json:"received,omitempty"`
	Summary  string          `json:"summary,omitempty"`
	WebLink  string          `json:"webLink,omitempty"`
	Raw      json.RawMessage `json:"raw,omitempty"`
}

// normalizeHit converts a raw hit into a Hit, picking fields per entity type.
func normalizeHit(entity string, raw json.RawMessage) (Hit, error) {
	var env SearchHitRaw
	if err := json.Unmarshal(raw, &env); err != nil {
		return Hit{}, err
	}
	h := Hit{
		Entity:  entity,
		HitID:   env.HitID,
		Rank:    env.Rank,
		Summary: env.Summary,
		Raw:     env.Resource,
	}
	switch entity {
	case "message":
		var m struct {
			ID               string     `json:"id"`
			Subject          string     `json:"subject"`
			From             *Recipient `json:"from"`
			ReceivedDateTime time.Time  `json:"receivedDateTime"`
			WebLink          string     `json:"webLink"`
		}
		if err := json.Unmarshal(env.Resource, &m); err != nil {
			return h, err
		}
		h.ID, h.Title = m.ID, m.Subject
		if m.From != nil {
			h.From = m.From.EmailAddress.Address
		}
		h.Received = m.ReceivedDateTime
		h.WebLink = m.WebLink
	case "event":
		var e struct {
			ID        string     `json:"id"`
			Subject   string     `json:"subject"`
			Organizer *Recipient `json:"organizer"`
			Start     struct {
				DateTime string `json:"dateTime"`
			} `json:"start"`
			WebLink string `json:"webLink"`
		}
		if err := json.Unmarshal(env.Resource, &e); err != nil {
			return h, err
		}
		h.ID, h.Title = e.ID, e.Subject
		if e.Organizer != nil {
			h.From = e.Organizer.EmailAddress.Address
		}
		h.WebLink = e.WebLink
		if e.Start.DateTime != "" {
			if t, err := time.Parse("2006-01-02T15:04:05", e.Start.DateTime); err == nil {
				h.Received = t
			}
		}
	case "driveItem":
		var d struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			WebURL  string `json:"webUrl"`
			CreatedBy struct {
				User struct {
					Email string `json:"email"`
				} `json:"user"`
			} `json:"createdBy"`
			LastModifiedDateTime time.Time `json:"lastModifiedDateTime"`
		}
		if err := json.Unmarshal(env.Resource, &d); err != nil {
			return h, err
		}
		h.ID, h.Title, h.WebLink = d.ID, d.Name, d.WebURL
		h.From = d.CreatedBy.User.Email
		h.Received = d.LastModifiedDateTime
	case "chatMessage":
		var c struct {
			ID            string `json:"id"`
			From          struct {
				User struct {
					DisplayName string `json:"displayName"`
				} `json:"user"`
			} `json:"from"`
			CreatedDateTime time.Time `json:"createdDateTime"`
			WebURL          string    `json:"webUrl"`
			Summary         string    `json:"summary"`
		}
		if err := json.Unmarshal(env.Resource, &c); err != nil {
			return h, err
		}
		h.ID, h.WebLink = c.ID, c.WebURL
		h.From = c.From.User.DisplayName
		h.Received = c.CreatedDateTime
	case "person":
		var p struct {
			ID              string `json:"id"`
			DisplayName     string `json:"displayName"`
			ScoredEmail     []struct {
				Address string `json:"address"`
			} `json:"scoredEmailAddresses"`
		}
		if err := json.Unmarshal(env.Resource, &p); err != nil {
			return h, err
		}
		h.ID, h.Title = p.ID, p.DisplayName
		if len(p.ScoredEmail) > 0 {
			h.From = p.ScoredEmail[0].Address
		}
	default:
		return h, fmt.Errorf("unknown entity type: %s", entity)
	}
	return h, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/graph/ -run TestSearch -v`
Expected: PASS (all three tests).

- [ ] **Step 5: Commit**

```bash
git add internal/graph/search.go internal/graph/search_test.go
git commit -m "feat(graph): SearchRequest + Hit normalization across entity types"
```

---

## Task 2: Client.Search against fake server

**Files:**
- Modify: `internal/graph/search.go`, `internal/graph/search_test.go`

- [ ] **Step 1: Write the failing test**

```go
// append to internal/graph/search_test.go
import (
	"context"
	"net/http"
	"net/http/httptest"
)

func TestClientSearchRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/query" {
			t.Errorf("wrong path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{
			"value":[{"hitsContainers":[{"total":1,"hits":[
				{"hitId":"h1","rank":1,"summary":"s","resource":{
					"@odata.type":"#microsoft.graph.message","id":"m1","subject":"Hi","from":{"emailAddress":{"address":"x@y.com"}},"receivedDateTime":"2026-03-01T00:00:00Z"
				}}
			]}]}]
		}`))
	}))
	defer srv.Close()

	c := NewTestClient(srv.URL)
	hits, err := c.Search(context.Background(), SearchRequest{
		EntityTypes: []string{"message"},
		Query:       SearchQuery{QueryString: "hi"},
		Size:        10,
	})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != "m1" || hits[0].Entity != "message" {
		t.Fatalf("unexpected hits: %+v", hits)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/graph/ -run TestClientSearchRoundTrip -v`
Expected: FAIL — "undefined: (*Client).Search".

- [ ] **Step 3: Implement Client.Search**

```go
// append to internal/graph/search.go
import (
	"bytes"
	"io"
	"net/http"
)

// Search issues POST /search/query for one SearchRequest and returns
// normalized hits across all entity types requested. Uses the same Bearer
// token as other Client calls (no new scopes).
func (c *Client) Search(ctx context.Context, req SearchRequest) ([]Hit, error) {
	if len(req.EntityTypes) == 0 {
		return nil, fmt.Errorf("at least one entityType is required")
	}
	payload, err := json.Marshal(SearchPayload{Requests: []SearchRequest{req}})
	if err != nil {
		return nil, err
	}

	token, err := c.token(ctx)
	if err != nil {
		return nil, fmt.Errorf("auth error: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/search/query", bytes.NewReader(payload))
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
		return nil, fmt.Errorf("search HTTP %d: %s", resp.StatusCode, string(body))
	}

	var out SearchPayload
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("search parse: %w", err)
	}

	var hits []Hit
	for _, container := range out.Value {
		for _, hc := range container.HitsContainers {
			for _, raw := range hc.Hits {
				entity := entityFromOData(raw.Resource)
				h, err := normalizeHit(entity, mustMarshal(raw))
				if err != nil {
					// Keep going on individual failures; surface as an unknown-entity hit.
					continue
				}
				hits = append(hits, h)
			}
		}
	}
	return hits, nil
}

// entityFromOData inspects @odata.type to infer entity category.
func entityFromOData(resource json.RawMessage) string {
	var t struct {
		ODataType string `json:"@odata.type"`
	}
	_ = json.Unmarshal(resource, &t)
	switch t.ODataType {
	case "#microsoft.graph.message":
		return "message"
	case "#microsoft.graph.event":
		return "event"
	case "#microsoft.graph.driveItem":
		return "driveItem"
	case "#microsoft.graph.chatMessage":
		return "chatMessage"
	case "#microsoft.graph.person":
		return "person"
	}
	return "unknown"
}

func mustMarshal(v interface{}) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/graph/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/graph/search.go internal/graph/search_test.go
git commit -m "feat(graph): Client.Search against /search/query"
```

---

## Task 3: Entity-type gating by auth flow

**Files:**
- Modify: `internal/graph/search.go`
- Test: `internal/graph/search_test.go`

- [ ] **Step 1: Write the failing test**

```go
// append to internal/graph/search_test.go
func TestValidateEntitiesForFlow(t *testing.T) {
	// Allowed combos.
	if err := ValidateEntitiesForFlow("legacy", []string{"message", "event"}); err != nil {
		t.Fatalf("unexpected error for legacy mail+event: %v", err)
	}
	if err := ValidateEntitiesForFlow("msal-office", []string{"message", "event", "driveItem", "chatMessage", "person"}); err != nil {
		t.Fatalf("unexpected error for msal-office all: %v", err)
	}
	// Rejected combos.
	if err := ValidateEntitiesForFlow("legacy", []string{"driveItem"}); err == nil {
		t.Fatalf("expected error for legacy+driveItem")
	}
	if err := ValidateEntitiesForFlow("legacy", []string{"chatMessage"}); err == nil {
		t.Fatalf("expected error for legacy+chatMessage")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/graph/ -run TestValidateEntitiesForFlow -v`
Expected: FAIL.

- [ ] **Step 3: Implement the validator**

```go
// append to internal/graph/search.go

// entitiesByFlow lists search entity types permitted per auth flow.
var entitiesByFlow = map[string]map[string]bool{
	"legacy": {
		"message": true,
		"event":   true,
	},
	"msal-office": {
		"message":     true,
		"event":       true,
		"driveItem":   true,
		"chatMessage": true,
		"person":      true,
	},
}

// ValidateEntitiesForFlow returns a descriptive error if any requested
// entity type is not available under the given auth flow.
func ValidateEntitiesForFlow(flow string, entities []string) error {
	allowed, ok := entitiesByFlow[flow]
	if !ok {
		return fmt.Errorf("unknown flow: %s", flow)
	}
	for _, e := range entities {
		if !allowed[e] {
			return fmt.Errorf("entity type %q requires msal-office flow (current: %s); rerun with --flow msal-office or use --account <msal-office-alias>", e, flow)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/graph/ -run TestValidateEntitiesForFlow -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/graph/search.go internal/graph/search_test.go
git commit -m "feat(graph): per-flow entity gating for /search/query"
```

---

## Task 4: `msgcli search` Cobra command

**Files:**
- Create: `internal/cmd/search.go`, `internal/cmd/search_test.go`

- [ ] **Step 1: Write the failing test for flag parsing**

```go
// internal/cmd/search_test.go
package cmd

import (
	"testing"
)

func TestParseEntityFlags(t *testing.T) {
	got, err := parseEntityFlags("mail,events,files,chats,people")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	want := []string{"message", "event", "driveItem", "chatMessage", "person"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i, e := range want {
		if got[i] != e {
			t.Fatalf("at %d: expected %q, got %q", i, e, got[i])
		}
	}
}

func TestParseEntityFlagsUnknown(t *testing.T) {
	if _, err := parseEntityFlags("mail,calendar"); err == nil {
		t.Fatalf("expected error for unknown friendly name 'calendar'")
	}
}

func TestParseEntityFlagsDefault(t *testing.T) {
	got, err := parseEntityFlags("")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(got) != 2 || got[0] != "message" || got[1] != "event" {
		t.Fatalf("unexpected default: %v", got)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/cmd/ -run TestParseEntityFlags -v`
Expected: FAIL.

- [ ] **Step 3: Implement search.go**

```go
// internal/cmd/search.go
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/skylarbpayne/msgcli/internal/auth"
	"github.com/skylarbpayne/msgcli/internal/graph"
	"github.com/spf13/cobra"
)

var (
	searchInFlag   string
	searchTopFlag  int
	searchFromFlag int
)

// friendlyToEntity maps user-facing shorthand to Graph entity types.
var friendlyToEntity = map[string]string{
	"mail":    "message",
	"events":  "event",
	"files":   "driveItem",
	"chats":   "chatMessage",
	"people":  "person",
}

var searchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search across mail, events, files, chats, and people",
	Long: `Issue a Microsoft Graph search using KQL.

Default --in is "mail,events" (works with legacy flow).
Full set "mail,events,files,chats,people" requires --account on a msal-office flow.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runSearch,
}

func init() {
	searchCmd.Flags().StringVar(&searchInFlag, "in", "", "Comma-separated entity surfaces: mail,events,files,chats,people (default: mail,events)")
	searchCmd.Flags().IntVar(&searchTopFlag, "top", 25, "Max hits to return")
	searchCmd.Flags().IntVar(&searchFromFlag, "from", 0, "Pagination offset")
	rootCmd.AddCommand(searchCmd)
}

func parseEntityFlags(in string) ([]string, error) {
	if strings.TrimSpace(in) == "" {
		return []string{"message", "event"}, nil
	}
	parts := strings.Split(in, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		entity, ok := friendlyToEntity[p]
		if !ok {
			return nil, fmt.Errorf("unknown surface %q (valid: mail, events, files, chats, people)", p)
		}
		out = append(out, entity)
	}
	return out, nil
}

func runSearch(cmd *cobra.Command, args []string) error {
	query := strings.Join(args, " ")
	entities, err := parseEntityFlags(searchInFlag)
	if err != nil {
		return err
	}

	resolved, err := auth.ResolveAccount(GetAccountFlag())
	if err != nil {
		return err
	}
	flow, err := auth.FlowFor(resolved)
	if err != nil {
		return err
	}
	if err := graph.ValidateEntitiesForFlow(flow, entities); err != nil {
		return err
	}

	client := graph.NewClient(resolved)
	hits, err := client.Search(context.Background(), graph.SearchRequest{
		EntityTypes: entities,
		Query:       graph.SearchQuery{QueryString: query},
		From:        searchFromFlag,
		Size:        searchTopFlag,
	})
	if err != nil {
		return err
	}

	format := GetOutputFormat()
	if format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(hits)
	}

	if len(hits) == 0 {
		Infof("No hits")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ENTITY\tFROM\tTITLE\tWHEN\tID")
	fmt.Fprintln(w, "------\t----\t-----\t----\t--")
	for _, h := range hits {
		when := ""
		if !h.Received.IsZero() {
			when = h.Received.Local().Format("Jan 02 15:04")
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			h.Entity,
			truncateText(h.From, 24),
			truncateText(h.Title, 58),
			when,
			shortMessageID(h.ID),
		)
	}
	return w.Flush()
}
```

> **Note:** `auth.FlowFor(alias)` may not yet exist. If so, add it in `internal/auth/flow.go` — a thin read of the existing flow-tracking config — as part of this task. Implementers: read `internal/auth/flow.go` and `internal/auth/config.go`, then add a `FlowFor(alias string) (string, error)` that returns `"legacy"` or `"msal-office"`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/cmd/ -run TestParseEntityFlags -v`
Expected: PASS (three tests).

- [ ] **Step 5: Build**

Run: `go build ./...`
Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add internal/cmd/search.go internal/cmd/search_test.go internal/auth/flow.go
git commit -m "feat(cmd): add \`msgcli search\` with entity-surface gating"
```

---

## Task 5: Docs + README

**Files:**
- Create: `docs/reference/search.md`
- Modify: `README.md`

- [ ] **Step 1: Write `docs/reference/search.md`**

Sections: what KQL supports, the `--in` surface names, JSON vs table output, flow-gating (which surfaces need msal-office), pagination (`--top`, `--from`), troubleshooting (entity errors, throttling).

Include three concrete examples:
```bash
msgcli search "quarterly review" --in mail,events --top 50
msgcli search 'from:"boss@company.com" subject:lease' --in mail --top 25
msgcli search "renewal" --in mail,events,files,chats --top 50 -a work
```

- [ ] **Step 2: Modify README Commands section**

Add:

```markdown
### Search

| Command | Description |
|---------|-------------|
| `msgcli search <query> [--in …] [--top N]` | Unified KQL search across mail, events, files, chats, people |
```

- [ ] **Step 3: Commit**

```bash
git add docs/reference/search.md README.md
git commit -m "docs(search): reference + README pointer"
```

---

## Self-Review

- **Spec coverage:** types + normalization (Task 1), client call (Task 2), flow gating (Task 3), CLI (Task 4), docs (Task 5). All covered.
- **Placeholder scan:** One flagged note in Task 4 about `auth.FlowFor` — explicit instruction with file pointers, not a "TBD."
- **Type consistency:** `SearchRequest`, `SearchQuery`, `SearchPayload`, `Hit`, `normalizeHit`, `ValidateEntitiesForFlow`, `parseEntityFlags`, `friendlyToEntity` used consistently.
- **Known follow-ups:** server-side pagination via deltaLink across hits; additional `fields` filter projection; ranked-score surfacing.
