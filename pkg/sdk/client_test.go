package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestToken(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/graphql" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"token":{"id":"tok-1","url":"http://example/tok-1","ip":"127.0.0.1","userAgent":"ua","createdAt":"now","expiresAt":"later","requestCount":0,"defaultStatus":200,"defaultContent":"","defaultContentType":"text/plain","timeout":0,"cors":false,"script":""}}}`))
	}))
	defer ts.Close()

	c := New(ts.URL)
	tok, err := c.Token(context.Background(), "tok-1")
	if err != nil {
		t.Fatalf("Token() error: %v", err)
	}
	if tok == nil || tok.ID != "tok-1" {
		t.Fatalf("unexpected token: %+v", tok)
	}
}

func TestAgentIDHeader(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Agent-Id"); got != "agent-123" {
			t.Fatalf("expected X-Agent-Id header, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"tokens":[]}}`))
	}))
	defer ts.Close()

	c := New(ts.URL, WithAgentID("agent-123"))
	if _, err := c.Tokens(context.Background()); err != nil {
		t.Fatalf("Tokens() error: %v", err)
	}
}

func TestGraphQLError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"errors": []map[string]any{{"message": "boom"}},
			"data":   nil,
		})
	}))
	defer ts.Close()

	c := New(ts.URL)
	_, err := c.Tokens(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if _, ok := err.(*ErrorResponse); !ok {
		t.Fatalf("expected *ErrorResponse, got %T", err)
	}
}
