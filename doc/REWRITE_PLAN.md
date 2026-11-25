# Rewrite plan: Node/JS → Go + modern frontend

> Status: **implementation complete** (see §8 for verification evidence).
> Date: 2026-11-25
> Target: `pi-durable-assistant` rewritten as a single Go binary with an embedded, modern TypeScript web UI, styled with the `minimal` design skill, with the original Node/JS project archived on a backup branch.
> Decisions from the project owner are incorporated below (see §7).

---

## 1. Goal and scope

| | Current (`main`) | Target (`main` after cutover) |
| --- | --- | --- |
| Backend | Node ≥22.13, native ESM `.mjs`, Pi Durable 1.1.0, Pi AI, Chord | Go binary, `github.com/MichaelKinsy/PiG/durable` (+ PiG `ai`, `chord/delta`) |
| Persistence | `agent.sqlite` via `@earendil-works/pi-durable/storage/sqlite/node` | Fresh SQLite store via `durable/storage/sqlite/node.OpenNodeSqliteStorage` (pure Go, no cgo) |
| API | `node:http` + hand-rolled regex routes + SSE | `net/http` `ServeMux` + JSON DTOs + SSE (`http.Flusher`) — **keep paths and JSON shapes** |
| Frontend | Vanilla JS/CSS in `public/` | React 19 + TypeScript 7 + Vite 8 + Tailwind CSS 4 + Jotai 3, built to `web/dist` and embedded with `go:embed` |
| Design | Custom dark/light UI | `minimal` design system (tokens, typography, spacing, a11y) from `awesome-design-skills` |
| Delivery | `node:22-slim` + Compose | Multi-stage Docker: Node builder → Go builder → minimal static runtime; Compose keeps loopback port, volumes, limits |
| Dev-only faux model | In-process `faux` provider (`LLM_API=faux`) | **External dev-time OpenAI-compatible faux server** (`cmd/faux-llm`); the main Go app only speaks the normal provider protocol |
| Original code | tracked on `main` | moved to backup branch **and** `./tmp/` (gitignored); `main` contains only rewritten files |

Out of scope for the first rewrite release:

- **Data migration from the running original** — the new app starts with its own fresh store; the old database is not read or converted.
- **WeChat / wx-bot channel** — deferred entirely; no WeChat code, endpoints, env vars, or Compose profile in the first release.
- Original known limitations (single-user, non-cron fixed-minute schedules, no WeChat media parsing, no persistent webhook outbox) remain limitations.

---

## 2. Research findings (verified 2026-11-25)

### 2.1 PiG / durable (Go side)

