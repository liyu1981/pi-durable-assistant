package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/liyu1981/pi-durable-assistant/internal/config"
)

func TestFriendlyModelError(t *testing.T) {
	for _, raw := range []string{
		"Provider is not configured: demo",
		"Unknown provider: demo",
		"No API key for provider: demo",
	} {
		got := friendlyModelError(raw)
		if !strings.Contains(got, "Settings > Models") {
			t.Fatalf("%q translated to %q", raw, got)
		}
	}
	if got := friendlyModelError("connection refused"); got != "connection refused" {
		t.Fatalf("unrelated error changed: %q", got)
	}
}

func TestAuthAndSameOrigin(t *testing.T) {
	server := &Server{Config: config.Config{Password: "secret"}}
	request := httptest.NewRequest("GET", "/api/info", nil)
	if server.authorized(request) {
		t.Fatal("request without credentials must be rejected")
	}
	request.SetBasicAuth("demo", "secret")
	if !server.authorized(request) {
		t.Fatal("valid credentials must pass")
	}
	request.SetBasicAuth("demo", "wrong")
	if server.authorized(request) {
		t.Fatal("invalid credentials must be rejected")
	}
}

func TestSameOrigin(t *testing.T) {
	request := httptest.NewRequest("POST", "http://localhost:3000/api/conversations", nil)
	if !sameOrigin(request) {
		t.Fatal("requests without Origin must pass")
	}
	request.Header.Set("Origin", "http://evil.example")
	if sameOrigin(request) {
		t.Fatal("cross-origin writes must be rejected")
	}
	request.Header.Set("Origin", "http://localhost:3000")
	if !sameOrigin(request) {
		t.Fatal("same-origin writes must pass")
	}
}
