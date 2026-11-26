// Package runtime wires PiG's harness, model provider and durable documents together.
package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/durable"
	"github.com/MichaelKinsy/PiG/durable/harness"
	"github.com/MichaelKinsy/PiG/durable/storage/sqlite/node"
	"github.com/liyu1981/pi-durable-assistant/internal/config"
	"github.com/liyu1981/pi-durable-assistant/internal/docs"
	"github.com/liyu1981/pi-durable-assistant/internal/knowledge"
	"github.com/liyu1981/pi-durable-assistant/internal/personal"
	"github.com/liyu1981/pi-durable-assistant/internal/settings"
	"github.com/liyu1981/pi-durable-assistant/internal/tools"
)

// Runtime owns the harness and all shared application state.
type Runtime struct {
	Config  config.Config
	Harness harness.Harness
	Root    harness.Conversation
	Models  *ai.Models
	// Settings persists the UI-owned model configuration in agent.sqlite.
	Settings *settings.Store

	mu         sync.RWMutex
	llm        settings.Config
	modelRef   durable.ModelRef
	modelLabel string
	redactor   *secretHolder
}

// secretHolder lets the harness's boot-time report callback redact a key that
// can later be changed from the UI.
type secretHolder struct {
	mu    sync.RWMutex
	value string
}

func (h *secretHolder) get() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.value
}

func (h *secretHolder) set(value string) {
	h.mu.Lock()
	h.value = value
	h.mu.Unlock()
}

func boolPtr(value bool) *bool { return &value }

func stringPtr(value string) *string { return &value }

// llmFromEnv seeds a brand-new database from the legacy environment
// variables. Once a row exists, the database is authoritative and env changes
// are ignored until the settings row is reset.
func llmFromEnv(cfg config.Config) settings.Config {
	return settings.Config{
		API:           cfg.API,
		BaseURL:       cfg.BaseURL,
		APIKey:        cfg.APIKey,
		Model:         cfg.Model,
		ContextWindow: cfg.ContextWindow,
		MaxTokens:     cfg.MaxTokens,
	}
}

// buildProvider constructs the single provider from stored settings. The
// provider ID stays constant so it can be replaced without a restart.
func buildProvider(cfg settings.Config) (*ai.ModelsProvider, error) {
	var streams *ai.ProviderStreams
	switch cfg.API {
	case "openai-completions":
		streams = ai.OpenAICompletionsAPI()
	case "openai-responses":
		streams = ai.OpenAIResponsesAPI()
	case "anthropic-messages":
		streams = ai.AnthropicMessagesAPI()
	default:
		return nil, fmt.Errorf("unsupported API %s", cfg.API)
	}
	if cfg.BaseURL == "" || cfg.Model == "" {
		return nil, fmt.Errorf("base URL and model are required")
	}
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
			ProviderID: settings.ProviderID, API: ai.API(cfg.API), BaseURL: cfg.BaseURL, Compat: compat,
		},
	}
	key, base := cfg.APIKey, cfg.BaseURL
	return ai.CreateProvider(ai.CreateProviderOptions{
		ID:      settings.ProviderID,
		Name:    stringPtr("Demo"),
		BaseURL: base,
		Auth: ai.ProviderAuth{APIKey: &ai.APIKeyAuth{
			Name: "Provider API key",
			Resolve: func(ctx context.Context, input ai.APIKeyAuthInput) (*ai.AuthResult, error) {
				if key == "" {
					return nil, nil
				}
				return &ai.AuthResult{
					Auth:   ai.ModelAuth{APIKey: key, BaseURL: base},
					Source: "settings", SourcePresent: true,
				}, nil
			},
		}},
		Models: []ai.AnyModel{&model},
		API:    streams,
	}), nil
}

// createModels returns an empty catalog when the stored configuration is not
// usable yet. The app must still boot so the user can reach the settings UI;
// sending a message before configuring a provider reports "no model" without
// taking down the server.
func createModels(cfg settings.Config) (*ai.Models, durable.ModelRef, string) {
	models := ai.CreateModels()
	if err := cfg.Validate(); err != nil {
		return models, durable.ModelRef{}, "Not configured"
	}
	provider, err := buildProvider(cfg)
	if err != nil {
		return models, durable.ModelRef{}, "Not configured"
	}
	models.SetProvider(provider)
	return models, durable.ModelRef{Provider: settings.ProviderID, ModelId: cfg.Model}, cfg.Model
}

