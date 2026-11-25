# Pi Durable Assistant (Go rewrite)

A self-hosted, single-user durable AI assistant: one static Go binary with an embedded React UI, backed by PiG's Durable runtime and SQLite. Features: crash recovery, resumable tasks, web chat over SSE, subagents, scheduled jobs, a task center, memory and skills, attachments, read-only note search, and optional webhook notifications.

> Experimental, single-user project. Not an official Pi or PiG product. Back up the SQLite volume and restrict access before exposing it on a VPS.

## Tech stack

- Go 1.26 / toolchain 1.27.4, `github.com/MichaelKinsy/PiG` v0.7.0 (`durable`, `durable/harness`, `ai`, pure-Go SQLite)
- React 19, TypeScript 7, Vite 8, Tailwind CSS 4, shadcn/ui with the Luma preset (`b2D0wqNxT`), Jotai 3, Biome 2, installed with pnpm; compiled into `web/dist` and embedded with `go:embed`
- Multi-stage Docker build; no Node.js required at runtime
- All build targets live in `package.json` (`pnpm <script>`); there is no Makefile

## Quick start (no real API key)

```bash
pnpm install
pnpm dev:install    # one-time: install air (Go live-reload)
pnpm dev            # watch + rebuild; app on 0.0.0.0:3111
```

In development the server always binds `0.0.0.0:3111`. Open http://localhost:3111.

`pnpm dev` starts the dev-only faux provider, waits until it is reachable, then starts `air`, which watches both `web/src` and the Go sources. After a change it debounces, rebuilds the frontend, rebuilds the binary, and restarts the server. The HTML entrypoint is served with `no-store` so a rebuild cannot leave a stale shell in the browser.

The deterministic local script (add todos, wait 10 seconds, complete todos, final answer) is served by `cmd/faux-llm`, a development-only process outside the main binary. Use `pnpm dev:faux` if you only need the provider.

## Connect a real model

Create `.env` (do not commit) with `DEMO_PASSWORD`, `LLM_API` (`openai-completions`, `openai-responses`, `anthropic-messages`), `LLM_BASE_URL`, `LLM_API_KEY`, and `LLM_MODEL`. Then:

```bash
pnpm build
./bin/pi-durable-assistant
```

With `DEMO_PASSWORD` set the server binds `0.0.0.0:3000`; without it, loopback only. Production requires a password.

## Self-host

```bash
cp .env.example .env      # set DEMO_PASSWORD and model configuration
pnpm docker               # docker compose build
docker compose up -d
docker compose ps
```

The container publishes only `127.0.0.1:3300` by default. Use an HTTPS reverse proxy and keep HTTP Basic Auth (`demo` user). SQLite lives in the named `/data` volume; never remove it as an upgrade step.

## HTTP API

`GET /api/info`, `GET|POST /api/conversations`, `GET /api/conversations/{id}/events` (SSE), `POST /api/conversations/{id}/{messages,fork,abort,archive,restore,remove}`, `GET /api/conversations/{id}/{management,export.md}`, `POST /api/conversations/{id}/{schedules,background}`, schedule/background/reminder management, `GET|POST /api/personal`, `GET /api/knowledge/{search,read}`, `POST /api/crash`. All endpoints use HTTP Basic Auth when a password is configured; writes require same-origin.

WeChat is intentionally not part of this release. All source, UI text, and documentation are English-only.

## Verification

```bash
pnpm check              # go test ./..., go build, web typecheck, Biome, Vite build
```

See `doc/REWRITE_PLAN.md`, `doc/PIG_SPIKE.md`, and `doc/DESIGN_QA.md`.

**License:** MIT. Not affiliated with Pi, PiG, or official WeChat teams.
