/** Types mirroring the Go JSON DTOs and the SSE `view` payload (simplifyView). */

export interface Attachment {
  kind: "image" | "text";
  name: string;
  mimeType?: string;
  size?: number;
  /** Only present on attachments received in a view: URL of the stored image. */
  url?: string;
  /** Index inside the user message; used to build the image URL. */
  index?: number;
  /** Base64 payload, only used when sending a message. */
  data?: string;
}

export interface ToolCallView {
  id: string | number;
  name: string;
  args?: unknown;
}

export interface UserEntry {
  id: number;
  kind: "user";
  text: string;
  attachments?: Attachment[];
}

export interface AssistantEntry {
  id: number;
  kind: "assistant";
  text: string;
  toolCalls?: ToolCallView[];
  error?: string;
}

export interface ToolEntry {
  id: number;
  kind: "tool";
  text: string;
  toolCallId?: string | number | null;
  name?: string | null;
  isError?: boolean;
}

export type ViewEntry = UserEntry | AssistantEntry | ToolEntry;

export interface LiveToolSlot {
  name?: string;
  callId?: string | number;
  status?: string;
  output?: string;
  args?: unknown;
  details?: unknown;
}

export interface LiveContentPart {
  type: string;
  text?: string;
  thinking?: string;
  name?: string;
  arguments?: unknown;
}

export interface LiveState {
  generation?: {
    message?: { content?: LiveContentPart[] | string };
    text?: string;
  } | null;
  text?: string;
  stream?: string;
  tools?: LiveToolSlot[];
  [key: string]: unknown;
}

export interface UsagePart {
  input?: number;
  output?: number;
  inputTokens?: number;
  outputTokens?: number;
  totalTokens?: number;
}

export interface Usage extends UsagePart {
  models?: Record<string, UsagePart>;
}

export interface TodoItem {
  text: string;
  done: boolean;
}

export interface MemoItem {
  text: string;
}

export interface TaskView {
  id: string;
  kind: string;
  status: string;
  phase: string | null;
  owner: string | null;
  background: boolean;
  conversationId: number;
  conversations?: unknown[];
}

export interface ReminderView {
  id: string | number;
  text: string;
  at: number;
  status: string;
}

export interface SubagentView {
  conversationId: number;
  task: string;
  status: string;
}

export interface ViewPayload {
  conversationId: number;
  entries: ViewEntry[];
  live: LiveState;
  busy: boolean;
  inbox: number;
  notes: {
    todos: TodoItem[];
    memos: Array<string | MemoItem>;
  };
  usage: Usage | null;
  tasks: TaskView[];
  reminders: ReminderView[];
  subagents: SubagentView[];
  parentId: number | null;
}

export interface Conversation {
  id: number;
  title: string;
  createdAt: string | number;
  archivedAt?: string | null;
  forkOf?: number | null;
  forkAtEntry?: number | null;
}

export interface ServerInfo {
  model: string;
  api: string;
  pid: number;
  startedAt: string;
  boots: number;
}

export interface ScheduleItem {
  id: string;
  text: string;
  nextAt: number | null;
  everyMinutes: number;
  runs: number;
  lastRunAt: number | null;
  status: string;
  revision: number;
  createdAt: number;
  jobId?: string | null;
}

export interface BackgroundItem {
  id: string;
  text: string;
  status: string;
  conversationId: number | null;
  createdAt: number;
  result?: string | null;
}

export interface ReminderItem {
  id: string | number;
  text: string;
  at: number;
  status: string;
}

export interface Management {
  schedules: ScheduleItem[];
  background: BackgroundItem[];
  reminders: ReminderItem[];
}

export interface MemoryItem {
  id: string;
  text: string;
  status: "pending" | "approved" | "rejected" | string;
  source: "user" | "agent" | string;
  createdAt: number;
  decidedAt: number | null;
}

export interface SkillItem {
  id: string;
  name: string;
  description: string;
  body: string;
  status: "pending" | "approved" | "rejected" | string;
  source: "user" | "agent" | string;
  createdAt: number;
  decidedAt: number | null;
}

export interface KnowledgeStatus {
  enabled: boolean;
  reason?: string;
  root?: string;
  allow: string[];
}

/** One entry of `GET /api/tools`: a tool the agent can call. */
export interface ToolInfo {
  name: string;
  description: string;
  /** Name of the installed extension that registered the tool. */
  extension: string;
  /** JSON Schema of the tool arguments. */
  parameters?: Record<string, unknown>;
}

export interface ToolsData {
  tools: ToolInfo[];
  extensions: string[];
}

export interface PersonalData {
  memory: MemoryItem[];
  skills: SkillItem[];
  knowledge: KnowledgeStatus;
}

export interface KnowledgeHit {
  path: string;
  status: string;
  revision: string;
  line: number;
  endLine: number;
  excerpt: string;
  score: number;
}

export interface KnowledgeSearchResult {
  query: string;
  includeHistorical: boolean;
  hits: KnowledgeHit[];
}

export interface KnowledgePage {
  path: string;
  status: string;
  revision: string;
  startLine: number;
  endLine: number;
  text: string;
  truncated: boolean;
}

export interface OutgoingAttachment {
  name: string;
  mimeType: string;
  data: string;
}

export interface MessagePayload {
  text: string;
  requestId: string;
  mode: "steer" | "followUp";
  attachments: OutgoingAttachment[];
}

export type SseStatus = "connecting" | "open" | "reconnecting" | "closed";

/** Model configuration stored in SQLite. The API key is never returned by the server. */
export interface LLMSettings {
  api: string;
  baseURL: string;
  apiKeySet: boolean;
  model: string;
  contextWindow: number;
  maxTokens: number;
  updatedAt: string;
}

/** Fields accepted by the settings endpoints; omitting apiKey keeps the stored key. */
export interface LLMSettingsInput {
  api: string;
  baseURL: string;
  apiKey?: string;
  model: string;
  contextWindow: number;
  maxTokens: number;
}

/** Fields needed to discover models; no model or token limits required. */
export interface LLMModelsInput {
  api: string;
  baseURL: string;
  apiKey?: string;
}

export interface LLMTestResult {
  ok: boolean;
  models: string[];
  latencyMs: number;
}