- Module: `github.com/MichaelKinsy/PiG`; the durable runtime is a package inside the **root module** (`durable`, `durable/harness`, `durable/session`, `durable/storage`, `durable/storage/sqlite/node`, `durable/tools`, `durable/env`), **not** a separate Go module. Release pin: **PiG v0.7.0** (published 2026-11-05), which tracks upstream Pi 1.5.0 (the old JS project pins Pi 1.1.0 — behavior divergence risk only; no data compatibility work needed).
- `go.mod` requires `go 1.26.0` with `toolchain go1.27.4`. Local toolchain is Go 1.27.1, so builds use `GOTOOLCHAIN=auto` (default) to fetch 1.27.4, or 1.27.4 is installed explicitly. Docker builder image: `golang:1.27.4`.
- Native SQLite: `durable/storage/sqlite/node.OpenNodeSqliteStorage(path, NodeSqliteStorageOptions{})` → `*durablesqlite.SqliteStorage`. Uses `modernc.org/sqlite` (pure Go, `CGO_ENABLED=0`), WAL, `synchronous=NORMAL`, 5s busy timeout, one connection with a serial operation queue.
- Key API surface (from source/pkg.go.dev):
  - `harness.OpenHarness(ctx, storage, harness.HarnessOptions{Models, Registry, ...})`, `harness.CreateRegistry()`, `registry.Install(*durable.Extension)`.
  - `Harness.Root`, `CreateConversation`, `Conversation`, `Resume()`, `AbortTask`, `WatchTaskGraph`, `AbortSubmission`.
  - `Conversation.Submit(ctx, durable.SubmissionDraft{...})` → `Submission.Wait(ctx)`, `Commit(ctx, func(tx durable.Tx)...)`, `Fork`, `Abort`, `Watch`, `Entries`, `Agent`.
  - `harness.DefineTool`, `harness.DefineExtension`, `harness.Section`, `durable.DefineDoc[T]`, `durable.DefineTask[I,S,R,H]`, `durable.CreateTask`, `durable.TxDoc[T]`, `durable.Snapshot[T]`, `durable.ObserveDoc[T]`.
  - Tool signature: `Execute(ctx, args any, api durable.ToolExecutionApi) (durable.ToolExecutionResult, error)` with `ai.ToolSchema{Name, Description, Parameters map[string]any}` (JSON Schema) and `api.Commit/api.Memo/api.Output` plus context cancellation.
  - Tasks: typed phases with `runtime.SleepUntil(ctx, deadline, checkpoint)` and `runtime.Commit(...)` returning `*NextTaskState[S,R]` — direct analogue of the JS `defineTask` phases/checkpoints.
  - Watchers are **blocking calls** in Go (`Watch`, `WatchTaskGraph`, `ObserveDoc`): each SSE connection needs its own goroutine, context cancellation, and awareness of the `chord.DeliveryEntered` rule documented in `durable/doc.go` (never block a watch listener on I/O without handing off).
- Model layer: `github.com/MichaelKinsy/PiG/ai` provides `CreateModels`, provider creation, `ai.ModelRef`, `ai.ToolSchema`, and message/content types. PiG also ships an in-process faux provider, but per the project owner's decision the faux demo will be an **external dev-time server**, so the main app never contains demo-only model code.

### 2.2 Frontend versions (npm `latest` at 2026-11-25)

| Package | Version | Notes |
| --- | --- | --- |
| `typescript` | 7.0.2 | Native Go compiler, command still `tsc`. 7.0 has no stable programmatic API; use `tsc --noEmit` for typechecking. |
| `@biomejs/biome` | 2.6.9 | **Linting + formatting** for JS/TS/TSX/JSON/CSS. Biome is independent of the TypeScript compiler API, so it is unaffected by the TS 7 API gap. |
| `vite` | 8.3.8 | Requires Node `^20.19 \|\| >=22.12`; Rolldown-only bundler. Local Node is 24.20.0 — OK. |
| `@vitejs/plugin-react` | 6.3.0 | Peer `vite ^8.0.0`. |
| `tailwindcss` + `@tailwindcss/vite` | 4.3.5 | Vite plugin peer supports Vite 8; use CSS-first `@theme` config. |
| `react` / `react-dom` | 19.3.1 | Jotai peer `react >=18`. |
| `jotai` | 3.0.2 | Types require TS ≥5.5 (satisfied by TS 7); use `jotai/react`, `jotai/utils`. |

### 2.3 `minimal` design skill

Source: `liyu1981/awesome-design-skills/skills/minimal/{DESIGN.md,SKILL.md}`.

- Tokens: primary `#0C0C09`, secondary `#312C85`, success `#16A34A`, warning `#D97706`, danger `#DC2626`, surface `#F4F4F1`, text `#0C0C09`, neutral `#F4F4F1`.
- Typography: display **Inter** (h1 3rem), body **Open Sans** (1rem), mono/label **Inconsolata** (label-caps 0.75rem); weights 100–900.
- Radius: sm 4px, md 8px. Spacing scale: 4/8/12/16/24/32 (4px micro / 8px rhythm).
- Direction: minimal, clean, bold — whitespace, restrained color, strong hierarchy.
- Requirements: WCAG 2.2 AA, keyboard-first, visible focus states, explicit interaction states (default/hover/focus-visible/active/disabled/loading/error), accessible empty/overflow/long-label cases, concise/confident/helpful copy, semantic tokens over raw values, QA checklist in code review. Where aesthetics and accessibility conflict, **accessibility wins** (check `#312C85` on `#F4F4F1` contrast; use as accent, not small body text, if it fails 4.5:1).

