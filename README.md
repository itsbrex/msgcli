# msgcli

> Agent-first CLI for Microsoft Outlook Mail & Calendar

[![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**msgcli** is a fast, script-friendly command-line interface for Microsoft Graph API. Built for AI agents and automation, inspired by [gogcli](https://github.com/steipete/gogcli).

```bash
# What's in my inbox?
$ msgcli mail list
• 📎 John Smith           RE: Project proposal              Jan 17 09:42
     ID: AAMkAGI2TG...

# Create a meeting
$ msgcli calendar create --subject "Sync" --start "2024-01-20T14:00" --end "2024-01-20T14:30" --attendees "team@company.com"
Event created successfully
ID: AAMkAGI2TG...

# Agent-friendly JSON output
$ msgcli mail list -o json | jq '.[0].subject'
"RE: Project proposal"
```

## Why msgcli?

- **Agent-first** — JSON output, `--no-input` flag, clean stdout/stderr separation
- **Multi-account** — Switch between personal and work Microsoft accounts
- **Secure** — Tokens stored in OS keychain (macOS Keychain, Windows Credential Manager, Linux Secret Service)
- **Flexible auth** — Legacy Azure app flow or macOS first-party flow
- **Full CRUD** — Mail and Calendar operations for real workflows

## Quick Start

### 1. Azure App Registration (Legacy Flow Only, 5 minutes)

1. Open [Azure Portal → App registrations](https://portal.azure.com/#blade/Microsoft_AAD_RegisteredApps/ApplicationsListBlade)
2. Click **"New registration"**
3. Configure:
   - **Name:** `msgcli`
   - **Account types:** "Accounts in any organizational directory and personal Microsoft accounts"
   - **Redirect URI:** Leave blank
4. Click **Register**
5. Copy the **Application (client) ID**
6. Go to **Authentication** → Enable **"Allow public client flows"** → Save

> 📖 Detailed guide with screenshots: [docs/SETUP.md](docs/SETUP.md)  
> If you use `msal-office`, you can skip this Azure app registration step.

### 2. Install

```bash
# Option A: Go install
go install github.com/skylarbpayne/msgcli/cmd/msgcli@latest

# Option B: Build from source
git clone https://github.com/skylarbpayne/msgcli.git
cd msgcli
make build
./bin/msgcli --help
```

### 3. Authenticate

Option A: Legacy flow (cross-platform)

```bash
# Store your client ID (one-time)
msgcli auth setup --default-flow legacy
# Enter client ID: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx

# Add an account
msgcli auth add personal --flow legacy
# To sign in, open a browser and go to:
#   https://microsoft.com/devicelogin
# Enter the code: ABCD1234
```

Option B: msal-office flow (macOS only)

```bash
# Set first-party flow as default (optional)
msgcli auth setup --default-flow msal-office

# Add account using OneAuth tenant discovery
msgcli auth add work --flow msal-office --email "you@company.com"
```

### 4. Use It

```bash
# List recent emails
msgcli mail list

# Read a specific email
msgcli mail get AAMkAGI2TG...

# Send an email
msgcli mail send --to "someone@example.com" --subject "Hello" --body "Hi there!"

# List this week's events
msgcli calendar list

# Check someone's availability
msgcli calendar availability --emails "colleague@company.com" --start "2024-01-20" --end "2024-01-21"
```

## Commands

### Authentication

| Command | Description |
|---------|-------------|
| `msgcli auth setup [--default-flow ...]` | Configure default auth flow and optional legacy client ID |
| `msgcli auth add <alias> --flow legacy|msal-office` | Add account via selected auth flow |
| `msgcli auth refresh [alias] --flow legacy|msal-office` | Force-refresh tokens for an account |
| `msgcli auth list` | List configured accounts |
| `msgcli auth status` | Show auth status and token validity |
| `msgcli auth remove <alias>` | Remove an account |

Flow notes:
- `legacy`: requires Azure app `client_id` (from config or `MSGCLI_CLIENT_ID`)
- `msal-office`: macOS-only; supports `--email` account targeting during `auth add`

### Mail

| Command | Description |
|---------|-------------|
| `msgcli mail list` | List messages (default: inbox, last 25) |
| `msgcli mail get <id>` | Read a specific message |
| `msgcli mail send` | Send a new message |
| `msgcli mail reply <id>` | Reply to a message |
| `msgcli mail delete <id> [<id>...]` | Delete one or more messages (batches >1 into one $batch call) |
| `msgcli mail move <id> [<id>...]` | Move one or more messages to another folder (batches >1 into one $batch call) |
| `msgcli mail folders` | List mail folders |

```bash
# Examples
msgcli mail list --folder sentitems --limit 10
msgcli mail list --query "from:boss@company.com"
msgcli mail send --to "a@b.com" --cc "c@d.com" --subject "Update" --body "Here's the update..."
msgcli mail reply AAMkAGI2TG... --body "Thanks!" --all
```

### Calendar

| Command | Description |
|---------|-------------|
| `msgcli calendar list` | List events (default: next 7 days) |
| `msgcli calendar get <id>` | Get event details |
| `msgcli calendar create` | Create a new event |
| `msgcli calendar update <id>` | Update an event |
| `msgcli calendar delete <id>` | Delete an event |
| `msgcli calendar respond <id>` | Accept/decline/tentative |
| `msgcli calendar availability` | Check free/busy for users |

```bash
# Examples
msgcli calendar list --start 2024-01-15 --end 2024-01-31
msgcli calendar create --subject "1:1" --start "2024-01-20T10:00" --end "2024-01-20T10:30" --location "Zoom"
msgcli calendar respond AAMkAGI2TG... --response accept --comment "See you there!"
msgcli calendar delete AAMkAGI2TG... --cancel --comment "Rescheduling"
```

### MCP Server

| Command | Description |
|---------|-------------|
| `msgcli mcp serve` | Run as an MCP server over stdio (launched by MCP clients) |
| `msgcli mcp install --client claude-code` | Register msgcli into Claude Code's MCP config |

Exposes all mail, calendar, and auth operations as MCP tools so Claude Code,
Claude Desktop, and Cursor can invoke them with keychain-backed auth.

See [docs/reference/mcp.md](docs/reference/mcp.md) for the full tool catalog.

### Batch

| Command | Description |
|---------|-------------|
| `msgcli batch [--file <path>]` | Run multiple Graph requests in one $batch call (JSONL in, JSON out) |

See [docs/reference/batch.md](docs/reference/batch.md) for the full JSONL schema and examples.

### Global Flags

```bash
-a, --account <alias>    # Use specific account (default: first configured)
-o, --output json|table  # Output format (default: auto-detect)
    --no-input           # Never prompt, fail if input needed
```

## For AI Agents

msgcli is designed for AI agent integration:

```bash
# Guaranteed non-interactive mode
msgcli mail list --no-input -o json

# Pipe content
echo "Meeting notes attached" | msgcli mail send --to "team@co.com" --subject "Notes" --stdin --no-input

# Check exit codes
msgcli mail list -o json && echo "Success" || echo "Failed"
```

**Exit codes:**
- `0` — Success
- `1` — Error (check stderr)
- `2` — Auth required

**Output separation:**
- `stdout` — Data (JSON or table)
- `stderr` — Progress, errors, prompts

## Multi-Account

```bash
# Add multiple accounts
msgcli auth add personal --flow legacy
msgcli auth add work --flow msal-office --email "you@company.com"

# Use specific account
msgcli mail list -a personal
msgcli calendar list -a work

# Check all accounts
msgcli auth status
```

## Environment Variables

| Variable | Description |
|----------|-------------|
| `MSGCLI_CLIENT_ID` | Azure client ID (alternative to `auth setup`) |
| `MSGCLI_AUTH_FLOW` | Default auth flow override (`legacy` or `msal-office`) |
| `MSGCLI_KEYRING_PASSWORD` | Keyring password for headless/CI environments |

## Building

```bash
make build      # Build to ./bin/msgcli
make install    # Install to $GOPATH/bin
make test       # Run tests
make release    # Cross-compile for all platforms
```

## How It Works

1. **Auth**: Device code flow (no client secret needed) — you authenticate in a browser, msgcli stores refresh tokens in your OS keychain
   - **Legacy**: custom Azure app + keyring token storage
   - **msal-office**: first-party client + macOS OneAuth tenant discovery + secure token files
2. **API**: Direct Microsoft Graph API calls with automatic token refresh
3. **Output**: JSON for machines, tables for humans (auto-detected based on TTY)

## Contributing

PRs welcome! Please open an issue first to discuss major changes.

## License

MIT

---

Built with ❤️ for AI agents everywhere.
