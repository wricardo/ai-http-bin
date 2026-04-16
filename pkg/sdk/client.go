package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-resty/resty/v2"
)

const (
	defaultBaseURL  = "https://ai-http-bin.ngrok.app"
	xAgentIDHeader  = "X-Agent-Id"
	graphqlPath     = "/graphql"
	defaultPage     = 1
	defaultPerPage  = 50
	defaultSortMode = "oldest"
)

// Option configures the Client.
type Option func(*Client)

// WithAgentID sets X-Agent-Id on every GraphQL request.
func WithAgentID(agentID string) Option {
	return func(c *Client) {
		c.agentID = agentID
	}
}

// WithRestyClient injects a preconfigured resty client.
func WithRestyClient(rc *resty.Client) Option {
	return func(c *Client) {
		if rc != nil {
			c.http = rc
		}
	}
}

// Client is an AI HTTP Bin GraphQL SDK client.
type Client struct {
	http     *resty.Client
	endpoint string
	agentID  string
}

// New creates a Client from a base URL (e.g. https://ai-http-bin.ngrok.app).
// If baseURL is empty, it defaults to the public hosted URL.
func New(baseURL string, opts ...Option) *Client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultBaseURL
	}
	endpoint := strings.TrimRight(baseURL, "/") + graphqlPath
	return NewWithEndpoint(endpoint, opts...)
}

// NewWithEndpoint creates a Client with a full GraphQL endpoint URL.
func NewWithEndpoint(endpoint string, opts ...Option) *Client {
	c := &Client{
		http:     resty.New(),
		endpoint: endpoint,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// SetAgentID updates X-Agent-Id for subsequent requests.
func (c *Client) SetAgentID(agentID string) {
	c.agentID = agentID
}

type graphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

type graphQLResponse[T any] struct {
	Data   *T             `json:"data"`
	Errors []GraphQLError `json:"errors"`
}

// GraphQLError is returned by the GraphQL API in the "errors" array.
type GraphQLError struct {
	Message    string         `json:"message"`
	Path       []any          `json:"path,omitempty"`
	Extensions map[string]any `json:"extensions,omitempty"`
}

// ErrorResponse describes an HTTP-level or GraphQL-level failure.
type ErrorResponse struct {
	StatusCode int
	Body       string
	Errors     []GraphQLError
}

func (e *ErrorResponse) Error() string {
	if len(e.Errors) > 0 {
		return fmt.Sprintf("graphql error: %s", e.Errors[0].Message)
	}
	return fmt.Sprintf("unexpected HTTP status: %d", e.StatusCode)
}

func (c *Client) do(ctx context.Context, query string, variables map[string]any, out any) error {
	payload := graphQLRequest{Query: query, Variables: variables}
	resp := graphQLResponse[json.RawMessage]{}

	req := c.http.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(payload).
		SetResult(&resp)

	if c.agentID != "" {
		req.SetHeader(xAgentIDHeader, c.agentID)
	}

	r, err := req.Post(c.endpoint)
	if err != nil {
		return err
	}
	if r.StatusCode() >= http.StatusBadRequest {
		return &ErrorResponse{StatusCode: r.StatusCode(), Body: r.String()}
	}
	if len(resp.Errors) > 0 {
		return &ErrorResponse{StatusCode: r.StatusCode(), Body: r.String(), Errors: resp.Errors}
	}
	if resp.Data == nil {
		return &ErrorResponse{StatusCode: r.StatusCode(), Body: r.String()}
	}

	return json.Unmarshal(*resp.Data, out)
}
