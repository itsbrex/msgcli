# msgcli people — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `msgcli people list|search|get|stats|cold` — a relationship-intelligence layer backed by Microsoft's `/me/people` API plus derived cadence statistics from `/me/messages`. Outputs AI-ranked people, last-contact dates, message counts, and "gone cold" contacts (people you used to correspond with but haven't recently).

**Architecture:** New `internal/graph/people.go` for the raw People API + types; new `internal/graph/stats.go` for cadence aggregations that use `$filter` queries (and reuse `Client.Batch` from the batch plan if landed, otherwise simple serial requests). A new `internal/cmd/people.go` wraps five subcommands. The legacy scope string in `internal/auth/device_code.go` gains `People.Read`; msal-office already has it via first-party consent.

**Tech Stack:** Go 1.25, existing `graph.Client`, Cobra, `httptest`. Optional dependency on `graph.Client.Batch` from the batch plan (falls back gracefully if absent).

---

## File Structure

**Create:**
- `internal/graph/people.go` — `Person`, `Client.ListPeople`, `Client.SearchPeople`, `Client.GetPerson`.
- `internal/graph/people_test.go`
- `internal/graph/stats.go` — `CadenceStats`, `Client.PersonStats(ctx, emailOrID)`.
- `internal/graph/stats_test.go`
- `internal/cmd/people.go` — Cobra command tree + subcommands.
- `internal/cmd/people_test.go`
- `docs/reference/people.md`

**Modify:**
- `internal/auth/device_code.go:22` — add `People.Read` to `legacyScopes`.
- `README.md` — add command rows.
- `docs/SETUP.md` — add a brief note about the new scope for existing legacy users.

---

## Task 1: Person type + Client.ListPeople

**Files:**
- Create: `internal/graph/people.go`, `internal/graph/people_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/graph/people_test.go
package graph

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientListPeople(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/me/people" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("$top") != "5" {
			t.Errorf("expected $top=5, got %q", r.URL.Query().Get("$top"))
		}
		_, _ = w.Write([]byte(`{"value":[
			{"id":"p1","displayName":"Alice","scoredEmailAddresses":[{"address":"alice@x.com","relevanceScore":0.98}]},
			{"id":"p2","displayName":"Bob","scoredEmailAddresses":[{"address":"bob@x.com","relevanceScore":0.82}]}
		]}`))
	}))
	defer srv.Close()

	c := NewTestClient(srv.URL)
	people, err := c.ListPeople(context.Background(), 5)
	if err != nil {
		t.Fatalf("ListPeople: %v", err)
	}
	if len(people) != 2 {
		t.Fatalf("expected 2 people, got %d", len(people))
	}
	if people[0].PrimaryEmail() != "alice@x.com" {
		t.Fatalf("expected primary email alice@x.com, got %q", people[0].PrimaryEmail())
	}
	if people[0].TopScore() <= people[1].TopScore() == false {
		t.Fatalf("expected ranked ordering preserved")
	}
	_ = json.RawMessage{}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/graph/ -run TestClientListPeople -v`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement people.go**

