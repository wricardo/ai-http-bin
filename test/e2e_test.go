package test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-resty/resty/v2"
	"github.com/wricardo/ai-http-bin/internal/server"
)

// serverURL is set once in TestMain and used by all tests.
var serverURL string

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintf(os.Stderr, "listen: %v\n", err)
		os.Exit(1)
	}

	serverURL = fmt.Sprintf("http://%s", ln.Addr().String())

	srv := server.New(serverURL)
	go srv.Serve(ln) //nolint:errcheck

	code := m.Run()

	srv.Shutdown(context.Background()) //nolint:errcheck
	os.Exit(code)
}

// --- client helpers ---

func client() *resty.Client {
	return resty.New().
		SetBaseURL(serverURL).
		SetHeader("Content-Type", "application/json").
		SetHeader("Accept", "application/json")
}

type gqlRequest struct {
	Query     string `json:"query"`
	Variables any    `json:"variables,omitempty"`
}

type gqlResponse[T any] struct {
	Data   T          `json:"data"`
	Errors []gqlError `json:"errors,omitempty"`
}

type gqlError struct {
	Message string `json:"message"`
}

func doGQL[T any](t *testing.T, query string, variables any) T {
	t.Helper()

	body, err := json.Marshal(gqlRequest{Query: query, Variables: variables})
	if err != nil {
		t.Fatalf("marshal gql request: %v", err)
	}

	resp, err := client().R().SetBody(body).Post("/graphql")
	if err != nil {
		t.Fatalf("gql request: %v", err)
	}
	if resp.StatusCode() != 200 {
		t.Fatalf("unexpected status %d: %s", resp.StatusCode(), resp.String())
	}

	var result gqlResponse[T]
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("gql errors: %v", result.Errors)
	}

	return result.Data
}

// --- shared types ---

type Token struct {
	ID                 string    `json:"id"`
	URL                string    `json:"url"`
	CreatedAt          string    `json:"createdAt"`
	RequestCount       int       `json:"requestCount"`
	DefaultStatus      int       `json:"defaultStatus"`
	DefaultContent     string    `json:"defaultContent"`
	DefaultContentType string    `json:"defaultContentType"`
	Timeout            int       `json:"timeout"`
	Cors               bool      `json:"cors"`
	Requests           []Request `json:"requests"`
}

type Request struct {
	ID          string `json:"id"`
	TokenID     string `json:"tokenId"`
	Method      string `json:"method"`
	URL         string `json:"url"`
	Hostname    string `json:"hostname"`
	Path        string `json:"path"`
	Headers     string `json:"headers"`
	Query       string `json:"query"`
	Body        string `json:"body"`
	IP          string `json:"ip"`
	UserAgent   string `json:"userAgent"`
	CreatedAt   string `json:"createdAt"`
}

type RequestPage struct {
	Data        []Request `json:"data"`
	Total       int       `json:"total"`
	PerPage     int       `json:"perPage"`
	CurrentPage int       `json:"currentPage"`
	IsLastPage  bool      `json:"isLastPage"`
	From        int       `json:"from"`
	To          int       `json:"to"`
}

// --- fixtures ---

const tokenFields = `
	id url createdAt requestCount
	defaultStatus defaultContent defaultContentType
	timeout cors
`

const requestFields = `
	id tokenId method url hostname path
	headers query body ip userAgent createdAt
`

// createToken creates a token with default settings and fails the test on error.
func createToken(t *testing.T) Token {
	t.Helper()
	type data struct {
		CreateToken Token `json:"createToken"`
	}
	return doGQL[data](t, `mutation { createToken {`+tokenFields+`} }`, nil).CreateToken
}

// sendWebhook posts to the token's URL and returns the resty response.
func sendWebhook(t *testing.T, tokenURL, method, body, contentType string) *resty.Response {
	t.Helper()
	req := resty.New().R().SetBody(body)
	if contentType != "" {
		req.SetHeader("Content-Type", contentType)
	}
	resp, err := req.Execute(method, tokenURL)
	if err != nil {
		t.Fatalf("sendWebhook: %v", err)
	}
	return resp
}

// --- tests ---

