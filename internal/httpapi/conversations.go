package httpapi

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
	"github.com/MichaelKinsy/PiG/chord"
	"github.com/MichaelKinsy/PiG/durable"
	"github.com/MichaelKinsy/PiG/durable/harness"
	"github.com/liyu1981/pi-durable-assistant/internal/attachments"
	"github.com/liyu1981/pi-durable-assistant/internal/docs"
	"github.com/liyu1981/pi-durable-assistant/internal/tasks"
)

type ToolCallView struct {
	ID   any    `json:"id"`
	Name string `json:"name"`
	Args any    `json:"args,omitempty"`
}

type EntryView struct {
	ID          int64              `json:"id"`
	Kind        string             `json:"kind"`
	Text        string             `json:"text"`
	Attachments []attachments.File `json:"attachments,omitempty"`
	ToolCalls   []ToolCallView     `json:"toolCalls,omitempty"`
	Error       string             `json:"error,omitempty"`
	ToolCallID  string             `json:"toolCallId,omitempty"`
	Name        string             `json:"name,omitempty"`
	IsError     bool               `json:"isError,omitempty"`
}

type ViewPayload struct {
	ConversationID int64                 `json:"conversationId"`
	Entries        []EntryView           `json:"entries"`
	Live           map[string]any        `json:"live"`
	Busy           bool                  `json:"busy"`
	Inbox          int                   `json:"inbox"`
	Notes          map[string]any        `json:"notes"`
	Usage          any                   `json:"usage"`
	Tasks          []map[string]any      `json:"tasks"`
	Reminders      []docs.ReminderItem   `json:"reminders"`
	Subagents      []docs.DelegationItem `json:"subagents"`
	ParentID       *int64                `json:"parentId"`
}

func contentText(content any) string {
	switch value := content.(type) {
	case string:
		return value
	case ai.UserText:
		return string(value)
	case ai.UserContentBlocks:
		parts := []string{}
		for _, part := range value {
			if text, ok := part.(ai.TextContent); ok {
				parts = append(parts, text.Text)
			}
		}
		return strings.Join(parts, "")
	default:
		return ""
	}
}

