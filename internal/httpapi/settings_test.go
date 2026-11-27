package httpapi

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/liyu1981/pi-durable-assistant/internal/config"
	"github.com/liyu1981/pi-durable-assistant/internal/runtime"
	"github.com/liyu1981/pi-durable-assistant/internal/settings"
)

func newTestServerWithModels(t *testing.T, models string) *testClient {
	t.Helper()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, models)
	}))
	t.Cleanup(provider.Close)
	cfg := config.Config{
		DataDir: t.TempDir(), Password: "secret",
	}
	rt, err := runtime.Open(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.ApplyLLM(context.Background(), settings.Config{
		API: "openai-completions", BaseURL: provider.URL + "/v1",
		APIKey: "test-key", Model: "test-model",
		ContextWindow: 128000, MaxTokens: 8192,
	}); err != nil {
		t.Fatal(err)
	}
	assets := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html></html>")}}
	server, err := NewServer(rt, fs.FS(assets))
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server)
	t.Cleanup(httpServer.Close)
	return &testClient{t: t, base: httpServer.URL, client: &http.Client{}, confirmations: rt.Confirmations}
}

func TestLLMModelsRoute(t *testing.T) {
	client := newTestServerWithModels(t, `{"data":[{"id":"gpt-4o-mini"},{"id":"gpt-4.1"}]}`)

	// Only the base URL and an optional key are needed to discover models.
	status, body := client.do("POST", "/api/settings/llm/models", map[string]any{
		"apiKey": "candidate-key",
	})
	if status != http.StatusOK || body["ok"] != true {
		t.Fatalf("models: %d %v", status, body)
	}
	models, ok := body["models"].([]any)
	if !ok || len(models) != 2 {
		t.Fatalf("models missing: %v", body)
	}

	// A blank key is allowed for local providers and no model is required.
	status, body = client.do("POST", "/api/settings/llm/models", map[string]any{"apiKey": ""})
	if status != http.StatusOK {
		t.Fatalf("models without key: %d %v", status, body)
	}
	if body["apiKey"] != nil {
		t.Fatalf("models response leaked a key: %v", body)
	}

	status, _ = client.do("POST", "/api/settings/llm/models", map[string]any{"baseURL": "not-a-url"})
	if status != http.StatusBadGateway {
		t.Fatalf("invalid base URL should fail: %d", status)
	}
}

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

func TestLLMProfileRoutes(t *testing.T) {
	client := newTestServer(t)

	status, body := client.do("GET", "/api/settings/llm/profiles", nil)
	if status != http.StatusOK {
		t.Fatalf("list profiles: %d %v", status, body)
	}
	initial, ok := body["profiles"].([]any)
	if !ok || len(initial) != 1 {
		t.Fatalf("expected seeded profile: %v", body)
	}

	status, body = client.do("POST", "/api/settings/llm/profiles", map[string]any{
		"name": "Second", "api": "openai-completions",
		"baseURL": "http://127.0.0.1:18789/v1", "model": "second-model",
		"contextWindow": 128000, "maxTokens": 8192,
	})
	if status != http.StatusCreated {
		t.Fatalf("create profile: %d %v", status, body)
	}
	created := body["profile"].(map[string]any)
	id := int64(created["id"].(float64))

	status, body = client.do("POST", "/api/settings/llm/profiles/"+fmt.Sprint(id)+"/activate", nil)
	if status != http.StatusOK {
		t.Fatalf("activate profile: %d %v", status, body)
	}
	status, body = client.do("GET", "/api/settings/llm", nil)
	if status != http.StatusOK || body["model"] != "second-model" {
		t.Fatalf("active settings: %d %v", status, body)
	}

	status, _ = client.do("DELETE", "/api/settings/llm/profiles/"+fmt.Sprint(id), nil)
	if status != http.StatusOK {
		t.Fatalf("delete profile: %d", status)
	}
	status, body = client.do("GET", "/api/settings/llm", nil)
	if status != http.StatusOK || body["model"] != "test-model" {
		t.Fatalf("fallback after delete: %d %v", status, body)
	}
}
