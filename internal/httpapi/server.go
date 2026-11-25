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
	redactHook = rt.Config.Redact
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", server.serveStatic)
	mux.HandleFunc("GET /api/info", server.info)
	mux.HandleFunc("GET /api/crash", server.notFound)
	mux.HandleFunc("POST /api/crash", server.crash)
	mux.HandleFunc("GET /api/conversations", server.listConversations)
	mux.HandleFunc("POST /api/conversations", server.createConversation)
	mux.HandleFunc("POST /api/conversations/{id}/archive", server.shelf("archive"))
	mux.HandleFunc("POST /api/conversations/{id}/restore", server.shelf("restore"))
	mux.HandleFunc("POST /api/conversations/{id}/remove", server.shelf("remove"))
	mux.HandleFunc("GET /api/conversations/{id}/events", server.events)
	mux.HandleFunc("POST /api/conversations/{id}/messages", server.postMessage)
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
	mux.HandleFunc("GET /api/knowledge/search", server.searchKnowledge)
	mux.HandleFunc("GET /api/knowledge/read", server.readKnowledge)
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
				fmt.Fprintln(os.Stderr, "请求出错：", s.Config.Redact(fmt.Sprint(recovered)))
				writeError(w, http.StatusInternalServerError, "服务器出错，详情见服务器日志")
			}
		}()
		if !s.authorized(r) {
			w.Header().Set("www-authenticate", `Basic realm="pi-durable-assistant", charset="UTF-8"`)
			writeError(w, http.StatusUnauthorized, "需要口令")
			return
		}
		if r.Method != http.MethodGet && !sameOrigin(r) {
			writeError(w, http.StatusForbidden, "只接受同源请求")
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
		payload = []byte(`{"error":"服务器出错，详情见服务器日志"}`)
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
		return nil, badRequest("请求体读取失败")
	}
	if int64(len(payload)) > limit {
		return nil, &httpError{status: http.StatusRequestEntityTooLarge, message: "请求太大"}
	}
	if len(payload) == 0 {
		return map[string]any{}, nil
	}
	var value map[string]any
	if err := json.Unmarshal(payload, &value); err != nil || value == nil {
		return nil, badRequest("请求不是合法 JSON 对象")
	}
	return value, nil
}

func handleError(w http.ResponseWriter, err error) {
	var httpErr *httpError
	if errors.As(err, &httpErr) {
		writeError(w, httpErr.status, httpErr.message)
		return
	}
	fmt.Fprintln(os.Stderr, "请求出错：", redactHook(err.Error()))
	writeError(w, http.StatusInternalServerError, "服务器出错，详情见服务器日志")
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "接口不存在")
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
		"model": s.Runtime.ModelLabel, "api": s.Config.API, "pid": os.Getpid(),
		"startedAt": s.StartedAt, "boots": boots,
	})
}

func (s *Server) crash(w http.ResponseWriter, r *http.Request) {
	if s.Config.Production && !s.Config.CrashDemo {
		writeError(w, http.StatusForbidden, "生产环境已禁用模拟崩溃")
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
	if strings.HasPrefix(name, "assets/") {
		w.Header().Set("cache-control", "public, max-age=31536000, immutable")
	}
	data, err := fs.ReadFile(s.Assets, name)
	if err != nil {
		if strings.Contains(name, ".") {
			writeError(w, http.StatusNotFound, "文件不存在")
			return
		}
		data, err = fs.ReadFile(s.Assets, "index.html")
		if err != nil {
			writeError(w, http.StatusNotFound, "文件不存在")
			return
		}
	}
	contentType := contentTypeFor(name)
	w.Header().Set("content-type", contentType)
	w.Write(data)
}

func contentTypeFor(name string) string {
	switch {
	case strings.HasSuffix(name, ".html"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(name, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(name, ".js"):
		return "text/javascript; charset=utf-8"
	case strings.HasSuffix(name, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(name, ".woff2"):
		return "font/woff2"
	case strings.HasSuffix(name, ".png"):
		return "image/png"
	case strings.HasSuffix(name, ".json"):
		return "application/json; charset=utf-8"
	default:
		return "application/octet-stream"
	}
}

func conversationID(id string) (durable.ConversationId, error) {
	var value durable.ConversationId
	if _, err := fmt.Sscan(id, &value); err != nil || value <= 0 {
		return 0, badRequest("会话不存在")
	}
	return value, nil
}

func (s *Server) getConversation(r *http.Request, rawID string) (harness.Conversation, error) {
	id, err := conversationID(rawID)
	if err != nil {
		return nil, &httpError{status: http.StatusNotFound, message: "会话不存在"}
	}
	conversation, err := s.Runtime.Harness.Conversation(r.Context(), id)
	if err != nil || conversation == nil {
		return nil, &httpError{status: http.StatusNotFound, message: "会话不存在"}
	}
	return conversation, nil
}