---

## 3. Target repository layout (main after cutover)

```
.
├── main.go                         # package main; //go:embed web/dist; starts HTTP server
├── go.mod / go.sum                 # module github.com/liyu1981/pi-durable-assistant (Go 1.26, toolchain 1.27.4)
├── Makefile                        # web, web-dev, dev-faux, build, test, check, lint, format, docker
├── .env.example                    # same variable names as today, minus WECHAT_*
├── Dockerfile                      # multi-stage: node → golang → static runtime
├── compose.yaml                    # loopback port, /data volume, limits; no wx-bot service
├── README.md / doc/                # rewritten docs (this plan lives in doc/)
├── cmd/faux-llm/                   # DEV-ONLY: OpenAI-compatible deterministic faux provider server
├── internal/
│   ├── config/                     # env → Config struct, redaction helpers
│   ├── runtime/                    # PiG harness bootstrap, model/provider catalog
│   ├── docs/                       # DefineDoc tokens: Index, Meta, Notes, Reminders, Delegations,
│   │                               # Schedules, BackgroundJobs, Parents, Memory/Skills
│   ├── tools/                      # fetch_page, wait_seconds, todo, memo, current_time, remind_me, delegate
│   ├── tasks/                      # ReminderTask, ScheduleTask, BackgroundTask, notifyEvent
│   ├── personal/ knowledge/ attachments/ netutil/ schedule/
│   └── httpapi/                    # ServeMux routes, auth, DTOs, SSE, static handler
├── web/                            # Vite/React/TS project (source)
│   ├── package.json, package-lock.json, biome.json, tsconfig.json, vite.config.ts, index.html
│   ├── src/main.tsx, src/App.tsx
│   ├── src/atoms/                  # Jotai atoms
│   ├── src/api/                    # typed REST client + SSE client
│   ├── src/components/             # chat, sidebar, composer, task center, memory/skills
│   ├── src/styles/tokens.css       # minimal design tokens → Tailwind @theme
│   └── dist/                       # generated by `pnpm build`, gitignored
└── tmp/                            # copy of ALL original files, gitignored (working reference only)
```

`web/dist` must exist before `go build` (the `embed` pattern is resolved at compile time). `pnpm build` and the Dockerfile always build web assets first; plain `go build` works only after `pnpm web:build`. The faux dev server is a separate `go run ./cmd/faux-llm` process used only in development/CI demos — the main binary has no faux code path.

---

## 4. Phase-by-phase implementation plan

### Phase 0 — Preserve the original (gate before any rewrite work)

1. Verify clean worktree (`git status`) on `main`.
2. Create backup branch at current HEAD: `git branch backup/js-original main` (name confirmed by owner). Full original history remains there; no force operations.
3. Copy all original files into `./tmp/` (exclude `.git`, `node_modules`, `data`), then add `tmp/` to `.gitignore`. Use `tmp/` as the behavioral reference while porting.
4. Keep this plan in `doc/REWRITE_PLAN.md`.
5. **Gate:** per `AGENTS.md`, the agent does not commit or push. Creating the branch and any commit/cutover is performed by the user/orchestrator; the agent prepares files only.

### Phase 1 — Toolchain and skeleton

