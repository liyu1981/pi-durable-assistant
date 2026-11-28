// Package httpapi implements the JSON API, SSE stream and embedded static assets.
package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"runtime/debug"
	"strings"
	"time"

	"github.com/MichaelKinsy/PiG/durable"
	"github.com/MichaelKinsy/PiG/durable/harness"
	"github.com/liyu1981/pi-durable-assistant/internal/config"
	"github.com/liyu1981/pi-durable-assistant/internal/docs"
	"github.com/liyu1981/pi-durable-assistant/internal/runtime"
)

var redactHook = func(value string) string { return value }

// Server holds shared state for all HTTP handlers.
type Server struct {
	Runtime   *runtime.Runtime
	Config    config.Config
	Assets    fs.FS
	StartedAt string
	handler   http.Handler
}

// NewServer creates the HTTP handler tree.
func NewServer(rt *runtime.Runtime, assets fs.FS) (*Server, error) {
	server := &Server{
		Runtime: rt, Config: rt.Config, Assets: assets,
		StartedAt: time.Now().UTC().Format(time.RFC3339),
	}
	redactHook = rt.Redact
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", server.serveStatic)
	mux.HandleFunc("GET /api/info", server.info)
	mux.HandleFunc("GET /api/settings/llm", server.getLLMSettings)
	mux.HandleFunc("PUT /api/settings/llm", server.putLLMSettings)
	mux.HandleFunc("POST /api/settings/llm/test", server.testLLMSettings)
	mux.HandleFunc("POST /api/settings/llm/models", server.listLLMMModels)
	mux.HandleFunc("GET /api/settings/llm/profiles", server.listLLMProfiles)
	mux.HandleFunc("POST /api/settings/llm/profiles", server.createLLMProfile)
	mux.HandleFunc("PUT /api/settings/llm/profiles/{id}", server.updateLLMProfile)
	mux.HandleFunc("DELETE /api/settings/llm/profiles/{id}", server.deleteLLMProfile)
	mux.HandleFunc("POST /api/settings/llm/profiles/{id}/activate", server.activateLLMProfile)
	mux.HandleFunc("GET /api/crash", server.notFound)
	mux.HandleFunc("POST /api/crash", server.crash)
	mux.HandleFunc("GET /api/conversations", server.listConversations)
	mux.HandleFunc("POST /api/conversations", server.createConversation)
	mux.HandleFunc("POST /api/conversations/{id}/archive", server.shelf("archive"))
	mux.HandleFunc("POST /api/conversations/{id}/restore", server.shelf("restore"))
	mux.HandleFunc("POST /api/conversations/{id}/remove", server.shelf("remove"))
	mux.HandleFunc("GET /api/conversations/{id}/events", server.events)
	mux.HandleFunc("POST /api/conversations/{id}/messages", server.postMessage)
	mux.HandleFunc("PUT /api/conversations/{id}/title", server.renameConversation)
	mux.HandleFunc("POST /api/conversations/{id}/fork", server.fork)
	mux.HandleFunc("POST /api/conversations/{id}/abort", server.abort)
	mux.HandleFunc("GET /api/conversations/{id}/images/{entry}/{index}", server.image)
	mux.HandleFunc("GET /api/conversations/{id}/management", server.management)
	mux.HandleFunc("GET /api/conversations/{id}/export.md", server.exportMarkdown)
	mux.HandleFunc("POST /api/conversations/{id}/schedules", server.createSchedule)
	mux.HandleFunc("POST /api/conversations/{id}/schedules/{uuid}/{action}", server.manageSchedule)
	mux.HandleFunc("POST /api/conversations/{id}/background", server.createBackground)
	mux.HandleFunc("POST /api/conversations/{id}/background/{uuid}/stop", server.stopBackground)
	mux.HandleFunc("POST /api/conversations/{id}/reminders/{uuid}/cancel", server.cancelReminder)
	mux.HandleFunc("GET /api/personal", server.getPersonal)
	mux.HandleFunc("POST /api/personal/{kind}", server.postPersonal)
	mux.HandleFunc("POST /api/personal/{kind}/{uuid}/{action}", server.decidePersonal)
	mux.HandleFunc("GET /api/tools", server.listTools)
	mux.HandleFunc("GET /api/system-prompt", server.getSystemPrompt)
	mux.HandleFunc("GET /api/knowledge/search", server.searchKnowledge)
	mux.HandleFunc("GET /api/knowledge/read", server.readKnowledge)
	mux.HandleFunc("GET /api/artifacts", server.listArtifacts)
	mux.HandleFunc("POST /api/artifacts", server.createArtifact)
	mux.HandleFunc("GET /api/artifacts/{id}", server.getArtifact)
	mux.HandleFunc("DELETE /api/artifacts/{id}", server.deleteArtifact)
	mux.HandleFunc("GET /api/artifacts/{id}/files", server.listArtifactFiles)
	mux.HandleFunc("GET /api/artifacts/{id}/files/{path...}", server.readArtifactFile)
	mux.HandleFunc("PUT /api/artifacts/{id}/files/{path...}", server.writeArtifactFile)
	mux.HandleFunc("GET /api/artifacts/{id}/raw/{path...}", server.rawArtifactFile)
	mux.HandleFunc("POST /api/artifacts/{id}/move", server.moveArtifactFile)
	mux.HandleFunc("PUT /api/artifacts/{id}/pin", server.pinArtifact)
	mux.HandleFunc("PUT /api/artifacts/{id}/unpin", server.unpinArtifact)
	mux.HandleFunc("GET /api/artifacts/confirmations", server.listArtifactConfirmations)
	mux.HandleFunc("POST /api/artifacts/confirmations/{id}/{decision}", server.decideArtifactConfirmation)
	server.handler = server.middleware(mux)
	return server, nil
}

