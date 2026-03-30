package script

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/dop251/goja"
	"github.com/wricardo/ai-http-bin/internal/store"
)

// Response is the result of a script execution.
type Response struct {
	Status      int
	Body        string
	ContentType string
	Headers     map[string]string
}

// Run executes a JS script for an incoming request.
// The script has access to:
//   - request: { method, path, query, body, headers, formData }
//   - store(key, value)  — persist a global variable
//   - load(key)          — read a global variable (returns "" if missing)
//   - del(key)           — delete a global variable
//   - respond(status, body, contentType, headers) — set the response (optional headers map)
//
// If the script does not call respond(), defaults are used.
func Run(src string, req *store.Request, s *store.Store) (*Response, error) {
	vm := goja.New()
	vm.SetFieldNameMapper(goja.TagFieldNameMapper("json", true))

	// Timeout: kill the script after 2 seconds
	done := make(chan struct{})
	go func() {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			vm.Interrupt("script timeout (2s)")
		}
	}()
	defer close(done)

	result := &Response{
		Status:      200,
		Body:        "",
		ContentType: "application/json",
		Headers:     map[string]string{},
	}

	// Expose request object
	queryMap := map[string]string{}
	_ = json.Unmarshal([]byte(req.Query), &queryMap)
	headersMap := map[string]string{}
	_ = json.Unmarshal([]byte(req.Headers), &headersMap)
	formMap := map[string]string{}
	_ = json.Unmarshal([]byte(req.FormData), &formMap)

	reqObj := map[string]any{
		"method":   req.Method,
		"path":     req.Path,
		"body":     req.Body,
		"query":    queryMap,
		"headers":  headersMap,
		"formData": formMap,
	}
	if err := vm.Set("request", reqObj); err != nil {
		return nil, fmt.Errorf("set request: %w", err)
	}

	// store(key, value) — persist global var
	if err := vm.Set("store", func(key, value string) {
		s.SetGlobalVar(key, value)
	}); err != nil {
		return nil, fmt.Errorf("set store: %w", err)
	}

	// load(key) — read global var, returns "" if missing
	if err := vm.Set("load", func(key string) string {
		v, _ := s.GetGlobalVar(key)
		return v
	}); err != nil {
		return nil, fmt.Errorf("set load: %w", err)
	}

	// del(key) — delete global var
	if err := vm.Set("del", func(key string) {
		s.DeleteGlobalVar(key)
	}); err != nil {
		return nil, fmt.Errorf("set del: %w", err)
	}

	// respond(status, body, contentType?, headers?)
	if err := vm.Set("respond", func(status int, body string, args ...any) {
		result.Status = status
		result.Body = body
		if len(args) > 0 {
			if ct, ok := args[0].(string); ok && ct != "" {
				result.ContentType = ct
			}
		}
		if len(args) > 1 {
			if hmap, ok := args[1].(map[string]any); ok {
				for k, v := range hmap {
					if sv, ok := v.(string); ok {
						result.Headers[k] = sv
					}
				}
			}
		}
	}); err != nil {
		return nil, fmt.Errorf("set respond: %w", err)
	}

	// JSON helpers
	if err := vm.Set("JSON", map[string]any{
		"stringify": func(v any) string {
			b, _ := json.Marshal(v)
			return string(b)
		},
		"parse": func(s string) any {
			var v any
			_ = json.Unmarshal([]byte(s), &v)
			return v
		},
	}); err != nil {
		return nil, fmt.Errorf("set JSON: %w", err)
	}

	if _, err := vm.RunString(src); err != nil {
		return nil, fmt.Errorf("script error: %w", err)
	}

	return result, nil
}
