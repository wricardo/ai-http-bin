package test

// Tests for JS scripting and global variable features.

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/go-resty/resty/v2"
)

// --- helpers ---

func createTokenWithScript(t *testing.T, script string) string {
	t.Helper()
	type resp struct {
		CreateToken struct {
			ID string `json:"id"`
		} `json:"createToken"`
	}
	data := doGQL[resp](t, `mutation($s: String!) {
		createToken(script: $s) { id }
	}`, map[string]any{"s": script})
	id := data.CreateToken.ID
	if id == "" {
		t.Fatal("createToken returned empty id")
	}
	return id
}

func setScript(t *testing.T, tokenID, script string) {
	t.Helper()
	type resp struct {
		SetScript struct {
			ID     string `json:"id"`
			Script string `json:"script"`
		} `json:"setScript"`
	}
	data := doGQL[resp](t, `mutation($id: ID!, $s: String!) {
		setScript(id: $id, script: $s) { id script }
	}`, map[string]any{"id": tokenID, "s": script})
	if data.SetScript.ID != tokenID {
		t.Fatalf("setScript returned wrong id: %s", data.SetScript.ID)
	}
}

func hitToken(t *testing.T, tokenID, method, path, body string) *webhookResponse {
	t.Helper()
	req := client().R()
	if body != "" {
		req.SetBody(body).SetHeader("Content-Type", "application/json")
	}
	url := "/" + tokenID
	if path != "" {
		url += "/" + path
	}
	var resp *resty.Response
	var err error
	switch method {
	case http.MethodGet:
		resp, err = req.Get(url)
	case http.MethodPost:
		resp, err = req.Post(url)
	case http.MethodPut:
		resp, err = req.Put(url)
	case http.MethodDelete:
		resp, err = req.Delete(url)
	default:
		t.Fatalf("unsupported method: %s", method)
	}
	if err != nil {
		t.Fatalf("hit token: %v", err)
	}
	return &webhookResponse{Response: resp}
}

type webhookResponse struct {
	*resty.Response
}

func (w *webhookResponse) mustStatus(t *testing.T, code int) {
	t.Helper()
	if w.StatusCode() != code {
		t.Fatalf("expected status %d, got %d: %s", code, w.StatusCode(), w.String())
	}
}

func (w *webhookResponse) mustJSONField(t *testing.T, field string) any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(w.Body(), &m); err != nil {
		t.Fatalf("parse response JSON: %v", err)
	}
	v, ok := m[field]
	if !ok {
		t.Fatalf("field %q not found in response: %s", field, w.String())
	}
	return v
}

// --- Script tests ---

func TestScriptSimpleRespond(t *testing.T) {
	id := createTokenWithScript(t, `respond(201, '{"ok":true}', "application/json");`)
	r := hitToken(t, id, http.MethodGet, "", "")
	r.mustStatus(t, 201)
	r.mustJSONField(t, "ok")
}

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

func TestScriptReadsRequestPath(t *testing.T) {
	script := `respond(200, JSON.stringify({ path: request.path }), "application/json");`
	id := createTokenWithScript(t, script)
	r := hitToken(t, id, http.MethodGet, "foo/bar", "")
	r.mustStatus(t, 200)
	if v := r.mustJSONField(t, "path"); v != "/foo/bar" {
		t.Fatalf("expected '/foo/bar', got %v", v)
	}
}

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

func TestScriptErrorReturns500WithHeader(t *testing.T) {
	id := createTokenWithScript(t, `throw new Error("boom");`)
	r := hitToken(t, id, http.MethodGet, "", "")
	r.mustStatus(t, 500)
	if r.Header().Get("X-Script-Error") == "" {
		t.Fatal("expected X-Script-Error header")
	}
}

func TestScriptTimeout(t *testing.T) {
	// Infinite loop — should be killed by the 2s timeout
	id := createTokenWithScript(t, `while(true){}`)
	r := hitToken(t, id, http.MethodGet, "", "")
	r.mustStatus(t, 500)
	if r.Header().Get("X-Script-Error") == "" {
		t.Fatal("expected X-Script-Error header on timeout")
	}
}