func simplifyView(ctx context.Context, server *Server, conversation harness.Conversation, view harness.ConversationView, graph harness.TaskGraph) (ViewPayload, error) {
	payload := ViewPayload{
		ConversationID: int64(conversation.Id()), Entries: []EntryView{}, Live: map[string]any{},
		Notes: map[string]any{"todos": []any{}, "memos": []any{}}, Tasks: []map[string]any{},
		Reminders: []docs.ReminderItem{}, Subagents: []docs.DelegationItem{},
	}
	for _, entry := range view.Entries {
		if len(entry.Model) == 0 {
			continue
		}
		message := entry.Model[0]
		switch typed := message.(type) {
		case ai.UserMessage:
			text, files := attachments.SplitUserContent(typed.Content)
			for i := range files {
				if files[i].Kind == "image" && files[i].Index != nil {
					files[i].URL = fmt.Sprintf("/api/conversations/%d/images/%d/%d", conversation.Id(), entry.Id, *files[i].Index)
				}
			}
			payload.Entries = append(payload.Entries, EntryView{ID: int64(entry.Id), Kind: "user", Text: text, Attachments: files})
		case ai.AssistantMessage:
			var sb strings.Builder
			calls := []ToolCallView{}
			for _, part := range typed.Content {
				switch block := part.(type) {
				case ai.TextContent:
					sb.WriteString(block.Text)
				case ai.ToolCall:
					calls = append(calls, ToolCallView{ID: block.ID, Name: block.Name, Args: block.ArgumentsValue()})
				}
			}
			item := EntryView{ID: int64(entry.Id), Kind: "assistant", Text: sb.String(), ToolCalls: calls}
			if typed.StopReason == ai.StopReasonError {
				item.Error = server.Config.Redact(typed.ErrorMessage)
			}
			payload.Entries = append(payload.Entries, item)
		case ai.ToolResultMessage:
			var sb strings.Builder
			for _, part := range typed.Content {
				if text, ok := part.(ai.TextContent); ok {
					sb.WriteString(text.Text)
				}
			}
			value := strings.ReplaceAll(sb.String(), "<harness>", "")
			value = strings.ReplaceAll(value, "</harness>", "")
			payload.Entries = append(payload.Entries, EntryView{
				ID: int64(entry.Id), Kind: "tool", Text: strings.TrimSpace(strings.TrimPrefix(value, "[error]")),
				ToolCallID: typed.ToolCallID, Name: typed.ToolName, IsError: typed.IsError,
			})
		}
	}
	if live, ok := view.Docs.Get("pi.live"); ok && live != nil {
		for _, key := range live.Keys() {
			value, _ := live.Get(key)
			payload.Live[key] = value
		}
	}
	payload.Busy = len(payload.Live) > 0
	if inbox, ok := view.Docs.Get("pi.inbox"); ok && inbox != nil {
		if value, exists := inbox.Get("items"); exists {
			if items, ok := value.([]any); ok {
				payload.Inbox = len(items)
			}
		}
	}
	notes, err := runtimeSnapshotConversation[docs.NotesDoc](ctx, server, docs.Notes, conversation.Id())
	if err == nil && notes != nil {
		payload.Notes["todos"] = notes.Todos
		payload.Notes["memos"] = notes.Memos
	}
	reminders, err := runtimeSnapshotConversation[docs.ReminderDoc](ctx, server, docs.Reminders, conversation.Id())
	if err == nil && reminders != nil {
		payload.Reminders = append(payload.Reminders, reminders.Items...)
	}
	sort.Slice(payload.Reminders, func(i, j int) bool { return payload.Reminders[i].At < payload.Reminders[j].At })
	delegations, err := runtimeSnapshotConversation[docs.DelegationDoc](ctx, server, docs.Delegations, conversation.Id())
	if err == nil && delegations != nil {
		payload.Subagents = delegations.Items
	}
	backgrounds, err := runtimeSnapshotConversation[docs.BackgroundDoc](ctx, server, docs.Backgrounds, conversation.Id())
	if err == nil && backgrounds != nil {
		for _, job := range backgrounds.Items {
			if job.ConversationID != nil {
				payload.Subagents = append(payload.Subagents, docs.DelegationItem{ConversationID: *job.ConversationID, Task: job.Text, Status: job.Status})
			}
		}
	}
	index, _ := runtimeSnapshot[docs.IndexDoc](ctx, server, docs.Index)
	if index != nil {
		if parent, ok := index.Children[strconv.FormatInt(int64(conversation.Id()), 10)]; ok {
			payload.ParentID = &parent
		}
	}
	parents, _ := runtimeSnapshot[docs.ParentDoc](ctx, server, docs.Parents)
	if parents != nil && payload.ParentID == nil {
		if parent, ok := parents.Children[strconv.FormatInt(int64(conversation.Id()), 10)]; ok {
			payload.ParentID = &parent
		}
	}
	mine := map[int64]bool{int64(conversation.Id()): true}
	for _, item := range payload.Subagents {
		mine[item.ConversationID] = true
	}
	if graph.Tasks != nil {
		for _, node := range graph.Tasks {
			if !mine[int64(node.ConversationId)] {
				continue
			}
			owner := ""
			if node.Owner != nil {
				owner = strconv.FormatInt(int64(*node.Owner), 10)
			}
			payload.Tasks = append(payload.Tasks, map[string]any{
				"id": strconv.FormatInt(int64(node.Id), 10), "kind": node.Kind, "status": string(node.State.Status),
				"phase": node.State.Phase, "owner": nullIfEmpty(owner), "background": node.Background,
				"conversationId": int64(node.ConversationId), "conversations": node.Conversations,
			})
		}
	}
	return payload, nil
}

func nullIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	conversation, err := s.getConversation(r, r.PathValue("id"))
	if err != nil {
		handleError(w, err)
		return
	}
	ctx := r.Context()
	viewWatch, err := conversation.Watch(ctx, nil)
	if err != nil {
		handleError(w, err)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming is not supported")
		viewWatch.Stop()
		return
	}
	w.Header().Set("content-type", "text/event-stream; charset=utf-8")
	w.Header().Set("cache-control", "no-store")
	w.Header().Set("connection", "keep-alive")
	w.Header().Set("x-accel-buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	notify := make(chan struct{}, 1)
	viewWatch.Start(func(ctx context.Context, call chord.WatchCall[harness.ConversationView]) error {
		chord.DeliveryEntered()
		select {
		case notify <- struct{}{}:
		default:
		}
		return nil
	})
	graphWatch, err := s.Runtime.Harness.WatchTaskGraph(ctx)
	if err != nil {
		return
	}
	graphWatch.Start(func(ctx context.Context, call chord.WatchCall[harness.TaskGraph]) error {
		chord.DeliveryEntered()
		select {
		case notify <- struct{}{}:
		default:
		}
		return nil
	})
	defer func() {
		viewWatch.Stop()
		graphWatch.Stop()
	}()

	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	var timer *time.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	push := func() {
		payload, err := simplifyView(ctx, s, conversation, viewWatch.Value(), graphWatch.Value())
		if err != nil {
			return
		}
		data, err := jsonMarshal(payload)
		if err != nil {
			return
		}
		if _, err := fmt.Fprintf(w, "event: view\ndata: %s\n\n", data); err != nil {
			return
		}
		flusher.Flush()
	}
	push()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case <-notify:
			if timer == nil {
				timer = time.NewTimer(150 * time.Millisecond)
			} else {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(150 * time.Millisecond)
			}
		case <-timerChannel(timer):
			push()
			timer = nil
		}
	}
}

func timerChannel(timer *time.Timer) <-chan time.Time {
	if timer == nil {
		return nil
	}
	return timer.C
}

func (s *Server) listConversations(w http.ResponseWriter, r *http.Request) {
	index, err := runtimeSnapshot[docs.IndexDoc](r.Context(), s, docs.Index)
	if err != nil {
		handleError(w, err)
		return
	}
	items := []docs.IndexItem{}
	if index != nil {
		items = append(items, index.Items...)
	}
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
	writeJSON(w, http.StatusOK, items)
}

func registerConversation(title string, extra docs.IndexItem) func(tx durable.Tx, id durable.ConversationId) error {
	return func(tx durable.Tx, id durable.ConversationId) error {
		draft, err := durable.TxDoc(tx, docs.Index)
		if err != nil {
			return err
		}
		index, err := durable.DecodeDoc[docs.IndexDoc](draft.Snapshot())
		if err != nil {
			return err
		}
		item := docs.IndexItem{ID: int64(id), CreatedAt: time.Now().UTC().Format(time.RFC3339), Title: title}
		item.ForkOf, item.ForkAtEntry, item.ArchivedAt = extra.ForkOf, extra.ForkAtEntry, extra.ArchivedAt
		index.Items = append(index.Items, item)
		return writeIndex(draft, index)
	}
}

func (s *Server) createConversation(w http.ResponseWriter, r *http.Request) {
	conversation, err := s.Runtime.Harness.CreateConversation(r.Context(), harness.ConversationCreateOptions{
		Ownership: durable.ConversationOwnership{Kind: durable.ConversationOwnerless},
		Agent:     &harness.AgentChange{Model: harness.SetTo(s.Runtime.Model())},
		Init:      registerConversation("New conversation", docs.IndexItem{}),
	})
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": conversation.Id()})
}

