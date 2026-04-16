package test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/gorilla/websocket"
	"github.com/wricardo/ai-http-bin/pkg/sdk"
)

// --- GraphQL types ---

// gqlRequest represents a GraphQL query request with optional variables.
type gqlRequest struct {
	Query     string `json:"query"`
	Variables any    `json:"variables,omitempty"`
}

// gqlResponse represents a GraphQL response with data and optional errors.
type gqlResponse[T any] struct {
	Data   T `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors,omitempty"`
}

// --- Shared types ---

// Token and Request are type aliases for SDK types used in tests.
type Token = sdk.Token
type Request = sdk.Request

// --- HTTP client helpers ---

// client returns a resty.Client configured with base URL, Content-Type, and Accept headers.
func client() *resty.Client {
	return resty.New().
		SetBaseURL(serverURL).
		SetHeader("Content-Type", "application/json").
		SetHeader("Accept", "application/json")
}

// ptr returns a pointer to the given value. Generic helper for optional SDK fields.
func ptr[T any](v T) *T { return &v }

// createToken creates a token with default settings and fails the test on error.
// Returns the created token; used by most tests for setup.
func createToken(t *testing.T) Token {
	t.Helper()
	tok, err := gqlClient.CreateToken(t.Context(), sdk.CreateTokenInput{})
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	if tok == nil {
		t.Fatal("create token returned nil")
	}
	return *tok
}

// sendWebhook posts a webhook to a token's URL. Optionally sets Content-Type header if provided.
// Returns the HTTP response; used to simulate webhook delivery to a token.
func sendWebhook(t *testing.T, tokenURL, method, body, contentType string) *resty.Response {
	t.Helper()
	req := resty.New().R().SetBody(body)
	if contentType != "" {
		req.SetHeader("Content-Type", contentType)
	}
	resp, err := req.Execute(method, tokenURL)
	if err != nil {
		t.Fatalf("sendWebhook: %v", err)
	}
	return resp
}

// --- GraphQL query helpers ---

// tokenRequestsField queries a token's nested requests field via GraphQL.
// Helper for accessing token.requests { id method body } relationship.
func tokenRequestsField(t *testing.T, tokenID string) []Request {
	t.Helper()

	type tokenData struct {
		Token struct {
			Requests []Request `json:"requests"`
		} `json:"token"`
	}

	body, err := json.Marshal(gqlRequest{
		Query: `query($id: ID!) {
			token(id: $id) {
				requests { id method body }
			}
		}`,
		Variables: map[string]any{"id": tokenID},
	})
	if err != nil {
		t.Fatalf("marshal gql request: %v", err)
	}

	resp, err := client().R().SetBody(body).Post("/graphql")
	if err != nil {
		t.Fatalf("gql request: %v", err)
	}
	if resp.StatusCode() != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", resp.StatusCode(), resp.String())
	}

	var result gqlResponse[tokenData]
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("gql errors: %+v", result.Errors)
	}
	return result.Data.Token.Requests
}

// firstRequest retrieves the first (most recent) captured webhook for a token.
func firstRequest(t *testing.T, tokenID string) Request {
	t.Helper()
	page, err := gqlClient.Requests(t.Context(), tokenID, sdk.RequestsOptions{})
	if err != nil {
		t.Fatalf("Requests: %v", err)
	}
	if len(page.Data) == 0 {
		t.Fatalf("no requests found for token %s", tokenID)
	}
	return page.Data[0]
}

// --- WebSocket helpers ---

// writeWS sends a JSON-encoded message over a WebSocket connection.
func writeWS(t *testing.T, conn *websocket.Conn, v any) {
	t.Helper()
	b, _ := json.Marshal(v)
	if err := conn.WriteMessage(websocket.TextMessage, b); err != nil {
		t.Fatalf("ws write: %v", err)
	}
}

// expectWSType reads WebSocket messages until one with the expected type is found, or times out.
func expectWSType(t *testing.T, conn *websocket.Conn, wantType string) {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("ws read (expecting %s): %v", wantType, err)
		}
		var m map[string]any
		json.Unmarshal(msg, &m) //nolint:errcheck
		if m["type"] == wantType {
			return
		}
	}
}

// readWSData reads WebSocket messages until a data type is found, then extracts and returns the requestReceived payload.
func readWSData(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("ws read: %v", err)
		}
		var envelope struct {
			Type    string `json:"type"`
			Payload struct {
				Data map[string]any `json:"data"`
			} `json:"payload"`
		}
		json.Unmarshal(msg, &envelope) //nolint:errcheck
		if envelope.Type != "data" {
			continue
		}
		raw := envelope.Payload.Data["requestReceived"]
		if raw == nil {
			t.Fatalf("no requestReceived in data: %v", envelope.Payload.Data)
		}
		return raw.(map[string]any)
	}
}

// --- Script execution helpers ---

// createTokenWithScript creates a token with a JavaScript script and returns the token ID.
func createTokenWithScript(t *testing.T, script string) string {
	t.Helper()
	tok, err := gqlClient.CreateToken(t.Context(), sdk.CreateTokenInput{Script: ptr(script)})
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if tok == nil || tok.ID == "" {
		t.Fatal("createToken returned empty id")
	}
	return tok.ID
}

// setScript updates an existing token's script via the SetScript mutation.
func setScript(t *testing.T, tokenID, script string) {
	t.Helper()
	tok, err := gqlClient.SetScript(t.Context(), tokenID, script)
	if err != nil {
		t.Fatalf("SetScript: %v", err)
	}
	if tok == nil || tok.ID != tokenID {
		t.Fatalf("setScript returned wrong id: %+v", tok)
	}
}

// hitToken sends an HTTP request to a token endpoint.
// Path is appended after the token ID. Returns a wrapped response with assertion helpers.
func hitToken(t *testing.T, tokenID, method, path, body string) *webhookResponse {
	t.Helper()
	req := client().R()
	if body != "" {
		req.SetBody(body).SetHeader("Content-Type", "application/json")
	}
	u := "/" + tokenID
	if path != "" {
		u += "/" + path
	}
	resp, err := req.Execute(method, u)
	if err != nil {
		t.Fatalf("hit token: %v", err)
	}
	return &webhookResponse{Response: resp}
}

// webhookResponse wraps resty.Response with assertion helpers for webhook testing.
type webhookResponse struct {
	*resty.Response
}

// mustStatus fails the test if response status does not match expected code.
func (w *webhookResponse) mustStatus(t *testing.T, code int) {
	t.Helper()
	if w.StatusCode() != code {
		t.Fatalf("expected status %d, got %d: %s", code, w.StatusCode(), w.String())
	}
}

// mustJSONField parses response body as JSON and returns the value of the given field, or fails the test.
func (w *webhookResponse) mustJSONField(t *testing.T, field string) any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(w.Body(), &m); err != nil {
		t.Fatalf("parse response JSON: %v", err)
	}
	v, ok := m[field]
	if !ok {
		t.Fatalf("field %q not found in response: %s", field, w.String())
	}
	return v
}
