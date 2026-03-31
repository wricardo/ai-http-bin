package server

import (
	"context"
	"net/http"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/gin-gonic/gin"
	"github.com/wricardo/ai-http-bin/graph"
	"github.com/wricardo/ai-http-bin/internal/store"
	"github.com/wricardo/ai-http-bin/internal/webhook"
)

// Option configures the server.
type Option func(*store.Store)

// WithMaxRequestsPerToken sets the per-token FIFO eviction limit (default 50).
// When a token exceeds this count the oldest request is dropped to make room.
func WithMaxRequestsPerToken(n int) Option {
	return func(s *store.Store) { s.MaxRequestsPerToken = n }
}

// WithTokenTTL sets how long a token lives after creation (default 24h).
// Pass 0 to disable expiry entirely (useful for self-hosted deployments).
func WithTokenTTL(d time.Duration) Option {
	return func(s *store.Store) { s.TokenTTL = d }
}

// New builds an http.Server wired to the given baseURL.
// baseURL is the externally reachable address used to build token URLs (e.g. "http://localhost:8082").
func New(baseURL string, opts ...Option) *http.Server {
	s := store.New()
	for _, o := range opts {
		o(s)
	}

	// Background goroutine: sweep expired tokens every minute.
	if s.TokenTTL > 0 {
		go func() {
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			for range ticker.C {
				s.CleanupExpired()
			}
		}()
	}

	resolver := &graph.Resolver{
		Store:   s,
		BaseURL: baseURL,
	}

	gqlSrv := handler.New(graph.NewExecutableSchema(graph.Config{
		Resolvers: resolver,
	}))
	gqlSrv.AddTransport(transport.POST{})
	gqlSrv.AddTransport(transport.Websocket{})

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), graph.GinContextKey{}, c)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})

	// Spec homepage — markdown readable by AI agents
	r.GET("/", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/markdown; charset=utf-8", []byte(specMarkdown(baseURL)))
	})
	r.GET("/llms.txt", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(llmsTxt(baseURL)))
	})

	r.POST("/graphql", gin.WrapH(gqlSrv))
	r.GET("/graphql", gin.WrapH(gqlSrv))
	r.GET("/playground", gin.WrapH(playground.Handler("AI HTTP Bin GraphQL", "/graphql")))
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// REST API
	api := &restAPI{store: s, baseURL: baseURL}
	api.register(r.Group("/api"))

	webhook.NewHandler(s).Register(r)
	registerUI(r)

	return &http.Server{Handler: r}
}
