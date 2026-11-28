// Package docs defines every durable document used by the application.
package docs

import (
	"github.com/MichaelKinsy/PiG/durable"
)

// Todo is one entry of the conversation notebook.
type Todo struct {
	Text string `json:"text"`
	Done bool   `json:"done"`
}

// NotesDoc is the per-conversation notebook, inherited as of a fork point.
type NotesDoc struct {
	Todos   []Todo         `json:"todos"`
	Memos   []string       `json:"memos"`
	Applied map[string]any `json:"applied"`
}

// IndexDoc lists conversations (Pi has no public conversation-list API).
type IndexDoc struct {
	Items    []IndexItem      `json:"items"`
	Children map[string]int64 `json:"children,omitempty"`
}

// IndexItem is one row of the conversation menu.
type IndexItem struct {
	ID          int64   `json:"id"`
	CreatedAt   string  `json:"createdAt"`
	Title       string  `json:"title"`
	TitleSource string  `json:"titleSource,omitempty"`
	ForkOf      *int64  `json:"forkOf,omitempty"`
	ForkAtEntry *int64  `json:"forkAtEntry,omitempty"`
	ArchivedAt  *string `json:"archivedAt,omitempty"`
	Removed     bool    `json:"removed,omitempty"`
}

// MetaDoc records boot count to demonstrate process restarts.
type MetaDoc struct {
	Boots      int     `json:"boots"`
	LastBootAt *string `json:"lastBootAt"`
}

// ReminderDoc is a conversation's durable reminder list.
type ReminderDoc struct {
	Items   []ReminderItem `json:"items"`
	Applied map[string]any `json:"applied"`
}

// ReminderItem is one scheduled reminder.
type ReminderItem struct {
	ID     int64  `json:"id"`
	Text   string `json:"text"`
	At     int64  `json:"at"`
	Status string `json:"status"`
}

// DelegationDoc tracks foreground subagents of a conversation.
type DelegationDoc struct {
	Items []DelegationItem `json:"items"`
}

// DelegationItem is one foreground subagent.
type DelegationItem struct {
	ConversationID int64  `json:"conversationId"`
	Task           string `json:"task"`
	Status         string `json:"status"`
}

// ScheduleDoc is the v3 periodic task list.
type ScheduleDoc struct {
	Items   []ScheduleItem `json:"items"`
	Applied map[string]any `json:"applied"`
}

// ScheduleItem is one periodic or one-off scheduled job.
type ScheduleItem struct {
	ID           string  `json:"id"`
	Text         string  `json:"text"`
	NextAt       int64   `json:"nextAt"`
	EveryMinutes float64 `json:"everyMinutes"`
	Revision     int     `json:"revision"`
	Status       string  `json:"status"`
	JobID        *int64  `json:"jobId,omitempty"`
	Runs         int     `json:"runs"`
	LastRunAt    *int64  `json:"lastRunAt,omitempty"`
	CreatedAt    int64   `json:"createdAt,omitempty"`
}

// BackgroundDoc is the v3 background job list.
type BackgroundDoc struct {
	Items   []BackgroundItem `json:"items"`
	Applied map[string]any   `json:"applied"`
}

// BackgroundItem is one background subagent job.
type BackgroundItem struct {
	ID             string  `json:"id"`
	Text           string  `json:"text"`
	Status         string  `json:"status"`
	TaskID         *int64  `json:"taskId,omitempty"`
	ConversationID *int64  `json:"conversationId,omitempty"`
	Result         *string `json:"result,omitempty"`
	CreatedAt      int64   `json:"createdAt,omitempty"`
}

// ParentDoc maps child conversations to their parents (session scope).
type ParentDoc struct {
	Children map[string]int64 `json:"children"`
}

func singleton[T any](kind string, scope durable.DocumentScope, history durable.DocumentHistory, fork durable.DocumentFork, initial func() T) durable.DocToken[T] {
	return durable.DefineDoc(durable.DocDefinition[T]{
		CommonDocDefinition: durable.CommonDocDefinition[T]{
			Kind:           kind,
			Version:        1,
			Initial:        initial,
			CheckpointWhen: func(T, []durable.Op, durable.CheckpointInfo) bool { return false },
		},
		DocumentSemantics: durable.DocumentSemantics{Scope: scope, History: history, Fork: fork},
	})
}

var (
	Notes = singleton("app.notes", durable.ScopeConversation, durable.HistoryRewindable, durable.ForkAsOf, func() NotesDoc {
		return NotesDoc{Applied: map[string]any{}}
	})
	Index = singleton("app.index", durable.ScopeSession, "", "", func() IndexDoc {
		return IndexDoc{Children: map[string]int64{}}
	})
	Meta      = singleton("app.meta", durable.ScopeSession, "", "", func() MetaDoc { return MetaDoc{} })
	Reminders = singleton("app.reminders", durable.ScopeConversation, durable.HistoryLatest, durable.ForkInitial, func() ReminderDoc {
		return ReminderDoc{Applied: map[string]any{}}
	})
	Delegations = singleton("app.delegations", durable.ScopeConversation, durable.HistoryLatest, durable.ForkInitial, func() DelegationDoc {
		return DelegationDoc{}
	})
	Schedules = singleton("app.v3.schedules", durable.ScopeConversation, durable.HistoryLatest, durable.ForkInitial, func() ScheduleDoc {
		return ScheduleDoc{Applied: map[string]any{}}
	})
	Backgrounds = singleton("app.v3.background", durable.ScopeConversation, durable.HistoryLatest, durable.ForkInitial, func() BackgroundDoc {
		return BackgroundDoc{Applied: map[string]any{}}
	})
	Parents = singleton("app.v3.parents", durable.ScopeSession, "", "", func() ParentDoc {
		return ParentDoc{Children: map[string]int64{}}
	})
)
