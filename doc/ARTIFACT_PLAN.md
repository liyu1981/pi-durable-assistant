# Artifact subsystem plan

Status: implemented (Go backend + REST API; agent tools + HITL confirmations).

## 1. Concept

The agent gets a durable, file-oriented subsystem it can use as workspace storage:

1. The agent asks for a new artifact (or the flow allocates one) and receives an artifact ID.
2. The agent creates files inside that artifact's folder through tools.
3. The system manages folders/paths; the agent addresses files by artifact ID (+ path) to read or update them.

## 2. Data model

- `Artifact`: ID, conversation scope, created/updated timestamps, pinned flag, size in bytes.
- `ArtifactFile`: artifact ID + path (+ content, size, updated timestamp); nested paths supported, root folder belongs to the artifact.

Cleanup granularity is artifact-level (LRU eviction of whole artifacts).

## 3. Agent tools

- `artifact_create` / new-artifact allocation → returns fresh artifact ID.
- `artifact_write` (artifactId, path, content) → create or update file; recomputes artifact size/updated-at.
- `artifact_read` (artifactId, path) → read file content.
- `artifact_list` → list artifacts/files visible to the conversation.
- `artifact_pin` / `artifact_unpin` (artifactId) → pin/unpin. Agent-initiated calls require human-in-the-loop confirmation (see §5).

## 4. Cleanup policy

- Global configurable total size limit (`ARTIFACT_MAX_TOTAL_BYTES` via env / `pida.env` / settings UI); decided default **4GB**.
- LRU: per-artifact last-used tracking (read/write/access). When unpinned artifacts exceed the limit, evict least-recently-used unpinned artifacts until under the cap.
- Pinned artifacts are exempt from cleanup and excluded from the size accounting.

## 5. Pin/unpin + human-in-the-loop

- REST: artifact CRUD + file read/write, plus pin/unpin endpoints (`/api/artifacts/...`).
- Direct pin/unpin via API/UI is allowed (user-initiated).
- Agent-initiated pin/unpin goes through a pending human confirmation: the agent tool records a pending action (artifactId, action, context); the user confirms via API/UI; the system then applies the pin/unpin. Audit logged.

## 6. Integration notes (for the later implementation phase)

- Wire into serve/runtime and harness tool registry alongside existing tools.
- Storage layout: filesystem under `DATA_DIR` (e.g. `artifacts/<artifactID>/...`) plus metadata, or durable-store-backed — decision pending.
- Cleanup: background sweeper + lazy check on access/write.
- Tests: tool flows, LRU eviction, pinned exemption, size accounting, HITL gate.

## 6b. API surface

- `GET /api/artifacts` — list all artifacts.
- `POST /api/artifacts` — create a new artifact.
- `GET /api/artifacts/{id}` — get one artifact.
- `DELETE /api/artifacts/{id}` — delete an artifact.
- `PUT /api/artifacts/{id}/pin|unpin` — user-initiated pin/unpin.
- `GET /api/artifacts/{id}/files` — list artifact files.
- `GET /api/artifacts/{id}/files/{path...}` — read a file.
- `PUT /api/artifacts/{id}/files/{path...}` — create/update a file (`{"content":"..."}`).
- `GET /api/artifacts/confirmations` — list pin/unpin requests.
- `POST /api/artifacts/confirmations/{id}/{approve|reject}` — human decision (applies pin/unpin on approval).

## 7. Decisions made

- Artifact namespace: **global**.
- LRU unit: **artifact granularity** (evict whole artifacts, LRU by last use).
- Default global size limit: **4GB** (`ARTIFACT_MAX_TOTAL_BYTES=4GB`).
- Pending-confirmation store for the HITL gate: **dedicated store**.
