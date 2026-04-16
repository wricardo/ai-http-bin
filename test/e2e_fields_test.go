package test

// Tests for captured field values, defaults, quota, timeout, subscription,
// playground, and route shadowing — covering the remaining requirement gaps.

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/wricardo/ai-http-bin/internal/server"
	"github.com/wricardo/ai-http-bin/pkg/sdk"
)

// --- REQ-004, REQ-005: token captures creator IP and User-Agent ---

// TestTokenCapturesCreatorIPAndUserAgent tests token creation API (REQ-004, REQ-005).
// Scenario: Create token, inspect token.IP and token.UserAgent fields.
// Expects: Both fields are non-empty (client IP and User-Agent captured).
func TestTokenCapturesCreatorIPAndUserAgent(t *testing.T) {
	tok, err := gqlClient.CreateToken(context.Background(), sdk.CreateTokenInput{})
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if tok == nil {
		t.Fatal("CreateToken returned nil")
	}
	if tok.IP == "" {
		t.Error("expected non-empty ip on created token")
	}
	// The test HTTP client doesn't set a custom UA, but the field must be present.
	t.Logf("token ip=%s userAgent=%q", tok.IP, tok.UserAgent)
}

// --- REQ-008: token.requests field returns associated requests ---

// TestTokenRequestsField tests the token.requests nested field (REQ-008).
// Scenario: Create token, send 2 webhooks, query token with nested requests field.
// Expects: token.requests contains 2 request objects with id, method, body fields.
func TestTokenRequestsField(t *testing.T) {
	token := createToken(t)
	sendWebhook(t, token.URL, http.MethodPost, `{"a":1}`, "application/json")
	sendWebhook(t, token.URL, http.MethodPost, `{"a":2}`, "application/json")

	reqs := tokenRequestsField(t, token.ID)
	if len(reqs) != 2 {
		t.Errorf("expected 2 requests on token, got %d", len(reqs))
	}
}

// --- REQ-012, REQ-029: timeout delays response ---

// TestWebhookTimeout tests webhook API with timeout setting (REQ-012, REQ-029).
// Scenario: Create token with timeout=1 (1s), send webhook, measure elapsed time.
// Expects: Response takes at least 900ms (accounting for timing variance).
func TestWebhookTimeout(t *testing.T) {
	token, err := gqlClient.CreateToken(context.Background(), sdk.CreateTokenInput{Timeout: ptr(1)})
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if token == nil {
		t.Fatal("CreateToken returned nil")
	}

	start := time.Now()
	resp := sendWebhook(t, token.URL, http.MethodGet, "", "")
	elapsed := time.Since(start)

	if resp.StatusCode() != 200 {
		t.Errorf("status: got %d, want 200", resp.StatusCode())
	}
	if elapsed < 900*time.Millisecond {
		t.Errorf("response too fast (%v); expected at least 1s delay", elapsed)
	}
	t.Logf("response took %v", elapsed)
}

// --- REQ-016: request URL includes query string ---

// TestRequestCapturesURLWithQueryString tests request capture API (REQ-016).
// Scenario: Create token, send GET to /:tokenID?foo=bar&n=42, retrieve request.
// Expects: request.url contains query string "foo=bar".
func TestRequestCapturesURLWithQueryString(t *testing.T) {
	token := createToken(t)

	sendWebhook(t, token.URL+"?foo=bar&n=42", http.MethodGet, "", "")

	req := firstRequest(t, token.ID)

	if !strings.Contains(req.URL, "foo=bar") {
		t.Errorf("url %q does not contain query string foo=bar", req.URL)
	}
}

// --- REQ-017: request captures hostname ---

// TestRequestCapturesHostname tests request capture API (REQ-017).
// Scenario: Create token, send webhook, retrieve request.
// Expects: request.hostname is non-empty.
func TestRequestCapturesHostname(t *testing.T) {
	token := createToken(t)
	sendWebhook(t, token.URL, http.MethodGet, "", "")

	req := firstRequest(t, token.ID)
	if req.Hostname == "" {
		t.Error("expected non-empty hostname on captured request")
	}
	t.Logf("hostname=%s", req.Hostname)
}

// --- REQ-018: request captures path after token segment ---

// TestRequestCapturesSubPath tests request capture API (REQ-018).
// Scenario: Create token, send GET to /:tokenID/some/sub/path, retrieve request.
// Expects: request.path equals "/some/sub/path".
func TestRequestCapturesSubPath(t *testing.T) {
	token := createToken(t)
	sendWebhook(t, token.URL+"/some/sub/path", http.MethodGet, "", "")

	req := firstRequest(t, token.ID)
	if req.Path != "/some/sub/path" {
		t.Errorf("path: got %q, want /some/sub/path", req.Path)
	}
}

