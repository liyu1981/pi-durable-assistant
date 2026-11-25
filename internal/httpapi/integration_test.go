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
	"testing"
	"testing/fstest"

	"github.com/liyu1981/pi-durable-assistant/internal/config"
	"github.com/liyu1981/pi-durable-assistant/internal/runtime"
)

type testClient struct {
	t      *testing.T
	base   string
	client *http.Client
}

func newTestServer(t *testing.T) *testClient {
	t.Helper()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"object":"list","data":[]}`)
	}))
	t.Cleanup(provider.Close)
	cfg := config.Config{
		DataDir:       t.TempDir(),
		Password:      "secret",
		API:           "openai-completions",
		BaseURL:       provider.URL + "/v1",
		APIKey:        "test-key",
		Model:         "test-model",
		ContextWindow: 128000,
		MaxTokens:     8192,
	}
	rt, err := runtime.Open(context.Background(), cfg)
	if err != nil {
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
	return &testClient{t: t, base: httpServer.URL, client: client}
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
	status, body = client.do("POST", "/api/personal/memory", map[string]any{"text": "记住我", "source": "user", "requestId": "memo-1"})
	if status != http.StatusCreated || body["item"] == nil {
		t.Fatalf("propose memory: %d %v", status, body)
	}
	first := body["item"].(map[string]any)
	status, body = client.do("POST", "/api/personal/memory", map[string]any{"text": "记住我", "source": "user", "requestId": "memo-1"})
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

func TestV3ManagementRoutes(t *testing.T) {
	client := newTestServer(t)
	id := client.createConversation()
	base := fmt.Sprintf("/api/conversations/%d", id)

	status, body := client.do("POST", base+"/schedules", map[string]any{
		"text": "每小时汇报", "minutesFromNow": 1, "everyMinutes": 60, "requestId": "schedule-1",
	})
	if status != http.StatusCreated || body["item"] == nil {
		t.Fatalf("create schedule: %d %v", status, body)
	}
	first := body["item"].(map[string]any)
	status, body = client.do("POST", base+"/schedules", map[string]any{
		"text": "每小时汇报", "minutesFromNow": 1, "everyMinutes": 60, "requestId": "schedule-1",
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

	status, body = client.do("POST", base+"/background", map[string]any{"text": "后台任务", "requestId": "bg-1"})
	if status != http.StatusCreated || body["item"] == nil {
		t.Fatalf("create background: %d %v", status, body)
	}
	backgroundID := body["item"].(map[string]any)["id"].(string)
	status, body = client.do("POST", base+"/background", map[string]any{"text": "后台任务", "requestId": "bg-1"})
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
