# Artifact subsystem plan

Status: implemented (Go backend + REST API; agent tools + HITL confirmations).

## 1. Concept

The agent gets a durable, file-oriented subsystem it can use as workspace storage:

1. The agent asks for a new artifact (or the flow allocates one) and receives an artifact ID.
2. The agent creates files inside that artifact's folder through tools.
3. The system manages folders/paths; the agent addresses files by artifact ID (+ path) to read or update them.

## 2. Data model

- `Artifact`: ID, global namespace, created/updated timestamps, pinned flag, size in bytes.
- `ArtifactFile`: artifact ID + path (+ content, size, updated timestamp); nested paths supported, root folder belongs to the artifact.

Cleanup granularity is artifact-level (LRU eviction of whole artifacts).

## 3. Agent tools

- `artifact_create` / new-artifact allocation → returns fresh artifact ID.
- `artifact_write` (artifactId, path, content) → create or update file; recomputes artifact size/updated-at.
- `artifact_read` (artifactId, path) → read file content.
- `artifact_list` → list artifacts/files in the global namespace.
- `artifact_move` → move or rename a file inside an artifact.
- `artifact_request_pin` / `artifact_request_unpin` (artifactId) → propose pin/unpin; require human-in-the-loop confirmation (see §5). User-initiated pin/unpin is via REST.

## 4. Cleanup policy

- Global configurable total size limit (`ARTIFACT_MAX_TOTAL_BYTES` via env / `pida.env`); decided default **4GB**.
- LRU: per-artifact last-used tracking (read/write/access). When unpinned artifacts exceed the limit, evict least-recently-used unpinned artifacts until under the cap.
- Pinned artifacts are exempt from cleanup and excluded from the size accounting.

## 5. Pin/unpin + human-in-the-loop

- REST: artifact CRUD + file read/write, plus pin/unpin endpoints (`/api/artifacts/...`).
- Direct pin/unpin via API/UI is allowed (user-initiated).
- Agent-initiated pin/unpin goes through a pending human confirmation: the agent tool records a pending action (artifactId, action, context); the user confirms via API/UI; the system then applies the pin/unpin. Audit logged.

## 6. Integration notes (implemented)

- Wire into serve/runtime and harness tool registry alongside existing tools.
- Storage layout: implemented as filesystem under `DATA_DIR/artifacts/<artifactID>/...` plus JSON metadata.
- Cleanup: lazy check on write and on boot; reads refresh LRU timestamps.
- Tests: store lifecycle, LRU eviction, pinned exemption, size accounting, HITL gate, REST routes.

## 6b. API surface

- `GET /api/artifacts` — list all artifacts.
- `POST /api/artifacts` — create a new artifact.
- `GET /api/artifacts/{id}` — get one artifact.
- `DELETE /api/artifacts/{id}` — delete an artifact.
- `PUT /api/artifacts/{id}/pin|unpin` — user-initiated pin/unpin.
- `GET /api/artifacts/{id}/files` — list artifact files.
- `GET /api/artifacts/{id}/files/{path...}` — read a file as JSON.
- `GET /api/artifacts/{id}/raw/{path...}` — read raw file bytes for preview.
- `POST /api/artifacts/{id}/move` — move/rename a file (`{"from","to"}`).
- `PUT /api/artifacts/{id}/files/{path...}` — create/update a file (`{"content":"..."}`).
- `GET /api/artifacts/confirmations` — list pin/unpin requests.
- `POST /api/artifacts/confirmations/{id}/{approve|reject}` — human decision (applies pin/unpin on approval).

## 7. Decisions made

- Artifact namespace: **global**.
- LRU unit: **artifact granularity** (evict whole artifacts, LRU by last use).
- Default global size limit: **4GB** (`ARTIFACT_MAX_TOTAL_BYTES=4GB`).
- Pending-confirmation store for the HITL gate: **dedicated store** (`DATA_DIR/artifact-confirmations.json`).

---