func TestCreateToken(t *testing.T) {
	token := createToken(t)

	if token.ID == "" {
		t.Error("expected non-empty id")
	}
	if token.URL == "" {
		t.Error("expected non-empty url")
	}
	if token.CreatedAt == "" {
		t.Error("expected non-empty createdAt")
	}
	if token.RequestCount != 0 {
		t.Errorf("requestCount: got %d, want 0", token.RequestCount)
	}
	if token.DefaultStatus != 200 {
		t.Errorf("defaultStatus: got %d, want 200", token.DefaultStatus)
	}
	if token.DefaultContentType != "text/plain" {
		t.Errorf("defaultContentType: got %q, want text/plain", token.DefaultContentType)
	}
	if token.Cors {
		t.Error("cors should default to false")
	}

	t.Logf("token id=%s url=%s", token.ID, token.URL)
}

func TestCreateTokenWithCustomResponse(t *testing.T) {
	type data struct {
		CreateToken Token `json:"createToken"`
	}
	result := doGQL[data](t, `
		mutation {
			createToken(
				defaultStatus: 201
				defaultContent: "hello"
				defaultContentType: "application/json"
				timeout: 0
				cors: true
			) {`+tokenFields+`}
		}
	`, nil)

	token := result.CreateToken

	if token.DefaultStatus != 201 {
		t.Errorf("defaultStatus: got %d, want 201", token.DefaultStatus)
	}
	if token.DefaultContent != "hello" {
		t.Errorf("defaultContent: got %q, want hello", token.DefaultContent)
	}
	if token.DefaultContentType != "application/json" {
		t.Errorf("defaultContentType: got %q, want application/json", token.DefaultContentType)
	}
	if !token.Cors {
		t.Error("cors should be true")
	}
}

func TestGetToken(t *testing.T) {
	created := createToken(t)

	type data struct {
		Token Token `json:"token"`
	}
	result := doGQL[data](t, `query($id: ID!) { token(id: $id) {`+tokenFields+`} }`,
		map[string]any{"id": created.ID})

	if result.Token.ID != created.ID {
		t.Errorf("token id: got %q, want %q", result.Token.ID, created.ID)
	}
}

func TestGetTokenNotFound(t *testing.T) {
	type data struct {
		Token *Token `json:"token"`
	}
	result := doGQL[data](t, `query { token(id: "00000000-0000-0000-0000-000000000000") {`+tokenFields+`} }`, nil)
	if result.Token != nil {
		t.Error("expected nil token for unknown id")
	}
}

func TestListTokens(t *testing.T) {
	// Create two tokens so the list is non-empty.
	a := createToken(t)
	b := createToken(t)

	type data struct {
		Tokens []Token `json:"tokens"`
	}
	result := doGQL[data](t, `query { tokens {`+tokenFields+`} }`, nil)

	ids := make(map[string]bool)
	for _, tok := range result.Tokens {
		ids[tok.ID] = true
	}
	if !ids[a.ID] {
		t.Errorf("token %s missing from list", a.ID)
	}
	if !ids[b.ID] {
		t.Errorf("token %s missing from list", b.ID)
	}
}

func TestUpdateToken(t *testing.T) {
	token := createToken(t)

	type data struct {
		UpdateToken Token `json:"updateToken"`
	}
	result := doGQL[data](t, `
		mutation($id: ID!) {
			updateToken(
				id: $id
				defaultStatus: 404
				defaultContent: "not here"
				defaultContentType: "text/html"
				cors: true
			) {`+tokenFields+`}
		}
	`, map[string]any{"id": token.ID})

	updated := result.UpdateToken
	if updated.DefaultStatus != 404 {
		t.Errorf("defaultStatus: got %d, want 404", updated.DefaultStatus)
	}
	if updated.DefaultContent != "not here" {
		t.Errorf("defaultContent: got %q, want 'not here'", updated.DefaultContent)
	}
	if updated.DefaultContentType != "text/html" {
		t.Errorf("defaultContentType: got %q, want text/html", updated.DefaultContentType)
	}
	if !updated.Cors {
		t.Error("cors should be true after update")
	}
}

