package graph

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/wricardo/ai-http-bin/graph/model"
	"github.com/wricardo/ai-http-bin/internal/store"
)

// GinContextKey is used to store *gin.Context in the GraphQL request context.
type GinContextKey struct{}

// GinContextFrom retrieves the *gin.Context injected by the server middleware.
func GinContextFrom(ctx context.Context) *gin.Context {
	gc, _ := ctx.Value(GinContextKey{}).(*gin.Context)
	return gc
}

// AgentIDFromContext extracts the X-Agent-Id header from the request context.
func AgentIDFromContext(ctx context.Context) string {
	gc := GinContextFrom(ctx)
	if gc == nil {
		return ""
	}
	return gc.GetHeader("X-Agent-Id")
}

func storeTokenToModel(s *store.Store, baseURL string, t *store.Token) *model.Token {
	var agentID *string
	if t.AgentID != "" {
		agentID = &t.AgentID
	}
	return &model.Token{
		ID:                 t.ID,
		AgentID:            agentID,
		URL:                s.TokenURL(baseURL, t.ID),
		IP:                 t.IP,
		UserAgent:          t.UserAgent,
		CreatedAt:          t.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		RequestCount:       s.RequestCount(t.ID),
		DefaultStatus:      t.DefaultStatus,
		DefaultContent:     t.DefaultContent,
		DefaultContentType: t.DefaultContentType,
		Timeout:            t.Timeout,
		Cors:               t.Cors,
	}
}

func storeRequestToModel(r *store.Request) *model.Request {
	return &model.Request{
		ID:        r.ID,
		TokenID:   r.TokenID,
		Method:    r.Method,
		URL:       r.URL,
		Hostname:  r.Hostname,
		Path:      r.Path,
		Headers:   r.Headers,
		Query:     r.Query,
		Body:      r.Body,
		FormData:  r.FormData,
		IP:        r.IP,
		UserAgent: r.UserAgent,
		CreatedAt: r.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// patchToken applies non-nil optional args onto an existing token.
func patchToken(t *store.Token, defaultStatus *int, defaultContent *string, defaultContentType *string, timeout *int, cors *bool) {
	if defaultStatus != nil {
		t.DefaultStatus = *defaultStatus
	}
	if defaultContent != nil {
		t.DefaultContent = *defaultContent
	}
	if defaultContentType != nil {
		t.DefaultContentType = *defaultContentType
	}
	if timeout != nil {
		t.Timeout = *timeout
	}
	if cors != nil {
		t.Cors = *cors
	}
}