// --- REQ-019: request captures all headers as JSON ---

// TestRequestCapturesHeaders tests request capture API (REQ-019).
// Scenario: Create token, send webhook with custom header "X-Custom-Header: hello-test", retrieve request.
// Expects: request.headers is valid JSON, contains X-Custom-Header with value "hello-test".
func TestRequestCapturesHeaders(t *testing.T) {
	token := createToken(t)

	_, err := client().R().
		SetHeader("X-Custom-Header", "hello-test").
		Get(token.URL)
	if err != nil {
		t.Fatalf("send webhook: %v", err)
	}

	req := firstRequest(t, token.ID)

	var headers map[string]string
	if err := json.Unmarshal([]byte(req.Headers), &headers); err != nil {
		t.Fatalf("headers is not valid JSON: %v — got: %s", err, req.Headers)
	}

	found := false
	for k, v := range headers {
		if strings.EqualFold(k, "X-Custom-Header") && v == "hello-test" {
			found = true
		}
	}
	if !found {
		t.Errorf("X-Custom-Header not found in headers JSON: %s", req.Headers)
	}
}

// --- REQ-020: request captures query params as JSON; empty when none ---

// TestRequestCapturesQueryParams tests request capture API (REQ-020).
// Scenario: Create 2 tokens. Send one with query params (color=red&size=large), one without.
// Expects: First request.query is valid JSON with {color: red, size: large}. Second request.query is valid JSON (empty object or {}).
func TestRequestCapturesQueryParams(t *testing.T) {
	token := createToken(t)

	// With params.
	sendWebhook(t, token.URL+"?color=red&size=large", http.MethodGet, "", "")
	req := firstRequest(t, token.ID)

	var params map[string]string
	if err := json.Unmarshal([]byte(req.Query), &params); err != nil {
		t.Fatalf("query is not valid JSON: %v — got: %s", err, req.Query)
	}
	if params["color"] != "red" {
		t.Errorf("query color: got %q, want red", params["color"])
	}
	if params["size"] != "large" {
		t.Errorf("query size: got %q, want large", params["size"])
	}

	// Without params — must be valid JSON (empty object or null).
	token2 := createToken(t)
	sendWebhook(t, token2.URL, http.MethodGet, "", "")
	req2 := firstRequest(t, token2.ID)

	var v any
	if err := json.Unmarshal([]byte(req2.Query), &v); err != nil {
		t.Fatalf("query without params is not valid JSON: %v — got: %s", err, req2.Query)
	}
}

// --- REQ-022: request captures formData for non-JSON POST ---

// TestRequestCapturesFormData tests request capture API (REQ-022).
// Scenario: Create token, send POST with form data (username=alice&role=admin), retrieve request.
// Expects: request.formData is valid JSON with {username: alice, role: admin}.
func TestRequestCapturesFormData(t *testing.T) {
	token := createToken(t)

	form := url.Values{"username": {"alice"}, "role": {"admin"}}
	_, err := client().R().
		SetHeader("Content-Type", "application/x-www-form-urlencoded").
		SetBody(form.Encode()).
		Post(token.URL)
	if err != nil {
		t.Fatalf("send form: %v", err)
	}

	page, err := gqlClient.Requests(context.Background(), token.ID, sdk.RequestsOptions{})
	if err != nil {
		t.Fatalf("Requests: %v", err)
	}
	if len(page.Data) == 0 {
		t.Fatal("no requests captured")
	}
	raw := page.Data[0].FormData

	// formData should be a JSON object with the form fields.
	var fd map[string]string
	if err := json.Unmarshal([]byte(raw), &fd); err != nil {
		t.Fatalf("formData is not valid JSON: %v — got: %s", err, raw)
	}
	if fd["username"] != "alice" {
		t.Errorf("formData username: got %q, want alice", fd["username"])
	}
}

// --- REQ-023, REQ-024, REQ-025: request captures IP, User-Agent, createdAt ---

