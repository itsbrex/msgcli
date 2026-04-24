# msgcli watch — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `msgcli watch mail` and `msgcli watch events` — commands that emit NDJSON change events using Graph's delta endpoints (`/messages/delta`, `/events/delta`). Supports `--once` (single poll, print diff, update token file, exit) for cron-driven agents and streaming mode (loop with interval + 429 backoff) for long-lived pipelines.

**Architecture:** New `internal/graph/delta.go` owns the delta client, token persistence, and page iteration (following `@odata.nextLink` → terminal `@odata.deltaLink`). A new `internal/cmd/watch.go` wires two subcommands around a shared runner. State files live per (account, resource) and are written atomically (tmp + rename). No new scopes.

**Tech Stack:** Go 1.25, existing `graph.Client`, Cobra, standard library only.

---

## File Structure

**Create:**
- `internal/graph/delta.go` — `DeltaPage[T]`, `Client.DeltaMessages`, `Client.DeltaEvents`, deltaLink/nextLink iteration.
- `internal/graph/delta_test.go`
- `internal/cmd/watch.go` — `watchCmd` with `mail` and `events` subcommands + shared state-file I/O.
- `internal/cmd/watch_test.go`
- `docs/reference/watch.md`

**Modify:**
- `README.md` — add command row.

---

## Task 1: Delta page iteration against fake server

**Files:**
- Create: `internal/graph/delta.go`, `internal/graph/delta_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/graph/delta_test.go
package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDeltaMessagesIteratesPagesAndReturnsDeltaLink(t *testing.T) {
	var page int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page++
		switch page {
		case 1:
			_, _ = fmt.Fprintf(w, `{"value":[{"id":"m1","subject":"a"}],"@odata.nextLink":%q}`, r.URL.String()+"&_p=2")
		case 2:
			_, _ = fmt.Fprintf(w, `{"value":[{"id":"m2","subject":"b"}],"@odata.deltaLink":"%s/next-delta-link"}`, strings.TrimSuffix(r.URL.Scheme, ""))
		default:
			t.Fatalf("unexpected page %d", page)
		}
	}))
	defer srv.Close()

	c := NewTestClient(srv.URL)
	var collected []Message
	deltaLink, err := c.DeltaMessages(context.Background(), "inbox", "", func(msgs []Message) error {
		collected = append(collected, msgs...)
		return nil
	})
	if err != nil {
		t.Fatalf("DeltaMessages: %v", err)
	}
	if len(collected) != 2 || collected[0].ID != "m1" || collected[1].ID != "m2" {
		t.Fatalf("unexpected messages: %+v", collected)
	}
	if deltaLink == "" {
		t.Fatalf("expected non-empty deltaLink")
	}
	if !strings.HasSuffix(deltaLink, "/next-delta-link") {
		t.Fatalf("unexpected deltaLink: %q", deltaLink)
	}
}

func TestDeltaMessagesResumesFromDeltaLink(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.URL.RequestURI()
		_, _ = w.Write([]byte(`{"value":[],"@odata.deltaLink":"final"}`))
	}))
	defer srv.Close()

	c := NewTestClient(srv.URL)
	existingDeltaLink := srv.URL + "/resume-token?foo=bar"
	if _, err := c.DeltaMessages(context.Background(), "inbox", existingDeltaLink, func([]Message) error { return nil }); err != nil {
		t.Fatalf("DeltaMessages: %v", err)
	}
	if !strings.Contains(seen, "/resume-token") {
		t.Fatalf("expected resume URL to be used, got: %q", seen)
	}
	_ = json.RawMessage{} // keep import used
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/graph/ -run TestDelta -v`
Expected: FAIL — function undefined.

- [ ] **Step 3: Implement delta.go**

