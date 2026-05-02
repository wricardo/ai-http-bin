package server

import (
	"strings"
	"testing"
)

func TestLLMsTxtDocumentsCurrentGraphQLShape(t *testing.T) {
	text := llmsTxt("https://example.test")

	if !strings.Contains(text, "LLM guide: https://example.test/ or https://example.test/llms.txt") {
		t.Fatalf("llms.txt should identify / and /llms.txt as the LLM guide")
	}
	if !strings.Contains(text, "requests(tokenId: $tokenId, sorting: \"newest\")") {
		t.Fatalf("llms.txt should include a concrete requests query example")
	}
	if !strings.Contains(text, "data {") {
		t.Fatalf("llms.txt should document RequestPage.data, not a non-existent requests field")
	}
	if strings.Contains(text, "API guide: https://example.test/") {
		t.Fatalf("llms.txt should not call / an API guide because / serves the LLM guide")
	}
}
