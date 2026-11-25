// Package tasks implements durable reminders, schedules, background jobs and notifications.
package tasks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/chord/delta"
	"github.com/MichaelKinsy/PiG/durable"
	"github.com/MichaelKinsy/PiG/durable/harness"
	"github.com/liyu1981/pi-durable-assistant/internal/docs"
	"github.com/liyu1981/pi-durable-assistant/internal/schedule"
)

// Phase is the durable checkpoint of every task in this package.
type Phase struct {
	Phase string `json:"phase"`
}

type noHooks struct{}
type noResult struct{}

func decode[T any](draft *delta.Object) (T, error) {
	doc, err := durable.DecodeDoc[T](draft.Snapshot())
	if err != nil || doc == nil {
		var zero T
		return zero, err
	}
	return *doc, nil
}

func write(draft *delta.Object, value any) error {
	for _, key := range draft.Keys() {
		draft.Delete(key)
	}
	jsonValue, err := durable.ToJsonValue(value)
	if err != nil {
		return err
	}
	object, ok := jsonValue.(*delta.JsonObject)
	if !ok {
		return fmt.Errorf("文档值必须是 JSON 对象")
	}
	for _, key := range object.Keys() {
		entry, _ := object.Get(key)
		if err := draft.Set(key, entry); err != nil {
			return err
		}
	}
	return nil
}

func terminal() *durable.TaskState[Phase, noResult] {
	return &durable.TaskState[Phase, noResult]{
		Status:  durable.TaskTerminal,
		Outcome: &durable.TaskOutcome[noResult]{Status: durable.OutcomeCompleted},
	}
}

// ReminderInput is the input of a one-shot reminder task.
type ReminderInput struct {
	At   int64  `json:"at"`
	Text string `json:"text"`
}

// ReminderTask waits until its deadline, then submits a follow-up to its conversation.
var ReminderTask = durable.DefineTask(durable.TaskDefinition[ReminderInput, Phase, noResult, noHooks]{
	Name: "app.reminder", Version: 1,
	Initial: func(ReminderInput) Phase { return Phase{Phase: "sleep"} },
	Phases: map[string]durable.PhaseHandler[ReminderInput, Phase, noResult, noHooks]{
		"sleep": func(ctx context.Context, task durable.RunningTask[ReminderInput, Phase, noResult], runtime durable.TaskRuntime[ReminderInput, Phase, noResult, noHooks]) error {
			return runtime.SleepUntil(ctx, float64(task.Input.At), Phase{Phase: "fire"})
		},
		"fire": func(ctx context.Context, task durable.RunningTask[ReminderInput, Phase, noResult], runtime durable.TaskRuntime[ReminderInput, Phase, noResult, noHooks]) error {
			cancelled := false
			err := runtime.Commit(ctx, func(tx durable.Tx, current durable.RunningTask[ReminderInput, Phase, noResult]) (*durable.NextTaskState[Phase, noResult], error) {
				draft, err := durable.TxDoc(tx, docs.Reminders, current.ConversationId)
				if err != nil {
					return nil, err
				}
				doc, err := decode[docs.ReminderDoc](draft)
				if err != nil {
					return nil, err
				}
				for _, item := range doc.Items {
					if item.ID == int64(task.Id) && item.Status == "cancelled" {
						cancelled = true
					}
				}
				if cancelled {
					return terminal(), nil
				}
				return nil, nil
			})
			if err != nil || cancelled {
				return err
			}
			conversation, err := runtime.Conversation(ctx, runtime.ConversationId())
			if err != nil || conversation == nil {
				return err
			}
			requestID := fmt.Sprintf("reminder:%d", task.Id)
			_, err = conversation.Submit(ctx, durable.SubmissionDraft{
				Type: durable.SubmissionTypeInput, Content: ai.UserText("[定时提醒] " + task.Input.Text),
				RequestId: &requestID, WhenBusy: durable.WhenBusyFollowUp,
			})
			if err != nil {
				return err
			}
			err = runtime.Commit(ctx, func(tx durable.Tx, current durable.RunningTask[ReminderInput, Phase, noResult]) (*durable.NextTaskState[Phase, noResult], error) {
				draft, err := durable.TxDoc(tx, docs.Reminders, current.ConversationId)
				if err != nil {
					return nil, err
				}
				doc, err := decode[docs.ReminderDoc](draft)
				if err != nil {
					return nil, err
				}
				for i := range doc.Items {
					if doc.Items[i].ID == int64(task.Id) {
						doc.Items[i].Status = "fired"
					}
				}
				if err := write(draft, doc); err != nil {
					return nil, err
				}
				return terminal(), nil
			})
			if err == nil {
				Notify(NotifyEvent{ID: requestID, Type: "reminder", Text: "定时提醒：" + task.Input.Text})
			}
			return err
		},
	},
	Abort: func(ctx context.Context, task durable.RunningTask[ReminderInput, Phase, noResult], runtime durable.TaskRuntime[ReminderInput, Phase, noResult, noHooks]) error {
		return runtime.Commit(ctx, func(tx durable.Tx, current durable.RunningTask[ReminderInput, Phase, noResult]) (*durable.NextTaskState[Phase, noResult], error) {
			return terminal(), nil
		})
	},
})