var _ http.Handler = (*Server)(nil)

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				fmt.Fprintf(os.Stderr, "request failed: %s\n%s\n", s.Runtime.Redact(fmt.Sprint(recovered)), debug.Stack())
				writeError(w, http.StatusInternalServerError, "server error; see server logs")
			}
		}()
		if !s.authorized(r) {
			w.Header().Set("www-authenticate", `Basic realm="pi-durable-assistant", charset="UTF-8"`)
			writeError(w, http.StatusUnauthorized, "password required")
			return
		}
		if r.Method != http.MethodGet && !sameOrigin(r) {
			writeError(w, http.StatusForbidden, "same-origin requests only")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) authorized(r *http.Request) bool {
	if s.Config.Password == "" {
		return true
	}
	user, password, ok := r.BasicAuth()
	if !ok || user != "demo" {
		return false
	}
	expected := "demo:" + s.Config.Password
	return subtle.ConstantTimeCompare([]byte(user+":"+password), []byte(expected)) == 1
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	return err == nil && parsed.Host == r.Host
}

type httpError struct {
	status  int
	message string
}

func (e *httpError) Error() string { return e.message }

func badRequest(format string, args ...any) error {
	return &httpError{status: http.StatusBadRequest, message: fmt.Sprintf(format, args...)}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	payload, err := json.Marshal(value)
	if err != nil {
		status = http.StatusInternalServerError
		payload = []byte(`{"error":"server error; see server logs"}`)
	}
	w.Header().Set("content-type", "application/json; charset=utf-8")
	w.Header().Set("cache-control", "no-store")
	w.WriteHeader(status)
	w.Write(payload)
}

func readBody(r *http.Request, limit int64) (map[string]any, error) {
	reader := io.LimitReader(r.Body, limit+1)
	payload, err := io.ReadAll(reader)
	if err != nil {
		return nil, badRequest("failed to read request body")
	}
	if int64(len(payload)) > limit {
		return nil, &httpError{status: http.StatusRequestEntityTooLarge, message: "request too large"}
	}
	if len(payload) == 0 {
		return map[string]any{}, nil
	}
	var value map[string]any
	if err := json.Unmarshal(payload, &value); err != nil || value == nil {
		return nil, badRequest("request must be a valid JSON object")
	}
	return value, nil
}

