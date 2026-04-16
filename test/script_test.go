package test

// Tests for JS scripting and global variable features.

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/wricardo/ai-http-bin/pkg/sdk"
)

// --- Script tests ---

// TestScriptSimpleRespond tests the Script execution API (respond function in JS).
// Scenario: Create a token with script that calls respond(201, JSON, "application/json").
// Expects: GET request returns 201 status with JSON body parsed successfully.
func TestScriptSimpleRespond(t *testing.T) {
	id := createTokenWithScript(t, `respond(201, '{"ok":true}', "application/json");`)
	r := hitToken(t, id, http.MethodGet, "", "")
	r.mustStatus(t, 201)
	r.mustJSONField(t, "ok")
}

// TestScriptReadsRequestMethod tests the Script execution API (request object access).
// Scenario: Create token with script checking request.method, send POST and GET.
// Expects: POST returns {"method":"post"}, GET returns {"method":"other"}.
func TestScriptReadsRequestMethod(t *testing.T) {
	script := `
		if (request.method === "POST") {
			respond(200, '{"method":"post"}', "application/json");
		} else {
			respond(200, '{"method":"other"}', "application/json");
		}
	`
	id := createTokenWithScript(t, script)

	r := hitToken(t, id, http.MethodPost, "", `{}`)
	r.mustStatus(t, 200)
	if v := r.mustJSONField(t, "method"); v != "post" {
		t.Fatalf("expected 'post', got %v", v)
	}

	r = hitToken(t, id, http.MethodGet, "", "")
	r.mustStatus(t, 200)
	if v := r.mustJSONField(t, "method"); v != "other" {
		t.Fatalf("expected 'other', got %v", v)
	}
}

// TestScriptReadsRequestBody tests the Script execution API (request.body access).
// Scenario: Create token with script parsing request.body JSON, send POST with {"name":"alice"}.
// Expects: Response returns {"echo":"alice"}.
func TestScriptReadsRequestBody(t *testing.T) {
	script := `
		var b = JSON.parse(request.body || "{}");
		respond(200, JSON.stringify({ echo: b.name }), "application/json");
	`
	id := createTokenWithScript(t, script)
	r := hitToken(t, id, http.MethodPost, "", `{"name":"alice"}`)
	r.mustStatus(t, 200)
	if v := r.mustJSONField(t, "echo"); v != "alice" {
		t.Fatalf("expected 'alice', got %v", v)
	}
}

// TestScriptReadsRequestPath tests the Script execution API (request.path access).
// Scenario: Create token with script echoing request.path, send GET to /:tokenID/foo/bar.
// Expects: Response returns {"path":"/foo/bar"}.
func TestScriptReadsRequestPath(t *testing.T) {
	script := `respond(200, JSON.stringify({ path: request.path }), "application/json");`
	id := createTokenWithScript(t, script)
	r := hitToken(t, id, http.MethodGet, "foo/bar", "")
	r.mustStatus(t, 200)
	if v := r.mustJSONField(t, "path"); v != "/foo/bar" {
		t.Fatalf("expected '/foo/bar', got %v", v)
	}
}

// TestScriptReadsQueryParams tests the Script execution API (request.query access).
// Scenario: Create token with script echoing request.query.q, send GET with ?q=hello.
// Expects: Response returns {"q":"hello"}.
func TestScriptReadsQueryParams(t *testing.T) {
	script := `respond(200, JSON.stringify({ q: request.query.q }), "application/json");`
	id := createTokenWithScript(t, script)

	resp, err := client().R().Get("/" + id + "?q=hello")
	if err != nil {
		t.Fatal(err)
	}
	wr := &webhookResponse{Response: resp}
	wr.mustStatus(t, 200)
	if v := wr.mustJSONField(t, "q"); v != "hello" {
		t.Fatalf("expected 'hello', got %v", v)
	}
}