func (s *Server) postMessage(w http.ResponseWriter, r *http.Request) {
	conversation, err := s.getConversation(r, r.PathValue("id"))
	if err != nil {
		handleError(w, err)
		return
	}
	body, err := readBody(r, 14_000_000)
	if err != nil {
		handleError(w, err)
		return
	}
	text := strings.TrimSpace(fmt.Sprint(body["text"]))
	if text == "<nil>" {
		text = ""
	}
	files, err := attachments.Parse(body["attachments"])
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if text == "" && len(files) == 0 {
		writeError(w, http.StatusBadRequest, "message cannot be empty")
		return
	}
	if len([]rune(text)) > 8000 {
		writeError(w, http.StatusRequestEntityTooLarge, "message too long")
		return
	}
	whenBusy := durable.WhenBusyFollowUp
	if body["mode"] == "steer" {
		whenBusy = durable.WhenBusySteer
	}
	var requestID *string
	if value, ok := body["requestId"].(string); ok && len(value) <= 100 {
		requestID = &value
	}
	submission, err := conversation.Submit(r.Context(), durable.SubmissionDraft{
		Type: durable.SubmissionTypeInput, Content: attachments.BuildUserContent(text, files),
		RequestId: requestID, WhenBusy: whenBusy,
	})
	if err != nil {
		handleError(w, err)
		return
	}
	title := text
	if title == "" && len(files) > 0 {
		title = "[attachment] " + files[0].Name
	}
	_, _ = s.Runtime.Root.Commit(r.Context(), func(tx durable.Tx) (any, error) {
		draft, err := durable.TxDoc(tx, docs.Index)
		if err != nil {
			return nil, err
		}
		index, err := durable.DecodeDoc[docs.IndexDoc](draft.Snapshot())
		if err != nil {
			return nil, err
		}
		runes := []rune(title)
		if len(runes) > 20 {
			runes = runes[:20]
		}
		for _, item := range index.Items {
			if item.ID == int64(conversation.Id()) && item.Title == "New conversation" {
				item.Title = string(runes)
			}
		}
		return nil, writeIndex(draft, index)
	})
	writeJSON(w, http.StatusAccepted, map[string]any{"submissionId": submission.Id()})
}

func (s *Server) fork(w http.ResponseWriter, r *http.Request) {
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
	entryIDValue, ok := body["entryId"].(float64)
	if !ok || entryIDValue <= 0 {
		writeError(w, http.StatusBadRequest, "missing entryId")
		return
	}
	index, _ := runtimeSnapshot[docs.IndexDoc](r.Context(), s, docs.Index)
	parentTitle := fmt.Sprint(conversation.Id())
	if index != nil {
		for _, item := range index.Items {
			if item.ID == int64(conversation.Id()) {
				parentTitle = item.Title
			}
		}
	}
	entryID := durable.EntryId(entryIDValue)
	parentID := int64(conversation.Id())
	forkAt := int64(entryIDValue)
	fork, err := conversation.Fork(r.Context(), entryID, harness.ConversationCreateOptions{
		Ownership: durable.ConversationOwnership{Kind: durable.ConversationOwnerless},
		Agent:     &harness.AgentChange{Model: harness.SetTo(s.Runtime.Model())},
		Init:      registerConversation("Fork of "+parentTitle, docs.IndexItem{ForkOf: &parentID, ForkAtEntry: &forkAt}),
	})
	if err != nil {
		handleError(w, err)
		return
	}
	_, _ = s.Runtime.Root.Commit(r.Context(), func(tx durable.Tx) (any, error) {
		draft, err := durable.TxDoc(tx, docs.Index)
		if err != nil {
			return nil, err
		}
		index, err := durable.DecodeDoc[docs.IndexDoc](draft.Snapshot())
		if err != nil {
			return nil, err
		}
		index.Children[strconv.FormatInt(int64(fork.Id()), 10)] = parentID
		return nil, writeIndex(draft, index)
	})
	writeJSON(w, http.StatusCreated, map[string]any{"id": fork.Id()})
}

