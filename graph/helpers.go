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
	expiresAt := ""
	if !t.ExpiresAt.IsZero() {
		expiresAt = t.ExpiresAt.Format("2006-01-02T15:04:05Z07:00")
	}
	return &model.Token{
		ID:                 t.ID,
		AgentID:            agentID,
		URL:                s.TokenURL(baseURL, t.ID),
		IP:                 t.IP,
		UserAgent:          t.UserAgent,
		CreatedAt:          t.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		ExpiresAt:          expiresAt,
		RequestCount:       s.RequestCount(t.ID),
		DefaultStatus:      t.DefaultStatus,
		DefaultContent:     t.DefaultContent,
		DefaultContentType: t.DefaultContentType,
		Timeout:            t.Timeout,
		Cors:               t.Cors,
		Script:             t.Script,
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

// resolveTokenPatch computes the effective field values for t with any
// non-nil args overlaid, without mutating t. t's fields must only ever be
// written through Store methods (which hold the lock), never directly.
func resolveTokenPatch(t *store.Token, defaultStatus *int, defaultContent *string, defaultContentType *string, timeout *int, cors *bool) (content, contentType string, status, to int, c bool) {
	content, contentType, status, to, c = t.DefaultContent, t.DefaultContentType, t.DefaultStatus, t.Timeout, t.Cors
	if defaultStatus != nil {
		status = *defaultStatus
	}
	if defaultContent != nil {
		content = *defaultContent
	}
	if defaultContentType != nil {
		contentType = *defaultContentType
	}
	if timeout != nil {
		to = *timeout
	}
	if cors != nil {
		c = *cors
	}
	return
}
