// Command faux-llm is a development-only OpenAI-compatible deterministic provider.
// It is never imported by the main application; see `make dev-faux`.
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

type message struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	ToolCalls  []toolCall      `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type chatRequest struct {
	Messages []message `json:"messages"`
	Stream   bool      `json:"stream"`
	Model    string    `json:"model"`
}

type choice struct {
	Index        int      `json:"index"`
	Message      message  `json:"message"`
	FinishReason string   `json:"finish_reason"`
	Delta        *message `json:"delta,omitempty"`
}

type response struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []choice `json:"choices"`
	Usage   struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func textContent(content string) json.RawMessage {
	payload, _ := json.Marshal(content)
	return payload
}

func arguments(value map[string]any) string {
	payload, _ := json.Marshal(value)
	return string(payload)
}

func call(id, name string, args map[string]any) toolCall {
	var result toolCall
	result.ID = id
	result.Type = "function"
	result.Function.Name = name
	result.Function.Arguments = arguments(args)
	return result
}

// step reproduces the original deterministic demo script:
// add three todos -> wait 10 seconds -> mark them done -> final answer.
func step(request chatRequest) choice {
	replies := 0
	for _, item := range request.Messages {
		if item.Role == "assistant" && len(item.ToolCalls) > 0 {
			replies++
		}
	}
	steps := []string{"Step 1: Research", "Step 2: Wait 10 seconds", "Step 3: Summarize"}
	switch replies {
	case 0:
		calls := []toolCall{}
		for i, text := range steps {
			calls = append(calls, call(fmt.Sprintf("call_todo_%d", i), "todo", map[string]any{"action": "add", "text": text}))
		}
		return choice{Index: 0, Message: message{Role: "assistant", Content: textContent(""), ToolCalls: calls}, FinishReason: "tool_calls"}
	case 1:
		return choice{Index: 0, Message: message{Role: "assistant", Content: textContent(""), ToolCalls: []toolCall{
			call("call_wait", "wait_seconds", map[string]any{"seconds": 10, "reason": "demo long task"}),
		}}, FinishReason: "tool_calls"}
	case 2:
		calls := []toolCall{}
		for i, text := range steps {
			calls = append(calls, call(fmt.Sprintf("call_done_%d", i), "todo", map[string]any{"action": "done", "text": text}))
		}
		return choice{Index: 0, Message: message{Role: "assistant", Content: textContent(""), ToolCalls: calls}, FinishReason: "tool_calls"}
	default:
		return choice{Index: 0, Message: message{Role: "assistant", Content: textContent("Demo task complete.")}, FinishReason: "stop"}
	}
}

func main() {
	port := "18789"
	if value := os.Getenv("FAUX_PORT"); value != "" {
		port = value
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		fmt.Fprint(w, `{"object":"list","data":[{"id":"faux-1","object":"model","owned_by":"faux"}]}`)
	})
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var request chatRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		selected := step(request)
		answer := response{ID: "chatcmpl-faux", Object: "chat.completion", Model: request.Model, Choices: []choice{selected}}
		answer.Usage.TotalTokens = 16
		w.Header().Set("content-type", "application/json")
		if !request.Stream {
			json.NewEncoder(w).Encode(answer)
			return
		}
		w.Header().Set("content-type", "text/event-stream")
		w.Header().Set("cache-control", "no-cache")
		flusher, _ := w.(http.Flusher)
		chunk := response{ID: answer.ID, Object: "chat.completion.chunk", Model: answer.Model, Choices: []choice{{
			Index: 0, Delta: &message{Role: "assistant", Content: selected.Message.Content, ToolCalls: selected.Message.ToolCalls},
			FinishReason: selected.FinishReason,
		}}}
		payload, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", payload)
		if flusher != nil {
			flusher.Flush()
		}
		done := response{ID: answer.ID, Object: "chat.completion.chunk", Model: answer.Model, Choices: []choice{{Index: 0, Delta: &message{}}}}
		payload, _ = json.Marshal(done)
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", payload)
		if flusher != nil {
			flusher.Flush()
		}
	})
	addr := "127.0.0.1:" + port
	fmt.Println("faux-llm dev server listening on", addr, "(dev only)")
	http.ListenAndServe(addr, mux)
}

var _ = strings.TrimSpace