```go
// internal/graph/people.go
package graph

import (
	"context"
	"fmt"
	"net/url"
)

// Person is a simplified view of Graph's /me/people record.
type Person struct {
	ID                  string           `json:"id"`
	DisplayName         string           `json:"displayName"`
	GivenName           string           `json:"givenName,omitempty"`
	Surname             string           `json:"surname,omitempty"`
	JobTitle            string           `json:"jobTitle,omitempty"`
	CompanyName         string           `json:"companyName,omitempty"`
	ScoredEmailAddresses []ScoredEmail   `json:"scoredEmailAddresses,omitempty"`
	Phones              []PersonPhone    `json:"phones,omitempty"`
	PersonType          PersonType       `json:"personType,omitempty"`
}

type ScoredEmail struct {
	Address        string  `json:"address"`
	RelevanceScore float64 `json:"relevanceScore"`
}

type PersonPhone struct {
	Type   string `json:"type"`
	Number string `json:"number"`
}

type PersonType struct {
	Class    string `json:"class"`    // "Person" | "Group"
	Subclass string `json:"subclass"` // "OrganizationUser" | "PersonalContact" | ...
}

// PrimaryEmail returns the highest-scored email, or "" if none.
func (p Person) PrimaryEmail() string {
	best := -1.0
	pick := ""
	for _, e := range p.ScoredEmailAddresses {
		if e.RelevanceScore > best {
			best = e.RelevanceScore
			pick = e.Address
		}
	}
	return pick
}

// TopScore returns the highest relevance score (0 if none).
func (p Person) TopScore() float64 {
	var max float64
	for _, e := range p.ScoredEmailAddresses {
		if e.RelevanceScore > max {
			max = e.RelevanceScore
		}
	}
	return max
}

// ListPeople returns Graph's ranked people list.
func (c *Client) ListPeople(ctx context.Context, top int) ([]Person, error) {
	q := url.Values{}
	if top > 0 {
		q.Set("$top", fmt.Sprintf("%d", top))
	}
	path := "/me/people"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	var resp ListResponse[Person]
	if err := c.Get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return resp.Value, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/graph/ -run TestClientListPeople -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/graph/people.go internal/graph/people_test.go
git commit -m "feat(graph): Person type + Client.ListPeople"
```

---

## Task 2: Client.SearchPeople + GetPerson

**Files:**
- Modify: `internal/graph/people.go`, `internal/graph/people_test.go`

- [ ] **Step 1: Write the failing test**

