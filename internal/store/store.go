package store

import (
	"fmt"
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
	DefaultStatus      int
	DefaultContent     string
	DefaultContentType string
	Timeout            int
	Cors               bool
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
	subscribers []*Subscriber
	subMu       sync.RWMutex

	MaxRequestsPerToken int
}

func New() *Store {
	return &Store{
		tokens:              make(map[string]*Token),
		requests:            make(map[string][]*Request),
		MaxRequestsPerToken: 500,
	}
}

func (s *Store) CreateToken(ip, userAgent, agentID string) *Token {
	t := &Token{
		ID:                 uuid.New().String(),
		AgentID:            agentID,
		IP:                 ip,
		UserAgent:          userAgent,
		CreatedAt:          time.Now(),
		DefaultStatus:      200,
		DefaultContent:     "",
		DefaultContentType: "text/plain",
	}
	s.mu.Lock()
	s.tokens[t.ID] = t
	s.requests[t.ID] = []*Request{}
	s.mu.Unlock()
	return t
}

func (s *Store) UpdateToken(id, defaultContent, defaultContentType string, defaultStatus, timeout int, cors bool) (*Token, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tokens[id]
	if !ok {
		return nil, false
	}
	t.DefaultContent = defaultContent
	t.DefaultContentType = defaultContentType
	t.DefaultStatus = defaultStatus
	t.Timeout = timeout
	t.Cors = cors
	return t, true
}

func (s *Store) ToggleCors(id string) (bool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tokens[id]
	if !ok {
		return false, false
	}
	t.Cors = !t.Cors
	return t.Cors, true
}

func (s *Store) GetToken(id string) (*Token, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tokens[id]
	return t, ok
}

func (s *Store) ListTokens() []*Token {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*Token, 0, len(s.tokens))
	for _, t := range s.tokens {
		result = append(result, t)
	}
	return result
}

func (s *Store) DeleteToken(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tokens[id]; !ok {
		return false
	}
	delete(s.tokens, id)
	delete(s.requests, id)
	return true
}

func (s *Store) IsOverQuota(tokenID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.requests[tokenID]) >= s.MaxRequestsPerToken
}

func (s *Store) AddRequest(req *Request) {
	req.ID = uuid.New().String()
	req.CreatedAt = time.Now()

	s.mu.Lock()
	s.requests[req.TokenID] = append(s.requests[req.TokenID], req)
	total := len(s.requests[req.TokenID])
	s.mu.Unlock()

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
	defer s.mu.Unlock()
	for tokenID, reqs := range s.requests {
		for i, r := range reqs {
			if r.ID == id {
				s.requests[tokenID] = append(reqs[:i], reqs[i+1:]...)
				return true
			}
		}
	}
	return false
}

func (s *Store) ClearRequests(tokenID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tokens[tokenID]; !ok {
		return false
	}
	s.requests[tokenID] = []*Request{}
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
	var result []*Token
	for _, t := range s.tokens {
		if t.AgentID == agentID {
			result = append(result, t)
		}
	}
	return result
}

func (s *Store) ClaimToken(tokenID, agentID string) (*Token, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tokens[tokenID]
	if !ok {
		return nil, false
	}
	if t.AgentID != "" {
		return nil, false // already claimed
	}
	t.AgentID = agentID
	return t, true
}

func (s *Store) TokenURL(baseURL, tokenID string) string {
	return fmt.Sprintf("%s/%s", baseURL, tokenID)
}
