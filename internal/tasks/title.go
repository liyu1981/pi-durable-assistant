package tasks

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/durable"
	"github.com/liyu1981/pi-durable-assistant/internal/docs"
)

// TitleInput is the durable input of a conversation-title task.
type TitleInput struct {
	FirstUser    string `json:"firstUser"`
	Answer       string `json:"answer"`
	Hint         string `json:"hint,omitempty"`
	LiteralTitle string `json:"literalTitle,omitempty"`
}

const titlePrompt = `Write one concise conversation title. Return only the title: no quotes, no prefix, no Markdown, no explanation. English only. Maximum 50 characters.`

// TitleTask generates a title with the active model and commits it to app.index.
// It never fails the generation run that asked for it.
var TitleTask = durable.DefineTask(durable.TaskDefinition[TitleInput, Phase, noResult, noHooks]{
	Name: "app.title", Version: 1,
	Initial: func(TitleInput) Phase { return Phase{Phase: "generate"} },
	Phases: map[string]durable.PhaseHandler[TitleInput, Phase, noResult, noHooks]{
		"generate": func(ctx context.Context, task durable.RunningTask[TitleInput, Phase, noResult], runtime durable.TaskRuntime[TitleInput, Phase, noResult, noHooks]) error {
			title := ""
			if strings.TrimSpace(task.Input.LiteralTitle) != "" {
				title = NormalizeTitle(task.Input.LiteralTitle)
			} else {
				generated, err := generateTitle(ctx, runtime, task.Input)
				if err != nil {
					runtime.Report(fmt.Errorf("title generation: %w", err))
				}
				title = NormalizeTitle(generated)
			}
			source := "auto"
			if strings.TrimSpace(task.Input.LiteralTitle) != "" {
				source = "user"
			}
			return runtime.Commit(ctx, func(tx durable.Tx, current durable.RunningTask[TitleInput, Phase, noResult]) (*durable.NextTaskState[Phase, noResult], error) {
				if title == "" {
					return terminal(), nil
				}
				draft, err := durable.TxDoc(tx, docs.Index)
				if err != nil {
					return nil, err
				}
				index, err := decode[docs.IndexDoc](draft)
				if err != nil {
					return nil, err
				}
				for i := range index.Items {
					if index.Items[i].ID == int64(current.ConversationId) {
						index.Items[i].Title = title
						index.Items[i].TitleSource = source
						break
					}
				}
				if err := write(draft, index); err != nil {
					return nil, err
				}
				return terminal(), nil
			})
		},
	},
	Abort: func(ctx context.Context, task durable.RunningTask[TitleInput, Phase, noResult], runtime durable.TaskRuntime[TitleInput, Phase, noResult, noHooks]) error {
		return runtime.Commit(ctx, func(tx durable.Tx, current durable.RunningTask[TitleInput, Phase, noResult]) (*durable.NextTaskState[Phase, noResult], error) {
			return terminal(), nil
		})
	},
})

type titleRuntime interface {
	Models() durable.Models
	Agent(ctx context.Context) (durable.Agent, error)
}

func generateTitle(ctx context.Context, runtime titleRuntime, input TitleInput) (string, error) {
	agent, err := runtime.Agent(ctx)
	if err != nil {
		return "", err
	}
	if agent.Model == nil {
		return "", fmt.Errorf("agent has no model")
	}
	model := runtime.Models().GetModel(agent.Model.Provider, agent.Model.ModelId)
	if model == nil {
		return "", fmt.Errorf("model %s/%s not found", agent.Model.Provider, agent.Model.ModelId)
	}
	user := strings.TrimSpace(input.FirstUser)
	if len([]rune(user)) > 4000 {
		user = string([]rune(user)[:4000])
	}
	answer := strings.TrimSpace(input.Answer)
	if len([]rune(answer)) > 2000 {
		answer = string([]rune(answer)[:2000])
	}
	details := "User: " + user + "\nAssistant: " + answer
	if hint := strings.TrimSpace(input.Hint); hint != "" {
		details += "\nUser title hint: " + hint
	}
	requestCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	message := runtime.Models().CompleteSimple(requestCtx, model, ai.Context{
		SystemPrompt: titlePrompt,
		Messages:     []ai.Message{ai.UserMessage{Content: ai.UserText(details)}},
	}, ai.StreamOptions{MaxTokens: 48, TemperatureSet: true})
	if message == nil || message.StopReason == ai.StopReasonError {
		if message == nil {
			return "", fmt.Errorf("no model response")
		}
		return "", fmt.Errorf("model call failed: %s", message.ErrorMessage)
	}
	return ai.ContentText(message.Content), nil
}

// NormalizeTitle turns model output into a single-line, bounded title.
func NormalizeTitle(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\n", " "))
	value = strings.ReplaceAll(value, "**", "")
	value = strings.ReplaceAll(value, "__", "")
	value = strings.Trim(strings.TrimSpace(value), "\"'`[] ")
	value = strings.Join(strings.Fields(value), " ")
	if utf8.RuneCountInString(value) > 50 {
		value = string([]rune(value)[:50])
	}
	return strings.TrimSpace(value)
}