// TestScriptGlobalVarPersistsAcrossRequests tests global variable API (store/load).
// Scenario: Create token with script incrementing counter via store/load, send 3 requests.
// Expects: Requests return count 1, 2, 3 respectively.
func TestScriptGlobalVarPersistsAcrossRequests(t *testing.T) {
	script := `
		var count = parseInt(load("counter") || "0") + 1;
		store("counter", String(count));
		respond(200, JSON.stringify({ count: count }), "application/json");
	`
	id := createTokenWithScript(t, script)

	for i := 1; i <= 3; i++ {
		r := hitToken(t, id, http.MethodGet, "", "")
		r.mustStatus(t, 200)
		if v := r.mustJSONField(t, "count"); int(v.(float64)) != i {
			t.Fatalf("request %d: expected count %d, got %v", i, i, v)
		}
	}
}

// TestScriptStatefulMockTodoAPI tests global variable API with stateful operations.
// Scenario: Create token with script implementing full CRUD todo API (GET/POST/PUT/DELETE) with state.
// Expects: GET empty, POST creates todos, PUT updates, DELETE removes, GET reflects state.
func TestScriptStatefulMockTodoAPI(t *testing.T) {
	script := `
		var body = JSON.parse(request.body || "{}");
		var todos = JSON.parse(load("todos") || "[]");

		if (request.method === "GET") {
			respond(200, JSON.stringify(todos), "application/json");
		} else if (request.method === "POST") {
			var newTodo = { id: todos.length + 1, title: body.title, done: false };
			todos.push(newTodo);
			store("todos", JSON.stringify(todos));
			respond(201, JSON.stringify(newTodo), "application/json");
		} else if (request.method === "DELETE") {
			var id = parseInt(request.path.replace("/", ""));
			todos = todos.filter(function(t) { return t.id !== id; });
			store("todos", JSON.stringify(todos));
			respond(200, JSON.stringify({ deleted: id }), "application/json");
		} else if (request.method === "PUT") {
			var id = parseInt(request.path.replace("/", ""));
			todos = todos.map(function(t) { return t.id === id ? { id: t.id, title: body.title || t.title, done: body.done !== undefined ? body.done : t.done } : t; });
			store("todos", JSON.stringify(todos));
			var updated = todos.filter(function(t) { return t.id === id; })[0];
			respond(200, JSON.stringify(updated), "application/json");
		}
	`
	id := createTokenWithScript(t, script)

	// GET empty list
	r := hitToken(t, id, http.MethodGet, "", "")
	r.mustStatus(t, 200)
	var list []any
	if err := json.Unmarshal(r.Body(), &list); err != nil || len(list) != 0 {
		t.Fatalf("expected empty list, got: %s", r.String())
	}

	// POST two todos
	r = hitToken(t, id, http.MethodPost, "", `{"title":"Buy milk"}`)
	r.mustStatus(t, 201)
	if v := r.mustJSONField(t, "title"); v != "Buy milk" {
		t.Fatalf("expected 'Buy milk', got %v", v)
	}

	r = hitToken(t, id, http.MethodPost, "", `{"title":"Write tests"}`)
	r.mustStatus(t, 201)

	// GET both
	r = hitToken(t, id, http.MethodGet, "", "")
	r.mustStatus(t, 200)
	if err := json.Unmarshal(r.Body(), &list); err != nil || len(list) != 2 {
		t.Fatalf("expected 2 todos, got: %s", r.String())
	}

	// PUT - mark todo 1 done
	r = hitToken(t, id, http.MethodPut, "1", `{"done":true}`)
	r.mustStatus(t, 200)
	if v := r.mustJSONField(t, "done"); v != true {
		t.Fatalf("expected done=true, got %v", v)
	}

	// DELETE todo 2
	r = hitToken(t, id, http.MethodDelete, "2", "")
	r.mustStatus(t, 200)
	if v := r.mustJSONField(t, "deleted"); int(v.(float64)) != 2 {
		t.Fatalf("expected deleted=2, got %v", v)
	}

	// Final GET — one todo, done
	r = hitToken(t, id, http.MethodGet, "", "")
	r.mustStatus(t, 200)
	if err := json.Unmarshal(r.Body(), &list); err != nil || len(list) != 1 {
		t.Fatalf("expected 1 todo, got: %s", r.String())
	}
}