func (s *Server) abort(w http.ResponseWriter, r *http.Request) {
	conversation, err := s.getConversation(r, r.PathValue("id"))
	if err != nil {
		handleError(w, err)
		return
	}
	if err := conversation.Abort(r.Context(), nil); err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) shelf(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conversation, err := s.getConversation(r, r.PathValue("id"))
		if err != nil {
			handleError(w, err)
			return
		}
		id := int64(conversation.Id())
		result, err := s.Runtime.Root.Commit(r.Context(), func(tx durable.Tx) (any, error) {
			draft, err := durable.TxDoc(tx, docs.Index)
			if err != nil {
				return nil, err
			}
			index, err := durable.DecodeDoc[docs.IndexDoc](draft.Snapshot())
			if err != nil {
				return nil, err
			}
			for i, item := range index.Items {
				if item.ID != id {
					continue
				}
				resultItem := item
				if action == "remove" {
					index.Items = append(index.Items[:i], index.Items[i+1:]...)
					resultItem.Removed = true
				} else if action == "archive" {
					at := time.Now().UTC().Format(time.RFC3339)
					index.Items[i].ArchivedAt = &at
					resultItem = index.Items[i]
				} else {
					index.Items[i].ArchivedAt = nil
					resultItem = index.Items[i]
				}
				if err := writeIndex(draft, index); err != nil {
					return nil, err
				}
				return resultItem, nil
			}
			return nil, badRequest("conversation is not in the list")
		})
		if err != nil {
			handleError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func (s *Server) image(w http.ResponseWriter, r *http.Request) {
	conversation, err := s.getConversation(r, r.PathValue("id"))
	if err != nil {
		handleError(w, err)
		return
	}
	entryID, _ := strconv.ParseInt(r.PathValue("entry"), 10, 64)
	imageIndex, _ := strconv.Atoi(r.PathValue("index"))
	minID, maxID := durable.EntryId(entryID), durable.EntryId(entryID)
	page, err := conversation.Entries(r.Context(), durable.EntryQuery{MinEntryId: &minID, MaxEntryId: &maxID}, 1, nil)
	if err != nil {
		handleError(w, err)
		return
	}
	var found *ai.ImageContent
	for _, entry := range page.Items {
		if entry.Kind != "pi.user" || len(entry.Model) == 0 {
			continue
		}
		if message, ok := entry.Model[0].(ai.UserMessage); ok {
			found = attachments.ImageAt(message.Content, imageIndex)
		}
	}
	if found == nil {
		writeError(w, http.StatusNotFound, "image not found")
		return
	}
	data, err := base64.StdEncoding.DecodeString(found.Data)
	if err != nil {
		writeError(w, http.StatusNotFound, "image not found")
		return
	}
	w.Header().Set("content-type", found.MimeType)
	w.Header().Set("cache-control", "private, max-age=31536000, immutable")
	w.Header().Set("x-content-type-options", "nosniff")
	w.Write(data)
}

func (s *Server) exportMarkdown(w http.ResponseWriter, r *http.Request) {
	conversation, err := s.getConversation(r, r.PathValue("id"))
	if err != nil {
		handleError(w, err)
		return
	}
	watch, err := conversation.Watch(r.Context(), nil)
	if err != nil {
		handleError(w, err)
		return
	}
	defer watch.Stop()
	payload, err := simplifyView(r.Context(), s, conversation, watch.Value(), harness.TaskGraph{Tasks: map[string]harness.TaskGraphNode{}})
	if err != nil {
		handleError(w, err)
		return
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Pi Durable conversation %d\n\n", conversation.Id())
	for _, entry := range payload.Entries {
		if entry.Kind != "user" && entry.Kind != "assistant" {
			continue
		}
		label := "User"
		if entry.Kind == "assistant" {
			label = "Assistant"
		}
		fmt.Fprintf(&sb, "## %s\n\n%s\n\n", label, entry.Text)
	}
	w.Header().Set("content-type", "text/markdown; charset=utf-8")
	w.Header().Set("content-disposition", fmt.Sprintf(`attachment; filename="conversation-%d.md"`, conversation.Id()))
	w.Write([]byte(sb.String()))
}

var _ = tasks.Notify