// Open initializes storage, the registry and the harness, then resumes unfinished work.
func Open(ctx context.Context, cfg config.Config) (*Runtime, error) {
	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return nil, err
	}
	dbPath := filepath.Join(cfg.DataDir, "agent.sqlite")
	store, err := settings.Open(dbPath, llmFromEnv(cfg))
	if err != nil {
		return nil, err
	}
	stored, err := store.Load()
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	models, model, label := createModels(stored)
	storage, err := node.OpenNodeSqliteStorage(dbPath, node.NodeSqliteStorageOptions{})
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	redactor := &secretHolder{value: stored.APIKey}
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
			fmt.Fprintln(os.Stderr, "Harness report:", RedactValue(redactor.get(), report.Error.Error()))
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
	rt := &Runtime{
		Config:     cfg,
		Harness:    harnessInstance,
		Root:       root,
		Models:     models,
		Settings:   store,
		llm:        stored,
		modelRef:   model,
		modelLabel: label,
		redactor:   redactor,
	}
	harnessInstance.Resume()
	return rt, nil
}

// RedactValue removes a secret from text before it is logged.
func RedactValue(secret, value string) string {
	if len(secret) >= 4 {
		return strings.ReplaceAll(value, secret, "***")
	}
	return value
}

// LLM returns the active configuration without the secret.
func (r *Runtime) LLM() settings.Public {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.llm.Public()
}

// LLMConfig returns a copy including the secret, for internal use only.
func (r *Runtime) LLMConfig() settings.Config {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.llm
}

// Model returns the model reference new conversations should inherit.
func (r *Runtime) Model() durable.ModelRef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.modelRef
}

// Label returns the model name shown in status and startup output.
func (r *Runtime) Label() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.modelLabel
}

// Redact removes the active API key from a string before logging it.
func (r *Runtime) Redact(value string) string {
	r.mu.RLock()
	secret := r.llm.APIKey
	r.mu.RUnlock()
	return RedactValue(secret, value)
}

// ApplyLLM stores a new configuration and switches the live provider to it.
// Existing conversations are updated too, so the next message in an open chat
// uses the new model without a restart.
func (r *Runtime) ApplyLLM(ctx context.Context, next settings.Config) error {
	if err := next.Validate(); err != nil {
		return err
	}
	provider, err := buildProvider(next)
	if err != nil {
		return err
	}
	previous := r.LLMConfig()
	if err := r.Settings.Save(next); err != nil {
		return err
	}

	r.Models.SetProvider(provider)
	r.redactor.set(next.APIKey)
	ref := durable.ModelRef{Provider: settings.ProviderID, ModelId: next.Model}
	r.mu.Lock()
	r.llm, r.modelRef, r.modelLabel = next, ref, next.Model
	r.mu.Unlock()

	if err := r.Root.Configure(ctx, harness.AgentChange{Model: harness.SetTo(ref)}); err != nil {
		r.rollback(ctx, previous)
		return err
	}
	// Other conversations are best effort: one locked or archived chat must not
	// undo a provider that already works for new messages.
	index, err := Snapshot[docs.IndexDoc](ctx, r, docs.Index)
	if err != nil || index == nil {
		return nil
	}
	for _, item := range index.Items {
		if item.Removed {
			continue
		}
		conversation, err := r.Harness.Conversation(ctx, durable.ConversationId(item.ID))
		if err != nil || conversation == nil {
			continue
		}
		if err := conversation.Configure(ctx, harness.AgentChange{Model: harness.SetTo(ref)}); err != nil {
			fmt.Fprintln(os.Stderr, "could not update conversation model:", item.ID, r.Redact(err.Error()))
		}
	}
	return nil
}

func (r *Runtime) rollback(ctx context.Context, previous settings.Config) {
	provider, err := buildProvider(previous)
	if err != nil {
		return
	}
	r.Models.SetProvider(provider)
	r.redactor.set(previous.APIKey)
	ref := durable.ModelRef{Provider: settings.ProviderID, ModelId: previous.Model}
	r.mu.Lock()
	r.llm, r.modelRef, r.modelLabel = previous, ref, previous.Model
	r.mu.Unlock()
	if err := r.Root.Configure(ctx, harness.AgentChange{Model: harness.SetTo(ref)}); err != nil {
		fmt.Fprintln(os.Stderr, "could not restore previous model:", r.Redact(err.Error()))
	}
	_ = r.Settings.Save(previous)
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