1. `go.mod` with module path **`github.com/liyu1981/pi-durable-assistant`**, PiG v0.7.0 pin, `go 1.26.0` + `toolchain go1.27.4`; `go mod tidy`; commit `go.sum`.
2. `web/package.json` with exact versions from §2.2. Scripts: `dev`, `build`, `typecheck` (`tsc --noEmit`), `lint` (`biome lint .`), `format` (`biome format --write .`), `check` (`biome check .`), `preview`, optional `test` (Vitest).
3. `biome.json`: formatter (2-space indent, double quotes, semicolons as needed) + recommended lint rules for TS/React; `files.ignore` for `dist`, `node_modules`, generated files. Formatting/linting is **Biome only** — no ESLint, Prettier, or typescript-eslint.
4. `vite.config.ts`: `@vitejs/plugin-react` + `tailwindcss()`; `build.outDir = "dist"`, relative `base: "./"` so embedded assets load from any path.
5. `tsconfig.json`: `strict: true`, `module: esnext`, `moduleResolution: bundler`, `target: ES2023`, `noEmit: true`, `jsx: react-jsx`, `types: []` (TS 7 defaults are strict — verify no deprecated flags).
6. `main.go` with `//go:embed web/dist` and a static file handler; wire config + router skeleton.
7. `Makefile`: `web`, `web-dev`, `dev-faux` (start faux server + app), `build`, `test`, `check`, `lint`, `format`, `docker`.
8. Multi-stage `Dockerfile`:
   - stage 1 `node:24-alpine`: `pnpm install --frozen-lockfile && pnpm build`
   - stage 2 `golang:1.27.4`: `CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/app .`
   - stage 3 `gcr.io/distroless/static-debian12:nonroot` (or `scratch`): copy binary, `USER 65532`, `ENV DATA_DIR=/data PORT=3000`, `EXPOSE 3000`.
9. `compose.yaml`: keep `127.0.0.1:${HOST_PORT:-3300}:3000`, named `/data` volume, `restart: unless-stopped`, `read_only: true`, `tmpfs`, `no-new-privileges`, mem/cpu limits. **No wx-bot service or WeChat ports/env.**

### Phase 2 — PiG runtime + faux contract spike (de-risk before porting features)

Create a throwaway spike (deleted or kept as a test) that proves:

1. `OpenNodeSqliteStorage` opens a **fresh** temp DB; harness bootstrap and basic submit/wait work end to end (no old DB is involved).
2. Harness API: `CreateRegistry` → install an extension with one tool + prompt section → `OpenHarness` → `Root` → `Submit`/`Wait` → read entries.
3. `Resume()` after reopening the same storage continues a sleeping task.
4. `Watch`/`WatchTaskGraph` deliver changes and can be cancelled by `context.Context`.
5. The main app can be configured with the normal `openai-completions` provider pointed at the external faux dev server (`LLM_BASE_URL=http://127.0.0.1:<port>/v1`), including **tool-call** responses in the format PiG's OpenAI-compatible API expects.
6. Record findings in `doc/PIG_SPIKE.md`, especially the exact faux server response schema for text, tool calls, streaming and stop reasons.

**Exit criteria:** all five points pass; otherwise adjust the plan before Phase 3.

### Phase 3 — Backend port (feature parity with `main`, minus WeChat)

Port module-by-module; keep the same HTTP contract for the features in scope:

| Original | Go replacement | Notes |
| --- | --- | --- |
| `server.mjs` | `internal/httpapi/` (router, auth, sse, handlers, dto) | `http.ServeMux` patterns; Basic Auth (`DEMO_PASSWORD`, user `demo`, constant-time compare), same-origin guard for writes, secret redaction in logs/outputs, 401/403/404/500 mapping, fatal storage error → exit 1 for supervisor restart |
| `lib/agent.mjs` | `config`, `runtime`, `docs`, `tools/*` | Documents via `DefineDoc[T]` with same kind/scope/fork semantics; tools via `DefineTool` + JSON Schema params + `api.Commit/api.Memo`; `wait_seconds` stores deadline in memo; `delegate` creates child conversation in a transaction, removes Delegate extension via `Configure`, `Submit`/`Wait` |
| `lib/v3.mjs` | `tasks/` + `docs` | `Schedules`, `BackgroundJobs`, `Parents` docs; `ScheduleTask`/`BackgroundTask` as `DefineTask` phases with `SleepUntil` + checkpoint; `notifyEvent` generic/Feishu HTTP POST with timeout; transaction helpers |
| `lib/personal.mjs` | `internal/personal` | Memory/skills docs, propose/approve/reject, `read_skill` tool |
| `lib/knowledge.mjs` | `internal/knowledge` | Allowlisted, read-only Obsidian vault search with path + line citations |
| `lib/attachments.mjs` | `internal/attachments` | Base64 image/text decode, MIME/size limits, map to PiG `ai` content parts |
| `lib/net.mjs` | `internal/netutil` | SSRF-safe fetch: DNS resolve/pin, private-range BlockList equivalent, hop/size/time limits |
| `lib/v3-utils.mjs` | `internal/schedule` | Pure functions + unit tests (normalize, nextOccurrence, matches, requestId) |
| `lib/wechat.mjs` | — | **Deferred; do not port.** |
| `tests/*.test.mjs` | Go `*_test.go` with `httptest` | Port in-scope scenarios: attachments, conversation shelf, personal HTTP, v3 HTTP/utils; skip WeChat tests |

