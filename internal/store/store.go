package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Token struct {
	ID                 string
	AgentID            string // empty = anonymous/guest
	IP                 string
	UserAgent          string
	CreatedAt          time.Time
	ExpiresAt          time.Time // zero means no expiry
	DefaultStatus      int
	DefaultContent     string
	DefaultContentType string
	Timeout            int
	Cors               bool
	Script             string // JS script; when set, overrides static response
}

type Request struct {
	ID        string
	TokenID   string
	Method    string
	URL       string
	Hostname  string
	Path      string
	Headers   string // JSON
	Query     string // JSON
	Body      string
	FormData  string // JSON, non-JSON requests only
	IP        string
	UserAgent string
	CreatedAt time.Time
}

type RequestEvent struct {
	Request   *Request
	Total     int
	Truncated bool
}

type Subscriber struct {
	TokenID string
	Ch      chan *RequestEvent
}

type Store struct {
	mu          sync.RWMutex
	tokens      map[string]*Token
	requests    map[string][]*Request // tokenID -> requests
	globalVars  map[string]string     // key -> value, shared across all tokens
	subscribers []*Subscriber
	subMu       sync.RWMutex

	persistencePath string

	// MaxRequestsPerToken is the FIFO eviction limit per token (default 50).
	// When a new request arrives and the count exceeds this, the oldest request is dropped.
	MaxRequestsPerToken int
	// TokenTTL is how long a token lives after creation (default 24h).
	// Zero disables expiry.
	TokenTTL time.Duration
}

func New() *Store {
	return &Store{
		tokens:              make(map[string]*Token),
		requests:            make(map[string][]*Request),
		globalVars:          make(map[string]string),
		MaxRequestsPerToken: 50,
		TokenTTL:            24 * time.Hour,
	}
}

func (s *Store) CreateToken(ip, userAgent, agentID string, noExpiry bool) *Token {
	now := time.Now()
	var expiresAt time.Time
	if s.TokenTTL > 0 && !noExpiry {
		expiresAt = now.Add(s.TokenTTL)
	}
	t := &Token{
		ID:                 uuid.New().String(),
		AgentID:            agentID,
		IP:                 ip,
		UserAgent:          userAgent,
		CreatedAt:          now,
		ExpiresAt:          expiresAt,
		DefaultStatus:      200,
		DefaultContent:     "",
		DefaultContentType: "text/plain",
	}
	s.mu.Lock()
	s.tokens[t.ID] = t
	s.requests[t.ID] = []*Request{}
	s.mu.Unlock()
	s.persistState()
	return t
}

func (s *Store) UpdateToken(id, defaultContent, defaultContentType string, defaultStatus, timeout int, cors bool) (*Token, bool) {
	s.mu.Lock()
	t, ok := s.tokens[id]
	if !ok {
		s.mu.Unlock()
		return nil, false
	}
	t.DefaultContent = defaultContent
	t.DefaultContentType = defaultContentType
	t.DefaultStatus = defaultStatus
	t.Timeout = timeout
	t.Cors = cors
	s.mu.Unlock()
	s.persistState()
	return t, true
}

func (s *Store) ToggleCors(id string) (bool, bool) {
	s.mu.Lock()
	t, ok := s.tokens[id]
	if !ok {
		s.mu.Unlock()
		return false, false
	}
	t.Cors = !t.Cors
	enabled := t.Cors
	s.mu.Unlock()
	s.persistState()
	return enabled, true
}

func (s *Store) GetToken(id string) (*Token, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tokens[id]
	if !ok {
		return nil, false
	}
	if !t.ExpiresAt.IsZero() && time.Now().After(t.ExpiresAt) {
		return nil, false
	}
	return t, true
}

func (s *Store) ListTokens() []*Token {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now()
	result := make([]*Token, 0, len(s.tokens))
	for _, t := range s.tokens {
		if t.ExpiresAt.IsZero() || now.Before(t.ExpiresAt) {
			result = append(result, t)
		}
	}
	return result
}

func (s *Store) DeleteToken(id string) bool {
	s.mu.Lock()
	if _, ok := s.tokens[id]; !ok {
		s.mu.Unlock()
		return false
	}
	delete(s.tokens, id)
	delete(s.requests, id)
	s.mu.Unlock()
	s.persistState()
	return true
}

