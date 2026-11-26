package httpapi

import (
	"fmt"
	"math"
	"net/http"

	"github.com/liyu1981/pi-durable-assistant/internal/settings"
)

// optionalString applies one field when present. An explicit empty string
// clears the value; an omitted key leaves the stored value untouched.
func optionalString(body map[string]any, key string, target *string) error {
	value, ok := body[key]
	if !ok {
		return nil
	}
	text, ok := value.(string)
	if !ok {
		return badRequest("%s must be a string", key)
	}
	*target = text
	return nil
}

func optionalInt(body map[string]any, key string, target *int) error {
	value, ok := body[key]
	if !ok {
		return nil
	}
	number, ok := value.(float64)
	if !ok || math.Trunc(number) != number || number > math.MaxInt || number < math.MinInt {
		return badRequest("%s must be an integer", key)
	}
	*target = int(number)
	return nil
}

func mergeLLM(current settings.Config, body map[string]any) (settings.Config, error) {
	next := current
	for _, field := range []struct {
		key    string
		target *string
	}{
		{"api", &next.API},
		{"baseURL", &next.BaseURL},
		{"apiKey", &next.APIKey},
		{"model", &next.Model},
	} {
		if err := optionalString(body, field.key, field.target); err != nil {
			return current, err
		}
	}
	for _, field := range []struct {
		key    string
		target *int
	}{
		{"contextWindow", &next.ContextWindow},
		{"maxTokens", &next.MaxTokens},
	} {
		if err := optionalInt(body, field.key, field.target); err != nil {
			return current, err
		}
	}
	return next, nil
}

func (s *Server) getLLMSettings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.Runtime.LLM())
}

func (s *Server) putLLMSettings(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r, 16_000)
	if err != nil {
		handleError(w, err)
		return
	}
	next, err := mergeLLM(s.Runtime.LLMConfig(), body)
	if err != nil {
		handleError(w, err)
		return
	}
	if err := next.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.Runtime.ApplyLLM(r.Context(), next); err != nil {
		handleError(w, fmt.Errorf("applying model settings: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, s.Runtime.LLM())
}

// testLLMSettings checks a candidate configuration without saving it. The
// browser sends the whole form so the user can test before committing.
// listLLMMModels discovers models from a candidate provider configuration.
// Only the base URL and protocol are required; the API key is optional.
func (s *Server) listLLMMModels(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r, 16_000)
	if err != nil {
		handleError(w, err)
		return
	}
	next, err := mergeLLM(s.Runtime.LLMConfig(), body)
	if err != nil {
		handleError(w, err)
		return
	}
	result, err := settings.ListModels(r.Context(), next)
	if err != nil {
		handleError(w, &httpError{status: http.StatusBadGateway, message: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "models": result.Models, "latencyMs": result.LatencyMS,
	})
}

func (s *Server) testLLMSettings(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r, 16_000)
	if err != nil {
		handleError(w, err)
		return
	}
	next, err := mergeLLM(s.Runtime.LLMConfig(), body)
	if err != nil {
		handleError(w, err)
		return
	}
	result, err := settings.Probe(r.Context(), next)
	if err != nil {
		handleError(w, &httpError{status: http.StatusBadGateway, message: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "models": result.Models, "latencyMs": result.LatencyMS,
	})
}