// ScheduleInput is the input of a periodic schedule task.
type ScheduleInput struct {
	ScheduleID string `json:"scheduleId"`
	Text       string `json:"text"`
	At         int64  `json:"at"`
	Revision   int    `json:"revision"`
}

// ScheduleTask sleeps, revalidates the schedule row, delivers a follow-up, then advances the period.
var ScheduleTask durable.Task[ScheduleInput, Phase, noResult, noHooks]

func init() {
	ScheduleTask = durable.DefineTask(durable.TaskDefinition[ScheduleInput, Phase, noResult, noHooks]{
		Name: "app.v3.schedule", Version: 1,
		Initial: func(ScheduleInput) Phase { return Phase{Phase: "sleep"} },
		Phases: map[string]durable.PhaseHandler[ScheduleInput, Phase, noResult, noHooks]{
			"sleep": func(ctx context.Context, task durable.RunningTask[ScheduleInput, Phase, noResult], runtime durable.TaskRuntime[ScheduleInput, Phase, noResult, noHooks]) error {
				return runtime.SleepUntil(ctx, float64(task.Input.At), Phase{Phase: "fire"})
			},
			"fire": func(ctx context.Context, task durable.RunningTask[ScheduleInput, Phase, noResult], runtime durable.TaskRuntime[ScheduleInput, Phase, noResult, noHooks]) error {
				valid := false
				err := runtime.Commit(ctx, func(tx durable.Tx, current durable.RunningTask[ScheduleInput, Phase, noResult]) (*durable.NextTaskState[Phase, noResult], error) {
					draft, err := durable.TxDoc(tx, docs.Schedules, current.ConversationId)
					if err != nil {
						return nil, err
					}
					doc, err := decode[docs.ScheduleDoc](draft)
					if err != nil {
						return nil, err
					}
					for _, item := range doc.Items {
						if item.ID == task.Input.ScheduleID && ScheduleMatches(item.Status, item.NextAt, item.Revision, task.Input) {
							valid = true
							return nil, nil
						}
					}
					return terminal(), nil
				})
				if err != nil {
					return err
				}
				if !valid {
					return nil
				}
				return runtime.Commit(ctx, func(tx durable.Tx, current durable.RunningTask[ScheduleInput, Phase, noResult]) (*durable.NextTaskState[Phase, noResult], error) {
					return &durable.TaskState[Phase, noResult]{Status: durable.TaskRunning, Checkpoint: &Phase{Phase: "deliver"}}, nil
				})
			},
			"deliver": func(ctx context.Context, task durable.RunningTask[ScheduleInput, Phase, noResult], runtime durable.TaskRuntime[ScheduleInput, Phase, noResult, noHooks]) error {
				valid := false
				var everyMinutes float64
				err := runtime.Commit(ctx, func(tx durable.Tx, current durable.RunningTask[ScheduleInput, Phase, noResult]) (*durable.NextTaskState[Phase, noResult], error) {
					draft, err := durable.TxDoc(tx, docs.Schedules, current.ConversationId)
					if err != nil {
						return nil, err
					}
					doc, err := decode[docs.ScheduleDoc](draft)
					if err != nil {
						return nil, err
					}
					for i := range doc.Items {
						item := &doc.Items[i]
						if item.ID != task.Input.ScheduleID || !ScheduleMatches(item.Status, item.NextAt, item.Revision, task.Input) {
							continue
						}
						valid = true
						everyMinutes = item.EveryMinutes
						item.Runs++
						item.LastRunAt = &task.Input.At
						if next := schedule.NextOccurrence(task.Input.At, item.EveryMinutes, schedule.NowMillis()); next != nil {
							item.NextAt = *next
							item.Revision++
							id, err := durable.CreateTask(tx, ScheduleTask, ScheduleInput{
								ScheduleID: item.ID, Text: item.Text, At: item.NextAt, Revision: item.Revision,
							}, durable.TaskOptions{
								Ownership:      durable.TaskOwnership{Kind: durable.TaskOwnedByConversation},
								ConversationId: &current.ConversationId, Background: true,
							})
							if err != nil {
								return nil, err
							}
							taskID := int64(id)
							item.JobID = &taskID
						} else {
							item.Status = "completed"
							item.NextAt = 0
						}
						if err := write(draft, doc); err != nil {
							return nil, err
						}
						return terminal(), nil
					}
					return terminal(), nil
				})
				if err != nil || !valid {
					return err
				}
				conversation, err := runtime.Conversation(ctx, runtime.ConversationId())
				if err != nil || conversation == nil {
					return err
				}
				requestID := fmt.Sprintf("schedule:%s:%d", task.Input.ScheduleID, task.Input.At)
				_, err = conversation.Submit(ctx, durable.SubmissionDraft{
					Type: durable.SubmissionTypeInput, Content: ai.UserText("[计划任务] " + task.Input.Text),
					RequestId: &requestID, WhenBusy: durable.WhenBusyFollowUp,
				})
				Notify(NotifyEvent{ID: requestID, Type: "schedule", Text: "计划任务已触发：" + task.Input.Text})
				_ = everyMinutes
				return err
			},
		},
		Abort: func(ctx context.Context, task durable.RunningTask[ScheduleInput, Phase, noResult], runtime durable.TaskRuntime[ScheduleInput, Phase, noResult, noHooks]) error {
			return runtime.Commit(ctx, func(tx durable.Tx, current durable.RunningTask[ScheduleInput, Phase, noResult]) (*durable.NextTaskState[Phase, noResult], error) {
				return terminal(), nil
			})
		},
	})
}