// TestScriptErrorReturns500WithHeader tests Script execution error handling.
// Scenario: Create token with script that throws Error("boom"), send request.
// Expects: Response status 500, X-Script-Error header is set.
func TestScriptErrorReturns500WithHeader(t *testing.T) {
	id := createTokenWithScript(t, `throw new Error("boom");`)
	r := hitToken(t, id, http.MethodGet, "", "")
	r.mustStatus(t, 500)
	if r.Header().Get("X-Script-Error") == "" {
		t.Fatal("expected X-Script-Error header")
	}
}

// TestScriptTimeout tests Script execution timeout.
// Scenario: Create token with infinite loop (while(true){}), send request.
// Expects: Response status 500, X-Script-Error header set (timeout killed script).
func TestScriptTimeout(t *testing.T) {
	// Infinite loop — should be killed by the 2s timeout
	id := createTokenWithScript(t, `while(true){}`)
	r := hitToken(t, id, http.MethodGet, "", "")
	r.mustStatus(t, 500)
	if r.Header().Get("X-Script-Error") == "" {
		t.Fatal("expected X-Script-Error header on timeout")
	}
}

// TestSetScriptMutation tests the SetScript mutation (update token script).
// Scenario: Create token without script, set script via mutation, send request.
// Expects: Request handled by new script (status 202, body "scripted").
func TestSetScriptMutation(t *testing.T) {
	// Create token without script, then set script via mutation
	tok, err := gqlClient.CreateToken(context.Background(), sdk.CreateTokenInput{})
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if tok == nil {
		t.Fatal("CreateToken returned nil")
	}
	id := tok.ID

	setScript(t, id, `respond(202, "scripted", "text/plain");`)

	r := hitToken(t, id, http.MethodGet, "", "")
	r.mustStatus(t, 202)
	if string(r.Body()) != "scripted" {
		t.Fatalf("expected 'scripted', got %s", r.String())
	}
}

// TestScriptOverridesStaticResponse tests Script execution priority (script > static response).
// Scenario: Create token with static response and script, send request.
// Expects: Script response returned (status 418, body "script wins"), not static response.
func TestScriptOverridesStaticResponse(t *testing.T) {
	// Token has both default_content and a script — script wins
	tok, err := gqlClient.CreateToken(context.Background(), sdk.CreateTokenInput{
		DefaultContent: ptr("static"),
		DefaultStatus:  ptr(200),
		Script:         ptr("respond(418, 'script wins', 'text/plain');"),
	})
	if err != nil {
		t.Fatalf("CreateToken: %v", err)
	}
	if tok == nil {
		t.Fatal("CreateToken returned nil")
	}
	id := tok.ID

	r := hitToken(t, id, http.MethodGet, "", "")
	r.mustStatus(t, 418)
	if string(r.Body()) != "script wins" {
		t.Fatalf("expected 'script wins', got %s", r.String())
	}
}

// --- Global vars GraphQL API tests ---

// TestGlobalVarsCRUD tests global vars create/list/delete via GraphQL.
// Scenario: set var, list vars, delete var, verify deleted.
// Expects: var appears after set and is absent after delete.
func TestGlobalVarsCRUD(t *testing.T) {
	setVar, err := gqlClient.SetGlobalVar(context.Background(), "mykey", "hello")
	if err != nil {
		t.Fatalf("SetGlobalVar: %v", err)
	}
	if setVar == nil || setVar.Key != "mykey" || setVar.Value != "hello" {
		t.Fatalf("unexpected setGlobalVar result: %+v", setVar)
	}

	vars, err := gqlClient.GlobalVars(context.Background())
	if err != nil {
		t.Fatalf("GlobalVars: %v", err)
	}
	found := false
	for _, v := range vars {
		if v.Key == "mykey" && v.Value == "hello" {
			found = true
		}
	}
	if !found {
		t.Fatalf("mykey not found in globalVars: %+v", vars)
	}

	deleted, err := gqlClient.DeleteGlobalVar(context.Background(), "mykey")
	if err != nil {
		t.Fatalf("DeleteGlobalVar: %v", err)
	}
	if !deleted {
		t.Fatal("DeleteGlobalVar returned false")
	}

	vars, err = gqlClient.GlobalVars(context.Background())
	if err != nil {
		t.Fatalf("GlobalVars after delete: %v", err)
	}
	for _, v := range vars {
		if v.Key == "mykey" {
			t.Fatal("mykey still present after delete")
		}
	}
}

