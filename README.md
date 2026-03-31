# AI HTTP Bin

A webhook inspection and mock HTTP endpoint service built for AI agents.

---

## What Is This

AI HTTP Bin gives you throwaway HTTP endpoints you can point webhooks at, send test requests to, and inspect everything that arrives. It also lets you mock any HTTP response — including dynamic ones driven by JavaScript.

It runs as a single Go binary with no external dependencies. All data lives in memory.

---

## Why It Exists

AI agents that interact with external services constantly need two things:

1. **A place to receive webhooks** — to verify callbacks are being sent, check what payload a service actually delivers, or confirm an integration is wired up correctly.
2. **Mock endpoints** — to stand in for APIs that don't exist yet, simulate specific responses, or test how a client handles edge cases like 429s or malformed JSON.

Existing tools (mockbin.io, requestbin) are built for humans clicking around dashboards. This is built for agents making HTTP calls.

---

## What It Does

- **Create a webhook URL** with a single POST. Any request sent to that URL is captured and stored.
- **Inspect captured requests** — method, headers, body, query params, form data, IP, timestamp — everything.
- **Mock any HTTP response** — set the status code, body, and content type on the token. Override status via path (`/token/404`).
- **Write JS scripts** on tokens to return dynamic responses, route by method, and maintain state across requests.
- **Real-time notifications** via WebSocket subscription — know the instant a request arrives.
- **Scope tokens to an agent identity** with a header — no signup, no OAuth, just bring a UUID.

---

## How It Helps

- `GET /` returns a markdown API spec. An agent reads one URL and can fully operate the service — no separate docs, no OpenAPI file to hunt for.
- No signup. No API keys. An agent picks a UUID and sends it as `X-Agent-Id`. That's the whole auth flow.
- Tokens are anonymous by default. Start without any identity and claim tokens later with no data loss.
- Both REST and GraphQL, so agents can use whichever fits their style.

---

## Quick Start

```bash
go run ./cmd/server
# => AI HTTP Bin running on :8082
```

### Create a webhook URL

```bash
curl -s -X POST http://localhost:8082/api/tokens | jq
```

```json
{
  "id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
  "url": "http://localhost:8082/f47ac10b-58cc-4372-a567-0e02b2c3d479",
  "expires_at": "2026-01-02T12:00:00Z",
  "default_status": 200,
  "default_content_type": "text/plain",
  "request_count": 0
}
```

Tokens expire 24 hours after creation. Each token stores at most 50 requests; once full, the oldest is dropped on each new arrival (FIFO).

### Send something to it

```bash
curl -X POST http://localhost:8082/f47ac10b-... \
  -H "Content-Type: application/json" \
  -d '{"event": "order.created", "id": 42}'
```

### See what arrived

```bash
curl -s http://localhost:8082/api/tokens/f47ac10b-.../requests | jq '.data[0].body'
# => "{\"event\": \"order.created\", \"id\": 42}"
```

---

## Mock Any Response

Create an endpoint that returns exactly what you need:

```bash
curl -s -X POST http://localhost:8082/api/tokens \
  -H "Content-Type: application/json" \
  -d '{
    "default_status": 201,
    "default_content": "{\"id\": 1, \"status\": \"created\"}",
    "default_content_type": "application/json"
  }' | jq '.url'
# => "http://localhost:8082/abc123..."

curl -s http://localhost:8082/abc123... | jq
# => {"id": 1, "status": "created"}
```

Override the status code via path:

```bash
curl -o /dev/null -w "%{http_code}" http://localhost:8082/abc123.../404
# => 404
```

Enable CORS or simulate latency at creation time:

```bash
curl -s -X POST http://localhost:8082/api/tokens \
  -H "Content-Type: application/json" \
  -d '{"cors": true, "timeout": 2}'
```

An expired or unknown token returns `410 Gone`. Every webhook response includes `X-Request-Id` and `X-Token-Id` headers.

---

## Scripted Mock Endpoints

Tokens can run a JS script on every request, giving you dynamic responses and persistent state across calls — enough to build a full stateful mock API on a single token.

```bash
curl -s -X POST http://localhost:8082/api/tokens \
  -H "Content-Type: application/json" \
  -d '{
    "script": "var items = JSON.parse(load(\"items\") || \"[]\"); if (request.method === \"POST\") { var b = JSON.parse(request.body || \"{}\"); items.push(b); store(\"items\", JSON.stringify(items)); respond(201, JSON.stringify(b), \"application/json\"); } else { respond(200, JSON.stringify(items), \"application/json\"); }"
  }' | jq '.url'
```

**Scripting API** (available inside every script):

| Name | Signature | Description |
|------|-----------|-------------|
| `request` | object | `method`, `path`, `body`, `query`, `headers`, `formData` |
| `respond` | `(status, body, contentType?, headers?)` | Set the HTTP response |
| `store` | `(key, value)` | Persist a value across requests |
| `load` | `(key) → string` | Read a persisted value (`""` if missing) |
| `del` | `(key)` | Delete a persisted value |
| `JSON.stringify` / `JSON.parse` | helpers | Serialize/parse JSON |

