package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/MichaelKinsy/PiG/chord/delta"
	"github.com/MichaelKinsy/PiG/durable"
	"github.com/google/uuid"
	"github.com/liyu1981/pi-durable-assistant/internal/docs"
	"github.com/liyu1981/pi-durable-assistant/internal/knowledge"
	"github.com/liyu1981/pi-durable-assistant/internal/personal"
	"github.com/liyu1981/pi-durable-assistant/internal/schedule"
	"github.com/liyu1981/pi-durable-assistant/internal/tasks"
)

func writeDoc(draft *delta.Object, value any) error {
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

func scheduleDTO(item docs.ScheduleItem) map[string]any {
	var jobID any
	if item.JobID != nil {
		jobID = strconv.FormatInt(*item.JobID, 10)
	}
	var lastRun any
	if item.LastRunAt != nil {
		lastRun = *item.LastRunAt
	}
	return map[string]any{
		"id": item.ID, "text": item.Text, "nextAt": item.NextAt, "everyMinutes": item.EveryMinutes,
		"runs": item.Runs, "lastRunAt": lastRun, "status": item.Status, "revision": item.Revision,
		"createdAt": item.CreatedAt, "jobId": jobID,
	}
}

func backgroundDTO(item docs.BackgroundItem) map[string]any {
	var conversationID any
	if item.ConversationID != nil {
		conversationID = *item.ConversationID
	}
	var result any
	if item.Result != nil {
		result = *item.Result
	}
	return map[string]any{
		"id": item.ID, "text": item.Text, "status": item.Status,
		"conversationId": conversationID, "createdAt": item.CreatedAt, "result": result,
	}
}

func (s *Server) management(w http.ResponseWriter, r *http.Request) {
	conversation, err := s.getConversation(r, r.PathValue("id"))
	if err != nil {
		handleError(w, err)
		return
	}
	schedules, err := runtimeSnapshotConversation[docs.ScheduleDoc](r.Context(), s, docs.Schedules, conversation.Id())
	if err != nil {
		handleError(w, err)
		return
	}
	backgrounds, err := runtimeSnapshotConversation[docs.BackgroundDoc](r.Context(), s, docs.Backgrounds, conversation.Id())
	if err != nil {
		handleError(w, err)
		return
	}
	reminders, err := runtimeSnapshotConversation[docs.ReminderDoc](r.Context(), s, docs.Reminders, conversation.Id())
	if err != nil {
		handleError(w, err)
		return
	}
	scheduleItems := []map[string]any{}
	if schedules != nil {
		for _, item := range schedules.Items {
			scheduleItems = append(scheduleItems, scheduleDTO(item))
		}
	}
	backgroundItems := []map[string]any{}
	if backgrounds != nil {
		for _, item := range backgrounds.Items {
			backgroundItems = append(backgroundItems, backgroundDTO(item))
		}
	}
	reminderItems := []docs.ReminderItem{}
	if reminders != nil {
		reminderItems = reminders.Items
	}
	writeJSON(w, http.StatusOK, map[string]any{"schedules": scheduleItems, "background": backgroundItems, "reminders": reminderItems})
}

func (s *Server) createSchedule(w http.ResponseWriter, r *http.Request) {
	conversation, err := s.getConversation(r, r.PathValue("id"))
	if err != nil {
		handleError(w, err)
		return
	}
	body, err := readBody(r, 100_000)
	if err != nil {
		handleError(w, err)
		return
	}
	normalized, err := schedule.Normalize(body, schedule.NowMillis())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	requestID, err := schedule.NormalizeRequestId(body["requestId"])
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var result map[string]any
	_, err = conversation.Commit(r.Context(), func(tx durable.Tx) (any, error) {
		draft, err := durable.TxDoc(tx, docs.Schedules, conversation.Id())
		if err != nil {
			return nil, err
		}
		doc, err := decodeDoc[docs.ScheduleDoc](draft)
		if err != nil {
			return nil, err
		}
		if requestID != nil {
			if prior, ok := doc.Applied[*requestID].(string); ok {
				for _, item := range doc.Items {
					if item.ID == prior {
						result = scheduleDTO(item)
						return result, nil
					}
				}
			}
		}
		item := docs.ScheduleItem{
			ID: uuid.NewString(), Text: normalized.Text, NextAt: normalized.At, EveryMinutes: normalized.EveryMinutes,
			Revision: 1, Status: "active", CreatedAt: schedule.NowMillis(),
		}
		jobID, err := tasks.CreateScheduleTask(tx, conversation.Id(), tasks.ScheduleInput{
			ScheduleID: item.ID, Text: item.Text, At: normalized.At, Revision: item.Revision,
		})
		if err != nil {
			return nil, err
		}
		jobIDValue := int64(jobID)
		item.JobID = &jobIDValue
		doc.Items = append(doc.Items, item)
		if requestID != nil {
			doc.Applied[*requestID] = item.ID
		}
		if err := writeDoc(draft, doc); err != nil {
			return nil, err
		}
		result = scheduleDTO(item)
		return result, nil
	})
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"item": result})
}

