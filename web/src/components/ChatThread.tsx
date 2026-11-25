import { useAtomValue, useSetAtom } from "jotai/react";
import { useEffect, useRef } from "react";
import { imageUrl } from "../api/client";
import type { LiveToolSlot, ToolCallView, ToolEntry, ViewEntry, ViewPayload } from "../api/types";
import { busyAtom, currentConversationIdAtom, draftAtom, sseStatusAtom, viewAtom } from "../atoms";
import { extractLiveInfo, formatFileSize, formatToolArgs, usageNumbers } from "../lib/format";
import { Markdown } from "./Markdown";
import { Badge, EmptyState, LoadingState } from "./ui";

interface ChatThreadProps {
  onFork: (entryId: number) => void;
  onSelectConversation: (id: number) => void;
}

const EXAMPLES = [
  "Summarize the homepage at https://pi.dev, then wait 20 seconds before the recap",
  "Remind me to stand up and move in 2 minutes",
  "Send a subagent to scan https://earendil.com for new articles and report back",
  "Remember: I prefer concise answers with bullet points",
];

const SYSTEM_PREFIXES = [
  { match: "[Reminder]", label: "Reminder" },
  { match: "[Scheduled task]", label: "Scheduled task" },
  { match: "[Background task result]", label: "Background task result" },
] as const;

const TASK_KINDS: Record<string, string> = {
  "pi.generation": "Model",
  "pi.tool": "Tool",
  "app.reminder": "Reminder",
  "pi.compaction": "Compaction",
};

const TASK_STATUSES: Record<string, string> = {
  pending: "Queued",
  running: "Running",
  waiting: "Waiting",
  completing: "Finalizing",
};

function taskTone(status: string): "accent" | "warning" | "danger" | "success" {
  if (status === "running") return "accent";
  if (status === "waiting" || status === "pending" || status === "completing") return "warning";
  if (status === "failed" || status === "error") return "danger";
  return "success";
}

function SystemNotice({
  label,
  tone = "warning",
  children,
}: {
  label: string;
  tone?: "warning" | "danger" | "accent";
  children: string;
}) {
  const edge =
    tone === "danger"
      ? "border-l-danger"
      : tone === "accent"
        ? "border-l-secondary"
        : "border-l-warning";
  return (
    <div className={`border-l-4 py-1 pl-4 ${edge}`}>
      <p className="label text-primary/70">{label}</p>
      <p className="mt-1 break-words text-sm">{children}</p>
    </div>
  );
}

function ToolCard({
  name,
  args,
  result,
  running,
  childId,
  onSelectConversation,
}: {
  name: string;
  args?: unknown;
  result?: ToolEntry;
  running?: LiveToolSlot;
  childId?: number;
  onSelectConversation: (id: number) => void;
}) {
  const isError = Boolean(result?.isError);
  const text = result?.text ?? "";
  const preview = text.length > 240 ? `${text.slice(0, 240)}...` : text;
  const runningOutput = running?.output?.trim().split(/\r?\n/).filter(Boolean).at(-1) ?? "";
  const displayName = name === "delegate" ? "Subagent" : name || "Tool call";
  return (
    <div
      className={`rounded-sm border px-3 py-2 ${isError ? "border-danger" : "border-primary/20"}`}
    >
      <div className="flex items-center justify-between gap-2">
        <span className="label font-bold">{displayName}</span>
        <Badge tone={isError ? "danger" : result ? "success" : "warning"}>
          {isError ? "Failed" : result ? "Done" : "Running"}
        </Badge>
      </div>
      {args !== undefined ? (
        <p className="mt-2 text-xs text-primary/70">Arguments: {formatToolArgs(args)}</p>
      ) : null}
      {childId !== undefined ? (
        <ButtonLink onClick={() => onSelectConversation(childId)}>
          Open subagent conversation
        </ButtonLink>
      ) : null}
      <p className="mt-2 whitespace-pre-wrap break-words text-sm">
        {result ? preview || (isError ? "Tool failed" : "Done") : runningOutput || "Running..."}
      </p>
    </div>
  );
}

function ButtonLink({ onClick, children }: { onClick: () => void; children: string }) {
  return (
    <button type="button" className="btn btn-quiet mt-2" onClick={onClick}>
      {children} <span aria-hidden="true">-&gt;</span>
    </button>
  );
}