# Part 2: move tool and in-conversation file presentation

Status: implemented. Reference implementation for the viewer:
`~/suwu-dev/frontend` (`src/routes/FileViewerPage.tsx`, `vite.config.ts`, `@file-viewer/*` v3.3.3).

## 8. `artifact_move` (move a file)

### 8.1 Store
- `Store.MoveFile(ctx, artifactID, from, to string) (*Artifact, error)`: rename a file inside one
  artifact, creating destination parent directories, overwriting an existing destination (same as
  `WriteFile`), then recompute `SizeBytes`, bump `UpdatedAt`, run the LRU sweep, and return the
  artifact. File-level only in v1 (directory moves can follow).
- **Path safety (fix while here):** add a `safePath(artifactID, p)` used by `WriteFile`, `ReadFile`,
  `MoveFile`, and `ListFiles` that rejects empty/absolute paths and any path that cleans outside the
  artifact root (`..`). Today `contentPath` joins user input without this check.

### 8.2 Agent tool
- `artifact_move` (artifactId, from, to) → `ReplaySafe`; returns `moved <from> to <to> in artifact <id>`
  or a plain error string, matching the other artifact tools.
- Add an `artifacts` prompt section so the model knows the move tool and the reference convention
  (see 9.3).

### 8.3 REST
- `POST /api/artifacts/{id}/move` with `{"from":"a/b.md","to":"c/d.md"}` → `{"artifact":{...}}`.
- Tests: move success, overwrite, missing source, created destination directories, `..`/absolute
  rejection, and the REST route.

## 9. In-conversation artifact presentation

Goal: the agent-created files render as embeds in the thread for common formats, with click-to-focus
full screen. Reuse `@file-viewer` exactly as suwu does (same preset/customization), embedded into
the Go binary.

### 9.1 Frontend dependency and build integration
- `web/package.json`: add `@file-viewer/react`, `@file-viewer/preset-standard` (decision: **standard
  preset**, same as suwu), and `pele` for Mermaid (decision: **include**). Dev dep:
  `@file-viewer/vite-plugin`.
- `web/vite.config.ts`: add `fileViewerRenderers({ preset: 'standard', copyAssets: true })` and
  `registerFileViewerMermaidLoader(() => import('./lib/fileViewerPeleMermaid'))`, where the shim
  mirrors suwu's `src/lib/fileViewerPeleMermaid.ts` (Pele `render()` adapted to the Mermaid API and
  dark-theme CSS variables). The plugin copies renderer assets into Vite's `build.outDir` (our
  `web/dist`), so they are embedded by `web/embed.go` and served as-is — no extra Go changes to ship
  assets.
- **Accepted size cost:** suwu's `standard` assets are ~38 MB vs our current `web/dist` ~2 MB and
  binary ~37 MB; expect a binary approaching ~75 MB. This is a deliberate decision.
- `internal/httpapi/server.go` `contentTypeFor`: add `.wasm` (application/wasm), `.mjs`/`.js`
  (text/javascript), `.ttf`/`.woff`/`.woff2` fonts, `.webp`, `.txt`/`.bcmap`/`.pfb`
  (text/plain or application/octet-stream), `.map` (application/json). Extend `serveStatic` caching:
  treat the viewer asset directory (`assets/**`, `vendor/**`) as immutable (`max-age=31536000`).
- Service worker (`web/public/sw.js`): ensure it does not precache viewer assets and does not
  intercept their navigation (they load lazily from the same origin).

### 9.2 Backend raw file bytes
- Current `GET /api/artifacts/{id}/files/{path...}` returns JSON `{"content": "..."}`, which is wrong
  for binary/PDF and inflates payloads. Add:
  `GET /api/artifacts/{id}/raw/{path...}` → raw bytes with `Content-Type` by extension,
  `Content-Disposition: inline; filename="..."`, `Content-Length`, and `cache-control: no-store`.