func (s *Server) manageSchedule(w http.ResponseWriter, r *http.Request) {
	conversation, err := s.getConversation(r, r.PathValue("id"))
	if err != nil {
		handleError(w, err)
		return
	}
	action := r.PathValue("action")
	if action != "pause" && action != "resume" && action != "cancel" {
		writeError(w, http.StatusNotFound, "接口不存在")
		return
	}
	var item docs.ScheduleItem
	_, err = conversation.Commit(r.Context(), func(tx durable.Tx) (any, error) {
		draft, err := durable.TxDoc(tx, docs.Schedules, conversation.Id())
		if err != nil {
			return nil, err
		}
		doc, err := decodeDoc[docs.ScheduleDoc](draft)
		if err != nil {
			return nil, err
		}
		for i := range doc.Items {
			if doc.Items[i].ID != r.PathValue("uuid") {
				continue
			}
			target := &doc.Items[i]
			switch action {
			case "pause":
				if target.Status != "active" {
					return nil, badRequest("当前状态不能暂停")
				}
				target.Status = "paused"
			case "cancel":
				if target.Status != "active" && target.Status != "paused" {
					return nil, badRequest("当前状态不能取消")
				}
				target.Status = "cancelled"
			case "resume":
				if target.Status != "paused" {
					return nil, badRequest("当前状态不能恢复")
				}
				target.Status = "active"
				if target.NextAt <= schedule.NowMillis() {
					target.NextAt = schedule.NowMillis() + 60_000
				}
				target.Revision++
				jobID, err := tasks.CreateScheduleTask(tx, conversation.Id(), tasks.ScheduleInput{
					ScheduleID: target.ID, Text: target.Text, At: target.NextAt, Revision: target.Revision,
				})
				if err != nil {
					return nil, err
				}
				jobIDValue := int64(jobID)
				target.JobID = &jobIDValue
			}
			target.Revision++
			item = *target
			return nil, writeDoc(draft, doc)
		}
		return nil, badRequest("计划任务不存在")
	})
	if err != nil {
		handleError(w, err)
		return
	}
	if action != "resume" && item.JobID != nil {
		_, _ = s.Runtime.Harness.AbortTask(r.Context(), durable.TaskId(*item.JobID))
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": scheduleDTO(item)})
}

func (s *Server) createBackground(w http.ResponseWriter, r *http.Request) {
	conversation, err := s.getConversation(r, r.PathValue("id"))
	if err != nil {
		handleError(w, err)
		return
	}
	body, err := readBody(r, 100_000)
	if err != nil {
		handleError(w, err)
		return
	}
	text, _ := body["text"].(string)
	if text == "" || len([]rune(text)) > 4000 {
		writeError(w, http.StatusBadRequest, "后台任务说明必须是 1～4000 字")
		return
	}
	requestID, err := schedule.NormalizeRequestId(body["requestId"])
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var item docs.BackgroundItem
	_, err = conversation.Commit(r.Context(), func(tx durable.Tx) (any, error) {
		draft, err := durable.TxDoc(tx, docs.Backgrounds, conversation.Id())
		if err != nil {
			return nil, err
		}
		doc, err := decodeDoc[docs.BackgroundDoc](draft)
		if err != nil {
			return nil, err
		}
		if requestID != nil {
			if prior, ok := doc.Applied[*requestID].(string); ok {
				for _, existing := range doc.Items {
					if existing.ID == prior {
						item = existing
						return item, nil
					}
				}
			}
		}
		item = docs.BackgroundItem{ID: uuid.NewString(), Text: text, Status: "running", CreatedAt: schedule.NowMillis()}
		taskID, err := tasks.CreateBackgroundTask(tx, conversation.Id(), tasks.BackgroundInput{JobID: item.ID, Text: text, Model: s.Runtime.ModelRef})
		if err != nil {
			return nil, err
		}
		taskIDValue := int64(taskID)
		item.TaskID = &taskIDValue
		doc.Items = append(doc.Items, item)
		if requestID != nil {
			doc.Applied[*requestID] = item.ID
		}
		return item, writeDoc(draft, doc)
	})
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"item": backgroundDTO(item)})
}