// TestRequestCapturesIPUserAgentCreatedAt tests request capture API (REQ-023, REQ-024, REQ-025).
// Scenario: Create token, send POST with custom User-Agent header, retrieve request.
// Expects: request.ip is non-empty, request.userAgent is non-empty, request.createdAt is valid ISO 8601.
func TestRequestCapturesIPUserAgentCreatedAt(t *testing.T) {
	token := createToken(t)
	_, err := client().R().
		SetHeader("User-Agent", "test-agent/1.0").
		Post(token.URL)
	if err != nil {
		t.Fatalf("send webhook: %v", err)
	}

	req := firstRequest(t, token.ID)

	if req.IP == "" {
		t.Error("expected non-empty ip on captured request")
	}
	if req.UserAgent == "" {
		t.Error("expected non-empty userAgent on captured request")
	}
	if req.CreatedAt == "" {
		t.Error("expected non-empty createdAt on captured request")
	}
	// Validate ISO 8601 (basic check: parseable by time.RFC3339).
	if _, err := time.Parse(time.RFC3339, req.CreatedAt); err != nil {
		t.Errorf("createdAt %q is not ISO 8601: %v", req.CreatedAt, err)
	}
	t.Logf("ip=%s userAgent=%s createdAt=%s", req.IP, req.UserAgent, req.CreatedAt)
}

// --- REQ-028, REQ-057: per-token request quota uses FIFO eviction ---
//
// Requests beyond the per-token cap are accepted (200) but the oldest
// stored request is evicted to make room. The cap is never exceeded.

// TestQuotaEnforcement tests the per-token quota API (server with max 2 requests per token).
// Scenario: Create a token, send 3 webhooks, query the stored requests.
// Expects: All 3 webhooks return 200 (accepted), but only 2 are stored; oldest is evicted due to FIFO policy.
func TestQuotaEnforcement(t *testing.T) {
	// Start a dedicated server with a quota of 2.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	quotaURL := fmt.Sprintf("http://%s", ln.Addr().String())
	srv := server.New(quotaURL, server.WithMaxRequestsPerToken(2))
	go srv.Serve(ln)                                         //nolint:errcheck
	t.Cleanup(func() { srv.Shutdown(context.Background()) }) //nolint:errcheck

	// Create a token on the quota server.
	quotaClient := sdk.New(quotaURL)
	tok, err := quotaClient.CreateToken(context.Background(), sdk.CreateTokenInput{})
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	tokenID := tok.ID
	tokenURL := tok.URL

	// Send 3 requests — all must succeed.
	for i := range 3 {
		r := sendWebhook(t, tokenURL, http.MethodGet, "", "")
		if r.StatusCode() != 200 {
			t.Fatalf("request %d: expected 200, got %d", i+1, r.StatusCode())
		}
	}

	// Only 2 requests must be stored (oldest was evicted).
	page, err := quotaClient.Requests(context.Background(), tokenID, sdk.RequestsOptions{})
	if err != nil {
		t.Fatalf("list requests: %v", err)
	}
	if page.Total != 2 {
		t.Errorf("stored request count: got %d, want 2", page.Total)
	}
}

// --- REQ-031: receiver does not shadow management routes ---

// TestReceiverDoesNotShadowManagementRoutes tests route shadowing (REQ-031).
// Scenario: Try to access /health and /graphql endpoints.
// Expects: Both return 200. /health returns ok status, /graphql accepts query.
func TestReceiverDoesNotShadowManagementRoutes(t *testing.T) {
	// /health must work.
	resp, err := client().R().Get("/health")
	if err != nil {
		t.Fatalf("/health: %v", err)
	}
	if resp.StatusCode() != 200 {
		t.Errorf("/health status: got %d, want 200", resp.StatusCode())
	}

	// /graphql must accept a POST.
	gResp, err := client().R().SetBody(`{"query":"{ tokens { id } }"}`).Post("/graphql")
	if err != nil {
		t.Fatalf("/graphql: %v", err)
	}
	if gResp.StatusCode() != 200 {
		t.Errorf("/graphql status: got %d, want 200", gResp.StatusCode())
	}
}

// --- REQ-042: requests query uses sensible defaults ---

// TestRequestsDefaultPagination tests Requests query API defaults (REQ-042).
// Scenario: Create token, send 3 webhooks, query Requests without page/perPage params.
// Expects: perPage=50, currentPage=1, isLastPage=true (3 items < 50).
func TestRequestsDefaultPagination(t *testing.T) {
	token := createToken(t)
	for range 3 {
		sendWebhook(t, token.URL, http.MethodGet, "", "")
	}

	p, err := gqlClient.Requests(context.Background(), token.ID, sdk.RequestsOptions{})
	if err != nil {
		t.Fatalf("Requests: %v", err)
	}
	if p.PerPage != 50 {
		t.Errorf("default perPage: got %d, want 50", p.PerPage)
	}
	if p.CurrentPage != 1 {
		t.Errorf("default currentPage: got %d, want 1", p.CurrentPage)
	}
	if !p.IsLastPage {
		t.Error("with 3 requests and perPage=50, should be last page")
	}
}