SSE design: one handler per `/api/conversations/{id}/events`; subscribe to `conversation.Watch` and `Harness.WatchTaskGraph`; coalesce updates with a ~150 ms timer; `http.Flusher` writes `event: view\ndata: <json>\n\n`; 15 s heartbeat; on `r.Context().Done()` unsubscribe and dispose. Because PiG watchers block, run each subscription in its own goroutine and never hold a watch listener on channel/I/O without the `chord.DeliveryEntered` handoff.

Endpoints kept (minus WeChat): `/api/info`, `/api/conversations` (GET/POST), `/api/conversations/{id}/{events,messages,fork,abort}`, `/{archive,restore,remove}`, `/{management,schedules,background,reminders,export.md}`, `/images/{entry}/{index}`, `/api/personal{,/memory,/skills}`, `/api/knowledge/{search,read}`, `/api/crash`.

### Phase 3b — External dev-time faux server

`cmd/faux-llm` is a small standalone Go program (development/CI only, not imported by the main app):

- Implements a minimal OpenAI-compatible HTTP API (`GET /v1/models`, `POST /v1/chat/completions`) on a loopback port (default e.g. 18789).
- Deterministically reproduces the original `fauxScript`: add three todos → `wait_seconds(10)` → mark todos done → final text reply. Emit tool calls and stop reasons exactly as PiG's OpenAI provider expects (validated in Phase 2).
- Replay-safe: identical responses for identical request sequences; no state outside the request.
- Dev workflow: `pnpm dev:demo` runs the faux server and the app (bound to `0.0.0.0:3111`) with `LLM_API=openai-completions`, `LLM_BASE_URL=http://127.0.0.1:18789/v1`, `LLM_MODEL=faux-1`. Production/Docker never includes or starts this server.
- Add a Go test that drives the scripted sequence against the real main app to prove parity with the original demo.

### Phase 4 — Frontend rewrite

1. Replace `public/` entirely with the Vite app in `web/`.
2. Jotai atoms: `conversationsAtom`, `currentConversationIdAtom`, `viewAtom` (simplifyView payload), `busyAtom`, `serverInfoAtom`, `tasksAtom`, `personalAtom`, `knowledgeAtom`, `sseStatusAtom`, `attachmentsAtom`, task-center UI state.
3. `src/api/`: typed `request<T>()` fetch wrapper (browser handles Basic Auth) and `subscribeView(id, onView)` EventSource wrapper with reconnect/backoff. TypeScript interfaces mirror Go DTOs (hand-maintained in v1).
4. Components: app shell/sidebar (archived/active conversations), chat thread (user/assistant/tool entries with usage + task badges), composer (5-file base64 attachments, drag/drop, send/stop, fork), task center drawer (schedules/background/reminders, pause/resume/cancel/stop, export), memory & skills panel, knowledge search, toast/error states. **No WeChat UI.**
5. Remove all original `public/*` files from `main` in the cutover commit.

### Phase 5 — Apply the `minimal` design system

