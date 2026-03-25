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
)

// --- REQ-004, REQ-005: token captures creator IP and User-Agent ---

func TestTokenCapturesCreatorIPAndUserAgent(t *testing.T) {
	type tokenData struct {
		CreateToken struct {
			ID        string `json:"id"`
			IP        string `json:"ip"`
			UserAgent string `json:"userAgent"`
		} `json:"createToken"`
	}

	result := doGQL[tokenData](t, `
		mutation {
			createToken { id ip userAgent }
		}
	`, nil)

	tok := result.CreateToken
	if tok.IP == "" {
		t.Error("expected non-empty ip on created token")
	}
	// The test HTTP client doesn't set a custom UA, but the field must be present.
	t.Logf("token ip=%s userAgent=%q", tok.IP, tok.UserAgent)
}

// --- REQ-008: token.requests field returns associated requests ---

func TestTokenRequestsField(t *testing.T) {
	token := createToken(t)
	sendWebhook(t, token.URL, http.MethodPost, `{"a":1}`, "application/json")
	sendWebhook(t, token.URL, http.MethodPost, `{"a":2}`, "application/json")

	type tokenData struct {
		Token struct {
			ID       string    `json:"id"`
			Requests []Request `json:"requests"`
		} `json:"token"`
	}

	result := doGQL[tokenData](t, `
		query($id: ID!) {
			token(id: $id) {
				id
				requests { id method body }
			}
		}
	`, map[string]any{"id": token.ID})

	if len(result.Token.Requests) != 2 {
		t.Errorf("expected 2 requests on token, got %d", len(result.Token.Requests))
	}
}

// --- REQ-012, REQ-029: timeout delays response ---

func TestWebhookTimeout(t *testing.T) {
	type data struct {
		CreateToken struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		} `json:"createToken"`
	}
	result := doGQL[data](t, `mutation { createToken(timeout: 1) { id url } }`, nil)
	token := result.CreateToken

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

func TestRequestCapturesURLWithQueryString(t *testing.T) {
	token := createToken(t)

	sendWebhook(t, token.URL+"?foo=bar&n=42", http.MethodGet, "", "")

	req := firstRequest(t, token.ID)

	if !strings.Contains(req.URL, "foo=bar") {
		t.Errorf("url %q does not contain query string foo=bar", req.URL)
	}
}

// --- REQ-017: request captures hostname ---

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

func TestRequestCapturesSubPath(t *testing.T) {
	token := createToken(t)
	sendWebhook(t, token.URL+"/some/sub/path", http.MethodGet, "", "")

	req := firstRequest(t, token.ID)
	if req.Path != "/some/sub/path" {
		t.Errorf("path: got %q, want /some/sub/path", req.Path)
	}
}

// --- REQ-019: request captures all headers as JSON ---

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

	type formRequest struct {
		ID       string `json:"id"`
		FormData string `json:"formData"`
	}
	type pageData struct {
		Requests struct {
			Data []formRequest `json:"data"`
		} `json:"requests"`
	}
	page := doGQL[pageData](t, `
		query($tokenId: ID!) {
			requests(tokenId: $tokenId) { data { id formData } }
		}
	`, map[string]any{"tokenId": token.ID})

	if len(page.Requests.Data) == 0 {
		t.Fatal("no requests captured")
	}
	raw := page.Requests.Data[0].FormData

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

// --- REQ-028, REQ-057: per-token request quota returns 410 ---

func TestQuotaEnforcement(t *testing.T) {
	// Start a dedicated server with a quota of 2.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	quotaURL := fmt.Sprintf("http://%s", ln.Addr().String())
	srv := server.New(quotaURL, server.WithMaxRequestsPerToken(2))
	go srv.Serve(ln) //nolint:errcheck
	t.Cleanup(func() { srv.Shutdown(context.Background()) }) //nolint:errcheck

	// Create a token on the quota server.
	type data struct {
		CreateToken struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		} `json:"createToken"`
	}
	body, _ := json.Marshal(gqlRequest{Query: `mutation { createToken { id url } }`})
	resp, err := client().SetBaseURL(quotaURL).R().SetBody(body).Post("/graphql")
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	var result gqlResponse[data]
	json.Unmarshal(resp.Body(), &result) //nolint:errcheck
	tokenURL := result.Data.CreateToken.URL

	// Fill quota.
	for i := range 2 {
		r := sendWebhook(t, tokenURL, http.MethodGet, "", "")
		if r.StatusCode() != 200 {
			t.Fatalf("request %d: expected 200, got %d", i+1, r.StatusCode())
		}
	}

	// Next request must be 410.
	over := sendWebhook(t, tokenURL, http.MethodGet, "", "")
	if over.StatusCode() != http.StatusGone {
		t.Errorf("over-quota status: got %d, want 410", over.StatusCode())
	}
}

// --- REQ-031: receiver does not shadow management routes ---

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
	body, _ := json.Marshal(gqlRequest{Query: `query { tokens { id } }`})
	gResp, err := client().R().SetBody(body).Post("/graphql")
	if err != nil {
		t.Fatalf("/graphql: %v", err)
	}
	if gResp.StatusCode() != 200 {
		t.Errorf("/graphql status: got %d, want 200", gResp.StatusCode())
	}
}

// --- REQ-042: requests query uses sensible defaults ---

func TestRequestsDefaultPagination(t *testing.T) {
	token := createToken(t)
	for range 3 {
		sendWebhook(t, token.URL, http.MethodGet, "", "")
	}

	type pageData struct {
		Requests struct {
			PerPage     int  `json:"perPage"`
			CurrentPage int  `json:"currentPage"`
			IsLastPage  bool `json:"isLastPage"`
		} `json:"requests"`
	}

	// Call without pagination args.
	result := doGQL[pageData](t, `
		query($tokenId: ID!) {
			requests(tokenId: $tokenId) { perPage currentPage isLastPage }
		}
	`, map[string]any{"tokenId": token.ID})

	p := result.Requests
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

// --- WebSocket helpers ---

func writeWS(t *testing.T, conn *websocket.Conn, v any) {
	t.Helper()
	b, _ := json.Marshal(v)
	if err := conn.WriteMessage(websocket.TextMessage, b); err != nil {
		t.Fatalf("ws write: %v", err)
	}
}

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

// --- shared helper ---

func firstRequest(t *testing.T, tokenID string) Request {
	t.Helper()
	type pageData struct {
		Requests struct {
			Data []Request `json:"data"`
		} `json:"requests"`
	}
	page := doGQL[pageData](t, `
		query($tokenId: ID!) {
			requests(tokenId: $tokenId) {
				data {`+requestFields+`}
			}
		}
	`, map[string]any{"tokenId": tokenID})
	if len(page.Requests.Data) == 0 {
		t.Fatalf("no requests found for token %s", tokenID)
	}
	return page.Requests.Data[0]
}
