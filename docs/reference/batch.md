# msgcli batch

## Purpose

The `batch` command sends up to 20 Microsoft Graph requests in a single HTTP call, with optional dependency chaining via `dependsOn`. This reduces network round-trips and eases rate-limit pressure on both client and server.

## JSONL Input Schema

Each line must be valid JSON. Fields are:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | Yes | Unique request identifier within this batch |
| `method` | string | Yes | HTTP method: `GET`, `POST`, `PATCH`, or `DELETE` |
| `url` | string | Yes | Microsoft Graph API path, e.g. `/me/messages` or `/me/mailFolders?$top=5` |
| `body` | object | No | JSON body for `POST` or `PATCH` requests |
| `headers` | object | No | Per-request headers as key-value pairs (string → string) |
| `dependsOn` | array | No | Array of request IDs that must complete before this one; must reference IDs in the same 20-request chunk |

## Examples

### Example 1: Simple GET Fanout

```bash
cat <<'EOF' | msgcli batch
{"id":"1","method":"GET","url":"/me"}
{"id":"2","method":"GET","url":"/me/messages?$top=5"}
{"id":"3","method":"GET","url":"/me/mailFolders"}
EOF
```

Queries your profile, recent messages, and mail folders in a single batch call.

### Example 2: Send + Flag with Dependency

```bash
cat <<'EOF' | msgcli batch
{"id":"send","method":"POST","url":"/me/sendMail","body":{"message":{"subject":"Hi","body":{"contentType":"Text","content":"Hello"},"toRecipients":[{"emailAddress":{"address":"you@example.com"}}]},"saveToSentItems":true},"headers":{"Content-Type":"application/json"}}
{"id":"flag","method":"GET","url":"/me/messages?$top=1","dependsOn":["send"]}
EOF
```

Sends an email first, then fetches the most recent message after the send completes. The `dependsOn` array ensures the flag request waits for the send.

### Example 3: Reading from File

```bash
msgcli batch --file requests.jsonl -a work
```

Reads batch requests from `requests.jsonl` and executes them using the `work` account.

## Output Schema

Responses are returned as a JSON array in the same order as input requests. Each response object contains:

```json
{
  "id": "1",
  "status": 200,
  "headers": { "content-type": "application/json", ... },
  "body": { ... }
}
```

- `id` (string) — echoes the request ID
- `status` (number) — HTTP status code for this sub-request (not the batch call itself)
- `headers` (object, optional) — response headers if present
- `body` (object/null, optional) — parsed JSON response body

**Critical:** A 200 status on the batch HTTP call does NOT imply every sub-request succeeded. Always check each response's `status` field: valid ranges are `>= 200 && < 300` for success.

## Troubleshooting

**Cross-chunk dependencies rejected**

Graph API requires `dependsOn` IDs to reside within the same 20-request chunk. If you have more than 20 requests, split them into separate batch calls or restructure so related requests are grouped together.

**Per-sub-request 429 (Too Many Requests)**

If an individual sub-request returns HTTP 429, the response includes a `Retry-After` header (value in seconds). Respect this delay before re-issuing the batch. Consider spreading requests across time or reducing concurrency.

**Whole-batch HTTP errors (401, 403)**

A 401 or 403 at the batch HTTP level indicates authentication or session issues, not per-request failures. Re-authenticate with `msgcli auth refresh [alias]` and retry.

**Malformed JSONL**

If a line is not valid JSON, msgcli will return an error before sending the batch. Double-check JSON syntax and ensure each line is complete.