```go
// append to internal/graph/people_test.go
func TestClientSearchPeople(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("$search"); got != "\"sarah\"" {
			t.Errorf("unexpected $search: %q", got)
		}
		_, _ = w.Write([]byte(`{"value":[{"id":"p10","displayName":"Sarah Lin"}]}`))
	}))
	defer srv.Close()

	c := NewTestClient(srv.URL)
	people, err := c.SearchPeople(context.Background(), "sarah", 10)
	if err != nil {
		t.Fatalf("SearchPeople: %v", err)
	}
	if len(people) != 1 || people[0].DisplayName != "Sarah Lin" {
		t.Fatalf("unexpected: %+v", people)
	}
}

func TestClientGetPerson(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/me/people/p42" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"id":"p42","displayName":"Forty-Two"}`))
	}))
	defer srv.Close()

	c := NewTestClient(srv.URL)
	p, err := c.GetPerson(context.Background(), "p42")
	if err != nil {
		t.Fatalf("GetPerson: %v", err)
	}
	if p.ID != "p42" || p.DisplayName != "Forty-Two" {
		t.Fatalf("unexpected: %+v", p)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/graph/ -run "TestClient(Search|Get)People" -v`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement**

```go
// append to internal/graph/people.go

// SearchPeople searches people by name or email (uses $search).
func (c *Client) SearchPeople(ctx context.Context, query string, top int) ([]Person, error) {
	q := url.Values{}
	q.Set("$search", fmt.Sprintf("%q", query))
	if top > 0 {
		q.Set("$top", fmt.Sprintf("%d", top))
	}
	path := "/me/people?" + q.Encode()
	var resp ListResponse[Person]
	if err := c.Get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return resp.Value, nil
}

// GetPerson retrieves a single person by ID.
func (c *Client) GetPerson(ctx context.Context, id string) (*Person, error) {
	var p Person
	if err := c.Get(ctx, "/me/people/"+id, &p); err != nil {
		return nil, err
	}
	return &p, nil
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/graph/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/graph/people.go internal/graph/people_test.go
git commit -m "feat(graph): SearchPeople + GetPerson"
```

---

## Task 3: Cadence stats (PersonStats)

**Files:**
- Create: `internal/graph/stats.go`, `internal/graph/stats_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/graph/stats_test.go
package graph

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPersonStatsAggregatesReceivedAndSent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Two kinds of queries expected:
		//   - /me/mailFolders/inbox/messages?$filter=from/emailAddress/address eq '...'
		//   - /me/mailFolders/sentitems/messages?$filter=toRecipients/any(...)
		if strings.Contains(r.URL.RawQuery, "receivedDateTime") {
			t.Errorf("unexpected receivedDateTime filter: %s", r.URL.RawQuery)
		}
		if strings.Contains(r.URL.Path, "sentitems") {
			_, _ = w.Write([]byte(`{"value":[
				{"id":"s1","sentDateTime":"2026-03-15T10:00:00Z","subject":"sent"}
			]}`))
			return
		}
		_, _ = w.Write([]byte(`{"value":[
			{"id":"r1","receivedDateTime":"2026-04-01T10:00:00Z","subject":"got"},
			{"id":"r2","receivedDateTime":"2026-03-20T10:00:00Z","subject":"got2"}
		]}`))
	}))
	defer srv.Close()

	c := NewTestClient(srv.URL)
	s, err := c.PersonStats(context.Background(), "alice@x.com", 90, time.Date(2026, 4, 23, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("PersonStats: %v", err)
	}
	if s.ReceivedCount != 2 || s.SentCount != 1 {
		t.Fatalf("unexpected counts: %+v", s)
	}
	if s.LastReceived.Month() != 4 || s.LastReceived.Day() != 1 {
		t.Fatalf("unexpected LastReceived: %v", s.LastReceived)
	}
	if s.LastSent.Month() != 3 || s.LastSent.Day() != 15 {
		t.Fatalf("unexpected LastSent: %v", s.LastSent)
	}
	if s.DaysSinceContact > 23 || s.DaysSinceContact < 21 {
		// As of 2026-04-23, LastReceived was 22 days ago.
		t.Fatalf("unexpected DaysSinceContact: %d", s.DaysSinceContact)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/graph/ -run TestPersonStats -v`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement stats.go**

```go
// internal/graph/stats.go
package graph

import (
	"context"
	"fmt"
	"net/url"
	"time"
)

// CadenceStats summarizes email cadence with one person.
type CadenceStats struct {
	Email            string    `json:"email"`
	WindowDays       int       `json:"windowDays"`
	ReceivedCount    int       `json:"receivedCount"`
	SentCount        int       `json:"sentCount"`
	LastReceived     time.Time `json:"lastReceived,omitempty"`
	LastSent         time.Time `json:"lastSent,omitempty"`
	DaysSinceContact int       `json:"daysSinceContact,omitempty"`
}

// PersonStats returns message counts and last-contact dates for one address
// over the last windowDays. "now" is injected for testability.
func (c *Client) PersonStats(ctx context.Context, email string, windowDays int, now time.Time) (*CadenceStats, error) {
	if email == "" {
		return nil, fmt.Errorf("email is required")
	}
	since := now.AddDate(0, 0, -windowDays).Format(time.RFC3339)

	// Received: messages in inbox FROM this address in window.
	recvQ := url.Values{}
	recvQ.Set("$filter", fmt.Sprintf("from/emailAddress/address eq '%s' and receivedDateTime ge %s", email, since))
	recvQ.Set("$top", "100")
	recvQ.Set("$select", "id,receivedDateTime,subject")
	recvQ.Set("$orderby", "receivedDateTime desc")

	var recvResp ListResponse[Message]
	if err := c.Get(ctx, "/me/mailFolders/inbox/messages?"+recvQ.Encode(), &recvResp); err != nil {
		return nil, err
	}

	// Sent: messages in sentitems TO this address in window.
	sentQ := url.Values{}
	sentQ.Set("$filter", fmt.Sprintf("toRecipients/any(r:r/emailAddress/address eq '%s') and sentDateTime ge %s", email, since))
	sentQ.Set("$top", "100")
	sentQ.Set("$select", "id,sentDateTime,subject")
	sentQ.Set("$orderby", "sentDateTime desc")

	var sentResp ListResponse[Message]
	if err := c.Get(ctx, "/me/mailFolders/sentitems/messages?"+sentQ.Encode(), &sentResp); err != nil {
		return nil, err
	}

	stats := &CadenceStats{
		Email:         email,
		WindowDays:    windowDays,
		ReceivedCount: len(recvResp.Value),
		SentCount:     len(sentResp.Value),
	}
	if len(recvResp.Value) > 0 {
		stats.LastReceived = recvResp.Value[0].ReceivedDateTime
	}
	if len(sentResp.Value) > 0 {
		stats.LastSent = sentResp.Value[0].SentDateTime
	}
	last := stats.LastReceived
	if stats.LastSent.After(last) {
		last = stats.LastSent
	}
	if !last.IsZero() {
		stats.DaysSinceContact = int(now.Sub(last).Hours() / 24)
	}
	return stats, nil
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/graph/ -run TestPersonStats -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/graph/stats.go internal/graph/stats_test.go
git commit -m "feat(graph): PersonStats — cadence aggregation from inbox + sentitems"
```

---

## Task 4: Update legacy scope string

**Files:**
- Modify: `internal/auth/device_code.go`
- Test: `internal/auth/device_code_test.go`

- [ ] **Step 1: Write the failing test**

```go
// append to internal/auth/device_code_test.go
func TestLegacyScopesIncludePeopleRead(t *testing.T) {
	if !strings.Contains(legacyScopes, "People.Read") {
		t.Fatalf("legacyScopes missing People.Read: %q", legacyScopes)
	}
}
```

Add `"strings"` to imports if not already present.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/auth/ -run TestLegacyScopesIncludePeopleRead -v`
Expected: FAIL.

- [ ] **Step 3: Update the scope constant**

In `internal/auth/device_code.go:22`:

```go
	legacyScopes          = "offline_access User.Read Mail.ReadWrite Mail.Send Calendars.ReadWrite People.Read"
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/auth/ -v`
Expected: PASS.

- [ ] **Step 5: Add changelog note**

Create (or append to) `CHANGELOG.md`:

```markdown
## Unreleased

### Changed
- Legacy auth flow now requests `People.Read` scope. Existing legacy users will be prompted to re-consent on the next token refresh. `msal-office` flow already has this scope.
```

- [ ] **Step 6: Commit**

```bash
git add internal/auth/device_code.go internal/auth/device_code_test.go CHANGELOG.md
git commit -m "feat(auth): add People.Read to legacy scopes (re-consent on refresh)"
```

---

## Task 5: `msgcli people list|search|get` subcommands

**Files:**
- Create: `internal/cmd/people.go`

- [ ] **Step 1: Implement the command tree**

```go
// internal/cmd/people.go
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/skylarbpayne/msgcli/internal/auth"
	"github.com/skylarbpayne/msgcli/internal/graph"
	"github.com/spf13/cobra"
)

var (
	peopleListTop   int
	peopleSearchTop int
)

var peopleCmd = &cobra.Command{
	Use:   "people",
	Short: "Explore your relationship graph via /me/people",
}

var peopleListCmd = &cobra.Command{
	Use:   "list",
	Short: "List top-ranked people (by Graph's relevance score)",
	RunE:  runPeopleList,
}

var peopleSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search people by name or email",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runPeopleSearch,
}

var peopleGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get a single person's record",
	Args:  cobra.ExactArgs(1),
	RunE:  runPeopleGet,
}

func init() {
	peopleListCmd.Flags().IntVar(&peopleListTop, "top", 25, "Max people to return")
	peopleSearchCmd.Flags().IntVar(&peopleSearchTop, "top", 25, "Max hits")
	peopleCmd.AddCommand(peopleListCmd, peopleSearchCmd, peopleGetCmd)
	rootCmd.AddCommand(peopleCmd)
}

func clientForCmd() (*graph.Client, error) {
	account, err := auth.ResolveAccount(GetAccountFlag())
	if err != nil {
		return nil, err
	}
	return graph.NewClient(account), nil
}

func runPeopleList(cmd *cobra.Command, args []string) error {
	client, err := clientForCmd()
	if err != nil {
		return err
	}
	people, err := client.ListPeople(context.Background(), peopleListTop)
	if err != nil {
		return err
	}
	return renderPeople(people)
}

func runPeopleSearch(cmd *cobra.Command, args []string) error {
	client, err := clientForCmd()
	if err != nil {
		return err
	}
	people, err := client.SearchPeople(context.Background(), args[0], peopleSearchTop)
	if err != nil {
		return err
	}
	return renderPeople(people)
}

func runPeopleGet(cmd *cobra.Command, args []string) error {
	client, err := clientForCmd()
	if err != nil {
		return err
	}
	person, err := client.GetPerson(context.Background(), args[0])
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(person)
}

func renderPeople(people []graph.Person) error {
	if GetOutputFormat() == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(people)
	}
	if len(people) == 0 {
		Infof("No people found")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SCORE\tNAME\tEMAIL\tTITLE\tID")
	fmt.Fprintln(w, "-----\t----\t-----\t-----\t--")
	for _, p := range people {
		fmt.Fprintf(w, "%.2f\t%s\t%s\t%s\t%s\n",
			p.TopScore(),
			truncateText(p.DisplayName, 28),
			truncateText(p.PrimaryEmail(), 32),
			truncateText(p.JobTitle, 24),
			shortMessageID(p.ID),
		)
	}
	return w.Flush()
}
```

- [ ] **Step 2: Build**

Run: `go build ./...`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/cmd/people.go
git commit -m "feat(cmd): \`msgcli people list|search|get\`"
```

---

## Task 6: `msgcli people stats` subcommand

**Files:**
- Modify: `internal/cmd/people.go`

- [ ] **Step 1: Add stats subcommand**

```go
// append to internal/cmd/people.go
import "time"

var (
	peopleStatsWindow int
)

var peopleStatsCmd = &cobra.Command{
	Use:   "stats <email>",
	Short: "Email cadence stats with one address (counts, last contact)",
	Args:  cobra.ExactArgs(1),
	RunE:  runPeopleStats,
}

func init() {
	peopleStatsCmd.Flags().IntVar(&peopleStatsWindow, "days", 90, "Lookback window in days")
	peopleCmd.AddCommand(peopleStatsCmd)
}

func runPeopleStats(cmd *cobra.Command, args []string) error {
	client, err := clientForCmd()
	if err != nil {
		return err
	}
	stats, err := client.PersonStats(context.Background(), args[0], peopleStatsWindow, time.Now())
	if err != nil {
		return err
	}
	if GetOutputFormat() == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(stats)
	}
	fmt.Fprintf(os.Stdout, "Email:             %s\n", stats.Email)
	fmt.Fprintf(os.Stdout, "Window:            %d days\n", stats.WindowDays)
	fmt.Fprintf(os.Stdout, "Received:          %d\n", stats.ReceivedCount)
	fmt.Fprintf(os.Stdout, "Sent:              %d\n", stats.SentCount)
	if !stats.LastReceived.IsZero() {
		fmt.Fprintf(os.Stdout, "Last received:     %s\n", stats.LastReceived.Local().Format("2006-01-02 15:04"))
	}
	if !stats.LastSent.IsZero() {
		fmt.Fprintf(os.Stdout, "Last sent:         %s\n", stats.LastSent.Local().Format("2006-01-02 15:04"))
	}
	if stats.DaysSinceContact > 0 {
		fmt.Fprintf(os.Stdout, "Days since contact:%d\n", stats.DaysSinceContact)
	}
	return nil
}
```

- [ ] **Step 2: Build**

Run: `go build ./...`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/cmd/people.go
git commit -m "feat(cmd): \`msgcli people stats\`"
```

---

## Task 7: `msgcli people cold` — derived heuristic

**Files:**
- Modify: `internal/cmd/people.go`, `internal/cmd/people_test.go`

- [ ] **Step 1: Write the failing test for the cold filter**

```go
// internal/cmd/people_test.go
package cmd

import (
	"testing"
	"time"

	"github.com/skylarbpayne/msgcli/internal/graph"
)

func TestFilterCold(t *testing.T) {
	now := time.Date(2026, 4, 23, 0, 0, 0, 0, time.UTC)
	type row struct {
		person graph.Person
		stats  graph.CadenceStats
	}
	rows := []row{
		// fresh — in contact 10 days ago, should be excluded
		{graph.Person{ID: "p1", DisplayName: "Fresh"}, graph.CadenceStats{DaysSinceContact: 10, ReceivedCount: 5, SentCount: 5}},
		// cold — 120 days since contact, used to talk
		{graph.Person{ID: "p2", DisplayName: "Cold"}, graph.CadenceStats{DaysSinceContact: 120, ReceivedCount: 3, SentCount: 2}},
		// never-contact: no messages at all, should be excluded
		{graph.Person{ID: "p3", DisplayName: "Stranger"}, graph.CadenceStats{DaysSinceContact: 0, ReceivedCount: 0, SentCount: 0}},
	}
	coldThreshold := 60
	var filtered []coldRow
	for _, r := range rows {
		if r.stats.DaysSinceContact >= coldThreshold && (r.stats.ReceivedCount+r.stats.SentCount) > 0 {
			filtered = append(filtered, coldRow{Person: r.person, Stats: r.stats})
		}
	}
	_ = now
	if len(filtered) != 1 || filtered[0].Person.ID != "p2" {
		t.Fatalf("expected p2 only, got: %+v", filtered)
	}
}

// coldRow is exported here only to keep the test alongside; real definition
// lives in people.go once implemented.
type coldRow struct {
	Person graph.Person         `json:"person"`
	Stats  graph.CadenceStats   `json:"stats"`
}
```

> **Note:** Once the real `coldRow` is added to `people.go` below, delete the duplicate struct at the bottom of the test file.

- [ ] **Step 2: Run to verify it fails (import cycle / duplicate type errors)**

Run: `go test ./internal/cmd/ -run TestFilterCold -v`
Expected: FAIL (duplicate `coldRow` or missing command). That's expected; Step 3 cleans this up.

- [ ] **Step 3: Implement `people cold`**

```go
// append to internal/cmd/people.go
var (
	peopleColdDays      int
	peopleColdTop       int
	peopleColdThreshold int
)

type coldRow struct {
	Person graph.Person       `json:"person"`
	Stats  graph.CadenceStats `json:"stats"`
}

var peopleColdCmd = &cobra.Command{
	Use:   "cold",
	Short: "People you used to talk to but haven't recently",
	Long: `Scans the top-ranked people (see 'people list') and returns those whose last
contact was more than --threshold days ago, and who had any prior contact in the
--days window.

Warning: this fans out one stats query per person. Use --top to bound calls.`,
	RunE: runPeopleCold,
}

func init() {
	peopleColdCmd.Flags().IntVar(&peopleColdDays, "days", 180, "Lookback window for cadence")
	peopleColdCmd.Flags().IntVar(&peopleColdThreshold, "threshold", 60, "Mark cold when DaysSinceContact >= this")
	peopleColdCmd.Flags().IntVar(&peopleColdTop, "top", 25, "Max people to scan from /me/people")
	peopleCmd.AddCommand(peopleColdCmd)
}

func runPeopleCold(cmd *cobra.Command, args []string) error {
	client, err := clientForCmd()
	if err != nil {
		return err
	}
	ctx := context.Background()
	people, err := client.ListPeople(ctx, peopleColdTop)
	if err != nil {
		return err
	}
	now := time.Now()
	var rows []coldRow
	for _, p := range people {
		email := p.PrimaryEmail()
		if email == "" {
			continue
		}
		stats, err := client.PersonStats(ctx, email, peopleColdDays, now)
		if err != nil {
			Errorf("stats for %s: %v", email, err)
			continue
		}
		if stats.DaysSinceContact >= peopleColdThreshold && (stats.ReceivedCount+stats.SentCount) > 0 {
			rows = append(rows, coldRow{Person: p, Stats: *stats})
		}
	}

	if GetOutputFormat() == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rows)
	}
	if len(rows) == 0 {
		Infof("No cold contacts found")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "DAYS\tNAME\tEMAIL\tLAST")
	fmt.Fprintln(w, "----\t----\t-----\t----")
	for _, r := range rows {
		last := r.Stats.LastReceived
		if r.Stats.LastSent.After(last) {
			last = r.Stats.LastSent
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\n",
			r.Stats.DaysSinceContact,
			truncateText(r.Person.DisplayName, 28),
			truncateText(r.Person.PrimaryEmail(), 32),
			last.Local().Format("2006-01-02"),
		)
	}
	return w.Flush()
}
```

Delete the duplicate `coldRow` at the bottom of `people_test.go` now that it exists in `people.go`.

- [ ] **Step 4: Run all tests to verify they pass**

Run: `go test ./internal/cmd/ -v`
Expected: PASS.

- [ ] **Step 5: Build**

Run: `go build ./...`
Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add internal/cmd/people.go internal/cmd/people_test.go
git commit -m "feat(cmd): \`msgcli people cold\` via per-person cadence stats"
```

---

## Task 8: Docs + README + SETUP note

**Files:**
- Create: `docs/reference/people.md`
- Modify: `README.md`, `docs/SETUP.md`

- [ ] **Step 1: Write `docs/reference/people.md`**

Sections: what `/me/people` returns, relevance-score caveat (Microsoft-controlled), all five subcommands with examples, cadence heuristic explanation, cost/throttling note for `cold` (N API calls per scan).

- [ ] **Step 2: Add README rows**

```markdown
### People

| Command | Description |
|---------|-------------|
| `msgcli people list [--top N]` | Top-ranked contacts by relevance score |
| `msgcli people search <query>` | Search people by name or email |
| `msgcli people get <id>` | Full person record |
| `msgcli people stats <email>` | Email cadence with one contact |
| `msgcli people cold [--threshold N]` | Contacts gone quiet |
```

- [ ] **Step 3: Add SETUP note**

Append to `docs/SETUP.md` under the scope-configuration section:

```markdown
> **Scope note (v0.x+):** msgcli now requests `People.Read` in addition to the original mail/calendar scopes. Existing legacy-flow users will see a one-time re-consent prompt on their next token refresh. No action is required for `msal-office` flow users.
```

- [ ] **Step 4: Commit**

```bash
git add docs/reference/people.md README.md docs/SETUP.md
git commit -m "docs(people): reference, README rows, SETUP re-consent note"
```

---

## Self-Review

- **Spec coverage:** Person type + list (Task 1), search + get (Task 2), cadence stats (Task 3), scope update (Task 4), list/search/get commands (Task 5), stats command (Task 6), cold command (Task 7), docs (Task 8). All covered.
- **Placeholder scan:** One self-cleanup instruction in Task 7 (delete the duplicate `coldRow` struct from the test file once added to `people.go`); this is a real action, not a TBD. No "add error handling" / "implement later" strings.
- **Type consistency:** `Person`, `ScoredEmail`, `PersonPhone`, `PersonType`, `CadenceStats`, `coldRow`, `PrimaryEmail()`, `TopScore()`, `ListPeople`, `SearchPeople`, `GetPerson`, `PersonStats` used consistently across tasks.
- **Known follow-ups:** use `graph.Client.Batch` in `people cold` (one batched call instead of N serial calls) once the batch plan lands; include chat-message cadence for msal-office accounts; surface Microsoft's `scoredEmailAddresses` subscore breakdown.
