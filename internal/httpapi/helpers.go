package httpapi

import (
	"context"
	"encoding/json"

	"github.com/MichaelKinsy/PiG/chord/delta"
	"github.com/MichaelKinsy/PiG/durable"
	"github.com/liyu1981/pi-durable-assistant/internal/docs"
	"github.com/liyu1981/pi-durable-assistant/internal/runtime"
)

func jsonMarshal(value any) (string, error) {
	payload, err := json.Marshal(value)
	return string(payload), err
}

func runtimeSnapshot[T any](ctx context.Context, s *Server, token durable.DocToken[T]) (*T, error) {
	return runtime.Snapshot[T](ctx, s.Runtime, token)
}

func runtimeSnapshotConversation[T any](ctx context.Context, s *Server, token durable.DocToken[T], id durable.ConversationId) (*T, error) {
	return runtime.SnapshotConversation[T](ctx, s.Runtime, token, id)
}

func setJSON(draft *delta.Object, key string, value any) error {
	jsonValue, err := durable.ToJsonValue(value)
	if err != nil {
		return err
	}
	return draft.Set(key, jsonValue)
}

func writeIndex(draft *delta.Object, index *docs.IndexDoc) error {
	if err := setJSON(draft, "items", index.Items); err != nil {
		return err
	}
	if index.Children == nil {
		index.Children = map[string]int64{}
	}
	return setJSON(draft, "children", index.Children)
}
