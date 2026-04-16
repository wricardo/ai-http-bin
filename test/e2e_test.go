package test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/wricardo/ai-http-bin/internal/server"
	"github.com/wricardo/ai-http-bin/pkg/sdk"
)

// serverURL is set once in TestMain and used by all tests.
var (
	serverURL string
	gqlClient *sdk.Client
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintf(os.Stderr, "listen: %v\n", err)
		os.Exit(1)
	}

	serverURL = fmt.Sprintf("http://%s", ln.Addr().String())
	gqlClient = sdk.New(serverURL)

	srv := server.New(serverURL)
	go srv.Serve(ln) //nolint:errcheck

	code := m.Run()

	srv.Shutdown(context.Background()) //nolint:errcheck
	os.Exit(code)
}

// --- tests ---

// TestCreateToken verifies that CreateToken (CreateToken mutation) returns a token with default values.
// Scenario: Create a token with no custom settings.
// Expects: Token has generated ID and URL, default status 200, content-type text/plain, CORS disabled.
func TestCreateToken(t *testing.T) {
	token := createToken(t)

	if token.ID == "" {
		t.Error("expected non-empty id")
	}
	if token.URL == "" {
		t.Error("expected non-empty url")
	}
	if token.CreatedAt == "" {
		t.Error("expected non-empty createdAt")
	}
	if token.RequestCount != 0 {
		t.Errorf("requestCount: got %d, want 0", token.RequestCount)
	}
	if token.DefaultStatus != 200 {
		t.Errorf("defaultStatus: got %d, want 200", token.DefaultStatus)
	}
	if token.DefaultContentType != "text/plain" {
		t.Errorf("defaultContentType: got %q, want text/plain", token.DefaultContentType)
	}
	if token.Cors {
		t.Error("cors should default to false")
	}

	t.Logf("token id=%s url=%s", token.ID, token.URL)
}

// TestCreateTokenWithCustomResponse tests the CreateToken mutation with custom response settings.
// Scenario: Create token with custom status (201), content, type (application/json), and CORS enabled.
// Expects: Token reflects all custom settings.
func TestCreateTokenWithCustomResponse(t *testing.T) {
	token, err := gqlClient.CreateToken(context.Background(), sdk.CreateTokenInput{
		DefaultStatus:      ptr(201),
		DefaultContent:     ptr("hello"),
		DefaultContentType: ptr("application/json"),
		Timeout:            ptr(0),
		Cors:               ptr(true),
	})
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if token == nil {
		t.Fatal("CreateToken returned nil")
	}

	if token.DefaultStatus != 201 {
		t.Errorf("defaultStatus: got %d, want 201", token.DefaultStatus)
	}
	if token.DefaultContent != "hello" {
		t.Errorf("defaultContent: got %q, want hello", token.DefaultContent)
	}
	if token.DefaultContentType != "application/json" {
		t.Errorf("defaultContentType: got %q, want application/json", token.DefaultContentType)
	}
	if !token.Cors {
		t.Error("cors should be true")
	}
}