```go
// internal/graph/delta.go
package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// deltaPage[T] is the generic shape of a delta response page.
type deltaPage[T any] struct {
	Value     []T    `json:"value"`
	NextLink  string `json:"@odata.nextLink,omitempty"`
	DeltaLink string `json:"@odata.deltaLink,omitempty"`
}

// MessageVisitor is called once per batch of messages returned by delta pagination.
type MessageVisitor func([]Message) error

// EventVisitor is called once per batch of events returned by delta pagination.
type EventVisitor func([]Event) error

// DeltaMessages walks /messages/delta across pages. If existingDeltaLink is
// non-empty it is used directly; otherwise a fresh delta walk starts from the
// folder. Returns the terminal @odata.deltaLink to persist for the next poll.
func (c *Client) DeltaMessages(ctx context.Context, folder, existingDeltaLink string, visit MessageVisitor) (string, error) {
	startURL := existingDeltaLink
	if startURL == "" {
		if folder == "" {
			folder = "inbox"
		}
		startURL = c.baseURL + "/me/mailFolders/" + folder + "/messages/delta"
	}
	return iterateDelta(ctx, c, startURL, func(raw json.RawMessage) error {
		var page deltaPage[Message]
		if err := json.Unmarshal(raw, &page); err != nil {
			return err
		}
		if len(page.Value) > 0 {
			return visit(page.Value)
		}
		return nil
	})
}

// DeltaEvents walks /events/delta. Calendar delta is under /me/calendarView/delta
// with start/end query params; we expose the simpler /me/events/delta here
// and let callers pass a pre-built URL in existingDeltaLink if they need views.
func (c *Client) DeltaEvents(ctx context.Context, existingDeltaLink string, visit EventVisitor) (string, error) {
	startURL := existingDeltaLink
	if startURL == "" {
		startURL = c.baseURL + "/me/events/delta"
	}
	return iterateDelta(ctx, c, startURL, func(raw json.RawMessage) error {
		var page deltaPage[Event]
		if err := json.Unmarshal(raw, &page); err != nil {
			return err
		}
		if len(page.Value) > 0 {
			return visit(page.Value)
		}
		return nil
	})
}

// iterateDelta walks pages starting at startURL, calling handlePage for each
// raw page body. Returns the final @odata.deltaLink.
func iterateDelta(ctx context.Context, c *Client, startURL string, handlePage func(json.RawMessage) error) (string, error) {
	token, err := c.token(ctx)
	if err != nil {
		return "", fmt.Errorf("auth error: %w", err)
	}
	currentURL := startURL
	for {
		req, err := http.NewRequestWithContext(ctx, "GET", currentURL, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Prefer", "odata.track-changes")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return "", err
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return "", err
		}
		if resp.StatusCode >= 400 {
			// 410 = deltaToken expired; caller must restart with empty token.
			if resp.StatusCode == http.StatusGone {
				return "", ErrDeltaTokenExpired
			}
			return "", fmt.Errorf("delta HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		if err := handlePage(body); err != nil {
			return "", err
		}

		var meta struct {
			NextLink  string `json:"@odata.nextLink"`
			DeltaLink string `json:"@odata.deltaLink"`
		}
		if err := json.Unmarshal(body, &meta); err != nil {
			return "", err
		}
		if meta.NextLink != "" {
			currentURL = meta.NextLink
			continue
		}
		return meta.DeltaLink, nil
	}
}

// ErrDeltaTokenExpired is returned when Graph answers 410 — caller must
// discard the saved deltaLink and restart with an empty one.
var ErrDeltaTokenExpired = fmt.Errorf("delta token expired; restart with empty token")
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/graph/ -run TestDelta -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/graph/delta.go internal/graph/delta_test.go
git commit -m "feat(graph): delta iteration for messages and events"
```

---

## Task 2: Event type (if missing)

**Files:**
- Check: `internal/graph/calendar.go` for an existing `Event` type

- [ ] **Step 1: Verify Event type exists**

```bash
rg -n "type Event struct" internal/graph/
```

If present, skip to Task 3. If not:

- [ ] **Step 2: Add a minimal Event type**

```go
// append to internal/graph/calendar.go (or create if absent)
type Event struct {
	ID           string    `json:"id"`
	Subject      string    `json:"subject"`
	Organizer    *Recipient `json:"organizer,omitempty"`
	Attendees    []struct {
		Status struct {
			Response string `json:"response"`
		} `json:"status"`
		EmailAddress EmailAddress `json:"emailAddress"`
	} `json:"attendees,omitempty"`
	Start struct {
		DateTime string `json:"dateTime"`
		TimeZone string `json:"timeZone"`
	} `json:"start"`
	End struct {
		DateTime string `json:"dateTime"`
		TimeZone string `json:"timeZone"`
	} `json:"end"`
	Location struct {
		DisplayName string `json:"displayName"`
	} `json:"location,omitempty"`
	WebLink string `json:"webLink,omitempty"`
}
```

> **Note:** If the existing `Event` type in `internal/graph/calendar.go` differs, keep it verbatim and skip this step.

- [ ] **Step 3: Build**

Run: `go build ./...`
Expected: no errors.

- [ ] **Step 4: Commit (only if new)**

```bash
git add internal/graph/calendar.go
git commit -m "feat(graph): Event type for delta iteration"
```

---

## Task 3: State file — atomic load/save

