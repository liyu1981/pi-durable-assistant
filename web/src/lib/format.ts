import type { LiveState, LiveToolSlot, Usage, UsagePart } from "../api/types";

const pad = (value: number) => String(value).padStart(2, "0");

export function formatFileSize(bytes: number | null | undefined): string {
  if (bytes === null || bytes === undefined || Number.isNaN(bytes)) return "0 B";
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

export function formatTime(value: string | number | null | undefined): string {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return String(value);
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
}

export function formatListTime(value: string | number | null | undefined): string {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  const now = new Date();
  const hm = `${pad(date.getHours())}:${pad(date.getMinutes())}`;
  const startToday = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime();
  const startThat = new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime();
  const days = Math.round((startToday - startThat) / 86_400_000);
  if (days === 0) return `今天 ${hm}`;
  if (days === 1) return `昨天 ${hm}`;
  return `${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}

export function formatDuration(ms: number): string {
  if (ms <= 0) return "0 秒";
  const seconds = Math.floor(ms / 1000);
  const minutes = Math.floor(seconds / 60);
  return minutes > 0 ? `${minutes} 分 ${seconds % 60} 秒` : `${seconds} 秒`;
}

export function formatToolArgs(args: unknown): string {
  if (args === null || args === undefined) return "无参数";
  if (typeof args === "string") return args;
  if (typeof args !== "object") return String(args);
  const entries = Object.entries(args as Record<string, unknown>);
  if (entries.length === 0) return "无参数";
  return entries
    .map(
      ([key, value]) =>
        `${key}: ${typeof value === "object" ? JSON.stringify(value) : String(value)}`,
    )
    .join(" · ");
}

export function usageNumbers(usage: Usage | null): {
  input: number;
  output: number;
  total: number;
} {
  if (!usage) return { input: 0, output: 0, total: 0 };
  let source: UsagePart = usage;
  if (usage.models && Object.keys(usage.models).length > 0) {
    source = Object.values(usage.models).reduce<UsagePart>(
      (sum, part) => ({
        input: (sum.input ?? 0) + (part.input ?? 0),
        output: (sum.output ?? 0) + (part.output ?? 0),
      }),
      {},
    );
  }
  const input = source.input ?? source.inputTokens ?? 0;
  const output = source.output ?? source.outputTokens ?? 0;
  return { input, output, total: source.totalTokens ?? input + output };
}

export interface LiveInfo {
  text: string;
  thinking: string;
  runningTools: LiveToolSlot[];
}

/** Interpret the loosely-shaped `pi.live` document from the SSE view. */
export function extractLiveInfo(live: LiveState | null | undefined): LiveInfo {
  if (!live) return { text: "", thinking: "", runningTools: [] };
  let text = "";
  let thinking = "";
  const runningTools: LiveToolSlot[] = [];

  const message = live.generation?.message;
  if (message) {
    const parts = Array.isArray(message.content)
      ? message.content
      : typeof message.content === "string"
        ? [{ type: "text", text: message.content }]
        : [];
    for (const part of parts) {
      if (part.type === "text" && part.text) text += part.text;
      else if (part.type === "thinking" && part.thinking) thinking += part.thinking;
      else if (part.type === "toolCall") {
        runningTools.push({
          name: part.name || "工具调用",
          args: part.arguments,
          status: "running",
        });
      }
    }
  }
  if (!text && typeof live.text === "string") text = live.text;
  if (!text && typeof live.generation?.text === "string") text = live.generation.text;
  if (!text && typeof live.stream === "string") text = live.stream;
  if (Array.isArray(live.tools)) runningTools.push(...live.tools);

  return { text: text.trim(), thinking: thinking.trim(), runningTools };
}

export function newRequestId(): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    return crypto.randomUUID();
  }
  return `req-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
}

/** Stable, unique React keys for lists whose items have no server-side id. */
export function makeKeyFactory(): (base: string) => string {
  const counts = new Map<string, number>();
  return (base: string) => {
    const count = (counts.get(base) ?? 0) + 1;
    counts.set(base, count);
    return count === 1 ? base : `${base}-${count}`;
  };
}
