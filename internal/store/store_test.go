package store

import (
	"path/filepath"
	"testing"
)

func TestCreateTokenNoExpiry(t *testing.T) {
	s := New()
	withExpiry := s.CreateToken("", "", "", false)
	if withExpiry.ExpiresAt.IsZero() {
		t.Fatalf("expected default token to have expiry")
	}

	noExpiry := s.CreateToken("", "", "", true)
	if !noExpiry.ExpiresAt.IsZero() {
		t.Fatalf("expected no-expiry token to have zero ExpiresAt, got %v", noExpiry.ExpiresAt)
	}
}

func TestTokenPersistenceRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tokens.json")

	s := New()
	if err := s.EnableTokenPersistence(path); err != nil {
		t.Fatalf("EnableTokenPersistence: %v", err)
	}

	tok := s.CreateToken("127.0.0.1", "ua", "", true)
	tok.DefaultStatus = 201
	tok.DefaultContent = "created"
	tok.DefaultContentType = "application/json"
	tok.Timeout = 2
	tok.Cors = true
	if _, ok := s.UpdateToken(tok.ID, tok.DefaultContent, tok.DefaultContentType, tok.DefaultStatus, tok.Timeout, tok.Cors); !ok {
		t.Fatalf("UpdateToken failed")
	}
	if !s.SetScript(tok.ID, "respond(200, 'ok', 'text/plain')") {
		t.Fatalf("SetScript failed")
	}
	if _, ok := s.ClaimToken(tok.ID, "agent-1"); !ok {
		t.Fatalf("ClaimToken failed")
	}
	s.SetGlobalVar("env", "dev")
	s.SetGlobalVar("region", "us-east-1")
	s.AddRequest(&Request{TokenID: tok.ID, Method: "POST", Path: "/hook", Body: `{"ok":true}`})
	s.AddRequest(&Request{TokenID: tok.ID, Method: "GET", Path: "/health"})

	s2 := New()
	if err := s2.EnableTokenPersistence(path); err != nil {
		t.Fatalf("EnableTokenPersistence(load): %v", err)
	}

	loaded, ok := s2.GetToken(tok.ID)
	if !ok {
		t.Fatalf("expected token %s to be loaded", tok.ID)
	}
	if loaded.AgentID != "agent-1" {
		t.Fatalf("agent id not persisted: got %q", loaded.AgentID)
	}
	if loaded.DefaultStatus != 201 || loaded.DefaultContent != "created" || loaded.DefaultContentType != "application/json" {
		t.Fatalf("default response fields not persisted: %+v", loaded)
	}
	if loaded.Script == "" {
		t.Fatalf("script was not persisted")
	}
	if !loaded.ExpiresAt.IsZero() {
		t.Fatalf("expected no-expiry token to remain non-expiring, got %v", loaded.ExpiresAt)
	}

	gv := s2.ListGlobalVars()
	if gv["env"] != "dev" || gv["region"] != "us-east-1" {
		t.Fatalf("global vars not persisted: %+v", gv)
	}

	reqs, total := s2.ListRequests(tok.ID, 1, 50, false)
	if total != 2 || len(reqs) != 2 {
		t.Fatalf("requests not persisted: total=%d len=%d", total, len(reqs))
	}
	if reqs[0].Method != "POST" || reqs[0].Body != `{"ok":true}` {
		t.Fatalf("unexpected first request after reload: %+v", reqs[0])
	}
	if reqs[1].Method != "GET" || reqs[1].Path != "/health" {
		t.Fatalf("unexpected second request after reload: %+v", reqs[1])
	}
}