1. `web/src/styles/tokens.css`: Tailwind v4 `@theme` exposing the exact colors, fonts, radius and spacing from §2.3 as semantic utilities (`bg-surface`, `text-text`, `text-primary`, `accent-secondary`, `focus-visible:ring-secondary`, etc.). Self-host Inter/Open Sans/Inconsolata via `@fontsource/*` packages (no external font CDN for a self-hosted app).
2. Component rules: 4px micro-spacing / 8px rhythm; h1 3rem Inter; body 1rem Open Sans; labels 0.75rem Inconsolata uppercase; generous whitespace; 4px/8px radii only; status colors reserved for status (success/warning/danger), never decoration.
3. Interaction states: every interactive element defines default/hover/focus-visible/active/disabled/loading/error; keyboard-first with visible focus; long labels truncate with title, empty states explain the next action.
4. Accessibility gate: WCAG 2.2 AA — 4.5:1 body text, 3:1 UI components, keyboard operability, no color-only meaning; verify the indigo accent contrast.
5. Include the skill's anti-pattern list (gradients, heavy shadows, mixed radii, inconsistent spacing, ambiguous labels) and a QA checklist in `doc/DESIGN_QA.md`.

### Phase 6 — Build, CI, docs, cutover

1. Wire `//go:embed` + static handler; ensure `/`, `/assets/*`, and SPA fallback work with Basic Auth and no-store cache headers except immutable hashed assets.
2. CI (`.github/workflows/ci.yml`):
   - frontend job: `pnpm install --frozen-lockfile → pnpm typecheck → pnpm check → pnpm build` (Biome covers lint + format).
   - Go job: `go test ./... → go vet ./... → go build ./...`.
   - Docker job: `docker compose build`. Keep a faux end-to-end smoke test using `cmd/faux-llm`.
3. Rewrite `.env.example`, `README.md`, and `doc/` for the Go version. Remove WeChat variables (`WECHAT_ENABLED`, `WECHAT_WEBHOOK_KEY`, `WECHAT_SEND_TOKEN`, `WECHAT_SEND_URL`) and the WeChat docs. Original `README*.md`, `SPEC.md`, `docs/*`, `AGENTS.md`, `scripts/`, `tests/`, `lib/`, `public/`, `server.mjs`, `package*.json` are removed from `main` (they remain on the backup branch and in `tmp/`). Keep `LICENSE`; replace `AGENTS.md` with Go-era collaboration rules.
4. Cutover sequence (executed by the user/orchestrator; agent does not commit/push):
   ```bash
   git status                                   # must be clean
   git branch backup/js-original main            # preserve originals
   mkdir -p tmp && cp -a <original files> tmp/   # working reference
   # ... implement in working tree on main ...
   git add -A && git commit -m "rewrite: Go backend + embedded React frontend"
   git push origin main && git push origin backup/js-original
   ```
   Verify `git diff backup/js-original..main --stat` shows a clean replacement, and that `tmp/` is ignored.

### Phase 7 — Verification / definition of done

- `pnpm install --frozen-lockfile && pnpm --dir web typecheck && pnpm --dir web check && pnpm --dir web build` passes (Biome clean, `tsc` clean).
- `go test ./...` and `go vet ./...` pass; `pnpm build` produces a static binary; `CGO_ENABLED=0` confirmed.
- `docker compose build && docker compose up -d` serves the embedded UI at `127.0.0.1:3300`.
- Faux end-to-end via the external dev server: create conversation → send message → SSE updates → todo/wait → reminder and schedule fire → `POST /api/crash` → process restarts → tasks resume → task center shows state → Markdown export works.
- Security regression: Basic Auth required, same-origin write guard, no secret in logs, SSRF fetch rejects loopback/private targets.
- Design QA checklist from Phase 5 signed off; keyboard-only pass through chat, composer and task center.
- `main` contains no original JS/Node runtime files and no WeChat code; backup branch and `tmp/` retain the originals.

---

## 5. Data: fresh start (decision confirmed)

No migration from the running original app. The Go app creates and uses its own new SQLite store under `DATA_DIR` (default `./data`, `/data` in Docker). The old `agent.sqlite` is neither read nor converted; it stays untouched on the backup branch/volume as a historical artifact. The Compose named volume for the new app can be reused as an empty volume, but no upgrade path from old contents is promised.