// TestGetToken tests the Token query API (fetch token by ID).
// Scenario: Create a token, then retrieve it by ID.
// Expects: Returned token matches created token ID.
func TestGetToken(t *testing.T) {
	created := createToken(t)

	tok, err := gqlClient.Token(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok == nil {
		t.Fatal("Token returned nil")
	}
	if tok.ID != created.ID {
		t.Errorf("token id: got %q, want %q", tok.ID, created.ID)
	}
}

// TestGetTokenNotFound tests the Token query API with invalid ID.
// Scenario: Query a token with a non-existent UUID.
// Expects: Returns nil, no error.
func TestGetTokenNotFound(t *testing.T) {
	tok, err := gqlClient.Token(context.Background(), "00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok != nil {
		t.Error("expected nil token for unknown id")
	}
}

// TestListTokens tests the Tokens query API (list all tokens).
// Scenario: Create two tokens, then list all tokens.
// Expects: Both created tokens appear in the list.
func TestListTokens(t *testing.T) {
	// Create two tokens so the list is non-empty.
	a := createToken(t)
	b := createToken(t)

	tokens, err := gqlClient.Tokens(context.Background())
	if err != nil {
		t.Fatalf("Tokens: %v", err)
	}

	ids := make(map[string]bool)
	for _, tok := range tokens {
		ids[tok.ID] = true
	}
	if !ids[a.ID] {
		t.Errorf("token %s missing from list", a.ID)
	}
	if !ids[b.ID] {
		t.Errorf("token %s missing from list", b.ID)
	}
}

// TestUpdateToken tests the UpdateToken mutation (modify token settings).
// Scenario: Create token, then update status (404), content, type (text/html), and enable CORS.
// Expects: All fields updated as requested.
func TestUpdateToken(t *testing.T) {
	token := createToken(t)

	updated, err := gqlClient.UpdateToken(context.Background(), token.ID, sdk.UpdateTokenInput{
		DefaultStatus:      ptr(404),
		DefaultContent:     ptr("not here"),
		DefaultContentType: ptr("text/html"),
		Cors:               ptr(true),
	})
	if err != nil {
		t.Fatalf("UpdateToken: %v", err)
	}
	if updated == nil {
		t.Fatal("UpdateToken returned nil")
	}
	if updated.DefaultStatus != 404 {
		t.Errorf("defaultStatus: got %d, want 404", updated.DefaultStatus)
	}
	if updated.DefaultContent != "not here" {
		t.Errorf("defaultContent: got %q, want 'not here'", updated.DefaultContent)
	}
	if updated.DefaultContentType != "text/html" {
		t.Errorf("defaultContentType: got %q, want text/html", updated.DefaultContentType)
	}
	if !updated.Cors {
		t.Error("cors should be true after update")
	}
}

// TestToggleCors tests the ToggleCors mutation (toggle CORS on/off).
// Scenario: Create token (cors=false), toggle twice.
// Expects: First toggle returns true (enabled), second toggle returns false (disabled).
func TestToggleCors(t *testing.T) {
	token := createToken(t) // cors starts false

	// Toggle on.
	enabled, err := gqlClient.ToggleCors(context.Background(), token.ID)
	if err != nil {
		t.Fatalf("ToggleCors 1: %v", err)
	}
	if !enabled {
		t.Error("expected cors=true after first toggle")
	}

	// Toggle off.
	enabled, err = gqlClient.ToggleCors(context.Background(), token.ID)
	if err != nil {
		t.Fatalf("ToggleCors 2: %v", err)
	}
	if enabled {
		t.Error("expected cors=false after second toggle")
	}
}

// TestDeleteToken tests the DeleteToken mutation (delete token).
// Scenario: Create token, delete it, attempt to retrieve.
// Expects: DeleteToken returns true, subsequent Token query returns nil.
func TestDeleteToken(t *testing.T) {
	token := createToken(t)

	ok, err := gqlClient.DeleteToken(context.Background(), token.ID)
	if err != nil {
		t.Fatalf("DeleteToken: %v", err)
	}
	if !ok {
		t.Error("deleteToken should return true")
	}

	tok, err := gqlClient.Token(context.Background(), token.ID)
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok != nil {
		t.Error("token should not exist after deletion")
	}
}

// TestWebhookReceivesRequest tests the webhook capture API (POST /:tokenID).
// Scenario: Send a JSON POST webhook to a token's URL.
// Expects: Response status 200, contains X-Token-Id header matching token ID, X-Request-Id header set.
func TestWebhookReceivesRequest(t *testing.T) {
	token := createToken(t)

	resp := sendWebhook(t, token.URL, http.MethodPost, `{"event":"test"}`, "application/json")

	if resp.StatusCode() != 200 {
		t.Errorf("webhook status: got %d, want 200", resp.StatusCode())
	}
	if resp.Header().Get("X-Token-Id") != token.ID {
		t.Errorf("X-Token-Id: got %q, want %q", resp.Header().Get("X-Token-Id"), token.ID)
	}
	if resp.Header().Get("X-Request-Id") == "" {
		t.Error("X-Request-Id header should be set")
	}
}

// TestWebhookUnknownTokenReturns410 tests webhook capture API with invalid token ID.
// Scenario: Send GET request to non-existent token URL.
// Expects: Returns 410 Gone status.
func TestWebhookUnknownTokenReturns410(t *testing.T) {
	resp := sendWebhook(t, serverURL+"/00000000-0000-0000-0000-000000000000", http.MethodGet, "", "")
	if resp.StatusCode() != http.StatusGone {
		t.Errorf("expected 410, got %d", resp.StatusCode())
	}
}

// TestWebhookCustomResponse tests webhook capture API with custom response settings.
// Scenario: Create token with status 202, content "accepted", type text/plain. Send POST webhook.
// Expects: Response has status 202, body "accepted", correct Content-Type header.
func TestWebhookCustomResponse(t *testing.T) {
	token, err := gqlClient.CreateToken(context.Background(), sdk.CreateTokenInput{
		DefaultStatus:      ptr(202),
		DefaultContent:     ptr("accepted"),
		DefaultContentType: ptr("text/plain"),
	})
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if token == nil {
		t.Fatal("CreateToken returned nil")
	}

	resp := sendWebhook(t, token.URL, http.MethodPost, "ping", "text/plain")

	if resp.StatusCode() != 202 {
		t.Errorf("status: got %d, want 202", resp.StatusCode())
	}
	if string(resp.Body()) != "accepted" {
		t.Errorf("body: got %q, want 'accepted'", string(resp.Body()))
	}
	if ct := resp.Header().Get("Content-Type"); ct != "text/plain" {
		t.Errorf("Content-Type: got %q, want text/plain", ct)
	}
}

// TestWebhookStatusCodeOverrideViaPath tests webhook capture API with path-based status override.
// Scenario: Create token with default status 200, send GET to /:tokenID/418.
// Expects: Response has status 418 (from path), not default 200.
func TestWebhookStatusCodeOverrideViaPath(t *testing.T) {
	token := createToken(t) // defaultStatus=200

	resp := sendWebhook(t, token.URL+"/418", http.MethodGet, "", "")

	if resp.StatusCode() != 418 {
		t.Errorf("status: got %d, want 418", resp.StatusCode())
	}
}

// TestWebhookCORSHeaders tests webhook capture API with CORS enabled.
// Scenario: Create token with cors=true, send GET webhook.
// Expects: Response includes Access-Control-Allow-Origin: * and Access-Control-Allow-Methods: *.
func TestWebhookCORSHeaders(t *testing.T) {
	token, err := gqlClient.CreateToken(context.Background(), sdk.CreateTokenInput{Cors: ptr(true)})
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if token == nil {
		t.Fatal("CreateToken returned nil")
	}

	resp := sendWebhook(t, token.URL, http.MethodGet, "", "")

	if resp.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("expected Access-Control-Allow-Origin: *")
	}
	if resp.Header().Get("Access-Control-Allow-Methods") != "*" {
		t.Error("expected Access-Control-Allow-Methods: *")
	}
}

