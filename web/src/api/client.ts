import type {
  Artifact,
  ArtifactConfirmation,
  ArtifactConfirmations,
  ArtifactFiles,
  ArtifactList,
  KnowledgeFileContent,
  KnowledgeFileInfo,
  KnowledgeFileList,
  KnowledgeVault,
  KnowledgeVaultCreated,
  KnowledgeVaultDelete,
  KnowledgeVaultList,
  LLMModelsInput,
  LLMProfile,
  LLMProfileInput,
  LLMProfiles,
  LLMSettings,
  LLMSettingsInput,
  LLMTestResult,
  MessagePayload,
  SseStatus,
  SystemPrompt,
  ViewPayload,
} from "./types";

export class ApiError extends Error {
  readonly status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

/** Same-origin JSON request wrapper. The browser supplies cached Basic Auth credentials. */
export async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers);
  if (init?.body !== undefined && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  let response: Response;
  try {
    response = await fetch(path, { ...init, headers });
  } catch (error) {
    throw new ApiError(0, error instanceof Error ? error.message : "Network request failed");
  }
  if (!response.ok) {
    let message = `HTTP ${response.status}`;
    try {
      const body = (await response.json()) as { error?: string };
      if (typeof body.error === "string" && body.error) message = body.error;
    } catch {
      // Keep the HTTP status message when the error body is not JSON.
    }
    throw new ApiError(response.status, message);
  }
  if (response.status === 204) return undefined as T;
  return (await response.json()) as T;
}

export function get<T>(path: string): Promise<T> {
  return request<T>(path);
}

export function post<T>(path: string, body?: unknown): Promise<T> {
  return request<T>(path, {
    method: "POST",
    body: body === undefined ? undefined : JSON.stringify(body),
  });
}

export function put<T>(path: string, body: unknown): Promise<T> {
  return request<T>(path, { method: "PUT", body: JSON.stringify(body) });
}

export function getLLMProfiles(): Promise<LLMProfiles> {
  return get<LLMProfiles>("/api/settings/llm/profiles");
}

export function createLLMProfile(value: LLMProfileInput): Promise<{ profile: LLMProfile }> {
  return post<{ profile: LLMProfile }>("/api/settings/llm/profiles", value);
}

export function updateLLMProfile(
  id: number,
  value: LLMProfileInput,
): Promise<{ profile: LLMProfile }> {
  return put<{ profile: LLMProfile }>(`/api/settings/llm/profiles/${id}`, value);
}

export function deleteLLMProfile(id: number): Promise<{ ok: boolean }> {
  return request<{ ok: boolean }>(`/api/settings/llm/profiles/${id}`, { method: "DELETE" });
}

export function activateLLMProfile(id: number): Promise<{ profile: LLMProfile }> {
  return post<{ profile: LLMProfile }>(`/api/settings/llm/profiles/${id}/activate`);
}

export function getLLMSettings(): Promise<LLMSettings> {
  return get<LLMSettings>("/api/settings/llm");
}

/** Save model settings; the change takes effect without a server restart. */
export function saveLLMSettings(value: LLMSettingsInput): Promise<LLMSettings> {
  return put<LLMSettings>("/api/settings/llm", value);
}

/** Discover models from a base URL and optional API key, without saving. */
export function fetchLLMModels(value: LLMModelsInput): Promise<LLMTestResult> {
  return post<LLMTestResult>("/api/settings/llm/models", value);
}

/** Test a candidate configuration (including an unsaved API key). */
export function testLLMSettings(value: LLMSettingsInput): Promise<LLMTestResult> {
  return post<LLMTestResult>("/api/settings/llm/test", value);
}

export function sendMessage(
  conversationId: number,
  payload: MessagePayload,
): Promise<{ submissionId: string }> {
  return post(`/api/conversations/${conversationId}/messages`, payload);
}

export function imageUrl(conversationId: number, entryId: number, index: number): string {
  return `/api/conversations/${conversationId}/images/${entryId}/${index}`;
}

export function exportUrl(conversationId: number): string {
  return `/api/conversations/${conversationId}/export.md`;
}

export interface ViewSubscription {
  close(): void;
}

/**
 * Subscribe to the SSE `view` stream for one conversation.
 * On failure the connection is closed and retried with exponential backoff
 * (1s ... 30s); `close` cancels all pending work.
 */
export function subscribeView(
  conversationId: number,
  onView: (view: ViewPayload) => void,
  onStatus: (status: SseStatus) => void = () => {},
): ViewSubscription {
  let source: EventSource | null = null;
  let closed = false;
  let attempts = 0;
  let timer: ReturnType<typeof setTimeout> | null = null;

  const connect = () => {
    if (closed) return;
    onStatus("connecting");
    const es = new EventSource(`/api/conversations/${conversationId}/events`);
    source = es;

    es.onopen = () => {
      attempts = 0;
      onStatus("open");
    };
    es.onerror = () => {
      if (closed || source !== es) return;
      es.close();
      source = null;
      onStatus("reconnecting");
      const delay = Math.min(30_000, 1000 * 2 ** attempts);
      attempts += 1;
      timer = setTimeout(connect, delay);
    };
    es.addEventListener("view", (event: Event) => {
      try {
        const payload = JSON.parse((event as MessageEvent<string>).data) as ViewPayload;
        onView(payload);
      } catch (error) {
        console.error("Failed to parse SSE view payload", error);
      }
    });
  };

  connect();

  return {
    close() {
      if (closed) return;
      closed = true;
      if (timer !== null) clearTimeout(timer);
      if (source) {
        source.close();
        source = null;
      }
      onStatus("closed");
    },
  };
}

