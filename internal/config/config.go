// Package config loads the application's environment configuration.
package config

import (
	"os"
	"strconv"
	"strings"
)

// Config is the complete runtime configuration. Secrets are never logged.
type Config struct {
	Port          int
	DataDir       string
	Password      string
	API           string
	BaseURL       string
	APIKey        string
	Model         string
	ContextWindow int
	MaxTokens     int
	NotifyURL     string
	NotifyKind    string
	NotifyMode    string
	ObsidianRoot  string
	ObsidianAllow []string
	CrashDemo     bool
	Production    bool
	Dev           bool
}

func env(name, fallback string) string {
	if v, ok := os.LookupEnv(name); ok && v != "" {
		return v
	}
	return fallback
}

func envInt(name string, fallback int) int {
	v, err := strconv.Atoi(env(name, ""))
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

// Load reads configuration from the process environment.
func Load() Config {
	allow := []string{"AI Learning", "X Ideas", "VPS Tutorials"}
	if extra := env("OBSIDIAN_ALLOW", ""); extra != "" {
		for _, part := range strings.Split(extra, ",") {
			if part = strings.TrimSpace(part); part != "" {
				allow = append(allow, part)
			}
		}
	}
	return Config{
		Port:          envInt("PORT", 3000),
		DataDir:       env("DATA_DIR", "./data"),
		Password:      os.Getenv("DEMO_PASSWORD"),
		API:           env("LLM_API", "openai-completions"),
		BaseURL:       os.Getenv("LLM_BASE_URL"),
		APIKey:        os.Getenv("LLM_API_KEY"),
		Model:         os.Getenv("LLM_MODEL"),
		ContextWindow: envInt("LLM_CONTEXT_WINDOW", 128000),
		MaxTokens:     envInt("LLM_MAX_TOKENS", 8192),
		NotifyURL:     os.Getenv("NOTIFY_WEBHOOK_URL"),
		NotifyKind:    env("NOTIFY_WEBHOOK_KIND", "generic"),
		NotifyMode:    env("NOTIFY_MODE", "errors"),
		ObsidianRoot:  os.Getenv("OBSIDIAN_ROOT"),
		ObsidianAllow: allow,
		CrashDemo:     os.Getenv("ENABLE_CRASH_DEMO") == "1",
		Production:    os.Getenv("NODE_ENV") == "production" || os.Getenv("GO_ENV") == "production",
		Dev:           os.Getenv("DEV") == "1" || os.Getenv("GO_ENV") == "development" || os.Getenv("NODE_ENV") == "development",
	}
}

// Redact removes the configured API key from text before logging or display.
func (c Config) Redact(value string) string {
	if len(c.APIKey) >= 8 {
		return strings.ReplaceAll(value, c.APIKey, "***")
	}
	return value
}
