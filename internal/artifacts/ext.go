package artifacts

import (
	"context"
	"fmt"
	"strings"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/durable"
	"github.com/MichaelKinsy/PiG/durable/harness"
)

func artifactTextResult(value string) durable.ToolExecutionResult {
	return durable.ToolExecutionResult{Output: []ai.ToolResultMessageContent{ai.TextContent{Text: value}}}
}

// Extension exposes the artifact file subsystem to the agent as tools.
func Extension(store *Store, confirmations *ConfirmationStore) *durable.Extension {
	return harness.DefineExtension(durable.Extension{
		Name: "artifacts",
		Tools: []*durable.ToolRegistration{
			harness.DefineTool(durable.ToolRegistration{
				ToolSchema: ai.ToolSchema{
					Name:        "artifact_create",
					Description: "Create a new artifact folder the agent can store files in",
					Parameters: map[string]any{
						"type":       "object",
						"properties": map[string]any{},
						"required":   []any{},
					},
				},
				Replay: durable.ReplaySafe,
				Execute: func(ctx context.Context, args any, api durable.ToolExecutionApi) (durable.ToolExecutionResult, error) {
					created, err := store.CreateArtifact(ctx)
					if err != nil {
						return artifactTextResult(fmt.Sprintf("failed to create artifact: %v", err)), nil
					}
					return artifactTextResult(fmt.Sprintf("created artifact %s", created.ID)), nil
				},
			}),
			harness.DefineTool(durable.ToolRegistration{
				ToolSchema: ai.ToolSchema{
					Name:        "artifact_write",
					Description: "Write or update a file inside an artifact",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"artifactId": map[string]any{"type": "string"},
							"path":       map[string]any{"type": "string"},
							"content":    map[string]any{"type": "string"},
						},
						"required": []any{"artifactId", "path", "content"},
					},
				},
				Replay: durable.ReplaySafe,
				Execute: func(ctx context.Context, args any, api durable.ToolExecutionApi) (durable.ToolExecutionResult, error) {
					input, _ := args.(map[string]any)
					artifactID, _ := input["artifactId"].(string)
					path, _ := input["path"].(string)
					content, _ := input["content"].(string)
					if artifactID == "" || path == "" || content == "" {
						return artifactTextResult("artifact_write requires artifactId, path and content"), nil
					}
					artifact, err := store.WriteFile(ctx, artifactID, path, []byte(content))
					if err != nil {
						return artifactTextResult(fmt.Sprintf("failed to write artifact file: %v", err)), nil
					}
					return artifactTextResult(fmt.Sprintf("wrote %s in artifact %s", path, artifact.ID)), nil
				},
			}),
			harness.DefineTool(durable.ToolRegistration{
				ToolSchema: ai.ToolSchema{
					Name:        "artifact_move",
					Description: "Move or rename a file inside an artifact",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"artifactId": map[string]any{"type": "string"},
							"from":       map[string]any{"type": "string"},
							"to":         map[string]any{"type": "string"},
						},
						"required": []any{"artifactId", "from", "to"},
					},
				},
				Replay: durable.ReplaySafe,
				Execute: func(ctx context.Context, args any, api durable.ToolExecutionApi) (durable.ToolExecutionResult, error) {
					input, _ := args.(map[string]any)
					artifactID, _ := input["artifactId"].(string)
					from, _ := input["from"].(string)
					to, _ := input["to"].(string)
					if artifactID == "" || from == "" || to == "" {
						return artifactTextResult("artifact_move requires artifactId, from and to"), nil
					}
					if _, err := store.MoveFile(ctx, artifactID, from, to); err != nil {
						return artifactTextResult(fmt.Sprintf("failed to move artifact file: %v", err)), nil
					}
					return artifactTextResult(fmt.Sprintf("moved %s to %s in artifact %s", from, to, artifactID)), nil
				},
			}),
			harness.DefineTool(durable.ToolRegistration{
				ToolSchema: ai.ToolSchema{
					Name:        "artifact_read",
					Description: "Read a file from an artifact",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"artifactId": map[string]any{"type": "string"},
							"path":       map[string]any{"type": "string"},
						},
						"required": []any{"artifactId", "path"},
					},
				},
				Replay: durable.ReplaySafe,
				Execute: func(ctx context.Context, args any, api durable.ToolExecutionApi) (durable.ToolExecutionResult, error) {
					input, _ := args.(map[string]any)
					artifactID, _ := input["artifactId"].(string)
					path, _ := input["path"].(string)
					if artifactID == "" || path == "" {
						return artifactTextResult("artifact_read requires artifactId and path"), nil
					}
					content, ok, err := store.ReadFile(ctx, artifactID, path)
					if err != nil {
						return artifactTextResult(fmt.Sprintf("failed to read artifact file: %v", err)), nil
					}
					if !ok {
						return artifactTextResult("artifact file not found"), nil
					}
					return artifactTextResult(string(content)), nil
				},
			}),
			harness.DefineTool(durable.ToolRegistration{
				ToolSchema: ai.ToolSchema{
					Name:        "artifact_list",
					Description: "List artifacts and their files",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"artifactId": map[string]any{"type": "string"},
						},
					},
				},
				Replay: durable.ReplaySafe,
				Execute: func(ctx context.Context, args any, api durable.ToolExecutionApi) (durable.ToolExecutionResult, error) {
					input, _ := args.(map[string]any)
					artifactID, _ := input["artifactId"].(string)
					if artifactID != "" {
						files, err := store.ListFiles(ctx, artifactID)
						if err != nil {
							return artifactTextResult(fmt.Sprintf("failed to list artifact files: %v", err)), nil
						}
						lines := []string{}
						for _, file := range files {
							lines = append(lines, fmt.Sprintf("%s (%d bytes)", file.Path, file.SizeBytes))
						}
						if len(lines) == 0 {
							return artifactTextResult("artifact has no files"), nil
						}
						return artifactTextResult(strings.Join(lines, "\n")), nil
					}
					artifacts, err := store.ListArtifacts(ctx)
					if err != nil {
						return artifactTextResult(fmt.Sprintf("failed to list artifacts: %v", err)), nil
					}
					lines := []string{}
					for _, artifact := range artifacts {
						lines = append(lines, fmt.Sprintf("%s pinned=%t size=%d", artifact.ID, artifact.Pinned, artifact.SizeBytes))
					}
					if len(lines) == 0 {
						return artifactTextResult("no artifacts"), nil
					}
					return artifactTextResult(strings.Join(lines, "\n")), nil
				},
			}),
			harness.DefineTool(durable.ToolRegistration{
				ToolSchema: ai.ToolSchema{
					Name:        "artifact_request_pin",
					Description: "Request human confirmation to pin an artifact so cleanup never removes it",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"artifactId": map[string]any{"type": "string"},
							"context":    map[string]any{"type": "string"},
						},
						"required": []any{"artifactId"},
					},
				},
				Replay: durable.ReplaySafe,
				Execute: func(ctx context.Context, args any, api durable.ToolExecutionApi) (durable.ToolExecutionResult, error) {
					return proposeArtifactConfirmation(ctx, args, confirmations, "pin"), nil
				},
			}),
			harness.DefineTool(durable.ToolRegistration{
				ToolSchema: ai.ToolSchema{
					Name:        "artifact_request_unpin",
					Description: "Request human confirmation to unpin an artifact so normal cleanup applies again",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"artifactId": map[string]any{"type": "string"},
							"context":    map[string]any{"type": "string"},
						},
						"required": []any{"artifactId"},
					},
				},
				Replay: durable.ReplaySafe,
				Execute: func(ctx context.Context, args any, api durable.ToolExecutionApi) (durable.ToolExecutionResult, error) {
					return proposeArtifactConfirmation(ctx, args, confirmations, "unpin"), nil
				},
			}),
		},
		Sections: []*durable.PromptSection{
			harness.Section("artifacts", func(ctx context.Context, input durable.PromptInput) (*string, error) {
				instructions := `Artifact files are created with artifact_create and artifact_write. Use artifact_move to rename or move a file. To show a file in the conversation, reference it with an artifact:// Markdown link: [report.pdf](artifact://<id>/<path>) renders a preview card, ![chart.png](artifact://<id>/<path>) embeds the image, and a bare artifact:// link opens the full viewer. Pin requests need human confirmation.`
				return &instructions, nil
			}),
		},
	})
}

func proposeArtifactConfirmation(ctx context.Context, args any, confirmations *ConfirmationStore, action string) durable.ToolExecutionResult {
	input, _ := args.(map[string]any)
	artifactID, _ := input["artifactId"].(string)
	contextText, _ := input["context"].(string)
	if artifactID == "" {
		return artifactTextResult("confirmation requires an artifactId")
	}
	item, err := confirmations.Propose(ctx, artifactID, action, contextText)
	if err != nil {
		return artifactTextResult(fmt.Sprintf("failed to request %s confirmation: %v", action, err))
	}
	return artifactTextResult(fmt.Sprintf("requested %s for artifact %s; confirmation %s is pending human approval", action, artifactID, item.ID))
}