// TestWebhookNoCORSHeadersWhenDisabled tests webhook capture API with CORS disabled.
// Scenario: Create token with cors=false (default), send GET webhook.
// Expects: Response does not include CORS headers.
func TestWebhookNoCORSHeadersWhenDisabled(t *testing.T) {
	token := createToken(t) // cors=false

	resp := sendWebhook(t, token.URL, http.MethodGet, "", "")

	if resp.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("CORS headers should not be present when cors=false")
	}
}

// TestGetRequest tests the Request query API (fetch captured webhook by ID).
// Scenario: Create token, send JSON POST webhook, retrieve by request ID.
// Expects: Returned request matches: method POST, body JSON content, tokenId matches.
func TestGetRequest(t *testing.T) {
	token := createToken(t)
	sendWebhook(t, token.URL, http.MethodPost, `{"x":1}`, "application/json")

	page, err := gqlClient.Requests(context.Background(), token.ID, sdk.RequestsOptions{})
	if err != nil {
		t.Fatalf("Requests: %v", err)
	}
	if page.Total != 1 {
		t.Fatalf("expected 1 request, got %d", page.Total)
	}
	req := page.Data[0]

	byID, err := gqlClient.Request(context.Background(), req.ID)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if byID == nil {
		t.Fatal("Request returned nil")
	}
	if byID.ID != req.ID {
		t.Errorf("request id mismatch: got %q, want %q", byID.ID, req.ID)
	}
	if byID.Method != "POST" {
		t.Errorf("method: got %q, want POST", byID.Method)
	}
	if byID.Body != `{"x":1}` {
		t.Errorf("body: got %q, want {\"x\":1}", byID.Body)
	}
	if byID.TokenID != token.ID {
		t.Errorf("tokenId: got %q, want %q", byID.TokenID, token.ID)
	}
}