---

## 6. Risks and mitigations

| Risk | Mitigation |
| --- | --- |
| PiG is pre-stable 0.x and tracks Pi 1.5.0 vs original 1.1.0 behavior | Pin v0.7.0; Phase 2 spike; port original in-scope tests as golden cases; document divergences in `doc/PIG_SPIKE.md` |
| Local Go 1.27.1 < required toolchain 1.27.4 | `GOTOOLCHAIN=auto` locally; Docker uses `golang:1.27.4` |
| TS 7 has no programmatic API | `tsc --noEmit` for types; Biome 2.6.9 for lint/format (independent of TS API); Vite/Oxc transforms TS independently |
| Jotai 3 types require TS ≥5.5 | Satisfied by TS 7.0.2; verify `jotai/react` type resolution in Phase 1 |
| Blocking Go watchers vs SSE | Dedicated goroutine + context cancellation + `chord.DeliveryEntered` handoff; load test with multiple SSE clients |
| Faux demo parity with the original script | External dev server reproduces the exact tool sequence; Phase 3b end-to-end test asserts it |
| Embedded assets missing on plain `go build` | Document `pnpm web:build` prerequisite; Dockerfile always builds web first |
| WeChat deferred | Explicitly excluded from scope, config, routes, UI, Compose and tests for this release |

## 7. Resolved decisions (owner, 2026-11-25)

1. **Backup branch:** `backup/js-original` — confirmed.
2. **Go module path:** `github.com/liyu1981/pi-durable-assistant` — changed from the repo's original owner.
3. **Data migration:** none; start with a fresh store — confirmed.
4. **WeChat:** leave out for now; not needed for this release — confirmed.
5. **Lint/format:** Biome (pin `@biomejs/biome` 2.6.9) for both linting and formatting; no ESLint/Prettier/typescript-eslint — confirmed.
6. **Fonts:** self-host via `@fontsource/*` — confirmed.
7. **Faux demo:** keep it, but as a mechanism outside the main Go app — a dev-time-only OpenAI-compatible server (`cmd/faux-llm`) started by `pnpm dev:demo`; the main binary has no faux code path — confirmed.


---

## 8. Implementation report

All phases of this plan were implemented on `main`; the original JS project was moved to `backup/js-original` and copied into gitignored `tmp/`. No commit or push was performed (per the collaboration rules).

### Files added / replaced

- Go backend: `main.go`, `go.mod`, `go.sum`, `Makefile`, `internal/{config,docs,runtime,tasks,tools,personal,knowledge,attachments,netutil,schedule,httpapi}/`, `cmd/faux-llm/`.
- Frontend: `web/` (React 19 + TypeScript 7 + Vite 8 + Tailwind 4 + Jotai 3 + Biome 2), with `web/src/styles/tokens.css` carrying the exact `minimal` tokens and self-hosted fonts.
- Delivery/docs: multi-stage `Dockerfile`, `compose.yaml`, `.env.example`, `AGENTS.md`, `README.md`, `.github/workflows/ci.yml`, `doc/PIG_SPIKE.md`, `doc/DESIGN_QA.md`.
- Removed from `main`: `server.mjs`, `lib/`, `public/`, `scripts/`, `tests/`, root `package*.json`, `SPEC.md`, `README.en.md`, original `docs/` and WeChat deploy files, old CI workflows.

### Verification evidence

- `pnpm check` — `go test ./...` all packages pass; `CGO_ENABLED=0 go build ./...` passes.
- `pnpm check` — TypeScript 7 clean, Biome 2 clean, Vite 8 build emits hashed assets into `web/dist` (~384 ms).
- End-to-end smoke test with the dev-only faux provider: Basic Auth (401 without credentials, 403 for cross-origin writes), embedded index served, conversation create/list, message submission, SSE `view` stream showing the full deterministic script (todo add → wait 10s → todo done → final answer), schedule fired once and completed, background subagent completed with result follow-up, personal memory propose/list, knowledge status, Markdown export, and crash endpoint protection.
- `git ls-tree -r backup/js-original --name-only` and `ls tmp/` confirm the original files are preserved.