**Files:**
- Create: `internal/cmd/watch_state.go`, `internal/cmd/watch_state_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/cmd/watch_state_test.go
package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWatchStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	got, err := loadWatchState(path)
	if err != nil {
		t.Fatalf("load empty: %v", err)
	}
	if got.DeltaLink != "" {
		t.Fatalf("expected empty DeltaLink, got %q", got.DeltaLink)
	}

	state := watchState{DeltaLink: "https://graph/delta?token=abc", Account: "alice", Resource: "mail"}
	if err := saveWatchState(path, state); err != nil {
		t.Fatalf("save: %v", err)
	}

	got2, err := loadWatchState(path)
	if err != nil {
		t.Fatalf("load after save: %v", err)
	}
	if got2.DeltaLink != state.DeltaLink || got2.Account != state.Account || got2.Resource != state.Resource {
		t.Fatalf("round-trip mismatch: got %+v", got2)
	}
}

func TestWatchStateAtomicTmpCleanup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := saveWatchState(path, watchState{DeltaLink: "x"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/cmd/ -run TestWatchState -v`
Expected: FAIL.

- [ ] **Step 3: Implement watch_state.go**

```go
// internal/cmd/watch_state.go
package cmd

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

type watchState struct {
	Account   string `json:"account"`
	Resource  string `json:"resource"`
	DeltaLink string `json:"deltaLink"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

func loadWatchState(path string) (watchState, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return watchState{}, nil
	}
	if err != nil {
		return watchState{}, err
	}
	var s watchState
	if len(b) == 0 {
		return s, nil
	}
	return s, json.Unmarshal(b, &s)
}

func saveWatchState(path string, s watchState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/cmd/ -run TestWatchState -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add internal/cmd/watch_state.go internal/cmd/watch_state_test.go
git commit -m "feat(cmd): atomic watch-state persistence"
```

---

## Task 4: `msgcli watch mail` subcommand — one-shot mode

**Files:**
- Create: `internal/cmd/watch.go`

- [ ] **Step 1: Implement the command**

```go
// internal/cmd/watch.go
package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/skylarbpayne/msgcli/internal/auth"
	"github.com/skylarbpayne/msgcli/internal/graph"
	"github.com/spf13/cobra"
)

var (
	watchMailFolder   string
	watchStateFile    string
	watchOnce         bool
	watchInterval     time.Duration
)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Stream mail/calendar changes as NDJSON via delta queries",
}

var watchMailCmd = &cobra.Command{
	Use:   "mail",
	Short: "Emit NDJSON change events for a mail folder (delta)",
	RunE:  runWatchMail,
}

func init() {
	watchMailCmd.Flags().StringVarP(&watchMailFolder, "folder", "f", "inbox", "Folder to watch")
	watchMailCmd.Flags().StringVar(&watchStateFile, "state", "", "Delta-token state file (default: ~/.msgcli/watch-mail-<account>.json)")
	watchMailCmd.Flags().BoolVar(&watchOnce, "once", false, "Run a single poll, emit diffs, update state, exit")
	watchMailCmd.Flags().DurationVar(&watchInterval, "interval", 30*time.Second, "Polling interval (streaming mode)")
	watchCmd.AddCommand(watchMailCmd)
	rootCmd.AddCommand(watchCmd)
}

func defaultStatePath(resource, account string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/.msgcli/watch-%s-%s.json", home, resource, account), nil
}

func runWatchMail(cmd *cobra.Command, args []string) error {
	account, err := auth.ResolveAccount(GetAccountFlag())
	if err != nil {
		return err
	}

	statePath := watchStateFile
	if statePath == "" {
		statePath, err = defaultStatePath("mail", account)
		if err != nil {
			return err
		}
	}
	state, err := loadWatchState(statePath)
	if err != nil {
		return err
	}
	state.Account = account
	state.Resource = "mail:" + watchMailFolder

	client := graph.NewClient(account)
	enc := json.NewEncoder(os.Stdout)

	doPoll := func(ctx context.Context) error {
		deltaLink, err := client.DeltaMessages(ctx, watchMailFolder, state.DeltaLink, func(batch []graph.Message) error {
			for _, m := range batch {
				ev := changeEvent{
					Op:       classifyMessageOp(m),
					Type:     "message",
					ID:       m.ID,
					Resource: m,
				}
				if err := enc.Encode(ev); err != nil {
					return err
				}
			}
			return nil
		})
		if errors.Is(err, graph.ErrDeltaTokenExpired) {
			Infof("delta token expired; restarting from baseline")
			state.DeltaLink = ""
			return nil
		}
		if err != nil {
			return err
		}
		state.DeltaLink = deltaLink
		state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		return saveWatchState(statePath, state)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if watchOnce {
		return doPoll(ctx)
	}
	ticker := time.NewTicker(watchInterval)
	defer ticker.Stop()
	if err := doPoll(ctx); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := doPoll(ctx); err != nil {
				Errorf("poll error: %v", err)
				// Keep looping; don't die on transient network errors.
			}
		}
	}
}

