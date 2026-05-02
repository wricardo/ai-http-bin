# AI HTTP Bin

A webhook inspection and mock HTTP endpoint service built for AI agents and developers.

Use it when you need a temporary webhook receiver, a request inspector, or a mock API endpoint for local development, integration testing, demos, or AI-agent workflows.

---

## What Is This

AI HTTP Bin gives you throwaway HTTP endpoints you can point webhooks at, send test requests to, and inspect everything that arrives. It also lets you mock HTTP responses, including dynamic behavior driven by JavaScript.

The core idea is simple: create a **token**, then send HTTP requests to `/:token`. Every request is captured for inspection, and the token controls what response the caller receives.

It runs as a single Go binary with no external service dependencies. All data lives in memory, so restarting the server clears tokens, captured requests, scripts, and global variables.

For deeper guides and reference material, see the project wiki: <https://github.com/wricardo/ai-http-bin/wiki>. 

---

## Why It Exists

AI agents that interact with external services constantly need two things:

1. **A place to receive webhooks** — to verify callbacks are being sent, check what payload a service actually delivers, or confirm an integration is wired up correctly.
2. **Mock endpoints** — to stand in for APIs that do not exist yet, simulate specific responses, or test client behavior for edge cases.

Existing tools are usually dashboard-first. AI HTTP Bin is API-first.

---

## What It Does

- **Create a webhook URL** with one GraphQL mutation.
- **Inspect captured requests** (method, headers, body, query params, form data, IP, timestamp).
- **Mock HTTP responses** by updating token defaults (status, body, content type, timeout, CORS).
- **Run JS scripts** on tokens to return dynamic responses and persist shared state.
- **Stream events in real time** with GraphQL subscription `requestReceived`.
- **Scope tokens to an agent identity** with `X-Agent-Id` (optional).

---

## Requirements

For normal use, you do **not** need Go.

- A downloaded `ai-http-bin` release binary from <https://github.com/wricardo/ai-http-bin/releases>
- `curl`
- `jq` for the copy-paste examples below

Go 1.25+ is only needed if you want to build or run the project from source.

---

## Quick Start

### 0. Install and start the server

macOS/Linux:

```bash
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64) ARCH=amd64 ;;
  arm64|aarch64) ARCH=arm64 ;;
  *) echo "unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
curl -fsSL "https://github.com/wricardo/ai-http-bin/releases/latest/download/ai-http-bin_${OS}_${ARCH}.tar.gz" \
  | tar -xz -C "$TMP"
mkdir -p "$HOME/.local/bin"
install -m 0755 "$TMP/ai-http-bin" "$HOME/.local/bin/ai-http-bin"

PORT=8082 "$HOME/.local/bin/ai-http-bin"
```

Windows PowerShell:

```powershell
$Arch = if ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq 'Arm64') { 'arm64' } else { 'amd64' }
$InstallDir = "$env:LOCALAPPDATA\Programs\ai-http-bin\bin"
$Zip = "$env:TEMP\ai-http-bin.zip"
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
Invoke-WebRequest "https://github.com/wricardo/ai-http-bin/releases/latest/download/ai-http-bin_windows_$Arch.zip" -OutFile $Zip
Expand-Archive -Force $Zip $InstallDir
$env:PORT = "8082"
& "$InstallDir\ai-http-bin.exe"
```

The server should print `AI HTTP Bin running on :8082`. You can omit `PORT` to bind to a random available local port.

### 1. Create a token

A token is your temporary webhook/mock endpoint. This command stores the token id in `$TOKEN`:

```bash
TOKEN=$(curl -s http://localhost:8082/graphql \
  -H "Content-Type: application/json" \
  -d '{"query":"mutation { createToken { id url expiresAt defaultStatus defaultContentType requestCount } }"}' \
  | jq -r '.data.createToken.id')

echo "$TOKEN"
```

### 2. Send a webhook request

```bash
curl -X POST "http://localhost:8082/$TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"event": "order.created", "id": 42}'
```

### 3. Inspect captured requests

You do not need to know GraphQL deeply to get started; the examples are meant to be copy-pasteable.

```bash
curl -s http://localhost:8082/graphql \
  -H "Content-Type: application/json" \
  -d "{\"query\":\"query { requests(tokenId: \\\"$TOKEN\\\", sorting: \\\"newest\\\") { total data { method path body headers createdAt } } }\"}" | jq
```

Tokens expire 24 hours after creation. Each token stores at most 50 requests; once full, the oldest request is dropped on each new arrival (FIFO).

