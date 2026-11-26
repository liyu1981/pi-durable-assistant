package settings

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const probeTimeout = 8 * time.Second

// ProbeResult is the outcome of a connection test against the configured
// provider. Only model IDs are returned; credentials are never echoed.
type ProbeResult struct {
	Models    []string `json:"models"`
	LatencyMS int64    `json:"latencyMs"`
}

// ListModels fetches the provider's model catalog using only the base URL,
// wire protocol and optional API key. It does not require a model to be chosen
// yet, and never persists or returns credentials.
func ListModels(ctx context.Context, cfg Config) (ProbeResult, error) {
	if err := cfg.ValidateConnection(); err != nil {
		return ProbeResult{}, err
	}
	return fetchModels(ctx, cfg)
}

// Probe performs a lightweight GET on the provider's models endpoint. It
// validates the URL, API key and wire protocol without generating tokens.
// Loopback is allowed so the local development provider can be tested too.
func Probe(ctx context.Context, cfg Config) (ProbeResult, error) {
	if err := cfg.Validate(); err != nil {
		return ProbeResult{}, err
	}
	return fetchModels(ctx, cfg)
}

func fetchModels(ctx context.Context, cfg Config) (ProbeResult, error) {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	url := strings.TrimRight(cfg.BaseURL, "/") + "/models"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ProbeResult{}, fmt.Errorf("invalid models URL: %w", err)
	}
	switch cfg.API {
	case "anthropic-messages":
		request.Header.Set("x-api-key", cfg.APIKey)
		request.Header.Set("anthropic-version", "2023-06-01")
	default:
		if cfg.APIKey != "" {
			request.Header.Set("Authorization", "Bearer "+cfg.APIKey)
		}
	}
	request.Header.Set("Accept", "application/json")

	start := time.Now()
	response, err := (&http.Client{}).Do(request)
	if err != nil {
		return ProbeResult{}, fmt.Errorf("provider request failed: %w", err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return ProbeResult{}, fmt.Errorf("reading provider response: %w", err)
	}
	result := ProbeResult{LatencyMS: time.Since(start).Milliseconds(), Models: []string{}}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		detail := strings.TrimSpace(string(raw))
		if len(detail) > 512 {
			detail = detail[:512]
		}
		if cfg.APIKey != "" {
			detail = strings.ReplaceAll(detail, cfg.APIKey, "***")
		}
		return result, fmt.Errorf("provider returned HTTP %d: %s", response.StatusCode, detail)
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err == nil {
		for _, model := range payload.Data {
			if model.ID != "" {
				result.Models = append(result.Models, model.ID)
			}
		}
	}
	if len(result.Models) > 20 {
		result.Models = result.Models[:20]
	}
	return result, nil
}