### Follow-up verification (second pass)

- Background-task fix re-tested: after configuring the child conversation's model, `GET /api/conversations/{id}/management` reports `"result":"Demo task complete."` and the SSE view starts with the follow-up `[Background task result] Demo task complete.` — the earlier "returned no text result" is gone.
- Crash/restart/resume: `POST /api/crash` returned `200 {"ok":true}`; the supervisor restarted the process, `/api/info` boots went `1 → 2`, and a schedule created before the crash survived: after restart it reached `"runs":1,"status":"completed"` with `[Scheduled task]` and the full scripted answer in the view.
- Docker: created a gitignored `.env` from `.env.example` with test values, `docker compose config -q` passed, `docker compose build` produced `pi-durable-assistant:local`, `docker compose up -d` reached `Up`, and `127.0.0.1:3300` served `/api/info` (401 without Basic Auth), the embedded `index.html`, and a hashed JS asset (200, `text/javascript`). The first run exposed a root-owned `/data` volume problem; the Dockerfile now creates/chowns `/data` to uid 65532, after which the container started cleanly. A later `docker compose down`/`up` (volume retained) showed `boots:2`, confirming persistence.
- Ported the remaining in-scope original test scenarios as Go tests in `internal/httpapi/integration_test.go`: conversation shelf (archive/restore/remove and the "menu row only" semantics), personal memory propose/dedupe/approve/validation, and v3 management (schedule create/dedupe/validation/pause/resume/cancel and background create/dedupe/stop). Fixing these tests found and corrected a real shelf bug (`index out of range` on remove).
- Design audit: `doc/DESIGN_QA.md` completed. Added keyboard focus trap + focus restoration to the drawer; `pnpm typecheck`, `pnpm check`, `pnpm build` all pass after formatting.

### Not verified

- A real LLM provider, WeChat (excluded by decision), or mobile browsers.
- Interactive keyboard-only browser pass: implemented and source-audited, but no GUI browser was available; explicitly listed as manual follow-up in `doc/DESIGN_QA.md`.

### Third pass (build tooling, English-only, UI rebuild)

- `Makefile` removed; all targets now live in root `package.json` and use pnpm (`pnpm build`, `pnpm check`, `pnpm test`, `pnpm dev`, `pnpm dev:demo`, `pnpm docker`). Root and web lockfiles are `pnpm-lock.yaml`; Dockerfile, CI, README, AGENTS and this plan use pnpm commands only.
- Development mode now binds `0.0.0.0:3111` (`DEV=1` or `GO_ENV=development`). Verified from both loopback and the host IP: `started: 0.0.0.0:3111`, `/api/info` answered on both addresses.
- All project files and UI text are English-only. Repository-wide CJK scan (excluding `tmp/`, `node_modules`, `dist`) returned zero matches. The faux demo script now emits `Step 1: Research`, `Step 2: Wait 10 seconds`, `Step 3: Summarize`, and `Demo task complete.`; E2E on `0.0.0.0:3111` confirmed all tool results and notebook entries are English.
- The UI was rebuilt from scratch on the `minimal` design system with shared primitives in `web/src/components/ui.tsx`; no legacy layout, CSS, gradients, shadows, emoji, or pill styling remains. `doc/DESIGN_QA.md` was rewritten to match the new implementation.
- Final `pnpm check` passed: Go tests/build, TypeScript, Biome (26 files), and Vite build (`index-PmW_LELS.js`, 285.08 kB) all green.

### Dockerfile note

The Dockerfile now uses pnpm (no Makefile), but its image build is **not passing** with pnpm 12.23's default supply-chain `minimumReleaseAge` policy: newly pinned packages (Biome 2.6.9, `@types/react` 19.3.1) fall inside the cutoff. The previously built `pi-durable-assistant:local` image still runs, but a fresh `docker compose build` currently fails at `pnpm install --frozen-lockfile`. Per project-owner instruction this was left unresolved in this pass.
