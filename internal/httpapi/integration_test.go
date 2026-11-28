package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/liyu1981/pi-durable-assistant/internal/artifacts"
	"github.com/liyu1981/pi-durable-assistant/internal/config"
	"github.com/liyu1981/pi-durable-assistant/internal/runtime"
	"github.com/liyu1981/pi-durable-assistant/internal/settings"
)

type testClient struct {
	t             *testing.T
	base          string
	client        *http.Client
	confirmations *artifacts.ConfirmationStore
}

func newTestServer(t *testing.T) *testClient {
	t.Helper()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"object":"list","data":[]}`)
	}))
	t.Cleanup(provider.Close)
	cfg := config.Config{
		DataDir:  t.TempDir(),
		Password: "secret",
	}
	rt, err := runtime.Open(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.ApplyLLM(context.Background(), settings.Config{
		API: "openai-completions", BaseURL: provider.URL + "/v1",
		APIKey: "test-key", Model: "test-model",
		ContextWindow: 128000, MaxTokens: 8192,
	}); err != nil {
		t.Fatal(err)
	}
	assets := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<html>durable assistant</html>")}}
	server, err := NewServer(rt, fs.FS(assets))
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server)
	t.Cleanup(httpServer.Close)
	client := &http.Client{}
	return &testClient{t: t, base: httpServer.URL, client: client, confirmations: rt.Confirmations}
}

func (c *testClient) do(method, path string, body any) (int, map[string]any) {
	c.t.Helper()
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			c.t.Fatal(err)
		}
		reader = bytes.NewReader(payload)
	}
	request, err := http.NewRequest(method, c.base+path, reader)
	if err != nil {
		c.t.Fatal(err)
	}
	request.SetBasicAuth("demo", "secret")
	if body != nil {
		request.Header.Set("content-type", "application/json")
	}
	response, err := c.client.Do(request)
	if err != nil {
		c.t.Fatal(err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	result := map[string]any{}
	if len(raw) > 0 {
		// Some endpoints (the conversation list) return JSON arrays.
		if err := json.Unmarshal(raw, &result); err != nil {
			result = map[string]any{}
		}
	}
	return response.StatusCode, result
}

func (c *testClient) createConversation() int64 {
	c.t.Helper()
	status, body := c.do("POST", "/api/conversations", map[string]any{})
	if status != http.StatusCreated {
		c.t.Fatalf("create conversation: %d %v", status, body)
	}
	return int64(body["id"].(float64))
}

func TestConversationShelfRoutes(t *testing.T) {
	client := newTestServer(t)
	id := client.createConversation()
	rawRequest, _ := http.NewRequest("GET", client.base+"/api/conversations", nil)
	rawRequest.SetBasicAuth("demo", "secret")
	rawResponse, err := client.client.Do(rawRequest)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(rawResponse.Body)
	rawResponse.Body.Close()
	var conversations []map[string]any
	if err := json.Unmarshal(raw, &conversations); err != nil {
		t.Fatalf("list must be an array: %s", raw)
	}
	if len(conversations) != 1 || int64(conversations[0]["id"].(float64)) != id {
		t.Fatalf("unexpected conversations: %+v", conversations)
	}

	status, body := client.do("POST", fmt.Sprintf("/api/conversations/%d/archive", id), map[string]any{})
	if status != http.StatusOK || body["archivedAt"] == nil {
		t.Fatalf("archive failed: %d %v", status, body)
	}
	status, body = client.do("POST", fmt.Sprintf("/api/conversations/%d/restore", id), map[string]any{})
	if status != http.StatusOK || body["archivedAt"] != nil {
		t.Fatalf("restore failed: %d %v", status, body)
	}
	status, body = client.do("POST", fmt.Sprintf("/api/conversations/%d/remove", id), map[string]any{})
	if status != http.StatusOK || body["removed"] != true {
		t.Fatalf("remove failed: %d %v", status, body)
	}
	// The original app drops only the menu row: the conversation itself still accepts messages.
	status, _ = client.do("POST", fmt.Sprintf("/api/conversations/%d/messages", id), map[string]any{"text": "x"})
	if status != http.StatusAccepted {
		t.Fatalf("removed conversation should still accept messages: %d", status)
	}
}

func TestPersonalRoutes(t *testing.T) {
	client := newTestServer(t)
	status, body := client.do("GET", "/api/personal", nil)
	if status != http.StatusOK || len(body) != 3 {
		t.Fatalf("personal status: %d %v", status, body)
	}
	status, body = client.do("POST", "/api/personal/memory", map[string]any{"text": "Remember me", "source": "user", "requestId": "memo-1"})
	if status != http.StatusCreated || body["item"] == nil {
		t.Fatalf("propose memory: %d %v", status, body)
	}
	first := body["item"].(map[string]any)
	status, body = client.do("POST", "/api/personal/memory", map[string]any{"text": "Remember me", "source": "user", "requestId": "memo-1"})
	if status != http.StatusCreated {
		t.Fatalf("dedupe propose: %d %v", status, body)
	}
	second := body["item"].(map[string]any)
	if first["id"] != second["id"] {
		t.Fatal("same requestId must return the same item")
	}
	id := first["id"].(string)
	status, body = client.do("POST", "/api/personal/memory/"+id+"/approve", map[string]any{})
	if status != http.StatusOK || body["item"].(map[string]any)["status"] != "approved" {
		t.Fatalf("approve: %d %v", status, body)
	}
	status, body = client.do("GET", "/api/personal", nil)
	if status != http.StatusOK {
		t.Fatalf("list: %d", status)
	}
	status, _ = client.do("POST", "/api/personal/memory", map[string]any{"text": ""})
	if status != http.StatusBadRequest {
		t.Fatalf("empty memory must be rejected: %d", status)
	}
	status, _ = client.do("POST", "/api/personal/memory/"+id+"/reject", map[string]any{})
	if status == http.StatusNotFound {
		t.Fatal("reject route should exist")
	}
}

func TestToolsRoute(t *testing.T) {
	client := newTestServer(t)
	status, body := client.do("GET", "/api/tools", nil)
	if status != http.StatusOK {
		t.Fatalf("tools status: %d %v", status, body)
	}
	raw, ok := body["tools"].([]any)
	if !ok || len(raw) == 0 {
		t.Fatalf("tools list missing or empty: %v", body)
	}
	names := map[string]bool{}
	for _, entry := range raw {
		item, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("tool entry must be an object: %v", entry)
		}
		name, _ := item["name"].(string)
		if name == "" {
			t.Fatalf("tool entry needs a name: %v", item)
		}
		if names[name] {
			t.Fatalf("duplicate tool %q", name)
		}
		names[name] = true
	}
	for _, want := range []string{"todo", "delegate", "propose_memory", "search_knowledge", "artifact_create", "artifact_move"} {
		if !names[want] {
			t.Fatalf("missing tool %q in %v", want, names)
		}
	}
	if extensions, ok := body["extensions"].([]any); !ok || len(extensions) != 5 {
		t.Fatalf("expected five installed extensions: %v", body["extensions"])
	}
}

func TestArtifactsRoutes(t *testing.T) {
	client := newTestServer(t)

	status, body := client.do("POST", "/api/artifacts", nil)
	if status != http.StatusCreated {
		t.Fatalf("create artifact: %d %v", status, body)
	}
	created, ok := body["artifact"].(map[string]any)
	if !ok {
		t.Fatalf("create artifact body missing artifact: %v", body)
	}
	artifactID, _ := created["id"].(string)
	if artifactID == "" {
		t.Fatalf("create artifact missing id: %v", body)
	}

	status, body = client.do("GET", "/api/artifacts/"+artifactID, nil)
	if status != http.StatusOK {
		t.Fatalf("get artifact: %d %v", status, body)
	}
	if got := body["artifact"].(map[string]any)["id"].(string); got != artifactID {
		t.Fatalf("get artifact mismatch: %v", body)
	}

	status, body = client.do("PUT", "/api/artifacts/"+artifactID+"/pin", nil)
	if status != http.StatusOK {
		t.Fatalf("pin artifact: %d %v", status, body)
	}
	if got := body["artifact"].(map[string]any)["pinned"].(bool); !got {
		t.Fatalf("pin artifact: %v", body)
	}

	status, body = client.do("PUT", "/api/artifacts/"+artifactID+"/unpin", nil)
	if status != http.StatusOK {
		t.Fatalf("unpin artifact: %d %v", status, body)
	}
	if got := body["artifact"].(map[string]any)["pinned"].(bool); got {
		t.Fatalf("unpin artifact: %v", body)
	}

	status, body = client.do("GET", "/api/artifacts", nil)
	if status != http.StatusOK {
		t.Fatalf("list artifacts: %d %v", status, body)
	}
	items, ok := body["artifacts"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("list artifacts missing: %v", body)
	}

	// A pin request proposed by the agent stays pending until a human approves.
	confirmation, err := client.confirmations.Propose(context.Background(), artifactID, "pin", "agent request")
	if err != nil {
		t.Fatal(err)
	}
	status, body = client.do("GET", "/api/artifacts/confirmations", nil)
	if status != http.StatusOK {
		t.Fatalf("list confirmations: %d %v", status, body)
	}
	pending, ok := body["confirmations"].([]any)
	if !ok || len(pending) != 1 {
		t.Fatalf("pending confirmation missing: %v", body)
	}
	status, body = client.do("GET", "/api/artifacts/"+artifactID, nil)
	if got := body["artifact"].(map[string]any)["pinned"].(bool); got {
		t.Fatalf("artifact pinned without human approval: %v", body)
	}

	status, body = client.do("POST", "/api/artifacts/confirmations/"+confirmation.ID+"/approve", nil)
	if status != http.StatusOK {
		t.Fatalf("approve confirmation: %d %v", status, body)
	}
	status, body = client.do("GET", "/api/artifacts/"+artifactID, nil)
	if !body["artifact"].(map[string]any)["pinned"].(bool) {
		t.Fatalf("artifact not pinned after approval: %v", body)
	}
}

func TestArtifactFileRoutes(t *testing.T) {
	client := newTestServer(t)
	status, body := client.do("POST", "/api/artifacts", nil)
	if status != http.StatusCreated {
		t.Fatalf("create artifact: %d %v", status, body)
	}
	artifactID := body["artifact"].(map[string]any)["id"].(string)

	status, body = client.do("PUT", "/api/artifacts/"+artifactID+"/files/reports/summary.md", map[string]any{"content": "hello"})
	if status != http.StatusOK {
		t.Fatalf("write artifact file: %d %v", status, body)
	}
	status, body = client.do("GET", "/api/artifacts/"+artifactID+"/files/reports/summary.md", nil)
	if status != http.StatusOK || body["content"] != "hello" {
		t.Fatalf("read artifact file: %d %v", status, body)
	}
	status, body = client.do("GET", "/api/artifacts/"+artifactID+"/files", nil)
	if status != http.StatusOK {
		t.Fatalf("list artifact files: %d %v", status, body)
	}
	files, ok := body["files"].([]any)
	if !ok || len(files) != 1 {
		t.Fatalf("list artifact files missing: %v", body)
	}
}

func TestArtifactMoveAndRawRoutes(t *testing.T) {
	client := newTestServer(t)
	status, body := client.do("POST", "/api/artifacts", nil)
	if status != http.StatusCreated {
		t.Fatalf("create artifact: %d %v", status, body)
	}
	id := body["artifact"].(map[string]any)["id"].(string)

	status, _ = client.do("PUT", "/api/artifacts/"+id+"/files/draft/note.md", map[string]any{"content": "raw content"})
	if status != http.StatusOK {
		t.Fatalf("write file: %d", status)
	}

	request, err := http.NewRequest("GET", client.base+"/api/artifacts/"+id+"/raw/draft/note.md", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.SetBasicAuth("demo", "secret")
	response, err := client.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK || string(raw) != "raw content" {
		t.Fatalf("raw file: %d %q", response.StatusCode, raw)
	}
	if got := response.Header.Get("content-type"); !strings.HasPrefix(got, "text/plain") {
		t.Fatalf("raw content-type = %q", got)
	}

	status, body = client.do("POST", "/api/artifacts/"+id+"/move", map[string]any{
		"from": "draft/note.md", "to": "final/note.md",
	})
	if status != http.StatusOK {
		t.Fatalf("move file: %d %v", status, body)
	}
	status, body = client.do("GET", "/api/artifacts/"+id+"/raw/final/note.md", nil)
	if status != http.StatusOK {
		t.Fatalf("read moved file: %d %v", status, body)
	}

	status, _ = client.do("POST", "/api/artifacts/"+id+"/move", map[string]any{
		"from": "final/note.md", "to": "../escape.md",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("unsafe move should be rejected: %d", status)
	}
}

func TestV3ManagementRoutes(t *testing.T) {
	client := newTestServer(t)
	id := client.createConversation()
	base := fmt.Sprintf("/api/conversations/%d", id)

	status, body := client.do("POST", base+"/schedules", map[string]any{
		"text": "Hourly report", "minutesFromNow": 1, "everyMinutes": 60, "requestId": "schedule-1",
	})
	if status != http.StatusCreated || body["item"] == nil {
		t.Fatalf("create schedule: %d %v", status, body)
	}
	first := body["item"].(map[string]any)
	status, body = client.do("POST", base+"/schedules", map[string]any{
		"text": "Hourly report", "minutesFromNow": 1, "everyMinutes": 60, "requestId": "schedule-1",
	})
	if status != http.StatusCreated || body["item"].(map[string]any)["id"] != first["id"] {
		t.Fatalf("schedule dedupe: %d %v", status, body)
	}
	scheduleID := first["id"].(string)

	status, _ = client.do("POST", base+"/schedules", map[string]any{"text": "", "minutesFromNow": 1})
	if status != http.StatusBadRequest {
		t.Fatalf("invalid schedule must be rejected: %d", status)
	}

	status, body = client.do("GET", base+"/management", nil)
	if status != http.StatusOK || len(body["schedules"].([]any)) != 1 {
		t.Fatalf("management list: %d %v", status, body)
	}

	for _, action := range []string{"pause", "resume", "cancel"} {
		status, body = client.do("POST", base+"/schedules/"+scheduleID+"/"+action, map[string]any{})
		if status != http.StatusOK || body["item"] == nil {
			t.Fatalf("schedule %s: %d %v", action, status, body)
		}
	}
	status, _ = client.do("POST", base+"/schedules/"+scheduleID+"/bogus", map[string]any{})
	if status != http.StatusNotFound {
		t.Fatalf("unknown action must 404: %d", status)
	}

	status, body = client.do("POST", base+"/background", map[string]any{"text": "background task", "requestId": "bg-1"})
	if status != http.StatusCreated || body["item"] == nil {
		t.Fatalf("create background: %d %v", status, body)
	}
	backgroundID := body["item"].(map[string]any)["id"].(string)
	status, body = client.do("POST", base+"/background", map[string]any{"text": "background task", "requestId": "bg-1"})
	if status != http.StatusCreated || body["item"].(map[string]any)["id"] != backgroundID {
		t.Fatalf("background dedupe: %d %v", status, body)
	}
	status, body = client.do("POST", base+"/background/"+backgroundID+"/stop", map[string]any{})
	if status != http.StatusOK || body["item"].(map[string]any)["status"] != "cancelled" {
		t.Fatalf("stop background: %d %v", status, body)
	}
	status, body = client.do("GET", base+"/management", nil)
	if status != http.StatusOK || len(body["background"].([]any)) != 1 {
		t.Fatalf("management background: %d %v", status, body)
	}
	status, _ = client.do("POST", base+"/reminders/missing/cancel", map[string]any{})
	if status != http.StatusBadRequest {
		t.Fatalf("missing reminder cancel must fail: %d", status)
	}
}

func TestSystemPromptRoute(t *testing.T) {
	client := newTestServer(t)
	status, body := client.do("GET", "/api/system-prompt", nil)
	if status != http.StatusOK {
		t.Fatalf("system prompt: %d %v", status, body)
	}
	if _, ok := body["sections"].([]any); !ok {
		t.Fatalf("system prompt sections missing: %v", body)
	}
	if _, ok := body["prompt"].(string); !ok {
		t.Fatalf("system prompt text missing: %v", body)
	}
}

func TestRenameConversationRoute(t *testing.T) {
	client := newTestServer(t)
	id := client.createConversation()

	status, body := client.do("PUT", fmt.Sprintf("/api/conversations/%d/title", id), map[string]any{
		"title": "  My renamed chat  ",
	})
	if status != http.StatusOK || body["title"] != "My renamed chat" {
		t.Fatalf("rename: %d %v", status, body)
	}

	status, _ = client.do("PUT", fmt.Sprintf("/api/conversations/%d/title", id), map[string]any{"title": ""})
	if status != http.StatusBadRequest {
		t.Fatalf("empty title should be rejected: %d", status)
	}
}