func TestToggleCors(t *testing.T) {
	token := createToken(t) // cors starts false

	type data struct {
		ToggleCors bool `json:"toggleCors"`
	}

	// Toggle on.
	r1 := doGQL[data](t, `mutation($id: ID!) { toggleCors(id: $id) }`, map[string]any{"id": token.ID})
	if !r1.ToggleCors {
		t.Error("expected cors=true after first toggle")
	}

	// Toggle off.
	r2 := doGQL[data](t, `mutation($id: ID!) { toggleCors(id: $id) }`, map[string]any{"id": token.ID})
	if r2.ToggleCors {
		t.Error("expected cors=false after second toggle")
	}
}

func TestDeleteToken(t *testing.T) {
	token := createToken(t)

	type deleteMutation struct {
		DeleteToken bool `json:"deleteToken"`
	}
	del := doGQL[deleteMutation](t, `mutation($id: ID!) { deleteToken(id: $id) }`,
		map[string]any{"id": token.ID})
	if !del.DeleteToken {
		t.Error("deleteToken should return true")
	}

	// Token should no longer exist.
	type query struct {
		Token *Token `json:"token"`
	}
	q := doGQL[query](t, `query($id: ID!) { token(id: $id) {`+tokenFields+`} }`,
		map[string]any{"id": token.ID})
	if q.Token != nil {
		t.Error("token should not exist after deletion")
	}
}

func TestWebhookReceivesRequest(t *testing.T) {
	token := createToken(t)

	resp := sendWebhook(t, token.URL, http.MethodPost, `{"event":"test"}`, "application/json")

	if resp.StatusCode() != 200 {
		t.Errorf("webhook status: got %d, want 200", resp.StatusCode())
	}
	if resp.Header().Get("X-Token-Id") != token.ID {
		t.Errorf("X-Token-Id: got %q, want %q", resp.Header().Get("X-Token-Id"), token.ID)
	}
	if resp.Header().Get("X-Request-Id") == "" {
		t.Error("X-Request-Id header should be set")
	}
}

func TestWebhookUnknownTokenReturns410(t *testing.T) {
	resp := sendWebhook(t, serverURL+"/00000000-0000-0000-0000-000000000000", http.MethodGet, "", "")
	if resp.StatusCode() != http.StatusGone {
		t.Errorf("expected 410, got %d", resp.StatusCode())
	}
}

func TestWebhookCustomResponse(t *testing.T) {
	type data struct {
		CreateToken Token `json:"createToken"`
	}
	result := doGQL[data](t, `
		mutation {
			createToken(
				defaultStatus: 202
				defaultContent: "accepted"
				defaultContentType: "text/plain"
			) {`+tokenFields+`}
		}
	`, nil)
	token := result.CreateToken

	resp := sendWebhook(t, token.URL, http.MethodPost, "ping", "text/plain")

	if resp.StatusCode() != 202 {
		t.Errorf("status: got %d, want 202", resp.StatusCode())
	}
	if string(resp.Body()) != "accepted" {
		t.Errorf("body: got %q, want 'accepted'", string(resp.Body()))
	}
	if ct := resp.Header().Get("Content-Type"); ct != "text/plain" {
		t.Errorf("Content-Type: got %q, want text/plain", ct)
	}
}

func TestWebhookStatusCodeOverrideViaPath(t *testing.T) {
	token := createToken(t) // defaultStatus=200

	resp := sendWebhook(t, token.URL+"/418", http.MethodGet, "", "")

	if resp.StatusCode() != 418 {
		t.Errorf("status: got %d, want 418", resp.StatusCode())
	}
}

func TestWebhookCORSHeaders(t *testing.T) {
	type data struct {
		CreateToken Token `json:"createToken"`
	}
	result := doGQL[data](t, `mutation { createToken(cors: true) {`+tokenFields+`} }`, nil)
	token := result.CreateToken

	resp := sendWebhook(t, token.URL, http.MethodGet, "", "")

	if resp.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("expected Access-Control-Allow-Origin: *")
	}
	if resp.Header().Get("Access-Control-Allow-Methods") != "*" {
		t.Error("expected Access-Control-Allow-Methods: *")
	}
}

