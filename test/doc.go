// Package test contains end-to-end tests for the ai-http-bin service.
//
// The tests are organized into three main groups:
//
// E2E tests (e2e_test.go) cover the core API: token lifecycle (create, read,
// update, delete), webhook capture, request retrieval, CORS handling, and health checks.
//
// Field and integration tests (e2e_fields_test.go) cover request field capture
// (headers, query params, form data, IP, User-Agent), timeouts, quotas with FIFO
// eviction, subscriptions over WebSocket, payload truncation, and route shadowing.
//
// Scripting tests (script_test.go) cover JavaScript execution, global variables,
// and mutation operations for dynamic responses.
//
// All tests run against a live test server started in TestMain.
package test
