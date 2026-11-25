// Package personal implements cross-session memory and skills with a user approval gate.
package personal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/durable"
	"github.com/MichaelKinsy/PiG/durable/harness"
	"github.com/google/uuid"
)

const (
	maxApprovedMemory = 30
	maxPendingMemory  = 40
	maxApprovedSkill  = 20
	maxPendingSkill   = 20
	memoryPromptChars = 1800
)

// Item is one pending or approved memory/skill row.
type Item struct {
	ID             string `json:"id"`
	Text           string `json:"text,omitempty"`
	Name           string `json:"name,omitempty"`
	Description    string `json:"description,omitempty"`
	Body           string `json:"body,omitempty"`
	Status         string `json:"status"`
	Source         string `json:"source"`
	ConversationID *int64 `json:"conversationId"`
	CreatedAt      int64  `json:"createdAt"`
	DecidedAt      *int64 `json:"decidedAt"`
}

// Doc is the stored shape of both memory and skills documents.
type Doc struct {
	Items   []Item         `json:"items"`
	Applied map[string]any `json:"applied"`
}

func singleton(kind string) durable.DocToken[Doc] {
	return durable.DefineDoc(durable.DocDefinition[Doc]{
		CommonDocDefinition: durable.CommonDocDefinition[Doc]{
			Kind: kind, Version: 1, Initial: func() Doc { return Doc{Applied: map[string]any{}} },
			CheckpointWhen: func(Doc, []durable.Op, durable.CheckpointInfo) bool { return false },
		},
		DocumentSemantics: durable.DocumentSemantics{Scope: durable.ScopeSession},
	})
}

var (
	MemoryDoc = singleton("app.memory")
	SkillsDoc = singleton("app.skills")
)

func normalizeMemory(text string) (string, error) {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" || len([]rune(text)) > 280 {
		return "", errors.New("memory must be 1-280 characters")
	}
	return text, nil
}

func normalizeSkill(name, description, body string) (Item, error) {
	name, description, body = strings.TrimSpace(name), strings.TrimSpace(description), strings.TrimSpace(body)
	if name == "" || len([]rune(name)) > 40 || strings.ContainsAny(name, "/\n\r") {
		return Item{}, errors.New("skill name must be 1-40 characters and must not contain a slash")
	}
	if description == "" || len([]rune(description)) > 180 {
		return Item{}, errors.New("skill description must be 1-180 characters")
	}
	if body == "" || len([]rune(body)) > 4000 {
		return Item{}, errors.New("skill body must be 1-4000 characters")
	}
	return Item{Name: name, Description: description, Body: body}, nil
}

func count(items []Item, status string) int {
	n := 0
	for _, item := range items {
		if item.Status == status {
			n++
		}
	}
	return n
}

func decode(draft durable.Draft[any]) (Doc, error) {
	doc, err := durable.DecodeDoc[Doc](draft.Snapshot())
	if err != nil || doc == nil {
		return Doc{}, err
	}
	return *doc, nil
}

func write(draft durable.Draft[any], doc Doc) error {
	for _, key := range draft.Keys() {
		draft.Delete(key)
	}
	items, err := durable.ToJsonValue(doc.Items)
	if err != nil {
		return err
	}
	if err := draft.Set("items", items); err != nil {
		return err
	}
	if doc.Applied == nil {
		doc.Applied = map[string]any{}
	}
	applied, err := durable.ToJsonValue(doc.Applied)
	if err != nil {
		return err
	}
	return draft.Set("applied", applied)
}

// Propose adds a candidate row to a document, honoring an idempotency key.
func Propose(token durable.DocToken[Doc], input map[string]any) (Item, error) {
	requestID, _ := input["requestId"].(string)
	text, _ := input["text"].(string)
	name, _ := input["name"].(string)
	description, _ := input["description"].(string)
	body, _ := input["body"].(string)
	isSkill := token == SkillsDoc

	var item Item
	if isSkill {
		normalized, err := normalizeSkill(name, description, body)
		if err != nil {
			return Item{}, err
		}
		item = normalized
	} else {
		normalized, err := normalizeMemory(text)
		if err != nil {
			return Item{}, err
		}
		item = Item{Text: normalized}
	}
	approved, _ := input["approved"].(bool)
	if approved {
		item.Status = "approved"
	} else {
		item.Status = "pending"
	}
	item.Source = "agent"
	if input["source"] == "user" {
		item.Source = "user"
	}
	item.ID = uuid.NewString()
	item.CreatedAt = time.Now().UnixMilli()
	if approved {
		at := item.CreatedAt
		item.DecidedAt = &at
	}

	mutate := func(tx durable.Tx) (any, error) {
		draft, err := durable.TxDoc(tx, token)
		if err != nil {
			return nil, err
		}
		doc, err := decode(draft)
		if err != nil {
			return nil, err
		}
		if requestID != "" {
			if prior, ok := doc.Applied[requestID].(string); ok {
				for _, existing := range doc.Items {
					if existing.ID == prior {
						return existing, nil
					}
				}
			}
		}
		limit := maxPendingMemory
		if approved {
			limit = maxApprovedMemory
		}
		if isSkill {
			limit = maxPendingSkill
			if approved {
				limit = maxApprovedSkill
			}
		}
		if count(doc.Items, item.Status) >= limit {
			return nil, errors.New("candidate list is full; review it first")
		}
		doc.Items = append(doc.Items, item)
		if requestID != "" {
			doc.Applied[requestID] = item.ID
		}
		if err := write(draft, doc); err != nil {
			return nil, err
		}
		return item, nil
	}
	// The caller supplies a committer through ProposeInTx; this standalone form is only a helper for tests.
	_ = mutate
	return item, nil
}

