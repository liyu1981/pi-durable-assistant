// Package settings owns configuration that the user changes in the UI. It is
// persisted in the application's SQLite database rather than the process
// environment, so a change survives a restart without editing .env.
package settings

import (
	"fmt"
	"strings"
)

// ProviderID is the single provider the runtime configures. Replacing the
// provider with the same ID lets model settings be applied without a restart.
const ProviderID = "demo"

const (
	DefaultAPI           = "openai-completions"
	DefaultContextWindow = 128000
	DefaultMaxTokens     = 8192
)

// SupportedAPIs are the wire protocols the runtime can talk to.
var SupportedAPIs = []string{"openai-completions", "openai-responses", "anthropic-messages"}

// Config is the stored model configuration. APIKey is secret: it is never
// included in an API response and is never logged or printed by this package.
type Config struct {
	API           string `json:"api"`
	BaseURL       string `json:"baseURL"`
	APIKey        string `json:"apiKey"`
	Model         string `json:"model"`
	ContextWindow int    `json:"contextWindow"`
	MaxTokens     int    `json:"maxTokens"`
	UpdatedAt     string `json:"updatedAt"`
}

// Public is the JSON shape returned to the browser. The key is represented by
// a presence flag so saving other fields does not need to resend the secret.
type Public struct {
	API           string `json:"api"`
	BaseURL       string `json:"baseURL"`
	APIKeySet     bool   `json:"apiKeySet"`
	Model         string `json:"model"`
	ContextWindow int    `json:"contextWindow"`
	MaxTokens     int    `json:"maxTokens"`
	UpdatedAt     string `json:"updatedAt"`
}

// Defaults returns a usable starting configuration. Endpoints and model names
// are intentionally empty: the user picks their provider in the UI.
func Defaults() Config {
	return Config{
		API:           DefaultAPI,
		ContextWindow: DefaultContextWindow,
		MaxTokens:     DefaultMaxTokens,
	}
}

// ValidateConnection checks only the fields needed to discover models from a
// provider. A model and token limits are not required until a configuration is
// saved, and a key is optional for local servers.
func (c Config) ValidateConnection() error {
	supported := false
	for _, api := range SupportedAPIs {
		if c.API == api {
			supported = true
			break
		}
	}
	if !supported {
		return fmt.Errorf("API must be one of %s", strings.Join(SupportedAPIs, ", "))
	}
	base := strings.TrimSpace(c.BaseURL)
	if base == "" {
		return fmt.Errorf("base URL is required")
	}
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		return fmt.Errorf("base URL must start with http:// or https://")
	}
	if len(base) > 2048 || len(c.APIKey) > 4096 {
		return fmt.Errorf("base URL or API key is too long")
	}
	return nil
}

// Validate rejects values that cannot produce a working provider. A key is
// optional because some local servers do not require authentication.
func (c Config) Validate() error {
	if err := c.ValidateConnection(); err != nil {
		return err
	}
	if strings.TrimSpace(c.Model) == "" {
		return fmt.Errorf("model is required")
	}
	if len(c.Model) > 256 {
		return fmt.Errorf("model is too long")
	}
	if c.ContextWindow < 1000 || c.ContextWindow > 100_000_000 {
		return fmt.Errorf("context window must be between 1000 and 100000000")
	}
	if c.MaxTokens < 1 || c.MaxTokens > c.ContextWindow {
		return fmt.Errorf("max tokens must be between 1 and the context window")
	}
	return nil
}

// Public strips the secret from a stored configuration.
func (c Config) Public() Public {
	return Public{
		API:           c.API,
		BaseURL:       c.BaseURL,
		APIKeySet:     c.APIKey != "",
		Model:         c.Model,
		ContextWindow: c.ContextWindow,
		MaxTokens:     c.MaxTokens,
		UpdatedAt:     c.UpdatedAt,
	}
}
