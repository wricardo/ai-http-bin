package server

import "strings"

func specMarkdown(baseURL string) string {
	return strings.ReplaceAll(specTemplate, "{{BASE_URL}}", baseURL)
}

func llmsTxt(baseURL string) string {
	return strings.ReplaceAll(llmsTxtTemplate, "{{BASE_URL}}", baseURL)
}

const llmsTxtTemplate = `# AI HTTP Bin

AI HTTP Bin provides disposable webhook URLs and a GraphQL API for managing tokens,
captured requests, scripts, and shared variables.

Base URL: {{BASE_URL}}

## Key endpoints

- API guide: {{BASE_URL}}/
- GraphQL endpoint: POST {{BASE_URL}}/graphql
- GraphQL playground: {{BASE_URL}}/playground
- Health check: {{BASE_URL}}/health
- Webhook receiver: ANY {{BASE_URL}}/<token-id> or {{BASE_URL}}/<token-id>/<path>

## Typical flow

1. Create a token with mutation createToken
2. Send any HTTP request to {{BASE_URL}}/<token-id>
3. Inspect traffic with query requests(tokenId: ...)

## GraphQL operations

Queries: token, tokens, request, requests, globalVars
Mutations: createToken, updateToken, setScript, toggleCors, deleteToken, claimToken,
deleteRequest, clearRequests, setGlobalVar, deleteGlobalVar
Subscriptions: requestReceived(tokenId)

## Authentication

Optional header: X-Agent-Id

- Without header: guest mode
- With header: token ownership and scoped token listing
- Claim anonymous token: claimToken(id)

## Webhook behavior

- Unknown/expired token returns HTTP 410
- Static response uses token defaults: status/content/contentType
- Exact numeric subpath (for example /404) overrides status
- If script exists, script response takes precedence
- Script execution timeout is 2 seconds; errors return 500 and X-Script-Error
`

const specTemplate = `# AI HTTP Bin API Guide

This service is managed through GraphQL.

Base URL: {{BASE_URL}}

GraphQL endpoint: POST {{BASE_URL}}/graphql
GraphQL playground: GET {{BASE_URL}}/playground
Health check: GET {{BASE_URL}}/health

Webhook receiver:
- ANY {{BASE_URL}}/<token-id>
- ANY {{BASE_URL}}/<token-id>/<path>

Core GraphQL operations:
- Queries: token, tokens, request, requests, globalVars
- Mutations: createToken, updateToken, setScript, toggleCors, deleteToken, claimToken,
  clearRequests, deleteRequest, setGlobalVar, deleteGlobalVar
- Subscriptions: requestReceived(tokenId)

Behavior notes:
- Tokens expire after 24 hours by default
- Each token stores up to 50 requests (FIFO eviction)
- Unknown or expired tokens return HTTP 410 on webhook endpoints
- Script execution is capped at 2 seconds
`
