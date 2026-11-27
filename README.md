# Pi Durable Assistant (Go rewrite)

A self-hosted, single-user durable AI assistant: one static Go binary with an embedded React UI, backed by PiG's Durable runtime and SQLite. Features: crash recovery, resumable tasks, web chat over SSE, subagents, scheduled jobs, a task center, memory and skills, attachments, read-only note search, and optional webhook notifications.

> Experimental, single-user project. Not an official Pi or PiG product. Back up the SQLite volume and restrict access before exposing it on a VPS.

## Tech stack

- Go 1.26 / toolchain 1.27.4, `github.com/MichaelKinsy/PiG` v0.7.0 (`durable`, `durable/harness`, `ai`, pure-Go SQLite)
- React 19, TypeScript 7, Vite 8, Tailwind CSS 4, shadcn/ui with the Luma preset (`b2D0wqNxT`), Jotai 3, Biome 2, installed with pnpm; compiled into `web/dist` and embedded with `go:embed`
- Installable PWA with a web manifest, service worker and offline app shell; logo assets in `web/public/assets/` (64/192/512/1024 plus apple-touch)
- Configurable HTTP, HTTPS, or both; multi-stage Docker build, no Node.js at runtime
- All build targets live in `package.json` (`pnpm <script>`); there is no Makefile

## Quick start (no real API key)

```bash
pnpm install
pnpm dev:install    # one-time: install air (Go live-reload)
pnpm dev            # watch + rebuild; app on https://localhost:3111
```

In development the server binds `0.0.0.0:3111` and serves **HTTPS only** with self-signed certs in the gitignored `certs/` folder. Generate them once with:

```bash
mkdir -p certs
mkcert -cert-file certs/localhost.pem -key-file certs/localhost-key.pem localhost 127.0.0.1
```

Open https://localhost:3111 (your browser will warn unless `mkcert -install` was run).

`pnpm dev` starts `air`, which watches both `web/src` and the Go sources. After a change it debounces, rebuilds the frontend, rebuilds the binary, and restarts the server. The HTML entrypoint is served with `no-store` so a rebuild cannot leave a stale shell in the browser.

`pnpm dev:demo` additionally starts the dev-only faux provider (`cmd/faux-llm`), waits until it is reachable, and points the dev server at it, so the full deterministic demo works without a real API key. Use `pnpm dev:faux` if you only need the provider.

## CLI

The binary is a small command-line tool. Install it where the service unit expects it with `CGO_ENABLED=0 go build -o ~/.local/bin/pida ./cmd/pida`, or use `dist/pida` from `pnpm build`:

```bash
pida                      # print usage
pida --version            # print the version
pida serve                # start the server
pida config               # write an example pida.env in the current directory
pida config --update      # upgrade pida.env, preserving configured values
pida service-install      # install a systemd user service
pida service-uninstall    # remove it
```

`pida serve` reads `~/.pida/pida.env` when it exists and stores state in `~/.pida`. Precedence is command-line flags, then the process environment, then the config file, then built-in defaults. `pida serve --help` lists the flags: `--port`, `--data-dir`, `--http-mode`, `--https-port`, `--cert-file`, `--key-file`, `--config`, `--password`, `--dev`, and `--production`.

`pida config` writes a commented example covering every supported key. Re-run `pida config --update` after an upgrade to add new keys while keeping values you already set; keys this version does not document are kept at the end of the file.

Artifacts are stored under `DATA_DIR/artifacts`; the agent creates, reads, updates and moves files through the artifact tools, and users can browse, pin and preview them in the Files panel. Artifact links in chat (`[file](artifact://<id>/<path>)`, `![image](artifact://...)`, or a bare link) open a self-hosted file viewer with a full-screen mode. A global LRU limit (`ARTIFACT_MAX_TOTAL_BYTES`, default 4GB, accepts KB/MB/GB) evicts unpinned artifacts when the limit is exceeded; pinned artifacts are exempt and excluded from the size accounting. Agent-initiated pin/unpin requests go through a pending confirmation store and require human approval.

`pida service-install` writes `~/.config/systemd/user/pida.service`, running `~/.local/bin/pida serve` with `DATA_DIR=~/.pida`, then prints the `systemctl --user` commands to enable it. Use `--print` to inspect the unit, `--force` to overwrite an existing one, and `--binary` / `--data-dir` to override the defaults.

## Connect a real model

Build and start the server, then open **System settings** in the app header (or the tools menu on small screens) and choose the **Models** tab:

```bash
pnpm build
./dist/pida serve
```

Pick a protocol (`openai-completions`, `openai-responses`, `anthropic-messages`), enter the base URL, model and optional API key, then use **Test connection** before **Save settings**. Changes are stored in the `app_llm_config` table of `DATA_DIR/agent.sqlite` and applied to the running server immediately; the stored key is never returned to the browser.

For a brand-new database only, the legacy `LLM_API`, `LLM_BASE_URL`, `LLM_API_KEY`, `LLM_MODEL`, `LLM_CONTEXT_WINDOW` and `LLM_MAX_TOKENS` variables seed the first row (handy for `pnpm dev:demo`). Once the row exists, SQLite is authoritative and changing `.env` has no effect until that row is reset.

With `DEMO_PASSWORD` set the server binds `0.0.0.0:3000`; without it, loopback only. Production requires a password.

## Self-host

```bash
cp .env.example .env      # set DEMO_PASSWORD and model configuration
pnpm docker               # docker compose build
docker compose up -d
docker compose ps
```

Set `HTTP_MODE=http|https|both` (default `http`); `both` redirects HTTP to HTTPS on `HTTPS_PORT` and expects `TLS_CERT_FILE` / `TLS_KEY_FILE`.

The container publishes only `127.0.0.1:3300` by default. Use an HTTPS reverse proxy and keep HTTP Basic Auth (`demo` user). SQLite lives in the named `/data` volume; never remove it as an upgrade step.

## HTTP API

`GET /api/info`, `GET|POST /api/conversations`, `GET /api/conversations/{id}/events` (SSE), `POST /api/conversations/{id}/{messages,fork,abort,archive,restore,remove}`, `GET /api/conversations/{id}/{management,export.md}`, `POST /api/conversations/{id}/{schedules,background}`, schedule/background/reminder management, `GET|POST /api/personal`, `GET /api/knowledge/{search,read}`, `GET|PUT /api/settings/llm`, `POST /api/settings/llm/test`, `POST /api/crash`. All endpoints use HTTP Basic Auth when a password is configured; writes require same-origin.

WeChat is intentionally not part of this release. All source, UI text, and documentation are English-only.

## Verification

```bash
pnpm check              # go test ./..., go build, web typecheck, Biome, Vite build
```

See `doc/ARCHITECTURE.md` for the agent loop, context and persistence model, plus `doc/REWRITE_PLAN.md`, `doc/PIG_SPIKE.md`, and `doc/DESIGN_QA.md`.

**License:** MIT. Not affiliated with Pi, PiG, or official WeChat teams.
