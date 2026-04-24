# Graph API Expansion Roadmap

> **For agentic workers:** This is a ROADMAP document, not a bite-sized implementation plan. Each of the 5 features below is an independent subsystem that warrants its own per-feature plan (TDD-granular, one file per feature) before execution. Use this document to decide sequencing and scope, then use superpowers:writing-plans to produce the detailed per-feature plans.

**Goal:** Identify and sequence the 5 highest-leverage Graph API additions to msgcli, ranked by leverage × novelty × fit with the "agent-first CLI" positioning, given current auth (legacy device-code + msal-office first-party).

**Architecture:** Additive to the current Cobra command tree under `cmd/msgcli` with new packages under `internal/graph` per surface. Reuses existing keychain-backed token storage and automatic refresh. Two of the five (`batch`, `mcp serve`) are foundational infrastructure that compounds the others and should land first.

**Tech Stack:** Go 1.21+, Microsoft Graph v1.0 (and `/beta` where noted), existing device-code + msal-office flows, OS keychain (macOS Keychain / Windows Credential Manager / Linux Secret Service).

---

## Current Auth Baseline

| Flow | Client | Scopes |
|------|--------|--------|
| `legacy` | user's own Azure app | `offline_access User.Read Mail.ReadWrite Mail.Send Calendars.ReadWrite` |
| `msal-office` | first-party Office client | `https://graph.microsoft.com/.default offline_access` + `https://outlook.office365.com/.default` — inherits Office's broad consent surface (Teams, Files, People, Search, etc.) |

Implications:
- Any scope added to `legacy` triggers user re-consent on next refresh — acceptable but must be documented.
- `msal-office` scopes are already broad; gating features on flow is the cleaner path for Teams/Files/Chat.
- Refresh tokens are per-resource on first-party; new resources (e.g. SharePoint) need a new `exchangeFirstPartyRefreshTokenForScope` call pattern like the existing `outlookScopes`.

---

## Feature 1: `msgcli search` — Unified Graph Search (KQL)

**Endpoint:** `POST /search/query` (v1.0 for mail/event/person; `/beta` for chat/driveItem cross-surface consistency)

**Value:** One KQL query returns ranked hits across mail + events + files + chats + people in a single call. Turns "grep my inbox" into "grep everything I've ever seen about X."

**Auth fit:**
- Works today on `legacy` for `message` and `event` entity types (existing Mail/Calendar scopes).
- `driveItem`, `chatMessage`, `person` entity types require `msal-office` flow (no new scope registration needed on user side).
- Add `People.Read` to legacy scope string if we want person results cross-flow.

**Command surface (proposed):**
```
msgcli search "quarterly review" --in mail,events,files --top 25 -o json
msgcli search --kql "from:boss AND received>=2026-01-01" --in mail
msgcli search --entity chatMessage "lease renewal" -a work  # msal-office only
```

**Key tasks (outline for the per-feature plan):**
1. `internal/graph/search` package: `Query`, `EntityType`, `Hit` types + `Client.Search(ctx, req)`.
2. KQL passthrough + convenience flags (`--from`, `--since`, `--subject`).
3. Entity-type gating by auth flow (fail fast with actionable error when legacy tries `driveItem`).
4. Result normalization across entity types into a single `Hit` shape for agent JSON.
5. `cmd/msgcli/search.go` + table/JSON renderers.
6. Tests: httptest fake for `/search/query`, entity-type gating, KQL escaping.

**Tradeoffs:**
- Ranking is Microsoft-controlled and opaque; no user tunables.
- `/search/query` has its own throttling separate from `/me/messages`.
- Pagination uses `from`/`size`, not `@odata.nextLink` — slight shape difference from rest of CLI.

**Estimated size:** Medium (1 package, 1 command, ~3-4 days with tests).

---

## Feature 2: `msgcli watch` — Delta Queries as a JSONL Event Stream

**Endpoints:** `/me/mailFolders/{id}/messages/delta`, `/me/calendar/events/delta`, `/me/contacts/delta`

**Value:** Converts the CLI into an event source. No webhook infrastructure, no daemon to write — just `msgcli watch mail --since-token state.json | jq … | msgcli mail reply`. Agents compose change-driven workflows as shell pipelines.

**Auth fit:** No new scopes. Delta endpoints respect existing Mail.ReadWrite / Calendars.ReadWrite consent.

**Command surface (proposed):**
```
msgcli watch mail --folder inbox --interval 30s --state ~/.msgcli/watch-mail.json
msgcli watch events --window 7d --state ~/.msgcli/watch-events.json
msgcli watch mail --once --since-token @state.json  # one poll, print diff, update token
```

