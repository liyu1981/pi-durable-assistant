package httpapi

import (
	"net/http"
	"testing"
)

func TestLLMSettingsRoutes(t *testing.T) {
	client := newTestServer(t)

	status, body := client.do("GET", "/api/settings/llm", nil)
	if status != http.StatusOK || body["model"] != "test-model" || body["apiKeySet"] != true {
		t.Fatalf("get settings: %d %v", status, body)
	}
	if _, leaked := body["apiKey"]; leaked {
		t.Fatal("the stored API key must never be returned to the browser")
	}

	// Sending only one field keeps the stored secret and everything else.
	status, body = client.do("PUT", "/api/settings/llm", map[string]any{"model": "new-model"})
	if status != http.StatusOK || body["model"] != "new-model" || body["apiKeySet"] != true {
		t.Fatalf("partial update: %d %v", status, body)
	}
	status, body = client.do("GET", "/api/info", nil)
	if status != http.StatusOK || body["model"] != "new-model" {
		t.Fatalf("info must show the active model: %d %v", status, body)
	}

	// The test endpoint checks a candidate without persisting it.
	status, body = client.do("POST", "/api/settings/llm/test", map[string]any{"model": "candidate"})
	if status != http.StatusOK || body["ok"] != true {
		t.Fatalf("connection test: %d %v", status, body)
	}
	status, body = client.do("GET", "/api/settings/llm", nil)
	if body["model"] != "new-model" {
		t.Fatalf("test changed saved settings: %v", body)
	}

	// Invalid input is rejected before touching the runtime.
	status, _ = client.do("PUT", "/api/settings/llm", map[string]any{"api": "bogus"})
	if status != http.StatusBadRequest {
		t.Fatalf("invalid API must be a 400, got %d", status)
	}
	status, body = client.do("PUT", "/api/settings/llm", map[string]any{"apiKey": ""})
	if status != http.StatusOK || body["apiKeySet"] != false {
		t.Fatalf("clearing the key: %d %v", status, body)
	}
}
