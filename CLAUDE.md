# CLAUDE.md — msgcli

Agent-first Go CLI for Microsoft Graph API (Outlook Mail & Calendar).

## Project Layout

```
cmd/msgcli/         # Cobra command definitions (auth, mail, calendar, mcp)
internal/auth/      # OAuth2 device-code flow, keyring, OneAuth (macOS), token store
internal/graph/     # Microsoft Graph API client (mail, calendar, batch)
internal/mcp/       # MCP server: protocol, registry, tools
bin/                # Build output (gitignored)
docs/               # Docs, setup guides
```

## Tech Stack

- **Go 1.25+** — no generics overuse; prefer table-driven tests
- **Cobra** for CLI commands
- **Microsoft Graph API** via `internal/graph`
- **OS keychain** (`github.com/99designs/keyring`) for token storage
- **MCP server** (`internal/mcp`) for AI agent tool-use

## Build & Test

```bash
make build      # go build -> bin/msgcli
make test       # go test -v ./...
make lint       # golangci-lint run
go run ./cmd/msgcli --help
```

## Code Conventions

- Error strings are lowercase and do not end with punctuation
- Always wrap errors with `fmt.Errorf("...: %w", err)` for context
- Prefer explicit returns over named returns
- JSON output always goes to stdout; errors/logs to stderr
- Use `--no-input` / `--output json` flags to stay agent-friendly
- Do not add unnecessary abstraction — keep commands thin, logic in `internal/`
- Table-driven tests with `t.Run` subtests

## Auth Patterns

Two flows coexist:
1. **Azure device-code flow** — `internal/auth/device_code.go` — requires client ID in `.env`
2. **macOS first-party (OneAuth)** — `internal/auth/oneauth_darwin.go` — no Azure app needed

Tokens stored via `internal/auth/token_store.go` → OS keychain.

## MCP Server

`msgcli mcp serve` exposes mail + calendar tools over stdio MCP protocol.  
`msgcli mcp install --client claude-code` auto-registers into Claude Code settings.  
Tools are registered in `internal/mcp/registry.go`.

## Things Claude Must NOT Do

- Never commit `.env` or any file containing client IDs / tokens
- Never use `os.Exit` outside of `main()`
- Never swallow errors silently — always propagate or log
- Never add `fmt.Println` debugging to production paths
- Do not modify `go.sum` manually
- Do not use `ioutil` (deprecated) — use `io` and `os` packages
- Do not break the `--output json` contract on any command

## Verification

Always verify changes with:
```bash
make build && make test
go vet ./...
```

For MCP changes, test the server with:
```bash
bin/msgcli mcp serve
```

## After Every Correction

Update this CLAUDE.md with any new rule learned so the mistake doesn't repeat.