// CleanupExpired deletes all tokens whose ExpiresAt is in the past, along with
// their captured requests. Safe to call from a background goroutine.
func (s *Store) CleanupExpired() {
	now := time.Now()
	changed := false
	s.mu.Lock()
	for id, t := range s.tokens {
		if !t.ExpiresAt.IsZero() && now.After(t.ExpiresAt) {
			delete(s.tokens, id)
			delete(s.requests, id)
			changed = true
		}
	}
	s.mu.Unlock()
	if changed {
		s.persistState()
	}
}

func (s *Store) AddRequest(req *Request) {
	req.ID = uuid.New().String()
	req.CreatedAt = time.Now()

	s.mu.Lock()
	s.requests[req.TokenID] = append(s.requests[req.TokenID], req)
	// FIFO eviction: drop the oldest request when the per-token cap is exceeded.
	if s.MaxRequestsPerToken > 0 && len(s.requests[req.TokenID]) > s.MaxRequestsPerToken {
		s.requests[req.TokenID] = s.requests[req.TokenID][1:]
	}
	total := len(s.requests[req.TokenID])
	s.mu.Unlock()
	s.persistState()

	event := buildEvent(req, total)

	s.subMu.RLock()
	for _, sub := range s.subscribers {
		if sub.TokenID == req.TokenID {
			select {
			case sub.Ch <- event:
			default:
			}
		}
	}
	s.subMu.RUnlock()
}

func buildEvent(req *Request, total int) *RequestEvent {
	ev := &RequestEvent{Request: req, Total: total}
	// Truncate large payloads before broadcast (>1MB)
	serialized := req.Body + req.Headers + req.UserAgent
	if len(serialized) > 1_000_000 {
		truncated := *req
		truncated.Body = ""
		truncated.Headers = ""
		truncated.UserAgent = ""
		ev.Request = &truncated
		ev.Truncated = true
	}
	return ev
}

func (s *Store) GetRequest(id string) (*Request, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, reqs := range s.requests {
		for _, r := range reqs {
			if r.ID == id {
				return r, true
			}
		}
	}
	return nil, false
}

func (s *Store) ListRequests(tokenID string, page, perPage int, newest bool) ([]*Request, int) {
	s.mu.RLock()
	all := make([]*Request, len(s.requests[tokenID]))
	copy(all, s.requests[tokenID])
	s.mu.RUnlock()

	total := len(all)
	if newest {
		for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
			all[i], all[j] = all[j], all[i]
		}
	}

	start := (page - 1) * perPage
	if start >= total {
		return []*Request{}, total
	}
	end := start + perPage
	if end > total {
		end = total
	}
	return all[start:end], total
}

func (s *Store) DeleteRequest(id string) bool {
	s.mu.Lock()
	for tokenID, reqs := range s.requests {
		for i, r := range reqs {
			if r.ID == id {
				s.requests[tokenID] = append(reqs[:i], reqs[i+1:]...)
				s.mu.Unlock()
				s.persistState()
				return true
			}
		}
	}
	s.mu.Unlock()
	return false
}

func (s *Store) ClearRequests(tokenID string) bool {
	s.mu.Lock()
	if _, ok := s.tokens[tokenID]; !ok {
		s.mu.Unlock()
		return false
	}
	s.requests[tokenID] = []*Request{}
	s.mu.Unlock()
	s.persistState()
	return true
}

func (s *Store) RequestCount(tokenID string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.requests[tokenID])
}

func (s *Store) Subscribe(tokenID string) *Subscriber {
	sub := &Subscriber{
		TokenID: tokenID,
		Ch:      make(chan *RequestEvent, 64),
	}
	s.subMu.Lock()
	s.subscribers = append(s.subscribers, sub)
	s.subMu.Unlock()
	return sub
}

func (s *Store) Unsubscribe(sub *Subscriber) {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	for i, existing := range s.subscribers {
		if existing == sub {
			s.subscribers = append(s.subscribers[:i], s.subscribers[i+1:]...)
			return
		}
	}
}

func (s *Store) ListTokensByAgent(agentID string) []*Token {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now()
	var result []*Token
	for _, t := range s.tokens {
		if t.AgentID == agentID && (t.ExpiresAt.IsZero() || now.Before(t.ExpiresAt)) {
			result = append(result, t)
		}
	}
	return result
}

