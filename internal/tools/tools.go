// Package tools defines the agent tools provided by the assistant extension.
package tools

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/chord/delta"
	"github.com/MichaelKinsy/PiG/durable"
	"github.com/MichaelKinsy/PiG/durable/harness"
	"github.com/liyu1981/pi-durable-assistant/internal/docs"
	"github.com/liyu1981/pi-durable-assistant/internal/netutil"
	"github.com/liyu1981/pi-durable-assistant/internal/tasks"
)

func textResult(value string) durable.ToolExecutionResult {
	return durable.ToolExecutionResult{Output: []ai.ToolResultMessageContent{ai.TextContent{Text: value}}}
}

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

func argsMap(args any) map[string]any {
	if m, ok := args.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func applyOnce(ctx context.Context, api durable.ToolExecutionApi, change func(*docs.NotesDoc) string) (string, error) {
	result := ""
	_, err := api.Commit(ctx, func(tx durable.Tx) (any, error) {
		draft, err := durable.TxDoc(tx, docs.Notes, api.ConversationId())
		if err != nil {
			return nil, err
		}
		notes, err := decode[docs.NotesDoc](draft)
		if err != nil {
			return nil, err
		}
		if notes.Applied == nil {
			notes.Applied = map[string]any{}
		}
		if prior, ok := notes.Applied[api.CallId()]; ok {
			result, _ = prior.(string)
			return result, nil
		}
		result = change(&notes)
		notes.Applied[api.CallId()] = result
		return result, write(draft, notes)
	})
	return result, err
}

var Todo = harness.DefineTool(durable.ToolRegistration{
	ToolSchema: ai.ToolSchema{
		Name: "todo", Description: "维护待办：add 新增一条，done 把匹配的一条标记完成",
		Parameters: map[string]any{"type": "object", "properties": map[string]any{
			"action": map[string]any{"type": "string", "enum": []any{"add", "done"}}, "text": map[string]any{"type": "string"},
		}, "required": []any{"action", "text"}},
	},
	Replay: durable.ReplaySafe,
	Execute: func(ctx context.Context, args any, api durable.ToolExecutionApi) (durable.ToolExecutionResult, error) {
		input := argsMap(args)
		action, _ := input["action"].(string)
		text, _ := input["text"].(string)
		result, err := applyOnce(ctx, api, func(notes *docs.NotesDoc) string {
			if action == "add" {
				notes.Todos = append(notes.Todos, docs.Todo{Text: text})
				return "已添加：" + text
			}
			for i := range notes.Todos {
				if !notes.Todos[i].Done && notes.Todos[i].Text == text {
					notes.Todos[i].Done = true
					return "已完成：" + notes.Todos[i].Text
				}
			}
			for i := range notes.Todos {
				if !notes.Todos[i].Done && strings.Contains(notes.Todos[i].Text, text) {
					notes.Todos[i].Done = true
					return "已完成：" + notes.Todos[i].Text
				}
			}
			return "没找到待办：" + text
		})
		if err != nil {
			return durable.ToolExecutionResult{}, err
		}
		return textResult(result), nil
	},
})

var Memo = harness.DefineTool(durable.ToolRegistration{
	ToolSchema: ai.ToolSchema{
		Name: "memo", Description: "记在当前会话的备忘；跨会话事实请用 propose_memory",
		Parameters: map[string]any{"type": "object", "properties": map[string]any{
			"text": map[string]any{"type": "string"},
		}, "required": []any{"text"}},
	},
	Replay: durable.ReplaySafe,
	Execute: func(ctx context.Context, args any, api durable.ToolExecutionApi) (durable.ToolExecutionResult, error) {
		text, _ := argsMap(args)["text"].(string)
		result, err := applyOnce(ctx, api, func(notes *docs.NotesDoc) string {
			notes.Memos = append(notes.Memos, text)
			return "已记下：" + text
		})
		if err != nil {
			return durable.ToolExecutionResult{}, err
		}
		return textResult(result), nil
	},
})

var CurrentTime = harness.DefineTool(durable.ToolRegistration{
	ToolSchema: ai.ToolSchema{Name: "current_time", Description: "获取当前北京时间", Parameters: map[string]any{"type": "object"}},
	Replay:     durable.ReplaySafe,
	Execute: func(ctx context.Context, args any, api durable.ToolExecutionApi) (durable.ToolExecutionResult, error) {
		location, err := time.LoadLocation("Asia/Shanghai")
		if err != nil {
			return textResult(time.Now().String()), nil
		}
		return textResult(time.Now().In(location).Format("2006-01-02 15:04:05")), nil
	},
})

var FetchPage = harness.DefineTool(durable.ToolRegistration{
	ToolSchema: ai.ToolSchema{
		Name: "fetch_page", Description: "抓取一个公开网页的正文文字（最多 6000 字）",
		Parameters: map[string]any{"type": "object", "properties": map[string]any{
			"url": map[string]any{"type": "string"},
		}, "required": []any{"url"}},
	},
	Replay: durable.ReplaySafe,
	Execute: func(ctx context.Context, args any, api durable.ToolExecutionApi) (durable.ToolExecutionResult, error) {
		rawURL, _ := argsMap(args)["url"].(string)
		status, body, err := netutil.FetchPublicPage(ctx, rawURL, ctx.Done())
		if err != nil {
			return textResult("抓取失败：" + err.Error()), nil
		}
		text := htmlToText(body)
		if len([]rune(text)) > 6000 {
			text = string([]rune(text)[:6000])
		}
		return textResult(fmt.Sprintf("HTTP %d\n%s", status, text)), nil
	},
})

var WaitSeconds = harness.DefineTool(durable.ToolRegistration{
	ToolSchema: ai.ToolSchema{
		Name: "wait_seconds", Description: "等待一段时间再继续（1 到 120 秒）",
		Parameters: map[string]any{"type": "object", "properties": map[string]any{
			"seconds": map[string]any{"type": "number"}, "reason": map[string]any{"type": "string"},
		}, "required": []any{"seconds"}},
	},
	Replay: durable.ReplaySafe,
	Execute: func(ctx context.Context, args any, api durable.ToolExecutionApi) (durable.ToolExecutionResult, error) {
		secondsValue, ok := argsMap(args)["seconds"].(float64)
		if !ok {
			secondsValue = 1
		}
		seconds := int(secondsValue)
		if seconds < 1 {
			seconds = 1
		}
		if seconds > 120 {
			seconds = 120
		}
		deadlineValue, err := api.MemoCandidate(ctx, "deadline", float64(time.Now().UnixMilli()+int64(seconds)*1000))
		if err != nil {
			return durable.ToolExecutionResult{}, err
		}
		deadline, ok := deadlineValue.(float64)
		if !ok {
			return textResult("已等待完成。"), nil
		}
		for {
			remaining := int64(deadline) - time.Now().UnixMilli()
			if remaining <= 0 {
				break
			}
			wait := 1000 * time.Millisecond
			if remaining < 1000 {
				wait = time.Duration(remaining) * time.Millisecond
			}
			select {
			case <-ctx.Done():
				return textResult("等待已取消。"), nil
			case <-time.After(wait):
			}
		}
		return textResult(fmt.Sprintf("已等待 %d 秒", seconds)), nil
	},
})

var RemindMe = harness.DefineTool(durable.ToolRegistration{
	ToolSchema: ai.ToolSchema{
		Name: "remind_me", Description: "设一个定时提醒：到点后你会收到一条以 [定时提醒] 开头的消息",
		Parameters: map[string]any{"type": "object", "properties": map[string]any{
			"minutes": map[string]any{"type": "number"}, "text": map[string]any{"type": "string"},
		}, "required": []any{"minutes", "text"}},
	},
	Replay: durable.ReplaySafe,
	Execute: func(ctx context.Context, args any, api durable.ToolExecutionApi) (durable.ToolExecutionResult, error) {
		input := argsMap(args)
		minutes, _ := input["minutes"].(float64)
		text, _ := input["text"].(string)
		if minutes < 0.25 {
			minutes = 0.25
		}
		if minutes > 1440 {
			minutes = 1440
		}
		requestID := api.CallId()
		result := ""
		_, err := api.Commit(ctx, func(tx durable.Tx) (any, error) {
			draft, err := durable.TxDoc(tx, docs.Reminders, api.ConversationId())
			if err != nil {
				return nil, err
			}
			doc, err := decode[docs.ReminderDoc](draft)
			if err != nil {
				return nil, err
			}
			if doc.Applied == nil {
				doc.Applied = map[string]any{}
			}
			if prior, ok := doc.Applied[requestID]; ok {
				result, _ = prior.(string)
				return result, nil
			}
			at := time.Now().UnixMilli() + int64(minutes*60000)
			id, err := tasks.CreateReminderTask(tx, api.ConversationId(), tasks.ReminderInput{At: at, Text: text})
			if err != nil {
				return nil, err
			}
			doc.Items = append(doc.Items, docs.ReminderItem{ID: int64(id), Text: text, At: at, Status: "scheduled"})
			location, _ := time.LoadLocation("Asia/Shanghai")
			when := time.UnixMilli(at).In(location).Format("15:04:05")
			result = fmt.Sprintf("已设好提醒：%s（北京时间）提醒“%s”", when, text)
			doc.Applied[requestID] = result
			return result, write(draft, doc)
		})
		if err != nil {
			return durable.ToolExecutionResult{}, err
		}
		return textResult(result), nil
	},
})

// Delegate is the foreground subagent extension. It is assigned in init to avoid a self-reference cycle.
var Delegate *durable.Extension

func init() {
	Delegate = harness.DefineExtension(durable.Extension{
		Name: "delegate",
		Tools: []*durable.ToolRegistration{
			harness.DefineTool(durable.ToolRegistration{
				ToolSchema: ai.ToolSchema{
					Name: "delegate", Description: "派一个子 Agent 独立完成一件事，等它做完拿回结果",
					Parameters: map[string]any{"type": "object", "properties": map[string]any{
						"task": map[string]any{"type": "string"},
					}, "required": []any{"task"}},
				},
				Replay: durable.ReplaySafe,
				Execute: func(ctx context.Context, args any, api durable.ToolExecutionApi) (durable.ToolExecutionResult, error) {
					taskText, _ := argsMap(args)["task"].(string)
					var childID durable.ConversationId
					_, err := api.Commit(ctx, func(tx durable.Tx) (any, error) {
						existing, err := tx.ScanConversations(durable.ConversationQuery{OwnerTaskId: &[]durable.TaskId{api.TaskId()}[0]}, 1, nil)
						if err == nil && len(existing.Items) > 0 {
							childID = existing.Items[0].Id
							return childID, nil
						}
						child, err := tx.CreateConversation(durable.CreateConversationOptions{
							Ownership: durable.ConversationOwnership{Kind: durable.ConversationOwnedByTask, TaskId: api.TaskId()},
						})
						if err != nil {
							return nil, err
						}
						childID = child.Id
						if err := harness.Configure(tx, childID, harness.AgentChange{
							Extensions:   harness.SetTo(harness.ExtensionChange{Edit: &harness.ExtensionEdit{Remove: []*durable.Extension{Delegate}}}),
							Instructions: harness.SetTo("你是主 Agent 派出的子 Agent，只负责完成交给你的这一件事，完成后用简短中文汇报结果。"),
						}); err != nil {
							return nil, err
						}
						draft, err := durable.TxDoc(tx, docs.Delegations, api.ConversationId())
						if err != nil {
							return nil, err
						}
						delegations, err := decode[docs.DelegationDoc](draft)
						if err != nil {
							return nil, err
						}
						delegations.Items = append(delegations.Items, docs.DelegationItem{
							ConversationID: int64(childID), Task: taskText, Status: "running",
						})
						return childID, write(draft, delegations)
					})
					if err != nil {
						return durable.ToolExecutionResult{}, err
					}
					if err := api.Details(ctx, durable.JsonValue(map[string]any{"conversationId": int64(childID)})); err != nil {
						return durable.ToolExecutionResult{}, err
					}
					child, err := api.Conversation(ctx, childID)
					if err != nil || child == nil {
						return durable.ToolExecutionResult{}, err
					}
					requestID := fmt.Sprintf("delegate:%d", api.TaskId())
					submission, err := child.Submit(ctx, durable.SubmissionDraft{
						Type: durable.SubmissionTypeInput, Content: ai.UserText(taskText),
						RequestId: &requestID, WhenBusy: durable.WhenBusyFollowUp,
					})
					if err != nil {
						return durable.ToolExecutionResult{}, err
					}
					settled, err := submission.Wait(ctx)
					if err != nil {
						return durable.ToolExecutionResult{}, err
					}
					answer := ""
					if settled.Answer != nil {
						_, err = api.Commit(ctx, func(tx durable.Tx) (any, error) {
							entry, err := tx.Entry(*settled.Answer)
							if err != nil || entry == nil || len(entry.Model) == 0 {
								return nil, nil
							}
							message, ok := entry.Model[0].(ai.AssistantMessage)
							if !ok {
								return nil, nil
							}
							for _, part := range message.Content {
								if content, ok := part.(ai.TextContent); ok {
									answer += content.Text
								}
							}
							draft, err := durable.TxDoc(tx, docs.Delegations, api.ConversationId())
							if err != nil {
								return nil, err
							}
							list, err := decode[docs.DelegationDoc](draft)
							if err != nil {
								return nil, err
							}
							for i := range list.Items {
								if list.Items[i].ConversationID == int64(childID) {
									list.Items[i].Status = "done"
								}
							}
							return nil, write(draft, list)
						})
						if err != nil {
							return durable.ToolExecutionResult{}, err
						}
					}
					if answer == "" {
						answer = "子 Agent 完成了，但没有文字结果"
					}
					return textResult(answer), nil
				},
			}),
		},
	})
}

// Assistant is the main agent extension.
var Assistant = harness.DefineExtension(durable.Extension{
	Name: "assistant",
	Tools: []*durable.ToolRegistration{
		FetchPage, WaitSeconds, Todo, Memo, CurrentTime, RemindMe,
	},
	Tasks: []durable.AnyTask{tasks.ReminderTask.Erased(), tasks.ScheduleTask.Erased(), tasks.BackgroundTask.Erased()},
	Sections: []*durable.PromptSection{
		harness.Section("preamble", func(ctx context.Context, input durable.PromptInput) (*string, error) {
			text := strings.Join([]string{
				"你是“小墨的 Durable 助手”，用简体中文简洁回答。",
				"遇到多步骤任务，先用 todo 列出步骤，做完一步标记一步。",
				"当场等一会儿再继续用 wait_seconds；几分钟以后才要做的事用 remind_me 设提醒。",
				"收到以 [定时提醒] 开头的消息时，那是你之前设的提醒到点了，用一两句话提醒用户这件事。",
				"能独立完成、步骤较多的调研可以用 delegate 派给子 Agent；需要后台独立运行用后台任务。",
				"有周期性或延迟执行的需求用 schedule_task（任务中心里也可以创建）。",
				"当前会话的备忘用 memo。跨会话要记住的稳定事实用 propose_memory，等用户批准。",
				"已批准的技能先用 read_skill 读全文再执行；新流程用 propose_skill 提交。",
				"用户自己的笔记用 search_knowledge 和 read_knowledge，回答要带路径和行号。",
				"查公开网页用 fetch_page。",
			}, "\n")
			return &text, nil
		}, harness.SectionOptions{Tag: boolPtr(false)}),
		harness.Section("notes", func(ctx context.Context, input durable.PromptInput) (*string, error) {
			notes, err := durable.Snapshot[docs.NotesDoc](ctx, input.Read, docs.Notes, input.ConversationId)
			if err != nil || notes == nil || (len(notes.Todos) == 0 && len(notes.Memos) == 0) {
				return nil, nil
			}
			lines := []string{"当前记事本："}
			for _, todo := range notes.Todos {
				mark := "[ ]"
				if todo.Done {
					mark = "[x]"
				}
				lines = append(lines, mark+" "+todo.Text)
			}
			for _, memo := range notes.Memos {
				lines = append(lines, "- "+memo)
			}
			text := strings.Join(lines, "\n")
			return &text, nil
		}),
	},
})

func boolPtr(value bool) *bool { return &value }

func htmlToText(html string) string {
	replacer := strings.NewReplacer("&nbsp;", " ", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&amp;", "&")
	text := regexp.MustCompile(`<(script|style|noscript)[\s\S]*?</\1>`).ReplaceAllString(html, " ")
	text = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(text, " ")
	text = replacer.Replace(text)
	text = regexp.MustCompile(`(?i)&#x([0-9a-f]+);`).ReplaceAllStringFunc(text, func(m string) string {
		parts := strings.SplitN(strings.TrimSuffix(strings.TrimPrefix(m, "&#x"), ";"), "x", 2)
		value, _ := strconv.ParseInt(parts[len(parts)-1], 16, 32)
		return string(rune(value))
	})
	text = regexp.MustCompile(`&#(\d+);`).ReplaceAllStringFunc(text, func(m string) string {
		value, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(m, "&#"), ";"))
		return string(rune(value))
	})
	return strings.Join(strings.Fields(text), " ")
}

var _ = errors.New