/** Artifact subsystem: folders of agent-managed files. */
/** Current system prompt sections for a conversation (or the root agent). */
export function getSystemPrompt(conversationId?: number | null): Promise<SystemPrompt> {
  const query = conversationId ? `?conversation=${encodeURIComponent(conversationId)}` : "";
  return get<SystemPrompt>(`/api/system-prompt${query}`);
}

export function getArtifacts(): Promise<ArtifactList> {
  return get<ArtifactList>("/api/artifacts");
}

export function createArtifact(): Promise<{ artifact: Artifact }> {
  return post<{ artifact: Artifact }>("/api/artifacts");
}

export function deleteArtifact(id: string): Promise<{ ok: boolean }> {
  return request<{ ok: boolean }>(`/api/artifacts/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export function pinArtifact(id: string): Promise<{ artifact: Artifact }> {
  return put<{ artifact: Artifact }>(`/api/artifacts/${encodeURIComponent(id)}/pin`, {});
}

export function unpinArtifact(id: string): Promise<{ artifact: Artifact }> {
  return put<{ artifact: Artifact }>(`/api/artifacts/${encodeURIComponent(id)}/unpin`, {});
}

export function listArtifactFiles(id: string): Promise<ArtifactFiles> {
  return get<ArtifactFiles>(`/api/artifacts/${encodeURIComponent(id)}/files`);
}

export function readArtifactFile(id: string, filePath: string): Promise<{ content: string }> {
  return get<{ content: string }>(artifactJSONPath(id, filePath));
}

export function writeArtifactFile(
  id: string,
  filePath: string,
  content: string,
): Promise<{ artifact: Artifact }> {
  return put<{ artifact: Artifact }>(artifactJSONPath(id, filePath), { content });
}

export function moveArtifactFile(
  id: string,
  from: string,
  to: string,
): Promise<{ artifact: Artifact }> {
  return post<{ artifact: Artifact }>(`/api/artifacts/${encodeURIComponent(id)}/move`, {
    from,
    to,
  });
}

export function getArtifactConfirmations(): Promise<ArtifactConfirmations> {
  return get<ArtifactConfirmations>("/api/artifacts/confirmations");
}

export function decideArtifactConfirmation(
  id: string,
  decision: "approve" | "reject",
): Promise<{ confirmation: ArtifactConfirmation }> {
  return post<{ confirmation: ArtifactConfirmation }>(
    `/api/artifacts/confirmations/${encodeURIComponent(id)}/${decision}`,
  );
}

export function artifactJSONPath(id: string, filePath: string): string {
  return `/api/artifacts/${encodeURIComponent(id)}/files/${encodeFilePath(filePath)}`;
}

export function artifactRawUrl(id: string, filePath: string): string {
  return `/api/artifacts/${encodeURIComponent(id)}/raw/${encodeFilePath(filePath)}`;
}

function encodeFilePath(filePath: string): string {
  return filePath.split("/").map(encodeURIComponent).join("/");
}

/** Knowledge subsystem: vault management. */

/** List all knowledge vaults. */
export function listKnowledgeVaults(): Promise<KnowledgeVaultList> {
  return get<KnowledgeVaultList>("/api/knowledge/vaults");
}

/** Create a new knowledge vault with a name and optional description. */
export function createKnowledgeVault(
  name: string,
  description?: string,
): Promise<KnowledgeVaultCreated> {
  return post<KnowledgeVaultCreated>("/api/knowledge/vaults", { name, description });
}

/** Delete a knowledge vault by ID. */
export function deleteKnowledgeVault(id: string): Promise<KnowledgeVaultDelete> {
  return request<KnowledgeVaultDelete>(`/api/knowledge/vaults/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
}

/** List files in a knowledge vault. */
export function listKnowledgeFiles(vaultId: string): Promise<KnowledgeFileList> {
  return get<KnowledgeFileList>(`/api/knowledge/vaults/${encodeURIComponent(vaultId)}/files`);
}

/** Read a knowledge file by vault ID and file path. */
export function readKnowledgeFile(
  vaultId: string,
  filePath: string,
): Promise<KnowledgeFileContent> {
  return get<KnowledgeFileContent>(
    `/api/knowledge/vaults/${encodeURIComponent(vaultId)}/files/${encodeFilePath(filePath)}`,
  );
}

/** Write/create a knowledge file in a vault. */
export function writeKnowledgeFile(
  vaultId: string,
  filePath: string,
  content: string,
): Promise<KnowledgeVault> {
  return put<KnowledgeVault>(
    `/api/knowledge/vaults/${encodeURIComponent(vaultId)}/files/${encodeFilePath(filePath)}`,
    { content },
  );
}

/** Rename a conversation; user-set titles are not overwritten automatically. */
export function renameConversation(id: number, title: string): Promise<{ title: string }> {
  return put<{ title: string }>(`/api/conversations/${id}/title`, { title });
}