func TestWebhookNoCORSHeadersWhenDisabled(t *testing.T) {
	token := createToken(t) // cors=false

	resp := sendWebhook(t, token.URL, http.MethodGet, "", "")

	if resp.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("CORS headers should not be present when cors=false")
	}
}

func TestGetRequest(t *testing.T) {
	token := createToken(t)
	sendWebhook(t, token.URL, http.MethodPost, `{"x":1}`, "application/json")

	// Fetch via GraphQL requests list to get the ID.
	type pageData struct {
		Requests RequestPage `json:"requests"`
	}
	page := doGQL[pageData](t, `
		query($tokenId: ID!) {
			requests(tokenId: $tokenId) {
				data {`+requestFields+`}
				total
			}
		}
	`, map[string]any{"tokenId": token.ID})

	if page.Requests.Total != 1 {
		t.Fatalf("expected 1 request, got %d", page.Requests.Total)
	}
	req := page.Requests.Data[0]

	// Now fetch by ID.
	type reqData struct {
		Request Request `json:"request"`
	}
	byID := doGQL[reqData](t, `query($id: ID!) { request(id: $id) {`+requestFields+`} }`,
		map[string]any{"id": req.ID})

	if byID.Request.ID != req.ID {
		t.Errorf("request id mismatch: got %q, want %q", byID.Request.ID, req.ID)
	}
	if byID.Request.Method != "POST" {
		t.Errorf("method: got %q, want POST", byID.Request.Method)
	}
	if byID.Request.Body != `{"x":1}` {
		t.Errorf("body: got %q, want {\"x\":1}", byID.Request.Body)
	}
	if byID.Request.TokenID != token.ID {
		t.Errorf("tokenId: got %q, want %q", byID.Request.TokenID, token.ID)
	}
}

func TestListRequestsPagination(t *testing.T) {
	token := createToken(t)

	// Send 5 requests.
	for i := range 5 {
		sendWebhook(t, token.URL, http.MethodPost, fmt.Sprintf(`{"i":%d}`, i), "application/json")
	}

	type pageData struct {
		Requests RequestPage `json:"requests"`
	}

	// Page 1, 2 per page.
	p1 := doGQL[pageData](t, `
		query($tokenId: ID!) {
			requests(tokenId: $tokenId, page: 1, perPage: 2) {
				data {`+requestFields+`}
				total perPage currentPage isLastPage from to
			}
		}
	`, map[string]any{"tokenId": token.ID}).Requests

	if p1.Total != 5 {
		t.Errorf("total: got %d, want 5", p1.Total)
	}
	if len(p1.Data) != 2 {
		t.Errorf("page 1 items: got %d, want 2", len(p1.Data))
	}
	if p1.IsLastPage {
		t.Error("page 1 should not be last page")
	}
	if p1.From != 1 || p1.To != 2 {
		t.Errorf("from/to: got %d/%d, want 1/2", p1.From, p1.To)
	}

	// Last page.
	p3 := doGQL[pageData](t, `
		query($tokenId: ID!) {
			requests(tokenId: $tokenId, page: 3, perPage: 2) {
				data {`+requestFields+`}
				total perPage currentPage isLastPage from to
			}
		}
	`, map[string]any{"tokenId": token.ID}).Requests

	if len(p3.Data) != 1 {
		t.Errorf("page 3 items: got %d, want 1", len(p3.Data))
	}
	if !p3.IsLastPage {
		t.Error("page 3 should be last page")
	}
}

func TestListRequestsSortingNewest(t *testing.T) {
	token := createToken(t)

	sendWebhook(t, token.URL, http.MethodPost, `{"order":"first"}`, "application/json")
	sendWebhook(t, token.URL, http.MethodPost, `{"order":"second"}`, "application/json")

	type pageData struct {
		Requests RequestPage `json:"requests"`
	}

	result := doGQL[pageData](t, `
		query($tokenId: ID!) {
			requests(tokenId: $tokenId, sorting: "newest") {
				data { body }
			}
		}
	`, map[string]any{"tokenId": token.ID}).Requests

	if len(result.Data) < 2 {
		t.Fatalf("expected at least 2 requests, got %d", len(result.Data))
	}
	if result.Data[0].Body != `{"order":"second"}` {
		t.Errorf("newest first: got body %q, want second", result.Data[0].Body)
	}
}