// TestListRequestsPagination tests the Requests query API with pagination.
// Scenario: Create token, send 5 webhooks, query with page=1 perPage=2, then page=3 perPage=2.
// Expects: Page 1 has 2 items, not last page. Page 3 has 1 item, is last page. Total is 5.
func TestListRequestsPagination(t *testing.T) {
	token := createToken(t)

	// Send 5 requests.
	for i := range 5 {
		sendWebhook(t, token.URL, http.MethodPost, fmt.Sprintf(`{"i":%d}`, i), "application/json")
	}

	// Page 1, 2 per page.
	p1, err := gqlClient.Requests(context.Background(), token.ID, sdk.RequestsOptions{Page: ptr(1), PerPage: ptr(2)})
	if err != nil {
		t.Fatalf("Requests page1: %v", err)
	}

	if p1.Total != 5 {
		t.Errorf("total: got %d, want 5", p1.Total)
	}
	if len(p1.Data) != 2 {
		t.Errorf("page 1 items: got %d, want 2", len(p1.Data))
	}
	if p1.IsLastPage {
		t.Error("page 1 should not be last page")
	}
	if p1.From != 1 || p1.To != 2 {
		t.Errorf("from/to: got %d/%d, want 1/2", p1.From, p1.To)
	}

	// Last page.
	p3, err := gqlClient.Requests(context.Background(), token.ID, sdk.RequestsOptions{Page: ptr(3), PerPage: ptr(2)})
	if err != nil {
		t.Fatalf("Requests page3: %v", err)
	}

	if len(p3.Data) != 1 {
		t.Errorf("page 3 items: got %d, want 1", len(p3.Data))
	}
	if !p3.IsLastPage {
		t.Error("page 3 should be last page")
	}
}

// TestListRequestsSortingNewest tests the Requests query API with sorting.
// Scenario: Create token, send 2 webhooks (first, second), query with sorting=newest.
// Expects: First result has body "second" (newest first).
func TestListRequestsSortingNewest(t *testing.T) {
	token := createToken(t)

	sendWebhook(t, token.URL, http.MethodPost, `{"order":"first"}`, "application/json")
	sendWebhook(t, token.URL, http.MethodPost, `{"order":"second"}`, "application/json")

	result, err := gqlClient.Requests(context.Background(), token.ID, sdk.RequestsOptions{Sorting: ptr("newest")})
	if err != nil {
		t.Fatalf("Requests newest: %v", err)
	}

	if len(result.Data) < 2 {
		t.Fatalf("expected at least 2 requests, got %d", len(result.Data))
	}
	if result.Data[0].Body != `{"order":"second"}` {
		t.Errorf("newest first: got body %q, want second", result.Data[0].Body)
	}
}

