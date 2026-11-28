// Package config loads the application's environment configuration.
package config

import (
	"os"
	"strconv"
)

// Config is the complete runtime configuration. Secrets are never logged.
type Config struct {
	Port          int
	DataDir       string
	Password      string
	NotifyURL     string
	NotifyKind    string
	NotifyMode    string
	CrashDemo     bool
	Production    bool
	Dev           bool
	HTTPMode      string
	HTTPSPort     int
	CertFile      string
	KeyFile       string
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
	return Config{
		Port:          envInt("PORT", 3000),
		DataDir:       env("DATA_DIR", "./data"),
		Password:      os.Getenv("DEMO_PASSWORD"),
		NotifyURL:     os.Getenv("NOTIFY_WEBHOOK_URL"),
		NotifyKind:    env("NOTIFY_WEBHOOK_KIND", "generic"),
		NotifyMode:    env("NOTIFY_MODE", "errors"),
		CrashDemo:     os.Getenv("ENABLE_CRASH_DEMO") == "1",
		Production:    os.Getenv("NODE_ENV") == "production" || os.Getenv("GO_ENV") == "production",
		Dev:           os.Getenv("DEV") == "1" || os.Getenv("GO_ENV") == "development" || os.Getenv("NODE_ENV") == "development",
		HTTPMode:      httpMode(),
		HTTPSPort:     envInt("HTTPS_PORT", 3443),
		CertFile:      env("TLS_CERT_FILE", "certs/localhost.pem"),
		KeyFile:       env("TLS_KEY_FILE", "certs/localhost-key.pem"),
	}
}

func httpMode() string {
	if value := os.Getenv("HTTP_MODE"); value != "" {
		return value
	}
	if os.Getenv("DEV") == "1" || os.Getenv("GO_ENV") == "development" || os.Getenv("NODE_ENV") == "development" {
		return "https"
	}
	return "http"
}