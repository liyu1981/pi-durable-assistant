package httpapi

import (
	"net/http"
	"strings"

	"github.com/MichaelKinsy/PiG/durable/harness"
)

// systemPromptSection is one named section of the model's system prompt.
type systemPromptSection struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// getSystemPrompt reports the system prompt currently in effect for a
// conversation. It replays committed pi.system entries, the same way the
// generation task does before composing the next model request.
func (s *Server) getSystemPrompt(w http.ResponseWriter, r *http.Request) {
	conversation := s.Runtime.Root
	if raw := r.URL.Query().Get("conversation"); raw != "" {
		id, err := conversationID(raw)
		if err != nil {
			handleError(w, err)
			return
		}
		found, err := s.Runtime.Harness.Conversation(r.Context(), id)
		if err != nil || found == nil {
			writeError(w, http.StatusNotFound, "conversation not found")
			return
		}
		conversation = found
	}
	view, err := conversation.Context(r.Context(), nil)
	if err != nil {
		handleError(w, err)
		return
	}
	replayed := harness.ReplaySections(view.Messages)
	sections := make([]systemPromptSection, 0, replayed.Len())
	parts := make([]string, 0, replayed.Len())
	for _, name := range replayed.Keys() {
		value, ok := replayed.Get(name)
		if !ok || value == "" {
			continue
		}
		sections = append(sections, systemPromptSection{Name: name, Value: value})
		parts = append(parts, value)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"conversationId": int64(conversation.Id()),
		"sections":       sections,
		"prompt":         strings.Join(parts, "\n\n"),
	})
}
