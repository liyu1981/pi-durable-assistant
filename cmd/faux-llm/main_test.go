package main

import (
	"encoding/json"
	"testing"
)

func TestDeterministicScript(t *testing.T) {
	steps := []struct {
		assistantToolCalls int
		wantFinish         string
		wantToolName       string
	}{
		{0, "tool_calls", "todo"},
		{1, "tool_calls", "wait_seconds"},
		{2, "tool_calls", "todo"},
		{3, "stop", ""},
	}
	for _, stepSpec := range steps {
		messages := []message{{Role: "user", Content: textContent("hello")}}
		for i := 0; i < stepSpec.assistantToolCalls; i++ {
			messages = append(messages, message{Role: "assistant", Content: textContent(""), ToolCalls: []toolCall{call("c", "todo", map[string]any{})}})
		}
		payload, _ := json.Marshal(chatRequest{Messages: messages, Model: "faux-1"})
		var request chatRequest
		if err := json.Unmarshal(payload, &request); err != nil {
			t.Fatal(err)
		}
		selected := step(request)
		if selected.FinishReason != stepSpec.wantFinish {
			t.Fatalf("step %d: finish reason %q", stepSpec.assistantToolCalls, selected.FinishReason)
		}
		if stepSpec.wantToolName != "" && (len(selected.Message.ToolCalls) == 0 || selected.Message.ToolCalls[0].Function.Name != stepSpec.wantToolName) {
			t.Fatalf("step %d: wrong tool call", stepSpec.assistantToolCalls)
		}
	}
}
