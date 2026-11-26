package settings

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func newProbeServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("unexpected probe path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer seed-key" {
			t.Errorf("missing provider auth header")
		}
		fmt.Fprint(w, `{"data":[{"id":"faux-1"}]}`)
	}))
	t.Cleanup(server.Close)
	return server
}

func testDefaults() Config {
	return Config{
		API: DefaultAPI, BaseURL: "http://127.0.0.1:18789/v1", APIKey: "seed-key",
		Model: "seed-model", ContextWindow: DefaultContextWindow, MaxTokens: DefaultMaxTokens,
	}
}

func TestMigrationsSeedAndPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.sqlite")
	store, err := Open(path, testDefaults())
	if err != nil {
		t.Fatal(err)
	}
	stored, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if stored.Model != "seed-model" || stored.APIKey != "seed-key" {
		t.Fatalf("seed was not applied: %+v", stored)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != len(migrations) {
		t.Fatalf("schema version = %d, want %d", version, len(migrations))
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM app_llm_config`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("settings rows = %d, want 1", count)
	}
	_ = db.Close()

	next := testDefaults()
	next.Model = "changed-model"
	next.APIKey = "changed-key"
	if err := store.Save(next); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	other := testDefaults()
	other.Model = "env-model"
	reopened, err := Open(path, other)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	stored, err = reopened.Load()
	if err != nil {
		t.Fatal(err)
	}
	if stored.Model != "changed-model" || stored.APIKey != "changed-key" || stored.UpdatedAt == "" {
		t.Fatalf("stored settings were not preserved: %+v", stored)
	}
}

func TestRefusesNewerSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 999`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	if _, err := Open(path, testDefaults()); err == nil {
		t.Fatal("a database from a newer build must be refused")
	}
}

func TestValidate(t *testing.T) {
	valid := testDefaults()
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	cases := map[string]func(*Config){
		"api":      func(c *Config) { c.API = "bogus" },
		"base URL": func(c *Config) { c.BaseURL = "ftp://example.com" },
		"model":    func(c *Config) { c.Model = "" },
		"window":   func(c *Config) { c.ContextWindow = 1 },
		"tokens":   func(c *Config) { c.MaxTokens = c.ContextWindow + 1 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := testDefaults()
			mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
}

func TestValidateConnectionAllowsMissingModel(t *testing.T) {
	cfg := testDefaults()
	cfg.Model = ""
	cfg.ContextWindow = 0
	cfg.MaxTokens = 0
	if err := cfg.ValidateConnection(); err != nil {
		t.Fatalf("connection validation must not require a model: %v", err)
	}
	cfg.BaseURL = "ftp://example.com"
	if err := cfg.ValidateConnection(); err == nil {
		t.Fatal("invalid base URL accepted by connection validation")
	}
}

func TestPublicHidesKey(t *testing.T) {
	cfg := testDefaults()
	public := cfg.Public()
	if !public.APIKeySet {
		t.Fatalf("public config lost the key flag: %+v", public)
	}
	cfg.APIKey = ""
	if cfg.Public().APIKeySet {
		t.Fatal("empty key must not be reported as set")
	}
}

func TestProbeUsesProviderModels(t *testing.T) {
	server := newProbeServer(t)
	cfg := testDefaults()
	cfg.BaseURL = server.URL + "/v1"
	result, err := Probe(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Models) != 1 || result.Models[0] != "faux-1" {
		t.Fatalf("unexpected models: %+v", result.Models)
	}
	if result.LatencyMS < 0 {
		t.Fatal("negative latency")
	}
}