Output is NDJSON: one line per change, each with `{op: "created|updated|deleted", type, id, resource}`.

**Key tasks (outline):**
1. `internal/graph/delta` package: token persistence, `@odata.deltaLink` rotation, `@odata.nextLink` pagination.
2. Polling loop with context cancellation + jittered backoff on 429.
3. State file schema (per account, per resource) with atomic writes.
4. `--once` mode for cron-driven agents; streaming mode for long-lived pipelines.
5. `cmd/msgcli/watch.go` with subcommands `mail`, `events`, `contacts`.
6. Tests: fake Graph server emitting delta pages + deltaLink transitions; state file round-trip; 429 handling.

**Tradeoffs:**
- Polling latency floor ~30s; for true push we'd need a webhook tunnel (separate, larger project).
- Delta tokens expire (typically ~30 days); expiry path must re-baseline cleanly.
- Deleted items surface as tombstones — renderer must handle them distinctly.

**Estimated size:** Medium (1 package, 1 command group, ~4-5 days with tests).

---

## Feature 3: `msgcli batch` — JSON Batch Requests with Dependency Chains

**Endpoint:** `POST /$batch` (up to 20 requests per call, supports `dependsOn`)

**Value:** Foundational infrastructure — 20× throughput, fewer round-trips, atomic-ish multi-step workflows ("archive all from X, flag newest, create follow-up event") in one call. Every existing and future command benefits; rate-limit resilience improves dramatically.

**Auth fit:** No new scopes. Each sub-request inherits the bearer token's existing scopes.

**Command surface (proposed):**
```
# Pipeline mode: newline-delimited JSON requests on stdin
cat ops.jsonl | msgcli batch --max-parallel 20 -o json

# Declarative: a batch file with dependsOn
msgcli batch run ops.yaml

# Internal: expose as a Go API for other commands to use under the hood
# (e.g. `msgcli mail archive --from x` becomes a batch of moves)
```

**Key tasks (outline):**
1. `internal/graph/batch` package: `Request`, `Response`, `Client.Batch(ctx, reqs)` with 20-item chunking.
2. `dependsOn` graph validator (no cycles, topologically sortable, within 20-item window).
3. Per-request retry policy distinguishing batch-level 429 from sub-request 429.
4. Refactor 1-2 existing commands (e.g. `mail move` multi-id, `mail delete` multi-id) to use batch internally as proof-of-integration.
5. `cmd/msgcli/batch.go` for explicit user-facing batch mode.
6. Tests: chunking, dependency ordering, partial-failure propagation, 429 replay.

**Tradeoffs:**
- Partial failures make error semantics harder (some sub-requests succeed, some fail).
- `dependsOn` is limited to sequential chains — no true parallel DAGs within a single batch.
- User-facing batch mode is power-user territory; value is mostly in internal adoption.

**Estimated size:** Medium-Large (1 package + refactor of 2-3 commands, ~5-6 days with tests).

**Sequencing note:** Build this FIRST. Features 1, 2, and 5 all benefit from having batch available when they ship (e.g., `watch` can batch-fetch bodies for N new messages in one call).

---

## Feature 4: `msgcli mcp serve` — Expose msgcli as an MCP Server

**Protocol:** Model Context Protocol (stdio transport primary; optional SSE later)

**Value:** Makes the "agent-first" positioning real. Claude Code / Claude Desktop / Cursor get authenticated Graph access via msgcli's keychain-backed tokens — no secrets in client configs, no OAuth app each host has to register, multi-account switching becomes a tool argument. Reuses 100% of existing command logic.

**Auth fit:** Reuses existing token storage verbatim. MCP tool calls run as the invoking user; account selection via `account` tool parameter mirrors the `-a` flag.

**Tool surface (proposed):**
Each top-level CLI verb maps to one MCP tool:
- `mail_list`, `mail_get`, `mail_send`, `mail_reply`, `mail_move`, `mail_delete`, `mail_folders`
- `calendar_list`, `calendar_get`, `calendar_create`, `calendar_update`, `calendar_delete`, `calendar_respond`, `calendar_availability`
- `auth_list`, `auth_status`
- (After feature 1): `search`
- (After feature 2): `watch_mail_once`, `watch_events_once`