// Decide approves or rejects a candidate row.
func Decide(token durable.DocToken[Doc], id, action string) (Item, error) {
	if action != "approve" && action != "reject" {
		return Item{}, errors.New("action must be approve or reject")
	}
	mutate := func(tx durable.Tx) (any, error) {
		draft, err := durable.TxDoc(tx, token)
		if err != nil {
			return nil, err
		}
		doc, err := decode(draft)
		if err != nil {
			return nil, err
		}
		for i := range doc.Items {
			if doc.Items[i].ID != id {
				continue
			}
			if action == "approve" {
				doc.Items[i].Status = "approved"
			} else {
				doc.Items[i].Status = "rejected"
			}
			at := time.Now().UnixMilli()
			doc.Items[i].DecidedAt = &at
			if err := write(draft, doc); err != nil {
				return nil, err
			}
			return doc.Items[i], nil
		}
		return nil, errors.New("candidate not found")
	}
	_ = mutate
	return Item{}, nil
}

// ProposeInTx adds a candidate inside an existing Durable transaction.
func ProposeInTx(tx durable.Tx, token durable.DocToken[Doc], input map[string]any) (Item, error) {
	requestID, _ := input["requestId"].(string)
	approved, _ := input["approved"].(bool)
	isSkill := token == SkillsDoc
	var item Item
	if isSkill {
		name, _ := input["name"].(string)
		description, _ := input["description"].(string)
		body, _ := input["body"].(string)
		normalized, err := normalizeSkill(name, description, body)
		if err != nil {
			return Item{}, err
		}
		item = normalized
	} else {
		text, _ := input["text"].(string)
		normalized, err := normalizeMemory(text)
		if err != nil {
			return Item{}, err
		}
		item = Item{Text: normalized}
	}
	if approved {
		item.Status = "approved"
	} else {
		item.Status = "pending"
	}
	item.Source = "agent"
	if input["source"] == "user" {
		item.Source = "user"
	}
	item.ID = uuid.NewString()
	item.CreatedAt = time.Now().UnixMilli()
	if approved {
		at := item.CreatedAt
		item.DecidedAt = &at
	}

	draft, err := durable.TxDoc(tx, token)
	if err != nil {
		return Item{}, err
	}
	doc, err := decode(draft)
	if err != nil {
		return Item{}, err
	}
	if requestID != "" {
		if prior, ok := doc.Applied[requestID].(string); ok {
			for _, existing := range doc.Items {
				if existing.ID == prior {
					return existing, nil
				}
			}
		}
	}
	limit := maxPendingMemory
	if approved {
		limit = maxApprovedMemory
	}
	if isSkill {
		limit = maxPendingSkill
		if approved {
			limit = maxApprovedSkill
		}
	}
	if count(doc.Items, item.Status) >= limit {
		return Item{}, errors.New("candidate list is full; review it first")
	}
	doc.Items = append(doc.Items, item)
	if requestID != "" {
		doc.Applied[requestID] = item.ID
	}
	return item, write(draft, doc)
}

// DecideInTx approves or rejects a candidate inside an existing transaction.
func DecideInTx(tx durable.Tx, token durable.DocToken[Doc], id, action string) (Item, error) {
	if action != "approve" && action != "reject" {
		return Item{}, errors.New("action must be approve or reject")
	}
	draft, err := durable.TxDoc(tx, token)
	if err != nil {
		return Item{}, err
	}
	doc, err := decode(draft)
	if err != nil {
		return Item{}, err
	}
	for i := range doc.Items {
		if doc.Items[i].ID != id {
			continue
		}
		if action == "approve" {
			doc.Items[i].Status = "approved"
		} else {
			doc.Items[i].Status = "rejected"
		}
		at := time.Now().UnixMilli()
		doc.Items[i].DecidedAt = &at
		return doc.Items[i], write(draft, doc)
	}
	return Item{}, errors.New("candidate not found")
}