// TestDeleteRequest tests the DeleteRequest mutation (delete captured webhook).
// Scenario: Create token, send webhook (1 total), delete it, list requests.
// Expects: DeleteRequest returns true, total count after delete is 0.
func TestDeleteRequest(t *testing.T) {
	token := createToken(t)
	sendWebhook(t, token.URL, http.MethodGet, "", "")

	page, err := gqlClient.Requests(context.Background(), token.ID, sdk.RequestsOptions{})
	if err != nil {
		t.Fatalf("Requests before delete: %v", err)
	}
	if page.Total != 1 {
		t.Fatalf("expected 1 request before delete, got %d", page.Total)
	}
	reqID := page.Data[0].ID

	ok, err := gqlClient.DeleteRequest(context.Background(), reqID)
	if err != nil {
		t.Fatalf("DeleteRequest: %v", err)
	}
	if !ok {
		t.Error("deleteRequest should return true")
	}

	after, err := gqlClient.Requests(context.Background(), token.ID, sdk.RequestsOptions{})
	if err != nil {
		t.Fatalf("Requests after delete: %v", err)
	}
	if after.Total != 0 {
		t.Errorf("expected 0 requests after delete, got %d", after.Total)
	}
}

// TestClearRequests tests the ClearRequests mutation (delete all webhooks for token).
// Scenario: Create token, send 2 webhooks, clear all, list requests.
// Expects: ClearRequests returns true, total count after clear is 0.
func TestClearRequests(t *testing.T) {
	token := createToken(t)

	sendWebhook(t, token.URL, http.MethodGet, "", "")
	sendWebhook(t, token.URL, http.MethodGet, "", "")

	ok, err := gqlClient.ClearRequests(context.Background(), token.ID)
	if err != nil {
		t.Fatalf("ClearRequests: %v", err)
	}
	if !ok {
		t.Error("clearRequests should return true")
	}

	page, err := gqlClient.Requests(context.Background(), token.ID, sdk.RequestsOptions{})
	if err != nil {
		t.Fatalf("Requests after clear: %v", err)
	}
	if page.Total != 0 {
		t.Errorf("expected 0 requests after clear, got %d", page.Total)
	}
}

// TestDeleteTokenAlsoDeletesRequests tests cascade delete (DeleteToken → deletes all captured webhooks).
// Scenario: Create token, send 2 webhooks, delete token, try to retrieve webhooks.
// Expects: Each webhook query returns nil (deleted).
func TestDeleteTokenAlsoDeletesRequests(t *testing.T) {
	token := createToken(t)

	// Capture a few requests so there is something to delete.
	sendWebhook(t, token.URL, http.MethodPost, "first", "text/plain")
	sendWebhook(t, token.URL, http.MethodPost, "second", "text/plain")

	before, err := gqlClient.Requests(context.Background(), token.ID, sdk.RequestsOptions{})
	if err != nil {
		t.Fatalf("Requests before token delete: %v", err)
	}
	if before.Total != 2 {
		t.Fatalf("expected 2 requests before delete, got %d", before.Total)
	}
	reqIDs := make([]string, len(before.Data))
	for i, r := range before.Data {
		reqIDs[i] = r.ID
	}

	ok, err := gqlClient.DeleteToken(context.Background(), token.ID)
	if err != nil {
		t.Fatalf("DeleteToken: %v", err)
	}
	if !ok {
		t.Fatal("deleteToken returned false")
	}

	for _, id := range reqIDs {
		req, err := gqlClient.Request(context.Background(), id)
		if err != nil {
			t.Fatalf("Request(%s): %v", id, err)
		}
		if req != nil {
			t.Errorf("request %s still exists after token deletion", id)
		}
	}
}

// TestHealthEndpoint tests the /health management endpoint.
// Scenario: Send GET request to /health.
// Expects: Status 200, response body contains {"status": "ok"}.
func TestHealthEndpoint(t *testing.T) {
	resp, err := client().R().Get("/health")
	if err != nil {
		t.Fatalf("health request: %v", err)
	}
	if resp.StatusCode() != 200 {
		t.Errorf("health status: got %d, want 200", resp.StatusCode())
	}

	var body map[string]string
	if err := json.Unmarshal(resp.Body(), &body); err != nil {
		t.Fatalf("unmarshal health: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("health body: got %v, want status=ok", body)
	}
}
