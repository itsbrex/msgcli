# MCP Server Reference

## What is it?

The Model Context Protocol (MCP) is an open standard that allows AI clients to call server-provided tools. msgcli exposes its mail, calendar, and auth operations as MCP tools via two commands: `msgcli mcp serve` (runs the server) and `msgcli mcp install` (registers it with Claude Code or other clients).

## Quick Setup

1. **Configure authentication** (if not already done):
   ```bash
   msgcli auth add personal --flow legacy
   # or
   msgcli auth add work --flow msal-office --email "you@company.com"
   ```
   See [docs/SETUP.md](../SETUP.md) for detailed steps.

2. **Register with Claude Code**:
   ```bash
   msgcli mcp install --client claude-code
   ```
   This adds an `mcpServers.msgcli` entry to `~/.claude/settings.json`.

3. **Verify** (optional):
   ```bash
   echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | msgcli mcp serve | jq '.result.tools | length'
   # → 16
   ```
   Restart Claude Code; tools like `mail_list` are now available.

## Tool Catalog

| Tool | Description | Key Args |
|------|-------------|----------|
| mail_list | List email messages from a folder (default: inbox, last 25) | account, folder, limit, query |
| mail_get | Get a single email message by ID | account, id |
| mail_send | Send an email message | account, to, subject, body, cc |
| mail_reply | Reply to an email message | account, id, comment, replyAll |
| mail_move | Move an email message to a different folder | account, id, folder |
| mail_delete | Delete an email message | account, id |
| mail_folders | List mail folders | account |
| calendar_list | List calendar events within an optional time range | account, start, end, limit |
| calendar_get | Get a single calendar event by ID | account, id |
| calendar_create | Create a new calendar event | account, subject, start, end, attendees, location, body |
| calendar_update | Update an existing calendar event | account, id, updates |
| calendar_delete | Delete a calendar event | account, id |
| calendar_respond | Respond to a calendar event invitation | account, id, response, comment |
| calendar_availability | Get free/busy schedule information for a set of email addresses | account, emails, start, end |
| auth_list | List all configured accounts (alias, email, flow) | (none) |
| auth_status | Show auth configuration and per-account token validity | account |

## Security Notes

- **Tokens stay local**: Refresh tokens are stored in your OS keychain (macOS Keychain, Windows Credential Manager, Linux Secret Service). msgcli never transmits them over the network.
- **Account selection**: The `account` argument maps to a configured alias in your keychain. MCP callers choose which account to act as.
- **Network traffic**: Only msgcli-to-Graph communication (HTTPS) leaves the machine. The MCP stdio channel between Claude Code and msgcli is local.
- **Interactive flows not exposed**: Device-code prompts (required for `auth add`) are terminal-only. Use `msgcli auth add` from a shell—it is not available as an MCP tool.

## Troubleshooting

**Tools not appearing in Claude Code?**
- Check `~/.claude/settings.json` for an `mcpServers.msgcli` entry.
- If missing or malformed, run `msgcli mcp install --client claude-code` again.
- Restart Claude Code completely.

**Test the server manually:**
```bash
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | msgcli mcp serve
```
Should output JSON with `result.tools` array. If you see an error, it is in the response body.

**Tool call fails or returns an error?**
Errors appear in-band in the MCP response with `isError: true` and error text in the `content` field. Check the message for details (e.g., "account not found", "message not found").

**Tokens expired or invalid?**
Run `msgcli auth refresh <alias>` from a terminal to force a token refresh. This does not revoke existing tokens; it refreshes them with Microsoft's servers.