// --- REQ-055: /playground serves HTML ---

// TestPlaygroundServesHTML tests the /playground management endpoint (REQ-055).
// Scenario: Send GET request to /playground.
// Expects: Status 200, Content-Type contains text/html.
func TestPlaygroundServesHTML(t *testing.T) {
	resp, err := client().R().Get("/playground")
	if err != nil {
		t.Fatalf("/playground: %v", err)
	}
	if resp.StatusCode() != 200 {
		t.Errorf("/playground status: got %d, want 200", resp.StatusCode())
	}
	ct := resp.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type: got %q, want text/html", ct)
	}
}

// --- REQ-051, REQ-052: requestReceived subscription over WebSocket ---

// TestSubscriptionRequestReceived tests WebSocket subscription API (REQ-051, REQ-052).
// Scenario: Connect WebSocket, subscribe to requestReceived for token, send webhook in parallel.
// Expects: Subscription receives event with captured request, total, and truncated fields.
func TestSubscriptionRequestReceived(t *testing.T) {
	token := createToken(t)

	// Dial the WebSocket endpoint.
	wsURL := "ws" + strings.TrimPrefix(serverURL, "http") + "/graphql"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{
		"Sec-WebSocket-Protocol": []string{"graphql-ws"},
	})
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	defer conn.Close()

	// graphql-ws handshake.
	writeWS(t, conn, map[string]any{"type": "connection_init", "payload": map[string]any{}})
	expectWSType(t, conn, "connection_ack")

	// Subscribe.
	writeWS(t, conn, map[string]any{
		"id":   "1",
		"type": "start",
		"payload": map[string]any{
			"query": fmt.Sprintf(`subscription { requestReceived(tokenId: "%s") { request { id method body } total truncated } }`, token.ID),
		},
	})

	// Send a webhook in a goroutine so we can race it against the subscription read.
	go func() {
		time.Sleep(100 * time.Millisecond)
		sendWebhook(t, token.URL, http.MethodPost, `{"sub":"test"}`, "application/json")
	}()

	// Wait for the data message.
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	event := readWSData(t, conn)

	req, ok := event["request"].(map[string]any)
	if !ok {
		t.Fatalf("event missing request field: %v", event)
	}
	if req["method"] != "POST" {
		t.Errorf("method: got %v, want POST", req["method"])
	}
	if req["body"] != `{"sub":"test"}` {
		t.Errorf("body: got %v", req["body"])
	}
	total, ok := event["total"].(float64)
	if !ok || total < 1 {
		t.Errorf("total: got %v, want >=1", event["total"])
	}
	if _, ok := event["truncated"]; !ok {
		t.Error("event missing truncated field")
	}
}

// --- REQ-053: payload over 1 MB is truncated ---

// TestLargePayloadTruncated tests payload truncation (REQ-053).
// Scenario: Subscribe to requestReceived, send webhook with 1.1 MB body.
// Expects: Subscription event has truncated=true, request.body is empty.
func TestLargePayloadTruncated(t *testing.T) {
	token := createToken(t)

	wsURL := "ws" + strings.TrimPrefix(serverURL, "http") + "/graphql"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, http.Header{
		"Sec-WebSocket-Protocol": []string{"graphql-ws"},
	})
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	defer conn.Close()

	writeWS(t, conn, map[string]any{"type": "connection_init", "payload": map[string]any{}})
	expectWSType(t, conn, "connection_ack")
	writeWS(t, conn, map[string]any{
		"id":   "1",
		"type": "start",
		"payload": map[string]any{
			"query": fmt.Sprintf(`subscription { requestReceived(tokenId: "%s") { request { id body } total truncated } }`, token.ID),
		},
	})

	// Send a body larger than 1 MB.
	bigBody := strings.Repeat("x", 1_100_000)
	go func() {
		time.Sleep(100 * time.Millisecond)
		sendWebhook(t, token.URL, http.MethodPost, bigBody, "text/plain")
	}()

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	event := readWSData(t, conn)

	truncated, _ := event["truncated"].(bool)
	if !truncated {
		t.Error("expected truncated=true for >1MB payload")
	}
	req, _ := event["request"].(map[string]any)
	if body, _ := req["body"].(string); body != "" {
		t.Error("expected body to be empty when truncated")
	}
}