func (s *Store) ClaimToken(tokenID, agentID string) (*Token, bool) {
	s.mu.Lock()
	t, ok := s.tokens[tokenID]
	if !ok {
		s.mu.Unlock()
		return nil, false
	}
	if t.AgentID != "" {
		s.mu.Unlock()
		return nil, false // already claimed
	}
	t.AgentID = agentID
	s.mu.Unlock()
	s.persistState()
	return t, true
}

func (s *Store) TokenURL(baseURL, tokenID string) string {
	return fmt.Sprintf("%s/%s", baseURL, tokenID)
}

func (s *Store) SetScript(tokenID, script string) bool {
	s.mu.Lock()
	t, ok := s.tokens[tokenID]
	if !ok {
		s.mu.Unlock()
		return false
	}
	t.Script = script
	s.mu.Unlock()
	s.persistState()
	return true
}

func (s *Store) SetGlobalVar(key, value string) {
	s.mu.Lock()
	s.globalVars[key] = value
	s.mu.Unlock()
	s.persistState()
}

func (s *Store) GetGlobalVar(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.globalVars[key]
	return v, ok
}

func (s *Store) DeleteGlobalVar(key string) {
	s.mu.Lock()
	delete(s.globalVars, key)
	s.mu.Unlock()
	s.persistState()
}

func (s *Store) ListGlobalVars() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]string, len(s.globalVars))
	for k, v := range s.globalVars {
		out[k] = v
	}
	return out
}

type persistedState struct {
	Version    int                   `json:"version"`
	Tokens     []*Token              `json:"tokens"`
	Requests   map[string][]*Request `json:"requests,omitempty"`
	GlobalVars map[string]string     `json:"globalVars,omitempty"`
}

// EnableTokenPersistence configures JSON-file persistence for store state.
// If the file already exists, it is loaded into memory at startup.
func (s *Store) EnableTokenPersistence(path string) error {
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	s.mu.Lock()
	s.persistencePath = path
	s.mu.Unlock()

	return s.loadState()
}

func (s *Store) loadState() error {
	s.mu.RLock()
	path := s.persistencePath
	s.mu.RUnlock()
	if path == "" {
		return nil
	}

	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var data persistedState
	if err := json.Unmarshal(b, &data); err != nil {
		return err
	}

	now := time.Now()
	tokens := make(map[string]*Token, len(data.Tokens))
	for _, t := range data.Tokens {
		if t == nil {
			continue
		}
		if !t.ExpiresAt.IsZero() && now.After(t.ExpiresAt) {
			continue
		}
		tokens[t.ID] = t
	}

	requests := make(map[string][]*Request, len(tokens))
	for tokenID := range tokens {
		stored := data.Requests[tokenID]
		if len(stored) == 0 {
			requests[tokenID] = []*Request{}
			continue
		}
		if s.MaxRequestsPerToken > 0 && len(stored) > s.MaxRequestsPerToken {
			stored = stored[len(stored)-s.MaxRequestsPerToken:]
		}
		copied := make([]*Request, 0, len(stored))
		for _, r := range stored {
			if r == nil {
				continue
			}
			copyReq := *r
			copied = append(copied, &copyReq)
		}
		requests[tokenID] = copied
	}

	globalVars := make(map[string]string, len(data.GlobalVars))
	for k, v := range data.GlobalVars {
		globalVars[k] = v
	}

	s.mu.Lock()
	s.tokens = tokens
	s.requests = requests
	s.globalVars = globalVars
	s.mu.Unlock()
	return nil
}

func (s *Store) persistState() {
	s.mu.RLock()
	path := s.persistencePath
	if path == "" {
		s.mu.RUnlock()
		return
	}
	data := persistedState{
		Version:    2,
		Tokens:     make([]*Token, 0, len(s.tokens)),
		Requests:   make(map[string][]*Request, len(s.requests)),
		GlobalVars: make(map[string]string, len(s.globalVars)),
	}
	for _, t := range s.tokens {
		copyToken := *t
		data.Tokens = append(data.Tokens, &copyToken)
	}
	for tokenID, reqs := range s.requests {
		copied := make([]*Request, 0, len(reqs))
		for _, r := range reqs {
			if r == nil {
				continue
			}
			copyReq := *r
			copied = append(copied, &copyReq)
		}
		data.Requests[tokenID] = copied
	}
	for k, v := range s.globalVars {
		data.GlobalVars[k] = v
	}
	s.mu.RUnlock()

	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}