- Scripts run ES5+ JavaScript via [goja](https://github.com/dop251/goja) — no Node, no CGO.
- Execution is limited to **2 seconds**. Errors return `500` with an `X-Script-Error` header.
- Global variables are shared across all tokens in the same server instance.

### Set a script after token creation

```bash
curl -s -X PUT http://localhost:8082/api/tokens/<token-id>/script \
  -H "Content-Type: application/json" \
  -d '{"script": "respond(418, \"I am a teapot\", \"text/plain\");"}'
```

### Global variables (shared state)

```bash
curl http://localhost:8082/api/vars                          # list all
curl -X PUT http://localhost:8082/api/vars/mykey \
  -H "Content-Type: application/json" -d '{"value":"hello"}' # set
curl -X DELETE http://localhost:8082/api/vars/mykey           # delete
```

---

## Agent Auth

No signup forms. No OAuth. An agent picks a UUID, sends it as a header, done.

```bash
# All requests with this header are "logged in"
-H "X-Agent-Id: agent-550e8400-e29b-41d4-a716-446655440000"
```

| Mode | How | What Happens |
|------|-----|--------------|
| Guest | No header | Tokens work, but aren't owned by anyone |
| Registered | Add `X-Agent-Id` header | Tokens are associated with your agent. `GET /api/tokens` returns only yours |
| Claim | `POST /api/claim/:token-id` with header | Adopts a guest token into your identity |

### The Upgrade Path

An agent starts anonymous — no friction. Later it decides to persist its session:

```bash
# Claim an anonymous token
curl -X POST http://localhost:8082/api/claim/f47ac10b-... \
  -H "X-Agent-Id: my-agent-id"

# Now list all my tokens
curl -s http://localhost:8082/api/tokens \
  -H "X-Agent-Id: my-agent-id" | jq
```

---

## REST API

All under `/api/`. All accept optional `X-Agent-Id` header.

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/tokens` | Create a token |
| `GET` | `/api/tokens` | List tokens (filtered by agent if header present) |
| `GET` | `/api/tokens/:id` | Get a token |
| `PUT` | `/api/tokens/:id` | Update a token |
| `PUT` | `/api/tokens/:id/script` | Set a JS script on the token |
| `DELETE` | `/api/tokens/:id` | Delete a token and all its requests |
| `GET` | `/api/tokens/:id/requests` | List captured requests (`?page=1&per_page=50&sorting=oldest`) |
| `DELETE` | `/api/tokens/:id/requests` | Clear all requests |
| `GET` | `/api/requests/:id` | Get a single request |
| `DELETE` | `/api/requests/:id` | Delete a single request |
| `POST` | `/api/claim/:id` | Claim an anonymous token (requires `X-Agent-Id`) |
| `GET` | `/api/vars` | List all global variables |
| `PUT` | `/api/vars/:key` | Set a global variable |
| `DELETE` | `/api/vars/:key` | Delete a global variable |

---

## GraphQL API

Full GraphQL at `POST /graphql`. Interactive playground at `GET /playground`.

```graphql
# Create
mutation { createToken(defaultStatus: 201, cors: true) { id url } }

# Create with script
mutation { createToken(script: "respond(200, load(\"hits\") || \"0\", \"text/plain\");") { id url } }

# Set script on existing token
mutation { setScript(id: "...", script: "respond(204, \"\", \"text/plain\");") { id script } }

# Inspect
query { requests(tokenId: "...", sorting: "newest") { data { method body headers } } }

# Global variables
query { globalVars { key value } }
mutation { setGlobalVar(key: "foo", value: "bar") { key value } }
mutation { deleteGlobalVar(key: "foo") }

# Real-time (WebSocket)
subscription { requestReceived(tokenId: "...") { request { method url body } } }
```

---

## All Endpoints

| Method | Path | Content-Type | Purpose |
|--------|------|--------------|---------|
| `GET` | `/` | `text/markdown` | Machine-readable API spec |
| `GET` | `/llms.txt` | `text/plain` | Compact spec for LLM context |
| `GET` | `/ui` | `text/html` | Web UI for request inspection |
| `*` | `/api/*` | `application/json` | REST API (see above) |
| `POST` | `/graphql` | `application/json` | GraphQL API |
| `GET` | `/playground` | `text/html` | GraphQL interactive playground |
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
| `form_data` | Parsed form fields (non-JSON requests) |
| `ip` | `127.0.0.1` |
| `user_agent` | `python-requests/2.31.0` |
| `created_at` | `2025-01-15T10:30:00Z` |

---

## Run It

```bash
# From source
go run ./cmd/server

# Build and run
go build -o ai-http-bin ./cmd/server && ./ai-http-bin
```

Listens on `:8082`. All data is in-memory — restart clears everything.

---

## License

MIT
