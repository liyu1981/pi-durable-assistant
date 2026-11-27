package httpapi

import (
	"fmt"
	"math"
	"net/http"
	"strconv"

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

type llmProfileDTO struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Active        bool   `json:"active"`
	API           string `json:"api"`
	BaseURL       string `json:"baseURL"`
	APIKeySet     bool   `json:"apiKeySet"`
	Model         string `json:"model"`
	ContextWindow int    `json:"contextWindow"`
	MaxTokens     int    `json:"maxTokens"`
	UpdatedAt     string `json:"updatedAt"`
}

func profileDTO(profile settings.Profile) llmProfileDTO {
	public := profile.Config.Public()
	return llmProfileDTO{
		ID: profile.ID, Name: profile.Name, Active: profile.Active,
		API: public.API, BaseURL: public.BaseURL, APIKeySet: public.APIKeySet,
		Model: public.Model, ContextWindow: public.ContextWindow,
		MaxTokens: public.MaxTokens, UpdatedAt: public.UpdatedAt,
	}
}

// listLLMProfiles returns every saved provider configuration.
func (s *Server) listLLMProfiles(w http.ResponseWriter, r *http.Request) {
	profiles, err := s.Runtime.Settings.ListProfiles()
	if err != nil {
		handleError(w, err)
		return
	}
	items := make([]llmProfileDTO, 0, len(profiles))
	for _, profile := range profiles {
		items = append(items, profileDTO(profile))
	}
	writeJSON(w, http.StatusOK, map[string]any{"profiles": items, "activeId": activeProfileID(profiles)})
}

func activeProfileID(profiles []settings.Profile) int64 {
	for _, profile := range profiles {
		if profile.Active {
			return profile.ID
		}
	}
	return 0
}

// createLLMProfile stores a new named provider configuration.
func (s *Server) createLLMProfile(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(r, 16_000)
	if err != nil {
		handleError(w, err)
		return
	}
	name, _ := body["name"].(string)
	cfg, err := mergeLLM(settings.Defaults(), body)
	if err != nil {
		handleError(w, err)
		return
	}
	profile, err := s.Runtime.Settings.CreateProfile(name, cfg)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"profile": profileDTO(profile)})
}

// updateLLMProfile updates a saved profile; an active profile is applied live.
func (s *Server) updateLLMProfile(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid profile id")
		return
	}
	existing, err := s.Runtime.Settings.GetProfile(id)
	if err != nil {
		handleError(w, err)
		return
	}
	body, err := readBody(r, 16_000)
	if err != nil {
		handleError(w, err)
		return
	}
	var name *string
	if raw, ok := body["name"]; ok {
		text, ok := raw.(string)
		if !ok {
			writeError(w, http.StatusBadRequest, "name must be a string")
			return
		}
		name = &text
	}
	cfg, err := mergeLLM(existing.Config, body)
	if err != nil {
		handleError(w, err)
		return
	}
	profile, err := s.Runtime.Settings.UpdateProfile(id, name, cfg)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if profile.Active {
		if err := s.Runtime.SwitchLLMProfile(r.Context(), id); err != nil {
			handleError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"profile": profileDTO(profile)})
}

// deleteLLMProfile removes a saved profile.
func (s *Server) deleteLLMProfile(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid profile id")
		return
	}
	profiles, err := s.Runtime.Settings.ListProfiles()
	if err != nil {
		handleError(w, err)
		return
	}
	wasActive := activeProfileID(profiles) == id
	if err := s.Runtime.Settings.DeleteProfile(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if wasActive {
		profiles, err = s.Runtime.Settings.ListProfiles()
		if err != nil {
			handleError(w, err)
			return
		}
		if len(profiles) > 0 {
			if err := s.Runtime.SwitchLLMProfile(r.Context(), profiles[0].ID); err != nil {
				handleError(w, err)
				return
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// activateLLMProfile switches the live provider to a saved profile.
func (s *Server) activateLLMProfile(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid profile id")
		return
	}
	if err := s.Runtime.SwitchLLMProfile(r.Context(), id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	profiles, err := s.Runtime.Settings.ListProfiles()
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"profile": profileDTO(profiles[activeIndex(profiles)])})
}

func activeIndex(profiles []settings.Profile) int {
	for i := range profiles {
		if profiles[i].Active {
			return i
		}
	}
	return 0
}