---

## Mock Any Response

Update token defaults through GraphQL:

```bash
curl -s http://localhost:8082/graphql \
  -H "Content-Type: application/json" \
  -d '{"query":"mutation { updateToken(id: \"<token-id>\", defaultStatus: 201, defaultContent: \"{\\\"id\\\":1,\\\"status\\\":\\\"created\\\"}\", defaultContentType: \"application/json\") { id defaultStatus defaultContentType } }"}' | jq
```

Now requests to `/<token-id>` return your configured response.

Override status via path:

```bash
curl -o /dev/null -w "%{http_code}" http://localhost:8082/<token-id>/404
# => 404
```

An expired or unknown token returns `410 Gone`. Every webhook response includes `X-Request-Id` and `X-Token-Id`.

---

## Scripted Mock Endpoints

Set a script at token creation time:

```bash
curl -s http://localhost:8082/graphql \
  -H "Content-Type: application/json" \
  -d '{
    "query":"mutation($script:String!){ createToken(script:$script){ id url } }",
    "variables":{
      "script":"var items = JSON.parse(load(\"items\") || \"[]\"); if (request.method === \"POST\") { var b = JSON.parse(request.body || \"{}\"); items.push(b); store(\"items\", JSON.stringify(items)); respond(201, JSON.stringify(b), \"application/json\"); } else { respond(200, JSON.stringify(items), \"application/json\"); }"
    }
  }' | jq
```

Set or replace script later:

```bash
curl -s http://localhost:8082/graphql \
  -H "Content-Type: application/json" \
  -d '{
    "query":"mutation($id:ID!,$script:String!){ setScript(id:$id, script:$script) { id script } }",
    "variables":{
      "id":"<token-id>",
      "script":"respond(418, \"I am a teapot\", \"text/plain\");"
    }
  }' | jq
```

Global variables (shared across all tokens in a server instance):

```bash
# list
curl -s http://localhost:8082/graphql \
  -H "Content-Type: application/json" \
  -d '{"query":"query { globalVars { key value } }"}' | jq

# set
curl -s http://localhost:8082/graphql \
  -H "Content-Type: application/json" \
  -d '{"query":"mutation { setGlobalVar(key: \"mykey\", value: \"hello\") { key value } }"}' | jq

# delete
curl -s http://localhost:8082/graphql \
  -H "Content-Type: application/json" \
  -d '{"query":"mutation { deleteGlobalVar(key: \"mykey\") }"}' | jq
```

Scripting runtime:

| Name | Signature | Description |
|------|-----------|-------------|
| `request` | object | `method`, `path`, `body`, `query`, `headers`, `formData` |
| `respond` | `(status, body, contentType?, headers?)` | Set the HTTP response |
| `store` | `(key, value)` | Persist a value |
| `load` | `(key) -> string` | Read persisted value (`""` when missing) |
| `del` | `(key)` | Delete persisted value |
| `JSON.stringify` / `JSON.parse` | helpers | Serialize/parse JSON |