func (s *Server) stopBackground(w http.ResponseWriter, r *http.Request) {
	conversation, err := s.getConversation(r, r.PathValue("id"))
	if err != nil {
		handleError(w, err)
		return
	}
	var item docs.BackgroundItem
	_, err = conversation.Commit(r.Context(), func(tx durable.Tx) (any, error) {
		draft, err := durable.TxDoc(tx, docs.Backgrounds, conversation.Id())
		if err != nil {
			return nil, err
		}
		doc, err := decodeDoc[docs.BackgroundDoc](draft)
		if err != nil {
			return nil, err
		}
		for i := range doc.Items {
			if doc.Items[i].ID != r.PathValue("uuid") {
				continue
			}
			if doc.Items[i].Status != "running" {
				return nil, badRequest("任务已结束或不存在")
			}
			doc.Items[i].Status = "cancelled"
			item = doc.Items[i]
			return nil, writeDoc(draft, doc)
		}
		return nil, badRequest("任务已结束或不存在")
	})
	if err != nil {
		handleError(w, err)
		return
	}
	if item.TaskID != nil {
		_, _ = s.Runtime.Harness.AbortTask(r.Context(), durable.TaskId(*item.TaskID))
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": backgroundDTO(item)})
}

func (s *Server) cancelReminder(w http.ResponseWriter, r *http.Request) {
	conversation, err := s.getConversation(r, r.PathValue("id"))
	if err != nil {
		handleError(w, err)
		return
	}
	var item docs.ReminderItem
	_, err = conversation.Commit(r.Context(), func(tx durable.Tx) (any, error) {
		draft, err := durable.TxDoc(tx, docs.Reminders, conversation.Id())
		if err != nil {
			return nil, err
		}
		doc, err := decodeDoc[docs.ReminderDoc](draft)
		if err != nil {
			return nil, err
		}
		for i := range doc.Items {
			if strconv.FormatInt(doc.Items[i].ID, 10) != r.PathValue("uuid") {
				continue
			}
			if doc.Items[i].Status != "scheduled" {
				return nil, badRequest("提醒已结束或不存在")
			}
			doc.Items[i].Status = "cancelled"
			item = doc.Items[i]
			return nil, writeDoc(draft, doc)
		}
		return nil, badRequest("提醒已结束或不存在")
	})
	if err != nil {
		handleError(w, err)
		return
	}
	_, _ = s.Runtime.Harness.AbortTask(r.Context(), durable.TaskId(item.ID))
	writeJSON(w, http.StatusOK, map[string]any{"item": item})
}

func (s *Server) getPersonal(w http.ResponseWriter, r *http.Request) {
	memory, err := runtimeSnapshot[personal.Doc](r.Context(), s, personal.MemoryDoc)
	if err != nil {
		handleError(w, err)
		return
	}
	skills, err := runtimeSnapshot[personal.Doc](r.Context(), s, personal.SkillsDoc)
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"memory": personal.PublicDoc(memory), "skills": personal.PublicDoc(skills),
		"knowledge": knowledge.Status(s.Config.ObsidianRoot),
	})
}

func (s *Server) postPersonal(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	if kind != "memory" && kind != "skills" && kind != "skill" {
		writeError(w, http.StatusNotFound, "接口不存在")
		return
	}
	body, err := readBody(r, 100_000)
	if err != nil {
		handleError(w, err)
		return
	}
	token := personal.MemoryDoc
	if kind != "memory" {
		token = personal.SkillsDoc
	}
	var item personal.Item
	_, err = s.Runtime.Root.Commit(r.Context(), func(tx durable.Tx) (any, error) {
		result, err := personal.ProposeInTx(tx, token, body)
		item = result
		return result, err
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"item": item})
}

func (s *Server) decidePersonal(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	if kind != "memory" && kind != "skills" && kind != "skill" {
		writeError(w, http.StatusNotFound, "接口不存在")
		return
	}
	token := personal.MemoryDoc
	if kind != "memory" {
		token = personal.SkillsDoc
	}
	var item personal.Item
	_, err := s.Runtime.Root.Commit(r.Context(), func(tx durable.Tx) (any, error) {
		result, err := personal.DecideInTx(tx, token, r.PathValue("uuid"), r.PathValue("action"))
		item = result
		return result, err
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": item})
}

func (s *Server) searchKnowledge(w http.ResponseWriter, r *http.Request) {
	roots := knowledge.AllowedRoots(s.Config.ObsidianRoot, s.Config.ObsidianAllow)
	hits, err := knowledge.Search(roots, r.URL.Query().Get("q"), r.URL.Query().Get("history") == "1")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	items := []map[string]any{}
	for _, hit := range hits {
		items = append(items, map[string]any{
			"path": hit.Path, "status": "current", "revision": "", "line": hit.Line,
			"endLine": hit.Line, "excerpt": hit.Text, "score": 1,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"query": r.URL.Query().Get("q"), "includeHistorical": r.URL.Query().Get("history") == "1", "hits": items})
}

func (s *Server) readKnowledge(w http.ResponseWriter, r *http.Request) {
	roots := knowledge.AllowedRoots(s.Config.ObsidianRoot, s.Config.ObsidianAllow)
	line, _ := strconv.Atoi(r.URL.Query().Get("line"))
	content, err := knowledge.Read(roots, r.URL.Query().Get("path"), line)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"path": r.URL.Query().Get("path"), "status": "current", "revision": "",
		"startLine": line, "endLine": line, "text": content, "truncated": len(content) >= 12_000,
	})
}

func decodeDoc[T any](draft *delta.Object) (T, error) {
	doc, err := durable.DecodeDoc[T](draft.Snapshot())
	if err != nil || doc == nil {
		var zero T
		return zero, err
	}
	return *doc, nil
}

var _ = fmt.Sprintf
var _ = time.Now