// BackgroundInput is the input of a background subagent task.
type BackgroundInput struct {
	JobID string           `json:"jobId"`
	Text  string           `json:"text"`
	Model durable.ModelRef `json:"model"`
}

// BackgroundTask runs a child conversation and reports its answer back to the parent.
var BackgroundTask = durable.DefineTask(durable.TaskDefinition[BackgroundInput, Phase, noResult, noHooks]{
	Name: "app.v3.background", Version: 1,
	Initial: func(BackgroundInput) Phase { return Phase{Phase: "run"} },
	Phases: map[string]durable.PhaseHandler[BackgroundInput, Phase, noResult, noHooks]{
		"run": func(ctx context.Context, task durable.RunningTask[BackgroundInput, Phase, noResult], runtime durable.TaskRuntime[BackgroundInput, Phase, noResult, noHooks]) error {
			var childID durable.ConversationId
			err := runtime.Commit(ctx, func(tx durable.Tx, current durable.RunningTask[BackgroundInput, Phase, noResult]) (*durable.NextTaskState[Phase, noResult], error) {
				child, err := tx.CreateConversation(durable.CreateConversationOptions{
					Ownership: durable.ConversationOwnership{Kind: durable.ConversationOwnedByTask, TaskId: runtime.TaskId()},
				})
				if err != nil {
					return nil, err
				}
				childID = child.Id
				if err := harness.Configure(tx, childID, harness.AgentChange{Model: harness.SetTo(task.Input.Model)}); err != nil {
					return nil, err
				}
				draft, err := durable.TxDoc(tx, docs.Backgrounds, current.ConversationId)
				if err != nil {
					return nil, err
				}
				doc, err := decode[docs.BackgroundDoc](draft)
				if err != nil {
					return nil, err
				}
				for i := range doc.Items {
					if doc.Items[i].ID == task.Input.JobID {
						id := int64(childID)
						doc.Items[i].ConversationID = &id
					}
				}
				return nil, write(draft, doc)
			})
			if err != nil {
				return err
			}
			child, err := runtime.Conversation(ctx, childID)
			if err != nil || child == nil {
				return err
			}
			requestID := "background:" + task.Input.JobID
			submission, err := child.Submit(ctx, durable.SubmissionDraft{
				Type: durable.SubmissionTypeInput, Content: ai.UserText(task.Input.Text),
				RequestId: &requestID, WhenBusy: durable.WhenBusyFollowUp,
			})
			if err != nil {
				return err
			}
			if _, err := submission.Wait(ctx); err != nil {
				return err
			}
			view, err := runtime.Context(ctx, childID, nil)
			if err != nil {
				return err
			}
			result := lastAssistantText(view.Entries)
			if result == "" {
				result = "后台子 Agent 没有返回文字结果"
			}
			err = runtime.Commit(ctx, func(tx durable.Tx, current durable.RunningTask[BackgroundInput, Phase, noResult]) (*durable.NextTaskState[Phase, noResult], error) {
				draft, err := durable.TxDoc(tx, docs.Backgrounds, current.ConversationId)
				if err != nil {
					return nil, err
				}
				doc, err := decode[docs.BackgroundDoc](draft)
				if err != nil {
					return nil, err
				}
				for i := range doc.Items {
					if doc.Items[i].ID == task.Input.JobID {
						doc.Items[i].Status = "done"
						doc.Items[i].Result = &result
					}
				}
				if err := write(draft, doc); err != nil {
					return nil, err
				}
				return terminal(), nil
			})
			if err != nil {
				return err
			}
			parent, err := runtime.Conversation(ctx, runtime.ConversationId())
			if err != nil || parent == nil {
				return err
			}
			resultRequestID := "background-result:" + task.Input.JobID
			_, err = parent.Submit(ctx, durable.SubmissionDraft{
				Type: durable.SubmissionTypeInput, Content: ai.UserText("[后台任务结果] " + result),
				RequestId: &resultRequestID, WhenBusy: durable.WhenBusyFollowUp,
			})
			Notify(NotifyEvent{ID: resultRequestID, Type: "background", Text: "后台任务完成：" + task.Input.Text})
			return err
		},
	},
	Abort: func(ctx context.Context, task durable.RunningTask[BackgroundInput, Phase, noResult], runtime durable.TaskRuntime[BackgroundInput, Phase, noResult, noHooks]) error {
		return runtime.Commit(ctx, func(tx durable.Tx, current durable.RunningTask[BackgroundInput, Phase, noResult]) (*durable.NextTaskState[Phase, noResult], error) {
			return terminal(), nil
		})
	},
})