func handleError(w http.ResponseWriter, err error) {
	var httpErr *httpError
	if errors.As(err, &httpErr) {
		writeError(w, httpErr.status, httpErr.message)
		return
	}
	fmt.Fprintln(os.Stderr, "request failed:", redactHook(err.Error()))
	writeError(w, http.StatusInternalServerError, "server error; see server logs")
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "route not found")
}

func (s *Server) info(w http.ResponseWriter, r *http.Request) {
	meta, err := runtime.Snapshot[docs.MetaDoc](r.Context(), s.Runtime, docs.Meta)
	if err != nil {
		meta = nil
	}
	boots := 0
	if meta != nil {
		boots = meta.Boots
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"model": s.Runtime.Label(), "api": s.Runtime.LLM().API, "pid": os.Getpid(),
		"startedAt": s.StartedAt, "boots": boots, "dev": s.Config.Dev,
	})
}

func (s *Server) crash(w http.ResponseWriter, r *http.Request) {
	if s.Config.Production && !s.Config.CrashDemo {
		writeError(w, http.StatusForbidden, "simulated crash is disabled in production")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	go func() {
		time.Sleep(200 * time.Millisecond)
		os.Exit(1)
	}()
}

func (s *Server) serveStatic(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "" || name == "." {
		name = "index.html"
	}
	if strings.HasPrefix(name, "assets/") || strings.HasPrefix(name, "vendor/") {
		w.Header().Set("cache-control", "public, max-age=31536000, immutable")
	} else {
		// The HTML entrypoint points at hashed assets; never let a dev rebuild serve a stale shell.
		w.Header().Set("cache-control", "no-store")
	}
	data, err := fs.ReadFile(s.Assets, name)
	if err != nil {
		if strings.Contains(name, ".") {
			writeError(w, http.StatusNotFound, "file not found")
			return
		}
		data, err = fs.ReadFile(s.Assets, "index.html")
		if err != nil {
			writeError(w, http.StatusNotFound, "file not found")
			return
		}
	}
	contentType := contentTypeFor(name)
	w.Header().Set("content-type", contentType)
	if name == "sw.js" {
		w.Header().Set("service-worker-allowed", "/")
	}
	w.Write(data)
}

func contentTypeFor(name string) string {
	switch {
	case strings.HasSuffix(name, ".html"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(name, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(name, ".js"), strings.HasSuffix(name, ".mjs"):
		return "text/javascript; charset=utf-8"
	case strings.HasSuffix(name, ".wasm"):
		return "application/wasm"
	case strings.HasSuffix(name, ".ttf"):
		return "font/ttf"
	case strings.HasSuffix(name, ".woff"):
		return "font/woff"
	case strings.HasSuffix(name, ".webp"):
		return "image/webp"
	case strings.HasSuffix(name, ".txt"),
		strings.HasSuffix(name, ".bcmap"),
		strings.HasSuffix(name, ".pfb"):
		return "text/plain; charset=utf-8"
	case strings.HasSuffix(name, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(name, ".woff2"):
		return "font/woff2"
	case strings.HasSuffix(name, ".png"):
		return "image/png"
	case strings.HasSuffix(name, ".json"):
		return "application/json; charset=utf-8"
	case strings.HasSuffix(name, ".webmanifest"):
		return "application/manifest+json"
	default:
		return "application/octet-stream"
	}
}

func conversationID(id string) (durable.ConversationId, error) {
	var value durable.ConversationId
	if _, err := fmt.Sscan(id, &value); err != nil || value <= 0 {
		return 0, badRequest("conversation not found")
	}
	return value, nil
}

func (s *Server) getConversation(r *http.Request, rawID string) (harness.Conversation, error) {
	id, err := conversationID(rawID)
	if err != nil {
		return nil, &httpError{status: http.StatusNotFound, message: "conversation not found"}
	}
	conversation, err := s.Runtime.Harness.Conversation(r.Context(), id)
	if err != nil || conversation == nil {
		return nil, &httpError{status: http.StatusNotFound, message: "conversation not found"}
	}
	return conversation, nil
}
