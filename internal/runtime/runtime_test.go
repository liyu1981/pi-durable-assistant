package runtime

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/liyu1981/pi-durable-assistant/internal/settings"
)

func TestBuildProviderResolvesWithoutAPIKey(t *testing.T) {
	cfg := settings.Config{
		API:           "openai-completions",
		BaseURL:       "http://127.0.0.1:11434/v1",
		Model:         "local-model",
		ContextWindow: 128000,
		MaxTokens:     8192,
	}
	provider, err := buildProvider(cfg)
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Auth.APIKey.Resolve(context.Background(), ai.APIKeyAuthInput{})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("keyless provider must still resolve as configured")
	}
	if result.Auth.APIKey == "" {
		t.Fatal("keyless provider needs a placeholder key for PiG's stream guard")
	}
	if result.Auth.BaseURL != cfg.BaseURL {
		t.Fatalf("base URL = %q, want %q", result.Auth.BaseURL, cfg.BaseURL)
	}
	if value, ok := result.Auth.Headers["Authorization"]; !ok || value != nil {
		t.Fatal("auth headers must be suppressed for a keyless provider")
	}
}

func TestKeylessProviderCompletesFromLocalServer(t *testing.T) {
	var sawAuthorization bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuthorization = r.Header.Get("Authorization") != "" || r.Header.Get("x-api-key") != ""
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: "+`{"id":"1","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`+"\n\n")
		fmt.Fprint(w, "data: "+`{"choices":[{"index":0,"delta":{"content":"hello"},"finish_reason":"stop"}]}`+"\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	cfg := settings.Config{
		API:           "openai-completions",
		BaseURL:       server.URL + "/v1",
		Model:         "local-model",
		ContextWindow: 128000,
		MaxTokens:     8192,
	}
	models := ai.CreateModels()
	models.SetProvider(buildProviderOrFail(t, cfg))
	model := models.GetModel(settings.ProviderID, cfg.Model)
	if model == nil {
		t.Fatal("model not registered")
	}

	message := models.Complete(context.Background(), model, ai.Context{
		Messages: []ai.Message{ai.UserMessage{Content: ai.UserText("hi")}},
	})
	if message.StopReason == ai.StopReasonError {
		t.Fatalf("model call failed: %s", message.ErrorMessage)
	}
	if sawAuthorization {
		t.Fatal("keyless provider must not send an Authorization header")
	}
	if !strings.Contains(ai.ContentText(message.Content), "hello") {
		t.Fatalf("unexpected response: %+v", message.Content)
	}
}

func buildProviderOrFail(t *testing.T, cfg settings.Config) *ai.ModelsProvider {
	t.Helper()
	provider, err := buildProvider(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return provider
}