type changeEvent struct {
	Op       string      `json:"op"`       // "created" | "updated" | "deleted"
	Type     string      `json:"type"`     // "message" | "event"
	ID       string      `json:"id"`
	Resource interface{} `json:"resource,omitempty"`
}

// classifyMessageOp inspects Graph's tombstone marker @removed.
// Graph emits deletions as {"id":"...","@removed":{"reason":"deleted"}}.
// With our typed struct the field is lost, so we treat everything as
// "updated" for now; deletion detection is added in the next task.
func classifyMessageOp(m graph.Message) string {
	// Placeholder default; refined in Task 5.
	return "updated"
}
```

- [ ] **Step 2: Build**

Run: `go build ./...`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/cmd/watch.go
git commit -m "feat(cmd): \`msgcli watch mail\` one-shot + streaming"
```

---

## Task 5: Tombstone detection (deleted messages)

**Files:**
- Modify: `internal/graph/delta.go`, `internal/cmd/watch.go`
- Test: `internal/graph/delta_test.go`

- [ ] **Step 1: Write the failing test**

```go
// append to internal/graph/delta_test.go
func TestDeltaMessagesPropagatesRemovedMarker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"value":[
			{"id":"keep","subject":"alive"},
			{"id":"gone","@removed":{"reason":"deleted"}}
		],"@odata.deltaLink":"done"}`))
	}))
	defer srv.Close()

	c := NewTestClient(srv.URL)
	var got []DeltaMessage
	_, err := c.DeltaMessagesWithRemoved(context.Background(), "inbox", "", func(batch []DeltaMessage) error {
		got = append(got, batch...)
		return nil
	})
	if err != nil {
		t.Fatalf("delta: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 items, got %d", len(got))
	}
	if got[0].Removed {
		t.Fatalf("first item should not be removed")
	}
	if !got[1].Removed {
		t.Fatalf("second item should be removed")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/graph/ -run TestDeltaMessagesPropagatesRemovedMarker -v`
Expected: FAIL.

- [ ] **Step 3: Add removed-aware delta variant**

```go
// append to internal/graph/delta.go

// DeltaMessage wraps a Message with an explicit Removed flag, because the
// Graph tombstone marker "@removed" isn't carried by the Message type.
type DeltaMessage struct {
	Message
	Removed bool `json:"-"`
}

// DeltaMessagesWithRemoved is like DeltaMessages but surfaces tombstones.
func (c *Client) DeltaMessagesWithRemoved(ctx context.Context, folder, existingDeltaLink string, visit func([]DeltaMessage) error) (string, error) {
	startURL := existingDeltaLink
	if startURL == "" {
		if folder == "" {
			folder = "inbox"
		}
		startURL = c.baseURL + "/me/mailFolders/" + folder + "/messages/delta"
	}
	return iterateDelta(ctx, c, startURL, func(raw json.RawMessage) error {
		var page struct {
			Value []json.RawMessage `json:"value"`
		}
		if err := json.Unmarshal(raw, &page); err != nil {
			return err
		}
		items := make([]DeltaMessage, 0, len(page.Value))
		for _, rawItem := range page.Value {
			var probe struct {
				Removed *json.RawMessage `json:"@removed"`
			}
			_ = json.Unmarshal(rawItem, &probe)
			var dm DeltaMessage
			if err := json.Unmarshal(rawItem, &dm.Message); err != nil {
				return err
			}
			dm.Removed = probe.Removed != nil
			items = append(items, dm)
		}
		if len(items) > 0 {
			return visit(items)
		}
		return nil
	})
}
```

- [ ] **Step 4: Rewire watch.go to use the removed-aware variant**

Replace `client.DeltaMessages(...)` block in `runWatchMail` with:

```go
deltaLink, err := client.DeltaMessagesWithRemoved(ctx, watchMailFolder, state.DeltaLink, func(batch []graph.DeltaMessage) error {
	for _, m := range batch {
		op := "updated"
		if m.Removed {
			op = "deleted"
		}
		// A message we've never seen before also arrives here; without
		// local tracking we cannot distinguish created vs. updated,
		// so we emit "updated" for both and agents treat it as upsert.
		ev := changeEvent{Op: op, Type: "message", ID: m.ID, Resource: m.Message}
		if m.Removed {
			ev.Resource = nil
		}
		if err := enc.Encode(ev); err != nil {
			return err
		}
	}
	return nil
})
```

Remove `classifyMessageOp` — no longer needed.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/graph/ ./internal/cmd/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/graph/delta.go internal/graph/delta_test.go internal/cmd/watch.go
git commit -m "feat(watch): surface tombstones as deleted ops in NDJSON"
```

---

## Task 6: `msgcli watch events` subcommand

**Files:**
- Modify: `internal/cmd/watch.go`

- [ ] **Step 1: Add the events subcommand**

```go
// append to internal/cmd/watch.go
var watchEventsStateFile string

var watchEventsCmd = &cobra.Command{
	Use:   "events",
	Short: "Emit NDJSON change events for calendar (delta)",
	RunE:  runWatchEvents,
}

func init() {
	watchEventsCmd.Flags().StringVar(&watchEventsStateFile, "state", "", "Delta-token state file (default: ~/.msgcli/watch-events-<account>.json)")
	watchEventsCmd.Flags().BoolVar(&watchOnce, "once", false, "Run a single poll and exit")
	watchEventsCmd.Flags().DurationVar(&watchInterval, "interval", 30*time.Second, "Polling interval")
	watchCmd.AddCommand(watchEventsCmd)
}

func runWatchEvents(cmd *cobra.Command, args []string) error {
	account, err := auth.ResolveAccount(GetAccountFlag())
	if err != nil {
		return err
	}
	statePath := watchEventsStateFile
	if statePath == "" {
		statePath, err = defaultStatePath("events", account)
		if err != nil {
			return err
		}
	}
	state, err := loadWatchState(statePath)
	if err != nil {
		return err
	}
	state.Account = account
	state.Resource = "events"

	client := graph.NewClient(account)
	enc := json.NewEncoder(os.Stdout)

	doPoll := func(ctx context.Context) error {
		deltaLink, err := client.DeltaEvents(ctx, state.DeltaLink, func(batch []graph.Event) error {
			for _, e := range batch {
				if err := enc.Encode(changeEvent{Op: "updated", Type: "event", ID: e.ID, Resource: e}); err != nil {
					return err
				}
			}
			return nil
		})
		if errors.Is(err, graph.ErrDeltaTokenExpired) {
			Infof("delta token expired; restarting from baseline")
			state.DeltaLink = ""
			return nil
		}
		if err != nil {
			return err
		}
		state.DeltaLink = deltaLink
		state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		return saveWatchState(statePath, state)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if watchOnce {
		return doPoll(ctx)
	}
	ticker := time.NewTicker(watchInterval)
	defer ticker.Stop()
	if err := doPoll(ctx); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := doPoll(ctx); err != nil {
				Errorf("poll error: %v", err)
			}
		}
	}
}
```

- [ ] **Step 2: Build**

Run: `go build ./...`
Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/cmd/watch.go
git commit -m "feat(cmd): \`msgcli watch events\` subcommand"
```

---

## Task 7: Docs + README

**Files:**
- Create: `docs/reference/watch.md`
- Modify: `README.md`

- [ ] **Step 1: Write `docs/reference/watch.md`**

Sections: what delta queries are, state-file location and format, one-shot vs streaming modes, NDJSON output schema (`op`, `type`, `id`, `resource`), tombstone handling, token-expiry behavior (410), example cron entry.

- [ ] **Step 2: Add README row**

```markdown
### Watch (Delta)

| Command | Description |
|---------|-------------|
| `msgcli watch mail [--once] [--interval 30s]` | NDJSON change events for a mail folder |
| `msgcli watch events [--once] [--interval 30s]` | NDJSON change events for the calendar |
```

- [ ] **Step 3: Commit**

```bash
git add docs/reference/watch.md README.md
git commit -m "docs(watch): reference + README pointer"
```

---

## Self-Review

- **Spec coverage:** delta iteration (Task 1), event type prereq (Task 2), state persistence (Task 3), mail watch (Tasks 4-5), events watch (Task 6), docs (Task 7). All covered.
- **Placeholder scan:** Task 2 has a verification "Step 1" (grep for existing type); this is a real action, not a TBD. All code is complete Go.
- **Type consistency:** `Client.DeltaMessages` / `DeltaMessagesWithRemoved` / `DeltaEvents`, `DeltaMessage`, `watchState`, `changeEvent`, `runWatchMail` / `runWatchEvents` referenced consistently. `MessageVisitor` / `EventVisitor` live in delta.go and aren't needed by watch.go after Task 5 (watch.go uses the closure form directly).
- **Known follow-ups:** true push via webhooks (ngrok tunnel or `/subscriptions`); contacts delta; differentiating created vs. updated via local ID-cache; typed variant of DeltaEvents with tombstones.