**Key tasks (outline):**
1. Pick Go MCP SDK (evaluate `mark3labs/mcp-go` vs rolling thin stdio handler).
2. `internal/mcp` package: tool registry, request/response marshaling, error mapping from Graph errors to MCP errors.
3. Each command exposes a pure `Run(ctx, params) (result, error)` entry point so both CLI and MCP call the same code path (refactor where needed).
4. `cmd/msgcli/mcp.go` — `msgcli mcp serve` subcommand with stdio transport.
5. Config generation helper: `msgcli mcp install --client claude-code` writes the right JSON into `~/.claude/settings.json` (or equivalent).
6. Tests: MCP handshake, tool call round-trip, error surface, account-arg propagation.

**Tradeoffs:**
- Introduces an MCP SDK dependency (new maintenance surface).
- Long-running process mode is a philosophical departure from one-shot CLI — must be optional, not the default.
- Some commands (interactive `auth add`) don't map cleanly to MCP and must be excluded or reshaped.

**Estimated size:** Medium-Large (~5-7 days with tests and the config-install helper).

---

## Feature 5: `msgcli people` — Relationship Graph via `/me/people`

**Endpoints:** `/me/people`, `/me/people?$search=`, `/me/contacts`, aggregations over `/me/messages`

**Value:** Microsoft returns AI-ranked people (by communication frequency, recency, co-attendance) merging AAD, Outlook contacts, and mail-derived contacts. For SDR/CRM-adjacent workflows this is the killer feature: "who do I talk to most about X, when did we last connect, who's gone cold?"

**Auth fit:**
- `msal-office`: already has `People.Read` via first-party consent.
- `legacy`: needs `People.Read` added to the scope string in `internal/auth/device_code.go:22` — triggers user re-consent on next refresh.

**Command surface (proposed):**
```
msgcli people list --top 25                     # ranked by Microsoft's relevance score
msgcli people search "sarah"                    # fuzzy name/email search
msgcli people get <id>                          # full person record
msgcli people stats <email>                     # derived: last contact, msg count, cadence
msgcli people cold --days 60                    # people you used to talk to but haven't lately
```

**Key tasks (outline):**
1. `internal/graph/people` package: `Person` type, `Client.ListPeople`, `Client.SearchPeople`, `Client.GetPerson`.
2. `internal/graph/stats` package: aggregate `/me/messages?$filter=from/emailAddress/address eq '…'` for cadence metrics (uses batch from Feature 3 if present).
3. Scope string update + changelog entry documenting re-consent for legacy users.
4. `cmd/msgcli/people.go` with subcommands `list`, `search`, `get`, `stats`, `cold`.
5. Tests: fake Graph server, relevance-score round-trip, cadence calculation edge cases (zero contact, only sent, only received).

**Tradeoffs:**
- Legacy users must re-consent on first refresh after upgrade (one-time friction).
- Microsoft's relevance score is opaque; `cold` detection is our heuristic, not a Graph primitive.
- `/me/people` is read-only — can't mutate the graph, only observe it.

**Estimated size:** Medium (~4-5 days with tests).

**Alternative #5:** `msgcli teams` — chat list/send via `/me/chats` and `/me/chats/{id}/messages`. Narrower (msal-office only, requires `Chat.ReadWrite`) but very compelling for work accounts. Suggest as Feature 6 / Feature 5-B.

---

## Recommended Sequencing

1. **Feature 3 — `batch`** (foundation; everything else benefits)
2. **Feature 4 — `mcp serve`** (foundation; makes agent-first real)
3. **Feature 1 — `search`** (highest end-user "wow" factor, reuses batch for hit hydration)
4. **Feature 2 — `watch`** (completes the event-driven story, pairs with MCP for reactive agents)
5. **Feature 5 — `people`** (domain value layer on top of the infra)

Rationale: the two infra features land first because their value compounds. Search before watch because search is synchronous and simpler to ship; watch needs stateful design decisions (where state files live, multi-account isolation) that benefit from the earlier features' patterns.

---

## Self-Review

This is a roadmap, not a TDD-bite-sized plan, so the usual "no placeholders" and "exact code in every step" checks don't apply here — they apply to the per-feature plans spawned from this document. What this roadmap commits to:

- Every feature names its Graph endpoint(s), the auth flows it works on, and the scope deltas required.
- Every feature lists concrete command surface sketches (not "TBD").
- Every feature lists a task outline with file/package boundaries (enough to spawn a real plan from).
- Sequencing is justified with dependencies, not vibes.

**Next step:** When ready to execute, run superpowers:writing-plans once per feature to produce `docs/superpowers/plans/2026-MM-DD-msgcli-<feature>.md` with full TDD-granular steps. Start with `batch`.