- Same Basic Auth middleware; frontend fetches with credentials and hands the viewer an
  `URL.createObjectURL(blob)` (the viewer's `url` prop cannot send custom headers), exactly as suwu.
- Tests: content-type selection, 404 for missing file, path-traversal rejection.

### 9.3 Reference convention (Markdown)
- Decision: use `artifact://<artifactId>/<path>` Markdown links (the recommended convention).
  Only Markdown links/images are embeddings; a bare `artifact://...` URL is a plain link.
  - `[report.pdf](artifact://art_ab12/reports/report.pdf)` → inline **card** with preview.
  - `![chart](artifact://art_ab12/charts/chart.png)` → inline **image** embed.
  - A bare `artifact://art_ab12/reports/report.pdf` (or any other text) → ordinary **link** that
    opens the focused viewer, with **no card and no preview**.
- Parse the scheme in `web/src/components/Markdown.tsx` (`renderInline`/link render path): for `[]`
  render `<ArtifactEmbed>`, for `![]` render the inline image, for bare URLs keep an `<a>` whose
  click opens the focus dialog. Add the convention to the `artifacts` extension prompt section so
  the model emits it.

### 9.4 Frontend components
- `lib/useArtifactBlob.ts`: fetch raw bytes with credentials, create/revoke object URLs, cache by
  `artifactId/path`.
- `components/ArtifactEmbed.tsx` (inline card, rendered only for `[]` links): header (file name,
  truncated artifact id, size) plus Open/Download/Copy-link actions; body by kind:
  - image → thumbnail `<img>` (blob URL);
  - text/markdown/code → compact code/text preview (Mermaid blocks render through the Pele shim,
    including dark-mode CSS variables);
  - pdf/office/spreadsheet → lazily mount `FileViewer` in a bounded container (~360px), using
    `options = { preset: standardPreset, rendererMode: 'replace', theme, toolbar: { position: 'bottom-right' } }`.
  - `@file-viewer/react` is loaded with dynamic `import()` so heavy chunks load only on first embed.
- `![...]` renders just the inline `<img>` embed (no card chrome); bare `artifact://` URLs render a
  plain link whose click opens the focused viewer with no card/preview.
- `components/ArtifactFocusDialog.tsx` (full screen): opened from a card, image, or bare link;
  full viewport on mobile (`h-dvh w-screen`) and large centered on desktop; header with name/path +
  download/copy-link/close; `FileViewer` fills `min-h-0 flex-1`. Escape closes (Dialog primitive
  already handles it).
- Theme: `artifactViewerThemeAtom` (jotai + persistence) initialized from the app theme; wire the
  viewer's `theme-change` event back into the atom, like suwu's `fileViewerThemeAtom`. The same
  theme applies to the Pele Mermaid shim.
- State: `artifactViewerAtom` (`{artifactId, path} | null`) drives the single global focus dialog,
  matching the existing `drawerAtom` / `systemSettingsOpenAtom` pattern.

### 9.5 Artifact browser and pin/unpin UI (this phase)
- A Files panel/Drawer tab listing artifacts and their files (existing `GET /api/artifacts` and
  files routes), reusing `ArtifactEmbed` + focus dialog; add pin/unpin buttons and a pending
  confirmations list so the HITL gate is usable from the UI.

## 10. Decisions and implementation

Implemented with the standard preset, Pele Mermaid, Markdown-only embeds and the Files panel in this phase.

1. Renderer set: **standard preset** (PDF, office, spreadsheet, archive, email, text, image, media),
   accepting the ~38 MB of assets / larger binary.
2. Link convention: **`artifact://<artifactId>/<path>` Markdown links**.
3. Embedding trigger: **only `![]` (inline image) and `[]` (card) are embeds**; a bare `artifact://`
   URL is a plain link that opens the focused viewer with no card and no preview.
4. Mermaid: **include Pele** via `registerFileViewerMermaidLoader`, with dark-mode CSS variables.
5. Artifact browser + pin/unpin UI: **include in this phase**.
