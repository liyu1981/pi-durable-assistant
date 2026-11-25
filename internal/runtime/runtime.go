// Package runtime wires PiG's harness, model provider and durable documents together.
package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/durable"
	"github.com/MichaelKinsy/PiG/durable/harness"
	"github.com/MichaelKinsy/PiG/durable/storage/sqlite/node"
	"github.com/liyu1981/pi-durable-assistant/internal/config"
	"github.com/liyu1981/pi-durable-assistant/internal/docs"
	"github.com/liyu1981/pi-durable-assistant/internal/knowledge"
	"github.com/liyu1981/pi-durable-assistant/internal/personal"
	"github.com/liyu1981/pi-durable-assistant/internal/tools"
)

// Runtime owns the harness and all shared application state.
type Runtime struct {
	Config     config.Config
	Harness    harness.Harness
	Root       harness.Conversation
	ModelRef   durable.ModelRef
	ModelLabel string
}

func boolPtr(value bool) *bool { return &value }

func createModels(cfg config.Config) (*ai.Models, durable.ModelRef, string, error) {
	models := ai.CreateModels()
	apiType := ai.API(cfg.API)
	var streams *ai.ProviderStreams
	switch cfg.API {
	case "openai-completions":
		streams = ai.OpenAICompletionsAPI()
	case "openai-responses":
		streams = ai.OpenAIResponsesAPI()
	case "anthropic-messages":
		streams = ai.AnthropicMessagesAPI()
	default:
		return nil, durable.ModelRef{}, "", fmt.Errorf("unsupported LLM_API:%s(for the local demo use pnpm dev:demo)", cfg.API)
	}
	if cfg.BaseURL == "" || cfg.Model == "" {
		return nil, durable.ModelRef{}, "", fmt.Errorf("missing LLM_BASE_URL or LLM_MODEL")
	}
	apiType = ai.API(cfg.API)
	compat := &ai.ModelCompat{}
	if cfg.API == "openai-completions" {
		compat.SupportsStore = boolPtr(false)
		compat.SupportsDeveloperRole = boolPtr(false)
	}
	model := ai.Model{
		ID:          cfg.Model,
		DisplayName: cfg.Model,
		Input:       []string{"text", "image"},
		Capabilities: ai.ModelCapabilities{
			ContextWindow: cfg.ContextWindow, MaxOutputTokens: cfg.MaxTokens, SupportsToolUse: true,
		},
		ProviderMeta: ai.ProviderMetadata{
			ProviderID: "demo", API: apiType, BaseURL: cfg.BaseURL, Compat: compat,
		},
	}
	key := cfg.APIKey
	provider := ai.CreateProvider(ai.CreateProviderOptions{
		ID:      "demo",
		Name:    stringPtr("Demo"),
		BaseURL: cfg.BaseURL,
		Auth: ai.ProviderAuth{APIKey: &ai.APIKeyAuth{
			Name: "LLM API key",
			Resolve: func(ctx context.Context, input ai.APIKeyAuthInput) (*ai.AuthResult, error) {
				if key == "" {
					return nil, nil
				}
				return &ai.AuthResult{
					Auth:   ai.ModelAuth{APIKey: key, BaseURL: cfg.BaseURL},
					Source: "LLM_API_KEY", SourcePresent: true,
				}, nil
			},
		}},
		Models: []ai.AnyModel{&model},
		API:    streams,
	})
	models.SetProvider(provider)
	return models, durable.ModelRef{Provider: "demo", ModelId: cfg.Model}, cfg.Model, nil
}

func stringPtr(value string) *string { return &value }

// Open initializes storage, the registry and the harness, then resumes unfinished work.
func Open(ctx context.Context, cfg config.Config) (*Runtime, error) {
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return nil, err
	}
	models, model, label, err := createModels(cfg)
	if err != nil {
		return nil, err
	}
	storage, err := node.OpenNodeSqliteStorage(filepath.Join(cfg.DataDir, "agent.sqlite"), node.NodeSqliteStorageOptions{})
	if err != nil {
		return nil, err
	}
	registry := harness.CreateRegistry()
	for _, extension := range []*durable.Extension{
		tools.Assistant, tools.Delegate, personal.Extension, knowledge.Extension,
	} {
		if err := registry.Install(extension); err != nil {
			return nil, err
		}
	}
	harnessInstance, err := harness.OpenHarness(ctx, storage, harness.HarnessOptions{
		Models:   models,
		Registry: registry,
		OnReport: func(report durable.Report) {
			fmt.Fprintln(os.Stderr, "Harness report:", cfg.Redact(report.Error.Error()))
		},
	})
	if err != nil {
		return nil, err
	}
	root, err := harnessInstance.Root(ctx, &harness.RootOptions{Agent: &harness.AgentChange{Model: harness.SetTo(model)}})
	if err != nil {
		return nil, err
	}
	_, err = root.Commit(ctx, func(tx durable.Tx) (any, error) {
		draft, err := durable.TxDoc(tx, docs.Meta)
		if err != nil {
			return nil, err
		}
		meta, err := durable.DecodeDoc[docs.MetaDoc](draft.Snapshot())
		if err != nil {
			return nil, err
		}
		meta.Boots++
		now := time.Now().UTC().Format(time.RFC3339)
		meta.LastBootAt = &now
		for _, key := range draft.Keys() {
			draft.Delete(key)
		}
		boots, err := durable.ToJsonValue(meta.Boots)
		if err != nil {
			return nil, err
		}
		if err := draft.Set("boots", boots); err != nil {
			return nil, err
		}
		stamp, err := durable.ToJsonValue(now)
		if err != nil {
			return nil, err
		}
		return nil, draft.Set("lastBootAt", stamp)
	})
	if err != nil {
		return nil, err
	}
	harnessInstance.Resume()
	return &Runtime{Config: cfg, Harness: harnessInstance, Root: root, ModelRef: model, ModelLabel: label}, nil
}

// Context returns the harness context.
func (r *Runtime) Context() context.Context { return context.Background() }

// Snapshot reads a session-scoped document.
func Snapshot[T any](ctx context.Context, rt *Runtime, token durable.DocToken[T]) (*T, error) {
	return durable.Snapshot[T](ctx, rt.Harness, token)
}

// SnapshotConversation reads a conversation-scoped document.
func SnapshotConversation[T any](ctx context.Context, rt *Runtime, token durable.DocToken[T], id durable.ConversationId) (*T, error) {
	return durable.Snapshot[T](ctx, rt.Harness, token, id)
}
