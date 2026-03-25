# Requirements

A webhook inspection and mock HTTP endpoint service built for AI agents. Agents create unique URLs, point HTTP clients at them, and inspect every captured request via API. The home page (`GET /`) serves a markdown specification that agents can read to self-onboard.

---

## Core Concepts

### Agent Identity

Agents authenticate via the `X-Agent-Id` header — a UUID they choose themselves.

| Mode | Behavior |
|------|----------|
| Anonymous (no header) | Tokens are created without ownership. Accessible to anyone with the token ID. |
| Registered (with header) | Tokens are associated with the agent. `GET /api/tokens` returns only that agent's tokens. |
| Claim | An anonymous token can be claimed by providing `X-Agent-Id` via `POST /api/claim/:id`. Once claimed, the token is associated with that agent. Already-claimed tokens cannot be re-claimed. |

### Token

A token represents a unique webhook endpoint. Creating a token gives you a URL. Anyone who sends an HTTP request to that URL has it recorded.

| Field | Type | Description |
|-------|------|-------------|
| id | string (UUID) | Unique identifier |
| agent_id | string | Agent that owns this token (empty for anonymous) |
| url | string | Full URL to send webhooks to |
| ip | string | IP address of the client that created the token |
| user_agent | string | User-Agent of the client that created the token |
| created_at | ISO 8601 timestamp | When the token was created |
| request_count | integer | Total requests received |
| default_status | integer | HTTP status code returned to callers (default: `200`) |
| default_content | string | Response body returned to callers (default: empty string) |
| default_content_type | string | Content-Type of the response (default: `text/plain`) |
| timeout | integer (0–10) | Seconds to wait before responding; `0` means no delay |
| cors | boolean | Whether to add CORS headers to webhook responses (default: `false`) |

### Request

A captured HTTP request.

| Field | Type | Description |
|-------|------|-------------|
| id | string (UUID) | Unique identifier |
| token_id | string | Parent token ID |
| method | string | HTTP method (GET, POST, PUT, PATCH, DELETE, etc.) |
| url | string | Full request URI including query string |
| hostname | string | Host header value |
| path | string | Path after the token segment (e.g. `/some/sub/path`) |
| headers | JSON string | All request headers as key→value |
| query | JSON string | All query parameters as key→value; empty object `{}` if none |
| body | string | Raw request body |
| form_data | JSON string | Parsed form fields when Content-Type is not `application/json`; `null` otherwise |
| ip | string | Client IP address |
| user_agent | string | Value of the User-Agent header |
| created_at | ISO 8601 timestamp | When the request was received |

---

## API

The service exposes a REST API, a GraphQL API, and a webhook receiver on the same port.

### Home Page / Spec

`GET /` returns a `text/markdown` document describing the full API. This is the entry point for AI agents.

### REST API

All under `/api/`. All accept optional `X-Agent-Id` header.

| Method | Path | Description |
|--------|------|-------------|
| POST | /api/tokens | Create a new token |
| GET | /api/tokens | List tokens (filtered by agent if header present) |
| GET | /api/tokens/:id | Get a token |
| PUT | /api/tokens/:id | Update a token |
| DELETE | /api/tokens/:id | Delete a token and its requests |
| GET | /api/tokens/:id/requests | List captured requests (paginated) |
| DELETE | /api/tokens/:id/requests | Clear all requests for a token |
| GET | /api/requests/:id | Get a single captured request |
| DELETE | /api/requests/:id | Delete a single request |
| POST | /api/claim/:id | Claim an anonymous token (requires X-Agent-Id) |

### Webhook Receiver (REST)

Accepts any HTTP method at `/:token` and `/:token/*path`.

- If the token does not exist, return `410 Gone`.
- If `timeout` is set on the token, sleep that many seconds before responding.
- Capture and store the request, then respond using the token's configured response (see Custom Response below).
- The receiver must not shadow the other API routes (`/graphql`, `/playground`, `/health`, `/api/*`, `/`).

#### Custom Response

1. **Status code** — use `default_status`. If the path segment immediately after the token is a valid HTTP status code (matching `[1-5][0-9][0-9]`), use that value instead.
2. **Body** — use `default_content`.
3. **Content-Type header** — use `default_content_type`.
4. **`X-Request-Id` header** — the UUID assigned to the captured request.
5. **`X-Token-Id` header** — the token UUID.
6. **CORS headers** — added if the token has `cors: true`.

#### CORS Headers (when enabled)

- `Access-Control-Allow-Origin: *`
- `Access-Control-Allow-Methods: *`
- `Access-Control-Allow-Headers: *`

### GraphQL API

**Endpoint:** `POST /graphql`
**Playground:** `GET /playground`

Supports the same `X-Agent-Id` header for authentication.

**Queries**

```graphql
token(id: ID!): Token
tokens: [Token!]!
request(id: ID!): Request
requests(tokenId: ID!, page: Int, perPage: Int, sorting: String): RequestPage!
```

**Mutations**

```graphql
createToken(defaultStatus: Int, defaultContent: String, defaultContentType: String, timeout: Int, cors: Boolean): Token!
updateToken(id: ID!, defaultStatus: Int, defaultContent: String, defaultContentType: String, timeout: Int, cors: Boolean): Token!
toggleCors(id: ID!): Boolean!
deleteToken(id: ID!): Boolean!
claimToken(id: ID!): Token!
deleteRequest(id: ID!): Boolean!
clearRequests(tokenId: ID!): Boolean!
```

**Subscriptions**

```graphql
requestReceived(tokenId: ID!): RequestEvent!
```

### Other Routes

| Method | Path | Description |
|--------|------|-------------|
| GET | / | API spec in markdown |
| GET | /health | Returns `{"status":"ok"}` |
| GET | /playground | GraphQL playground UI |

---

## Behavior

- Tokens don't have a maximum number of requests they can hold. They can hold as many requests as are sent to them, limited only by system resources.
- Deleting a token also deletes all of its captured requests.
- Large payloads (over 1 MB) are truncated in WebSocket subscription broadcasts.

---
