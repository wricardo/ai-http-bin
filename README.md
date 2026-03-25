# AI HTTP Bin

**Webhook inspection + mock endpoints, built for AI agents.**

An AI agent reads `GET /` and knows exactly how to create webhook URLs, mock any HTTP response, and inspect every request that comes in. No API keys, no dashboards, no docs to search — just one markdown endpoint.

```bash
# Start the server
go run ./cmd/server

# An agent's entire onboarding:
curl http://localhost:8082/
```

---

## Why This Exists

AI agents that interact with external services need two things constantly:

1. **Mock endpoints** — to simulate APIs, test callback URLs, or stand in for services that don't exist yet
2. **Request inspection** — to verify what was actually sent, debug webhook integrations, or confirm payloads

Existing tools (mockbin.io, requestbin) are built for humans clicking around in browsers. This is built for agents making HTTP calls.

### What Makes It Different

- **`GET /` returns a markdown API spec.** An agent reads one URL and can fully operate the service. No separate docs site, no OpenAPI file to find.
- **Zero-friction auth.** No signup flow. No API keys to provision. An agent picks a UUID and starts using it as `X-Agent-Id`. That's the entire registration process.
- **Guest-to-registered upgrade.** An agent can start anonymously, then claim its tokens later by providing an `X-Agent-Id`. No data loss, no re-creation.
- **Both REST and GraphQL.** Simple REST for simple tasks, GraphQL for filtering/pagination/subscriptions.
- **Real-time via WebSocket.** Subscribe to a token and get notified the instant a request arrives.

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
  "default_status": 200,
  "default_content_type": "text/plain",
  "request_count": 0
}
```

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
| `DELETE` | `/api/tokens/:id` | Delete a token and all its requests |
| `GET` | `/api/tokens/:id/requests` | List captured requests (`?page=1&per_page=50&sorting=oldest`) |
| `DELETE` | `/api/tokens/:id/requests` | Clear all requests |
| `GET` | `/api/requests/:id` | Get a single request |
| `DELETE` | `/api/requests/:id` | Delete a single request |
| `POST` | `/api/claim/:id` | Claim an anonymous token (requires `X-Agent-Id`) |

---

## GraphQL API

Full GraphQL at `POST /graphql`. Interactive playground at `GET /playground`.

```graphql
# Create
mutation { createToken(defaultStatus: 201, cors: true) { id url } }

# Inspect
query { requests(tokenId: "...", sorting: "newest") { data { method body headers } } }

# Real-time (WebSocket)
subscription { requestReceived(tokenId: "...") { request { method url body } } }
```

---

## All Endpoints

| Method | Path | Content-Type | Purpose |
|--------|------|--------------|---------|
| `GET` | `/` | `text/markdown` | Machine-readable API spec |
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
