package server

import "strings"

func specMarkdown(baseURL string) string {
	return strings.ReplaceAll(specTemplate, "{{BASE_URL}}", baseURL)
}

const specTemplate = `# AI HTTP Bin — API for AI Agents

A webhook inspection and mock HTTP endpoint service built for AI agents.
Create unique URLs, send requests to them, and inspect every captured request via API.

Base URL: ` + "`{{BASE_URL}}`" + `

---

## Quick Start

### 1. Create a token (anonymous)

` + "```" + `bash
curl -s -X POST {{BASE_URL}}/api/tokens | jq
` + "```" + `

Response:

` + "```" + `json
{
  "id": "a1b2c3d4-...",
  "url": "{{BASE_URL}}/a1b2c3d4-...",
  "default_status": 200,
  "default_content": "",
  "default_content_type": "text/plain",
  "timeout": 0,
  "cors": false,
  "request_count": 0
}
` + "```" + `

### 2. Send a request to your webhook URL

` + "```" + `bash
curl -X POST {{BASE_URL}}/a1b2c3d4-... \
  -H "Content-Type: application/json" \
  -d '{"hello": "world"}'
` + "```" + `

### 3. Inspect captured requests

` + "```" + `bash
curl -s {{BASE_URL}}/api/tokens/a1b2c3d4-.../requests | jq
` + "```" + `

---

## Authentication

Authentication is optional. The service supports two modes:

### Anonymous (Guest)

No headers needed. Create tokens and use them freely. Tokens are accessible by
anyone who knows the token ID.

### Registered Agent

Provide the ` + "`X-Agent-Id`" + ` header with a UUID you control. This is your identity.

` + "```" + `bash
curl -s -X POST {{BASE_URL}}/api/tokens \
  -H "X-Agent-Id: my-agent-550e8400-e29b-41d4-a716-446655440000"
` + "```" + `

When ` + "`X-Agent-Id`" + ` is provided:
- New tokens are associated with your agent ID
- ` + "`GET /api/tokens`" + ` returns only your tokens
- You can claim previously anonymous tokens (see below)

### Claiming Anonymous Tokens

If you started as a guest and want to persist your session, claim your tokens:

` + "```" + `bash
curl -s -X POST {{BASE_URL}}/api/claim/<token-id> \
  -H "X-Agent-Id: my-agent-550e8400-e29b-41d4-a716-446655440000"
` + "```" + `

Once claimed, the token is associated with your agent ID. Already-claimed tokens
cannot be re-claimed.

---

## REST API

All responses are JSON. All endpoints accept ` + "`X-Agent-Id`" + ` header (optional).

### Tokens

#### Create Token

` + "```" + `
POST /api/tokens
Content-Type: application/json (optional)
` + "```" + `

Body (all fields optional):

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| default_status | int | 200 | HTTP status code returned to webhook callers |
| default_content | string | "" | Response body returned to webhook callers |
| default_content_type | string | "text/plain" | Content-Type of webhook response |
| timeout | int | 0 | Seconds to wait before responding (0-10) |
| cors | bool | false | Add CORS headers to webhook responses |

Returns: ` + "`201 Created`" + ` with token object.

#### List Tokens

` + "```" + `
GET /api/tokens
` + "```" + `

With ` + "`X-Agent-Id`" + `: returns only that agent's tokens.
Without: returns all tokens.

Returns: ` + "`200 OK`" + ` with array of token objects.

#### Get Token

` + "```" + `
GET /api/tokens/:id
` + "```" + `

Returns: ` + "`200 OK`" + ` with token object, or ` + "`404`" + `.

#### Update Token

` + "```" + `
PUT /api/tokens/:id
Content-Type: application/json
` + "```" + `

Body: same fields as Create Token (all optional, only provided fields are updated).

Returns: ` + "`200 OK`" + ` with updated token object.

#### Delete Token

` + "```" + `
DELETE /api/tokens/:id
` + "```" + `

Deletes the token and all its captured requests.

Returns: ` + "`200 OK`" + ` with ` + "`{\"deleted\": true}`" + `.

### Requests

#### List Requests for Token

` + "```" + `
GET /api/tokens/:id/requests?page=1&per_page=50&sorting=oldest
` + "```" + `

Query parameters:

| Param | Default | Description |
|-------|---------|-------------|
| page | 1 | Page number |
| per_page | 50 | Results per page |
| sorting | "oldest" | "oldest" or "newest" |

Returns: ` + "`200 OK`" + ` with ` + "`{\"data\": [...], \"total\": N, \"page\": N}`" + `.

#### Get Single Request

` + "```" + `
GET /api/requests/:id
` + "```" + `

Returns: ` + "`200 OK`" + ` with request object.

#### Delete Single Request

` + "```" + `
DELETE /api/requests/:id
` + "```" + `

Returns: ` + "`200 OK`" + ` with ` + "`{\"deleted\": true}`" + `.

#### Clear All Requests for Token

` + "```" + `
DELETE /api/tokens/:id/requests
` + "```" + `

Returns: ` + "`200 OK`" + ` with ` + "`{\"cleared\": true}`" + `.

### Claim Token

` + "```" + `
POST /api/claim/:id
X-Agent-Id: <your-agent-id>
` + "```" + `

Associates an anonymous token with your agent ID. Requires ` + "`X-Agent-Id`" + ` header.
Fails if token is already claimed.

Returns: ` + "`200 OK`" + ` with token object, or ` + "`409 Conflict`" + `.

---

## Webhook Receiver

Any HTTP method to ` + "`/:token`" + ` or ` + "`/:token/*path`" + ` is captured and stored.

### Response Behavior

- **Status**: Uses the token's ` + "`default_status`" + `. If the first path segment after
  the token is a valid HTTP status code (e.g. ` + "`/418`" + `), that overrides the default.
- **Body**: Uses ` + "`default_content`" + `.
- **Content-Type**: Uses ` + "`default_content_type`" + `.
- **Headers**: ` + "`X-Request-Id`" + ` and ` + "`X-Token-Id`" + ` are always set.
- **CORS**: If ` + "`cors`" + ` is enabled on the token, standard CORS headers are added.
- **Timeout**: If set, the server waits N seconds before responding.
- **Unknown token**: Returns ` + "`410 Gone`" + `.

### Example: Mock a 201 JSON endpoint

` + "```" + `bash
# Create a token that returns JSON
curl -s -X POST {{BASE_URL}}/api/tokens \
  -H "Content-Type: application/json" \
  -d '{
    "default_status": 201,
    "default_content": "{\"id\": 1, \"status\": \"created\"}",
    "default_content_type": "application/json"
  }' | jq

# Use the returned URL as a mock endpoint
curl -s {{BASE_URL}}/<token-id> | jq
# => {"id": 1, "status": "created"}
` + "```" + `

### Example: Status code override via path

` + "```" + `bash
curl -s -o /dev/null -w "%{http_code}" {{BASE_URL}}/<token-id>/404
# => 404
` + "```" + `

---

## Captured Request Object

Each captured request contains:

| Field | Type | Description |
|-------|------|-------------|
| id | UUID | Unique request ID |
| token_id | UUID | Parent token |
| method | string | HTTP method (GET, POST, etc.) |
| url | string | Full request URI with query string |
| hostname | string | Host header value |
| path | string | Path after the token segment |
| headers | JSON string | All request headers |
| query | JSON string | Query parameters (` + "`{}`" + ` if none) |
| body | string | Raw request body |
| form_data | JSON string | Parsed form fields (non-JSON requests only) |
| ip | string | Client IP |
| user_agent | string | User-Agent header |
| created_at | ISO 8601 | When the request was received |

---

## GraphQL API

A full GraphQL API is also available at ` + "`/graphql`" + ` (POST).
Interactive playground at ` + "`/playground`" + `.

The GraphQL API supports the same ` + "`X-Agent-Id`" + ` header for authentication.

### Queries

` + "```" + `graphql
token(id: ID!): Token
tokens: [Token!]!
request(id: ID!): Request
requests(tokenId: ID!, page: Int, perPage: Int, sorting: String): RequestPage!
` + "```" + `

### Mutations

` + "```" + `graphql
createToken(defaultStatus: Int, defaultContent: String, defaultContentType: String, timeout: Int, cors: Boolean): Token!
updateToken(id: ID!, defaultStatus: Int, defaultContent: String, defaultContentType: String, timeout: Int, cors: Boolean): Token!
toggleCors(id: ID!): Boolean!
deleteToken(id: ID!): Boolean!
claimToken(id: ID!): Token!
deleteRequest(id: ID!): Boolean!
clearRequests(tokenId: ID!): Boolean!
` + "```" + `

### Subscriptions (WebSocket)

` + "```" + `graphql
requestReceived(tokenId: ID!): RequestEvent!
` + "```" + `

---

## Other Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | /health | Returns ` + "`{\"status\":\"ok\"}`" + ` |
| GET | /playground | GraphQL interactive playground |
| GET | / | This spec document (text/markdown) |
`