- Scripts run ES5+ JavaScript via [goja](https://github.com/dop251/goja).
- Execution is limited to **2 seconds**.
- Errors return `500` with `X-Script-Error` header.

---

## Agent Identity (`X-Agent-Id`)

No signup forms. No OAuth. Supply `X-Agent-Id` if you want ownership semantics.

```bash
curl -s http://localhost:8082/graphql \
  -H "Content-Type: application/json" \
  -H "X-Agent-Id: agent-550e8400-e29b-41d4-a716-446655440000" \
  -d '{"query":"query { tokens { id url agentId } }"}' | jq
```

Modes:

| Mode | How | What Happens |
|------|-----|--------------|
| Guest | No header | Tokens work, unowned |
| Registered | Add `X-Agent-Id` | New tokens are owned by that agent; `tokens` query is scoped |
| Claim | `claimToken(id: ...)` with header | Adopts a guest token |

Claim example:

```bash
curl -s http://localhost:8082/graphql \
  -H "Content-Type: application/json" \
  -H "X-Agent-Id: my-agent-id" \
  -d '{"query":"mutation { claimToken(id: \"<token-id>\") { id agentId } }"}' | jq
```

---

## GraphQL API

Full GraphQL at `POST /graphql`. Interactive playground at `GET /playground`.

```graphql
# Create
mutation { createToken(defaultStatus: 201, cors: true) { id url } }

# Update defaults
mutation { updateToken(id: "...", defaultContentType: "application/json", timeout: 2) { id timeout defaultContentType } }

# Set script
mutation { setScript(id: "...", script: "respond(204, \"\", \"text/plain\");") { id script } }

# Inspect
query { requests(tokenId: "...", sorting: "newest") { data { method body headers } total } }

# Globals
query { globalVars { key value } }
mutation { setGlobalVar(key: "foo", value: "bar") { key value } }
mutation { deleteGlobalVar(key: "foo") }

# Real-time
subscription { requestReceived(tokenId: "...") { request { method url body } total truncated } }
```

---

## All Endpoints

| Method | Path | Content-Type | Purpose |
|--------|------|--------------|---------|
| `GET` | `/` | `text/plain` | Compact API guide |
| `GET` | `/llms.txt` | `text/plain` | LLM-friendly guide |
| `GET` | `/ui` | `text/html` | Web UI (backed by GraphQL) |
| `POST` | `/graphql` | `application/json` | GraphQL API |
| `GET` | `/playground` | `text/html` | GraphQL playground |
| `GET` | `/health` | `application/json` | Health check (`{"status":"ok"}`) |
| `ANY` | `/:token` | configurable | Webhook receiver |
| `ANY` | `/:token/*path` | configurable | Webhook receiver with sub-path |

---

## Captured Request Fields

Every request hitting a token URL is stored with:

| Field | Example |
|-------|---------|
| `method` | `POST` |
| `url` | `/f47ac.../callback?verify=true` |
| `hostname` | `localhost:8082` |
| `path` | `/callback` |
| `headers` | `{"Content-Type": "application/json", ...}` |
| `query` | `{"verify": "true"}` |
| `body` | `{"event": "order.created"}` |
| `formData` | Parsed form fields (non-JSON requests) |
| `ip` | `127.0.0.1` |
| `userAgent` | `python-requests/2.31.0` |
| `createdAt` | `2025-01-15T10:30:00Z` |

---

## Install / Run It

### Download a release binary, no Go required

Copy-paste install for macOS/Linux:

```bash
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64) ARCH=amd64 ;;
  arm64|aarch64) ARCH=arm64 ;;
  *) echo "unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
curl -fsSL "https://github.com/wricardo/ai-http-bin/releases/latest/download/ai-http-bin_${OS}_${ARCH}.tar.gz" \
  | tar -xz -C "$TMP"
mkdir -p "$HOME/.local/bin"
install -m 0755 "$TMP/ai-http-bin" "$HOME/.local/bin/ai-http-bin"

# Run it with:
PORT=8082 "$HOME/.local/bin/ai-http-bin"
```

Copy-paste install for Windows PowerShell:

```powershell
$Arch = if ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq 'Arm64') { 'arm64' } else { 'amd64' }
$InstallDir = "$env:LOCALAPPDATA\Programs\ai-http-bin\bin"
$Zip = "$env:TEMP\ai-http-bin.zip"
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
Invoke-WebRequest "https://github.com/wricardo/ai-http-bin/releases/latest/download/ai-http-bin_windows_$Arch.zip" -OutFile $Zip
Expand-Archive -Force $Zip $InstallDir
$env:PORT = "8082"
& "$InstallDir\ai-http-bin.exe"
```

Release archives are also available manually at <https://github.com/wricardo/ai-http-bin/releases> for Linux, macOS, and Windows on amd64/arm64.

Maintainers publish these artifacts by pushing a version tag such as `v0.1.0`; GitHub Actions runs GoReleaser and attaches the archives plus `checksums.txt` to the release.

### Run from source

```bash
# From source
go run ./cmd/server

# Build and run
go build -o ai-http-bin ./cmd/server && ./ai-http-bin
```

By default it binds to a random available local port (or `:$PORT` when `PORT` is set). All data is in-memory, so restart clears state.

---

## Go SDK (GraphQL)

Go SDK path:

`github.com/wricardo/ai-http-bin/pkg/sdk`

```go
package main

import (
	"context"
	"fmt"

	"github.com/wricardo/ai-http-bin/pkg/sdk"
)

func main() {
	c := sdk.New("https://ai-http-bin.ngrok.app", sdk.WithAgentID("my-agent-id"))

	tok, _ := c.CreateToken(context.Background(), sdk.CreateTokenInput{})
	fmt.Println(tok.ID, tok.URL)

	reqs, _ := c.Requests(context.Background(), tok.ID, sdk.RequestsOptions{})
	fmt.Println("captured:", reqs.Total)
}
```

## License

MIT