func lastAssistantText(items []durable.EntryRecord) string {
	for _, entry := range items {
		if entry.Kind != "pi.assistant" || len(entry.Model) == 0 {
			continue
		}
		message, ok := entry.Model[0].(ai.AssistantMessage)
		if !ok {
			continue
		}
		var sb strings.Builder
		for _, part := range message.Content {
			if text, ok := part.(ai.TextContent); ok {
				sb.WriteString(text.Text)
			}
		}
		if sb.Len() > 0 {
			return sb.String()
		}
	}
	return ""
}

// NotifyEvent is a notification delivered to the optional webhook.
type NotifyEvent struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Text   string `json:"text"`
	Failed bool   `json:"failed"`
}

// Notify posts an event to the configured generic or Feishu webhook, best-effort.
func Notify(event NotifyEvent) {
	url := os.Getenv("NOTIFY_WEBHOOK_URL")
	mode := os.Getenv("NOTIFY_MODE")
	if mode == "" {
		mode = "errors"
	}
	if url == "" || mode == "off" || (mode == "errors" && !event.Failed) {
		return
	}
	var payload any
	if os.Getenv("NOTIFY_WEBHOOK_KIND") == "feishu" {
		payload = map[string]any{"msg_type": "text", "content": map[string]any{"text": event.Text}}
	} else {
		payload = map[string]any{"id": event.ID, "type": event.Type, "text": event.Text, "failed": event.Failed, "at": time.Now().UTC().Format(time.RFC3339)}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return
	}
	request.Header.Set("content-type", "application/json")
	request.Header.Set("x-pi-event-id", event.ID)
	response, err := (&http.Client{Timeout: 8 * time.Second}).Do(request)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Webhook 推送失败；细节已隐藏，避免泄露 URL 中的密钥")
		return
	}
	response.Body.Close()
}

// CreateReminderTask creates a durable reminder inside a transaction.
func CreateReminderTask(tx durable.Tx, conversationID durable.ConversationId, input ReminderInput) (durable.TaskId, error) {
	return durable.CreateTask(tx, ReminderTask, input, durable.TaskOptions{
		Ownership: durable.TaskOwnership{Kind: durable.TaskOwnedByConversation}, ConversationId: &conversationID, Background: true,
	})
}

// CreateScheduleTask creates a durable schedule task inside a transaction.
func CreateScheduleTask(tx durable.Tx, conversationID durable.ConversationId, input ScheduleInput) (durable.TaskId, error) {
	return durable.CreateTask(tx, ScheduleTask, input, durable.TaskOptions{
		Ownership: durable.TaskOwnership{Kind: durable.TaskOwnedByConversation}, ConversationId: &conversationID, Background: true,
	})
}

// CreateBackgroundTask creates a durable background task inside a transaction.
func CreateBackgroundTask(tx durable.Tx, conversationID durable.ConversationId, input BackgroundInput) (durable.TaskId, error) {
	return durable.CreateTask(tx, BackgroundTask, input, durable.TaskOptions{
		Ownership: durable.TaskOwnership{Kind: durable.TaskOwnedByConversation}, ConversationId: &conversationID, Background: true,
	})
}

// ScheduleMatches reports whether a due task still matches the stored schedule row.
func ScheduleMatches(status string, nextAt int64, revision int, input ScheduleInput) bool {
	return schedule.Matches(status, nextAt, revision, input.At, input.Revision)
}
