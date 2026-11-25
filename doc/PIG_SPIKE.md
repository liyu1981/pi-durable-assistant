# PiG durable integration notes (verified 2026-11-25)

These notes record the API facts verified while rewriting the Node/JS app with `github.com/MichaelKinsy/PiG` v0.7.0.

## Module layout

- `github.com/MichaelKinsy/PiG` is the root Go module. `durable`, `durable/harness`, `durable/storage/sqlite/node`, `ai`, and `chord` are packages of that module (no separate `go.mod` for durable).
- `go.mod` requires `go 1.26.0`, `toolchain go1.27.4`; Go 1.27.1 with `GOTOOLCHAIN=auto` downloads the pinned toolchain automatically.
- SQLite: `node.OpenNodeSqliteStorage(path, node.NodeSqliteStorageOptions{})` returns `*durablesqlite.SqliteStorage`. It uses pure-Go `modernc.org/sqlite`, WAL, `synchronous=NORMAL`, 5s busy timeout, and a serial operation queue; `CGO_ENABLED=0` builds work.

## Runtime smoke-test results

Verified against a fresh `DATA_DIR` with `LLM_API=openai-completions` pointed at the dev-only `cmd/faux-llm` server:

1. `harness.OpenHarness` + `Root` + `Resume` work; boot count increments across restarts.
2. `Conversation.Submit` → `Submission.Wait` drives the deterministic script: three `todo` calls → `wait_seconds(10)` → three `todo` done calls → final text. All tool results appear in the SSE `view` payload.
3. Durable tasks survive a process restart: `Resume()` continues sleeping tasks.
4. `Conversation.Watch` and `Harness.WatchTaskGraph` deliver changes; **each SSE connection needs its own watches** (`Start` is single-use per handle). Watch listeners that block must call `chord.DeliveryEntered()` first.
5. `DefineDoc` requires conversation-scoped documents to declare `History` (`latest` or `rewindable`) and `Fork`; session-scoped documents must leave both empty.
6. `delta.Object.Set` accepts **strict JSON only**. Typed Go structs/slices must be converted with `durable.ToJsonValue` before being stored in a document draft.
7. Conversation-scoped documents require the conversation ID argument: `durable.TxDoc(tx, token, conversationID)`.
8. Tool execution API exposes `Commit`, `MemoCandidate`, `Details`, `Conversation`, and `CallId` — enough to port the original idempotency scheme.
9. Model provider: `ai.CreateProvider` with `API: ai.OpenAICompletionsAPI()` (or Responses/Anthropic variants), `Auth: ai.ProviderAuth{APIKey: ...}`, `Models: []ai.AnyModel{&ai.Model{...}}`. A non-empty `LLM_API_KEY` is required even for the local faux server; `make dev-faux` sets it.

## Known intentional divergences from the JS original

- Old app pins Pi Durable 1.1.0; PiG tracks Pi 1.5.0. Behavior differences are possible; in-scope Node tests were ported as Go tests where practical.
- No data migration: the Go app starts with a fresh SQLite store.
- No WeChat channel, no in-process faux provider in the main binary.
- `pi.live`/`pi.inbox` shapes come from PiG's view documents; the SSE payload keeps the original JSON keys for frontend compatibility.