func TestSetScriptMutation(t *testing.T) {
	// Create token without script, then set script via mutation
	type createResp struct {
		CreateToken struct{ ID string `json:"id"` } `json:"createToken"`
	}
	data := doGQL[createResp](t, `mutation { createToken { id } }`, nil)
	id := data.CreateToken.ID

	setScript(t, id, `respond(202, "scripted", "text/plain");`)

	r := hitToken(t, id, http.MethodGet, "", "")
	r.mustStatus(t, 202)
	if string(r.Body()) != "scripted" {
		t.Fatalf("expected 'scripted', got %s", r.String())
	}
}

func TestScriptOverridesStaticResponse(t *testing.T) {
	// Token has both default_content and a script — script wins
	type resp struct {
		CreateToken struct{ ID string `json:"id"` } `json:"createToken"`
	}
	data := doGQL[resp](t, `mutation {
		createToken(defaultContent: "static", defaultStatus: 200, script: "respond(418, 'script wins', 'text/plain');") { id }
	}`, nil)
	id := data.CreateToken.ID

	r := hitToken(t, id, http.MethodGet, "", "")
	r.mustStatus(t, 418)
	if string(r.Body()) != "script wins" {
		t.Fatalf("expected 'script wins', got %s", r.String())
	}
}

// --- Global vars REST API tests ---

func TestGlobalVarsRESTCRUD(t *testing.T) {
	c := client()

	// Set a var
	resp, err := c.R().SetBody(`{"value":"hello"}`).Put("/api/vars/mykey")
	if err != nil || resp.StatusCode() != 200 {
		t.Fatalf("set var failed: %v %s", err, resp.String())
	}

	// List vars
	resp, err = c.R().Get("/api/vars")
	if err != nil || resp.StatusCode() != 200 {
		t.Fatalf("list vars failed: %v %s", err, resp.String())
	}
	var vars []map[string]string
	if err := json.Unmarshal(resp.Body(), &vars); err != nil {
		t.Fatalf("parse vars: %v", err)
	}
	found := false
	for _, v := range vars {
		if v["key"] == "mykey" && v["value"] == "hello" {
			found = true
		}
	}
	if !found {
		t.Fatalf("mykey not found in vars: %s", resp.String())
	}

	// Delete var
	resp, err = c.R().Delete("/api/vars/mykey")
	if err != nil || resp.StatusCode() != 200 {
		t.Fatalf("delete var failed: %v %s", err, resp.String())
	}

	// Verify deleted
	resp, err = c.R().Get("/api/vars")
	if err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(resp.Body(), &vars)
	for _, v := range vars {
		if v["key"] == "mykey" {
			t.Fatal("mykey still present after delete")
		}
	}
}

func TestGlobalVarsGraphQL(t *testing.T) {
	type setResp struct {
		SetGlobalVar struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"setGlobalVar"`
	}
	data := doGQL[setResp](t, `mutation {
		setGlobalVar(key: "gqlkey", value: "gqlval") { key value }
	}`, nil)
	if data.SetGlobalVar.Key != "gqlkey" || data.SetGlobalVar.Value != "gqlval" {
		t.Fatalf("unexpected setGlobalVar result: %+v", data.SetGlobalVar)
	}

	type listResp struct {
		GlobalVars []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"globalVars"`
	}
	listData := doGQL[listResp](t, `query { globalVars { key value } }`, nil)
	found := false
	for _, v := range listData.GlobalVars {
		if v.Key == "gqlkey" && v.Value == "gqlval" {
			found = true
		}
	}
	if !found {
		t.Fatalf("gqlkey not found in globalVars: %+v", listData.GlobalVars)
	}

	type delResp struct {
		DeleteGlobalVar bool `json:"deleteGlobalVar"`
	}
	delData := doGQL[delResp](t, `mutation { deleteGlobalVar(key: "gqlkey") }`, nil)
	if !delData.DeleteGlobalVar {
		t.Fatal("deleteGlobalVar returned false")
	}
}

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
