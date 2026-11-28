# Automatic conversation titles — plan

Status: implemented.

## 1. Current behavior

- New conversations are inserted into `app.index` as `New conversation`.
- On the first user message (`POST /api/conversations/{id}/messages`) the title is
  set to that message's text, truncated to 20 runes, and only while it is still
  `New conversation`.
- Forks use `Fork of <parent title>`.
- There is no LLM-generated title and no user rename API.

## 2. Goal

Generate a concise, meaningful title from the conversation as it develops,
instead of relying on the raw first message, without blocking or breaking the
normal agent run, and without overwriting a title the user chose.

## 3. Proposed design

### 3.1 Title ownership

Add a source marker to `docs.IndexItem`:

- `titleSource: "auto" | "user" | "fork"` (default for existing rows: `auto`).
- Auto-title logic only updates rows with `titleSource: "auto"`.
- A user rename sets the title and `titleSource: "user"` so automatic updates
  never overwrite it.
- Forks start as `titleSource: "fork"` and are not auto-renamed.

Add `PUT /api/conversations/{id}/title` with `{"title": "..."}`. The UI gains an
inline rename control in the conversation header/sidebar.

### 3.2 Trigger

Use the harness generation **`AfterRun` hook** on the assistant extension:

- Run only after the **first successful final answer** in a conversation. Use
  a durable memo (`titleGenerated`) keyed by the generation task/conversation so
  a hook replay after a crash cannot generate twice.
- Decision: **one shot after the first answer**. No periodic refresh. Later
  retitling happens only when the user asks for it (see 3.6).
- The hook asks for a short durable child task via `RunEndTasks`; the title task
  is created and awaited with the generation, so the result is committed before
  the run is marked complete.

`AfterRun` deliberately does not run on failed/aborted runs, so bad answers do
not produce titles.

### 3.3 Title task

Define a small durable task in `internal/tasks` (for example `TitleTask`):

Input:

- conversation ID,
- first user message text (bounded, e.g. 4k chars),
- final assistant answer text (bounded, e.g. 2k chars).

Behavior:

- Decision: use the **active model** (`ai.Models.CompleteSimple`). No separate
  title model profile.
- Tiny system instruction: return one title under **50 runes**, no quotes, no
  prefix, English only.
- Disable tools, use low `maxTokens` (e.g. 48), short timeout, and no retries.
- Strip markdown/code fences and newlines; clamp to **50 runes**.
- On success, commit an update to the conversation's row in `app.index` only if
  the row still exists and `titleSource == "auto"`.
- On failure, leave the current deterministic title unchanged; the task returns
  successfully so it never fails the generation run.

### 3.4 Fallback and normalization

- Before the title task finishes, keep the current first-message-derived title.
- If the model returns empty/invalid text, keep the fallback.
- Titles are plain text, single-line, trimmed, and length-limited.
- Existing titles from old databases continue to work; `titleSource` defaults
  to `auto` when absent.

### 3.5 UI

- The sidebar and conversation header show the updated title from the existing
  `app.index`/view payload; no extra polling is needed because the update is a
  committed durable change watched by the SSE stream.
- Add a small rename action (pencil icon) next to the title in the conversation
  header and sidebar item. Saving calls the new title endpoint and marks the
  title as user-owned.

### 3.6 Agent retitle tool (user-requested)

Decision: add an agent tool instead of a skill for retitling on demand.

- `retitle_conversation` (optional `hint` string) creates a new `TitleTask` for
  the current conversation with the same active-model generation and 50-rune
  normalization.
- It is allowed to overwrite an `auto` or `user` title **only when the user
  explicitly asks in the conversation**; the tool call itself is the user's
  request. The task sets `titleSource: "auto"` for a generated title (or
  `"user"` if a literal `title` argument is supplied).
- No human-in-the-loop confirmation is required because the user initiated it
  in the conversation.
- The tool returns a plain confirmation string with the resulting title.

### 3.7 Manual rename UI

Decision: expose the rename action in **both** the sidebar conversation items
and the conversation header.

## 4. Files and APIs touched

- `internal/docs/docs.go`: add `TitleSource` to `IndexItem`.
- `internal/tasks/`: new `TitleTask` and tests.
- `internal/tools/tools.go`: register the `AfterRun` hook, the `TitleTask`, and
  the `retitle_conversation` tool on the assistant extension.
- `internal/httpapi/conversations.go`: title rename route and helper shared
  with auto-title logic.
- `web/src/api/client.ts`, `web/src/api/types.ts`: rename endpoint/types.
- `web/src/components/Sidebar.tsx`, `web/src/App.tsx`: inline rename UI.
- Integration tests: auto-title after successful run, failure fallback, user
  title is not overwritten, fork title unchanged.

## 5. Decisions made

1. Retitle trigger: **one shot after the first answer**, plus an explicit
   `retitle_conversation` agent tool when the user asks.
2. Model: **active model**; no separate title profile.
3. Title limit: **50 runes**.
4. Manual rename: available in **both sidebar and header**.
