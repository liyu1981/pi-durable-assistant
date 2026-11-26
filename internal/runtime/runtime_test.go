package runtime

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/liyu1981/pi-durable-assistant/internal/config"
	"github.com/liyu1981/pi-durable-assistant/internal/settings"
)

func TestUnconfiguredBootCanBeConfiguredLive(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"data":[]}`)
	}))
	defer provider.Close()

	cfg := config.Config{DataDir: t.TempDir()}
	rt, err := Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("an unconfigured app must still boot: %v", err)
	}
	if rt.Label() != "Not configured" || rt.Model().ModelId != "" {
		t.Fatalf("unexpected initial model: %s %+v", rt.Label(), rt.Model())
	}

	next := settings.Defaults()
	next.BaseURL = provider.URL + "/v1"
	next.Model = "faux-1"
	if err := rt.ApplyLLM(context.Background(), next); err != nil {
		t.Fatalf("applying first configuration: %v", err)
	}
	if rt.Label() != "faux-1" || rt.Model().ModelId != "faux-1" || rt.LLM().APIKeySet {
		t.Fatalf("settings were not applied: %s %+v %+v", rt.Label(), rt.Model(), rt.LLM())
	}
	// A restart with changed env must keep the stored configuration.
	if err := rt.Harness.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	_ = rt.Settings.Close()
	cfg.Model = "env-model"
	reopened, err := Open(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Settings.Close()
	if reopened.Label() != "faux-1" {
		t.Fatalf("environment must not override stored settings: %s", reopened.Label())
	}
}