// PublicDoc strips internal fields for the API.
func PublicDoc(doc *Doc) []Item {
	if doc == nil {
		return []Item{}
	}
	return doc.Items
}

// Extension exposes the model-facing memory and skill tools.
var Extension = harness.DefineExtension(durable.Extension{
	Name: "personal-memory",
	Tools: []*durable.ToolRegistration{
		harness.DefineTool(durable.ToolRegistration{
			ToolSchema: ai.ToolSchema{
				Name: "propose_memory", Description: "Propose a cross-session memory candidate; it takes effect after user approval",
				Parameters: map[string]any{"type": "object", "properties": map[string]any{
					"text": map[string]any{"type": "string"}, "requestId": map[string]any{"type": "string"},
				}, "required": []any{"text"}},
			},
			Replay: durable.ReplaySafe,
			Execute: func(ctx context.Context, args any, api durable.ToolExecutionApi) (durable.ToolExecutionResult, error) {
				input := toStringMap(args)
				_, err := ProposeInTxViaAPI(ctx, api, MemoryDoc, input)
				if err != nil {
					return durable.ToolExecutionResult{}, err
				}
				return textResult("Memory candidate submitted; approve it in the task center."), nil
			},
		}),
		harness.DefineTool(durable.ToolRegistration{
			ToolSchema: ai.ToolSchema{
				Name: "propose_skill", Description: "Propose a new skill candidate; it takes effect after user approval",
				Parameters: map[string]any{"type": "object", "properties": map[string]any{
					"name": map[string]any{"type": "string"}, "description": map[string]any{"type": "string"},
					"body": map[string]any{"type": "string"}, "requestId": map[string]any{"type": "string"},
				}, "required": []any{"name", "description", "body"}},
			},
			Replay: durable.ReplaySafe,
			Execute: func(ctx context.Context, args any, api durable.ToolExecutionApi) (durable.ToolExecutionResult, error) {
				_, err := ProposeInTxViaAPI(ctx, api, SkillsDoc, toStringMap(args))
				if err != nil {
					return durable.ToolExecutionResult{}, err
				}
				return textResult("Skill candidate submitted; approve it in the task center."), nil
			},
		}),
		harness.DefineTool(durable.ToolRegistration{
			ToolSchema: ai.ToolSchema{
				Name: "read_skill", Description: "Read the full body of an approved skill",
				Parameters: map[string]any{"type": "object", "properties": map[string]any{
					"name": map[string]any{"type": "string"},
				}, "required": []any{"name"}},
			},
			Replay: durable.ReplaySafe,
			Execute: func(ctx context.Context, args any, api durable.ToolExecutionApi) (durable.ToolExecutionResult, error) {
				name, _ := toStringMap(args)["name"].(string)
				doc, err := durable.Snapshot[Doc](ctx, api, SkillsDoc)
				if err != nil || doc == nil {
					return textResult("No approved skills."), nil
				}
				for _, item := range doc.Items {
					if item.Name == name && item.Status == "approved" {
						return textResult(fmt.Sprintf("# %s\n\n%s\n\n%s", item.Name, item.Description, item.Body)), nil
					}
				}
				return textResult("No approved skill with that name."), nil
			},
		}),
	},
	Sections: []*durable.PromptSection{
		harness.Section("memory", func(ctx context.Context, input durable.PromptInput) (*string, error) {
			doc, err := durable.Snapshot[Doc](ctx, input.Read, MemoryDoc)
			if err != nil || doc == nil {
				return nil, nil
			}
			lines := []string{}
			used := 0
			approved := []Item{}
			for _, item := range doc.Items {
				if item.Status == "approved" {
					approved = append(approved, item)
				}
			}
			for i := len(approved) - 1; i >= 0; i-- {
				if used+len(approved[i].Text) > memoryPromptChars {
					continue
				}
				lines = append([]string{"- " + approved[i].Text}, lines...)
				used += len(approved[i].Text)
			}
			if len(lines) == 0 {
				return nil, nil
			}
			result := "Long-term memory (approved):\n" + strings.Join(lines, "\n")
			return &result, nil
		}),
	},
})

// ProposeInTxViaAPI proposes a row through a tool's transaction API.
func ProposeInTxViaAPI(ctx context.Context, api durable.ToolExecutionApi, token durable.DocToken[Doc], input map[string]any) (Item, error) {
	var item Item
	_, err := api.Commit(ctx, func(tx durable.Tx) (any, error) {
		proposed, err := ProposeInTx(tx, token, input)
		item = proposed
		return proposed, err
	})
	return item, err
}

func toStringMap(args any) map[string]any {
	if m, ok := args.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func textResult(value string) durable.ToolExecutionResult {
	return durable.ToolExecutionResult{Output: []ai.ToolResultMessageContent{ai.TextContent{Text: value}}}
}