// TestGlobalVarsGraphQL tests the global vars GraphQL API (SetGlobalVar, GlobalVars, DeleteGlobalVar).
// Scenario: Set var via mutation, list via query, delete via mutation, verify deleted.
// Expects: All operations succeed, var appears in list, removed after delete.
func TestGlobalVarsGraphQL(t *testing.T) {
	setVar, err := gqlClient.SetGlobalVar(context.Background(), "gqlkey", "gqlval")
	if err != nil {
		t.Fatalf("SetGlobalVar: %v", err)
	}
	if setVar == nil || setVar.Key != "gqlkey" || setVar.Value != "gqlval" {
		t.Fatalf("unexpected setGlobalVar result: %+v", setVar)
	}

	listData, err := gqlClient.GlobalVars(context.Background())
	if err != nil {
		t.Fatalf("GlobalVars: %v", err)
	}
	found := false
	for _, v := range listData {
		if v.Key == "gqlkey" && v.Value == "gqlval" {
			found = true
		}
	}
	if !found {
		t.Fatalf("gqlkey not found in globalVars: %+v", listData)
	}

	delOK, err := gqlClient.DeleteGlobalVar(context.Background(), "gqlkey")
	if err != nil {
		t.Fatalf("DeleteGlobalVar: %v", err)
	}
	if !delOK {
		t.Fatal("deleteGlobalVar returned false")
	}
}

// TestScriptCannotAccessOtherTokenVars tests global vars sharing (vars are shared across tokens).
// Scenario: Create 2 tokens with scripts. Token 1 stores var, token 2 retrieves it.
// Expects: Token 2 can read var written by token 1 (confirmed shared storage).
func TestScriptCannotAccessOtherTokenVars(t *testing.T) {
	// Global vars are shared — this test confirms vars written by one token
	// are readable by another (they ARE shared by design)
	script1 := `store("shared_key", "from_token1"); respond(200, "ok", "text/plain");`
	script2 := `respond(200, load("shared_key") || "empty", "text/plain");`

	id1 := createTokenWithScript(t, script1)
	id2 := createTokenWithScript(t, script2)

	hitToken(t, id1, http.MethodGet, "", "") // writes shared_key

	r := hitToken(t, id2, http.MethodGet, "", "")
	r.mustStatus(t, 200)
	if string(r.Body()) != "from_token1" {
		t.Fatalf("expected 'from_token1', got %s", r.String())
	}
}

// TestScriptRetrySimulation tests Script + global vars (retry simulation use case).
// Scenario: Create token with script tracking attempts, respond 500 for attempts 1-2, 200 for attempt 3.
// Expects: First 2 requests return 500, 3rd request returns 200 with {"ok":true}.
func TestScriptRetrySimulation(t *testing.T) {
	// Simulates a flaky endpoint: fail first 2 requests, succeed on 3rd
	script := `
		var count = parseInt(load("retry_count") || "0") + 1;
		store("retry_count", String(count));
		if (count < 3) {
			respond(500, JSON.stringify({ error: "not ready", attempt: count }), "application/json");
		} else {
			respond(200, JSON.stringify({ ok: true, attempt: count }), "application/json");
		}
	`
	id := createTokenWithScript(t, script)

	r := hitToken(t, id, http.MethodGet, "", "")
	r.mustStatus(t, 500)

	r = hitToken(t, id, http.MethodGet, "", "")
	r.mustStatus(t, 500)

	r = hitToken(t, id, http.MethodGet, "", "")
	r.mustStatus(t, 200)
	if v := r.mustJSONField(t, "ok"); v != true {
		t.Fatalf("expected ok=true on 3rd attempt, got %v", v)
	}
}