func TestDeleteRequest(t *testing.T) {
	token := createToken(t)
	sendWebhook(t, token.URL, http.MethodGet, "", "")

	type pageData struct {
		Requests RequestPage `json:"requests"`
	}
	page := doGQL[pageData](t, `
		query($tokenId: ID!) { requests(tokenId: $tokenId) { data { id } total } }
	`, map[string]any{"tokenId": token.ID}).Requests

	if page.Total != 1 {
		t.Fatalf("expected 1 request before delete, got %d", page.Total)
	}
	reqID := page.Data[0].ID

	type delData struct {
		DeleteRequest bool `json:"deleteRequest"`
	}
	del := doGQL[delData](t, `mutation($id: ID!) { deleteRequest(id: $id) }`,
		map[string]any{"id": reqID})
	if !del.DeleteRequest {
		t.Error("deleteRequest should return true")
	}

	// Confirm gone.
	after := doGQL[pageData](t, `
		query($tokenId: ID!) { requests(tokenId: $tokenId) { data { id } total } }
	`, map[string]any{"tokenId": token.ID}).Requests
	if after.Total != 0 {
		t.Errorf("expected 0 requests after delete, got %d", after.Total)
	}
}

func TestClearRequests(t *testing.T) {
	token := createToken(t)

	sendWebhook(t, token.URL, http.MethodGet, "", "")
	sendWebhook(t, token.URL, http.MethodGet, "", "")

	type clearData struct {
		ClearRequests bool `json:"clearRequests"`
	}
	cleared := doGQL[clearData](t, `mutation($tokenId: ID!) { clearRequests(tokenId: $tokenId) }`,
		map[string]any{"tokenId": token.ID})
	if !cleared.ClearRequests {
		t.Error("clearRequests should return true")
	}

	type pageData struct {
		Requests RequestPage `json:"requests"`
	}
	page := doGQL[pageData](t, `
		query($tokenId: ID!) { requests(tokenId: $tokenId) { data { id } total } }
	`, map[string]any{"tokenId": token.ID}).Requests
	if page.Total != 0 {
		t.Errorf("expected 0 requests after clear, got %d", page.Total)
	}
}

func TestDeleteTokenAlsoDeletesRequests(t *testing.T) {
	token := createToken(t)

	// Capture a few requests so there is something to delete.
	sendWebhook(t, token.URL, http.MethodPost, "first", "text/plain")
	sendWebhook(t, token.URL, http.MethodPost, "second", "text/plain")

	// Collect the request IDs before deletion.
	type pageData struct {
		Requests RequestPage `json:"requests"`
	}
	before := doGQL[pageData](t, `
		query($tokenId: ID!) { requests(tokenId: $tokenId) { data { id } total } }
	`, map[string]any{"tokenId": token.ID}).Requests

	if before.Total != 2 {
		t.Fatalf("expected 2 requests before delete, got %d", before.Total)
	}
	reqIDs := make([]string, len(before.Data))
	for i, r := range before.Data {
		reqIDs[i] = r.ID
	}

	// Delete the token.
	type delData struct {
		DeleteToken bool `json:"deleteToken"`
	}
	doGQL[delData](t, `mutation($id: ID!) { deleteToken(id: $id) }`, map[string]any{"id": token.ID})

	// Each individual request must no longer be retrievable.
	type reqData struct {
		Request *Request `json:"request"`
	}
	for _, id := range reqIDs {
		result := doGQL[reqData](t, `query($id: ID!) { request(id: $id) { id } }`,
			map[string]any{"id": id})
		if result.Request != nil {
			t.Errorf("request %s still exists after token deletion", id)
		}
	}
}

func TestHealthEndpoint(t *testing.T) {
	resp, err := client().R().Get("/health")
	if err != nil {
		t.Fatalf("health request: %v", err)
	}
	if resp.StatusCode() != 200 {
		t.Errorf("health status: got %d, want 200", resp.StatusCode())
	}

	var body map[string]string
	if err := json.Unmarshal(resp.Body(), &body); err != nil {
		t.Fatalf("unmarshal health: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("health body: got %v, want status=ok", body)
	}
}