function entryHasContent(view: ViewPayload | null): boolean {
  if (!view) return false;
  return (view.entries ?? []).some((entry: ViewEntry) => {
    if (entry.kind === "assistant")
      return (
        Boolean(entry.text?.trim()) || Boolean(entry.error) || (entry.toolCalls?.length ?? 0) > 0
      );
    if (entry.kind === "user")
      return Boolean(entry.text?.trim()) || (entry.attachments?.length ?? 0) > 0;
    return true;
  });
}

export function ChatThread({ onFork, onSelectConversation }: ChatThreadProps) {
  const view = useAtomValue(viewAtom);
  const busy = useAtomValue(busyAtom);
  const sseStatus = useAtomValue(sseStatusAtom);
  const currentId = useAtomValue(currentConversationIdAtom);
  const setDraft = useSetAtom(draftAtom);
  const scrollRef = useRef<HTMLElement | null>(null);
  const nearBottom = useRef(true);

  useEffect(() => {
    const element = scrollRef.current;
    if (view && element && nearBottom.current) element.scrollTop = element.scrollHeight;
  }, [view]);

  const onScroll = () => {
    const element = scrollRef.current;
    if (element)
      nearBottom.current = element.scrollHeight - element.scrollTop - element.clientHeight < 80;
  };

  const entries = view?.entries ?? [];
  const results = new Map<string, ToolEntry>();
  const liveTools = new Map<string, LiveToolSlot>();
  for (const entry of entries) {
    if (entry.kind === "tool" && entry.toolCallId !== null && entry.toolCallId !== undefined) {
      results.set(String(entry.toolCallId), entry);
    }
  }
  const live = extractLiveInfo(view?.live);
  for (const slot of live.runningTools) {
    if (slot.callId !== undefined) liveTools.set(String(slot.callId), slot);
  }
  const renderedCalls = new Set<string>();
  const hiddenErrors = new Set<number>();
  for (let i = 1; i < entries.length; i += 1) {
    const previous = entries[i - 1];
    const current = entries[i];
    if (
      previous.kind === "assistant" &&
      current.kind === "assistant" &&
      previous.error &&
      previous.error === current.error
    ) {
      hiddenErrors.add(i);
    }
  }

  const rendered = entries.map((entry, index) => {
    const system =
      entry.kind === "user"
        ? SYSTEM_PREFIXES.find((item) => entry.text.startsWith(item.match))
        : undefined;
    if (system) {
      return (
        <SystemNotice key={entry.id} label={system.label}>
          {entry.text.slice(system.match.length).trimStart()}
        </SystemNotice>
      );
    }

    if (entry.kind === "user") {
      return (
        <div key={entry.id} className="ml-auto flex w-full max-w-[85%] flex-col items-end gap-2">
          {entry.attachments && entry.attachments.length > 0 ? (
            <div className="flex flex-wrap justify-end gap-2">
              {entry.attachments.map((attachment) => {
                const url =
                  attachment.url ??
                  (currentId !== null && attachment.index !== undefined
                    ? imageUrl(currentId, entry.id, attachment.index)
                    : undefined);
                if (attachment.kind === "image" && url) {
                  return (
                    <a key={url} href={url} target="_blank" rel="noopener noreferrer">
                      <img
                        src={url}
                        alt={attachment.name}
                        loading="lazy"
                        className="max-h-44 max-w-60 rounded-sm border border-primary/20 object-contain"
                      />
                    </a>
                  );
                }
                return (
                  <span
                    key={attachment.name}
                    className="label inline-flex items-center gap-2 rounded-sm border border-primary/20 px-2 py-1"
                    title={attachment.name}
                  >
                    <span className="text-secondary">File</span>
                    {attachment.name} - {formatFileSize(attachment.size)}
                  </span>
                );
              })}
            </div>
          ) : null}
          {entry.text.trim() ? (
            <div className="whitespace-pre-wrap break-words rounded-md bg-primary px-4 py-3 text-surface">
              {entry.text}
            </div>
          ) : null}
          <button
            type="button"
            className="btn btn-quiet"
            onClick={() => onFork(entry.id)}
            title="Fork a new conversation from this message"
          >
            Fork here
          </button>
        </div>
      );
    }

    if (entry.kind === "assistant") {
      if (entry.error) {
        if (hiddenErrors.has(index)) return null;
        return (
          <SystemNotice key={entry.id} label="Model call failed" tone="danger">
            {entry.error}
          </SystemNotice>
        );
      }
      const calls: ToolCallView[] = entry.toolCalls ?? [];
      return (
        <div key={entry.id} className="flex flex-col gap-3">
          {entry.text.trim() ? (
            <div className="prose-min border-l-2 border-secondary pl-4">
              <Markdown text={entry.text} />
            </div>
          ) : null}
          {calls.map((call) => {
            const id = String(call.id);
            renderedCalls.add(id);
            const task =
              typeof call.args === "object" && call.args !== null
                ? String((call.args as Record<string, unknown>).task ?? "")
                : "";
            const match =
              view?.subagents.find((agent) => agent.task === task) ?? view?.subagents.at(-1);
            return (
              <ToolCard
                key={id}
                name={call.name}
                args={call.args}
                result={results.get(id)}
                running={liveTools.get(id)}
                childId={call.name === "delegate" ? match?.conversationId : undefined}
                onSelectConversation={onSelectConversation}
              />
            );
          })}
        </div>
      );
    }

    if (renderedCalls.has(String(entry.toolCallId))) return null;
    return (
      <ToolCard
        key={entry.id}
        name={entry.name ?? "Tool call"}
        result={entry}
        running={liveTools.get(String(entry.toolCallId))}
        onSelectConversation={onSelectConversation}
      />
    );
  });

  const unrendered = live.runningTools.filter(
    (slot) => slot.callId === undefined || !renderedCalls.has(String(slot.callId)),
  );
  const showLive =
    busy &&
    (Boolean(live.text) ||
      Boolean(live.thinking) ||
      unrendered.length > 0 ||
      Boolean(view?.live?.generation));
  const { input, output, total } = usageNumbers(view?.usage ?? null);
  const tasks = view?.tasks ?? [];
  const showLoading = view === null && sseStatus !== "closed";

  return (
    <section
      className="scroll-area flex-1 px-6 py-8"
      ref={scrollRef}
      onScroll={onScroll}
      aria-live="polite"
      aria-label="Conversation thread"
    >
      <div className="mx-auto flex w-full max-w-3xl flex-col gap-8">
        {showLoading ? (
          <LoadingState label="Connecting to conversation" />
        ) : !entryHasContent(view) && !busy ? (
          <EmptyState
            title="Start a durable conversation"
            hint="Sessions survive restarts and can schedule reminders or delegate subagents. Select a prompt to fill the composer:"
          >
            <div className="grid gap-2 text-left sm:grid-cols-2">
              {EXAMPLES.map((example) => (
                <button
                  type="button"
                  key={example}
                  className="rounded-sm border border-primary/20 px-3 py-3 text-left text-sm hover:border-secondary"
                  onClick={() => setDraft(example)}
                >
                  {example}
                  <span className="label mt-2 block text-primary/70">Use this prompt</span>
                </button>
              ))}
            </div>
          </EmptyState>
        ) : (
          rendered
        )}

        {showLive ? (
          <div className="flex flex-col gap-3 border-l-2 border-secondary pl-4">
            {live.text ? (
              <div className="prose-min">
                <Markdown text={live.text} />
                <span className="cursor" aria-hidden="true" />
              </div>
            ) : live.thinking ? (
              <p className="text-sm text-primary/70">Thinking: {live.thinking}</p>
            ) : view?.live?.generation ? (
              <p className="text-sm text-primary/70">
                Thinking
                <span className="cursor" aria-hidden="true" />
              </p>
            ) : null}
            {unrendered.map((slot) => (
              <ToolCard
                key={`live-${slot.callId ?? slot.name ?? "tool"}`}
                name={slot.name ?? "Tool call"}
                args={slot.args}
                running={slot}
                onSelectConversation={onSelectConversation}
              />
            ))}
          </div>
        ) : null}

        {tasks.length > 0 ? (
          <div className="flex flex-wrap items-center gap-2 border-t border-primary/20 pt-4">
            <span className="label text-primary/70">Tasks</span>
            {tasks.slice(0, 4).map((task) => (
              <Badge key={task.id} tone={taskTone(task.status)}>
                {TASK_KINDS[task.kind] ?? task.kind}: {TASK_STATUSES[task.status] ?? task.status}
              </Badge>
            ))}
            {tasks.length > 4 ? (
              <span className="label text-primary/70">+{tasks.length - 4} more</span>
            ) : null}
          </div>
        ) : null}

        {view && total > 0 ? (
          <div className="flex flex-wrap items-center gap-3 border-t border-primary/20 pt-4">
            <span className="label text-primary/70">
              Session tokens: in {input} / out {output}
            </span>
            <span className="h-1 max-w-40 flex-1 bg-primary/10" aria-hidden="true">
              <span
                className="block h-full bg-secondary"
                style={{ width: `${Math.round((input / total) * 100)}%` }}
              />
            </span>
          </div>
        ) : null}
      </div>
    </section>
  );
}
