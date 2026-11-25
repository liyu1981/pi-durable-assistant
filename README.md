# Pi Durable Assistant (Go rewrite)

A self-hosted, single-user durable AI assistant: a **single static Go binary** with an embedded React UI, backed by PiG's Durable runtime and SQLite. Features: crash recovery, resumable tasks, web chat with SSE, subagents, scheduled jobs, task center, memory/skills, attachments, read-only Obsidian search, and optional webhook notifications.

> Experimental, single-user project. Not an official Pi or PiG product. Back up the SQLite volume and restrict access before exposing it on a VPS.

## Tech stack

- Go 1.26 / toolchain 1.27.4, `github.com/MichaelKinsy/PiG` v0.7.0 (`durable`, `durable/harness`, `ai`, pure-Go SQLite)
- React 19, TypeScript 7, Vite 8, Tailwind CSS 4, Jotai 3, Biome 2 — compiled into `web/dist` and embedded with `go:embed`
- Docker multi-stage build + Compose; no Node.js required at runtime
- `make` targets mirror the whole workflow

## Quick start (no real API key)

```bash
make web                 # one-time: build embedded frontend
make dev-faux            # starts dev-only faux provider :18789 and app :3000
```

Open http://127.0.0.1:3000. The deterministic local demo script (add todos → wait 10s → complete todos → final answer) is served by `cmd/faux-llm`, a **development-only** process outside the main binary.

## Connect a real model

Create `.env` (do not commit) with `DEMO_PASSWORD`, `LLM_API` (`openai-completions`, `openai-responses`, `anthropic-messages`), `LLM_BASE_URL`, `LLM_API_KEY`, `LLM_MODEL`. Then:

```bash
make build
./bin/pi-durable-assistant
```

With `DEMO_PASSWORD` set the server binds `0.0.0.0`; without it, loopback only. Production requires a password.

## Self-host

```bash
cp .env.example .env      # set DEMO_PASSWORD and model configuration
docker compose up -d --build
docker compose ps
```

The container publishes only `127.0.0.1:3300` by default. Use an HTTPS reverse proxy and keep HTTP Basic Auth (`demo` user). SQLite lives in the named `/data` volume — never remove it as an upgrade step.

## HTTP API

`GET /api/info`, `GET|POST /api/conversations`, `GET /api/conversations/{id}/events` (SSE), `POST /api/conversations/{id}/{messages,fork,abort,archive,restore,remove}`, `GET /api/conversations/{id}/{management,export.md}`, `POST /api/conversations/{id}/{schedules,background}`, schedule/background/reminder management, `GET|POST /api/personal`, `GET /api/knowledge/{search,read}`, `POST /api/crash`. All endpoints use HTTP Basic Auth when a password is configured; writes require same-origin.

WeChat is intentionally not part of this release.

## Verification

```bash
make check                                     # go test ./... + go build
cd web && npm run typecheck && npm run check && npm run build
```

See `doc/REWRITE_PLAN.md` for the full rewrite plan, `doc/PIG_SPIKE.md` for verified PiG API notes, and `doc/DESIGN_QA.md` for the design checklist.

**License:** MIT. Not affiliated with Pi, PiG, or official WeChat teams.
