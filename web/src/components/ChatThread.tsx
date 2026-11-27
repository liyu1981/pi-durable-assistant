import {
  Alert02Icon,
  ArrowDown01Icon,
  ArrowUpRightIcon,
  GitForkIcon,
  Loading03Icon,
  Message01Icon,
  SparklesIcon,
} from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { useAtomValue, useSetAtom } from "jotai/react";
import { useCallback, useEffect, useRef, useState } from "react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { ScrollArea } from "@/components/ui/scroll-area";
import { imageUrl } from "../api/client";
import type { LiveToolSlot, ToolCallView, ToolEntry, ViewEntry, ViewPayload } from "../api/types";
import { busyAtom, currentConversationIdAtom, draftAtom, sseStatusAtom, viewAtom } from "../atoms";
import { extractLiveInfo, formatFileSize, formatToolArgs, usageNumbers } from "../lib/format";
import { Markdown } from "./Markdown";
import { EmptyState, LoadingState, toneVariant } from "./ui";

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
  if (tone === "danger") {
    return (
      <Alert variant="destructive" className="gap-1 py-2">
        <AlertTitle className="text-xs uppercase tracking-wide">{label}</AlertTitle>
        <AlertDescription className="text-sm text-foreground">{children}</AlertDescription>
      </Alert>
    );
  }
  return (
    <Alert className="gap-1 border-l-2 border-l-primary py-2">
      <HugeiconsIcon icon={Alert02Icon} strokeWidth={2} />
      <AlertTitle className="text-xs uppercase tracking-wide text-muted-foreground">
        {label}
      </AlertTitle>
      <AlertDescription className="text-sm text-foreground">{children}</AlertDescription>
    </Alert>
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
  const [expanded, setExpanded] = useState(false);
  const isError = Boolean(result?.isError);
  const text = result?.text ?? "";
  const preview = text.length > 240 ? `${text.slice(0, 240)}...` : text;
  const runningOutput = running?.output?.trim().split(/\r?\n/).filter(Boolean).at(-1) ?? "";
  const displayName = name === "delegate" ? "Subagent" : name || "Tool call";
  const status = isError ? "Failed" : result ? "Done" : "Running";
  const fullText = result
    ? preview || (isError ? "Tool failed" : "Done")
    : runningOutput || "Running...";
  const summary = fullText.replace(/\s+/g, " ").trim();
  const concise = summary.length > 120 ? `${summary.slice(0, 120)}...` : summary;
  return (
    <Card size="sm" className={`gap-2 ${isError ? "ring-1 ring-destructive/40" : ""}`}>
      <CardHeader className="gap-1">
        <button
          type="button"
          onClick={() => setExpanded((value) => !value)}
          aria-expanded={expanded}
          className="flex w-full items-center gap-2 text-left"
        >
          <HugeiconsIcon
            icon={ArrowDown01Icon}
            strokeWidth={2}
            className={`size-4 shrink-0 text-muted-foreground transition-transform ${expanded ? "rotate-180" : ""}`}
          />
          <CardTitle className="min-w-0 flex-1 truncate text-sm">{displayName}</CardTitle>
          <Badge variant={toneVariant(isError ? "danger" : result ? "success" : "warning")}>
            {status}
          </Badge>
        </button>
        {!expanded ? (
          <p className="truncate pl-6 text-xs text-muted-foreground" title={summary}>
            {concise}
          </p>
        ) : null}
      </CardHeader>
      {expanded ? (
        <CardContent className="flex flex-col gap-2 text-sm">
          {args !== undefined ? (
            <p className="text-xs text-muted-foreground">Arguments: {formatToolArgs(args)}</p>
          ) : null}
          {childId !== undefined ? (
            <Button
              variant="link"
              size="xs"
              className="h-auto justify-start px-0"
              onClick={() => onSelectConversation(childId)}
            >
              Open subagent conversation
              <HugeiconsIcon icon={ArrowUpRightIcon} strokeWidth={2} />
            </Button>
          ) : null}
          <p className="whitespace-pre-wrap break-words">{fullText}</p>
        </CardContent>
      ) : null}
    </Card>
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
  const scrollHostRef = useRef<HTMLDivElement>(null);
  const nearBottom = useRef(true);

  const getViewport = useCallback(
    () =>
      scrollHostRef.current?.querySelector<HTMLElement>('[data-slot="scroll-area-viewport"]') ??
      null,
    [],
  );

  useEffect(() => {
    const viewport = getViewport();
    if (!viewport) return;
    const onScroll = () => {
      nearBottom.current = viewport.scrollHeight - viewport.scrollTop - viewport.clientHeight < 80;
    };
    viewport.addEventListener("scroll", onScroll);
    return () => viewport.removeEventListener("scroll", onScroll);
  }, [getViewport]);

  useEffect(() => {
    if (view && nearBottom.current) {
      const viewport = getViewport();
      if (viewport) viewport.scrollTop = viewport.scrollHeight;
    }
  }, [view, getViewport]);

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
                        className="max-h-44 max-w-60 rounded-xl border border-border object-contain"
                      />
                    </a>
                  );
                }
                return (
                  <Badge key={attachment.name} variant="outline" title={attachment.name}>
                    <HugeiconsIcon icon={Message01Icon} strokeWidth={2} />
                    {attachment.name} - {formatFileSize(attachment.size)}
                  </Badge>
                );
              })}
            </div>
          ) : null}
          {entry.text.trim() ? (
            <div className="whitespace-pre-wrap break-words rounded-2xl bg-primary px-4 py-3 text-sm text-primary-foreground">
              {entry.text}
            </div>
          ) : null}
          <Button
            variant="ghost"
            size="xs"
            onClick={() => onFork(entry.id)}
            title="Fork a new conversation from this message"
          >
            <HugeiconsIcon icon={GitForkIcon} strokeWidth={2} />
            Fork here
          </Button>
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
            <div>
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
    <section aria-label="Conversation thread" aria-live="polite" className="min-h-0 flex-1">
      <div ref={scrollHostRef} className="h-full min-h-0">
        <ScrollArea className="h-full">
          <div className="mx-auto flex w-full max-w-3xl flex-col gap-8 px-6 py-8">
            {showLoading ? (
              <LoadingState label="Connecting to conversation" />
            ) : !entryHasContent(view) && !busy ? (
              <EmptyState
                title="Start a durable conversation"
                hint="Sessions survive restarts and can schedule reminders or delegate subagents. Select a prompt to fill the composer:"
              >
                <div className="grid gap-2 text-left sm:grid-cols-2">
                  {EXAMPLES.map((example) => (
                    <Button
                      key={example}
                      variant="outline"
                      className="h-auto justify-start whitespace-normal py-3 text-left"
                      onClick={() => setDraft(example)}
                    >
                      {example}
                    </Button>
                  ))}
                </div>
              </EmptyState>
            ) : (
              rendered
            )}

            {showLive ? (
              <div className="flex flex-col gap-3">
                <Badge variant="secondary" className="w-fit gap-1">
                  <HugeiconsIcon icon={Loading03Icon} strokeWidth={2} className="animate-spin" />
                  Streaming
                </Badge>
                {live.text ? (
                  <div>
                    <Markdown text={live.text} />
                    <span
                      className="ml-0.5 inline-block h-4 w-1.5 animate-pulse bg-primary align-middle"
                      aria-hidden="true"
                    />
                  </div>
                ) : live.thinking ? (
                  <p className="text-sm text-muted-foreground">Thinking: {live.thinking}</p>
                ) : view?.live?.generation ? (
                  <p className="text-sm text-muted-foreground">
                    <HugeiconsIcon icon={SparklesIcon} strokeWidth={2} className="mr-1 inline" />
                    Thinking
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
              <div className="flex flex-wrap items-center gap-2 border-t border-border pt-4">
                <span className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                  Tasks
                </span>
                {tasks.slice(0, 4).map((task) => (
                  <Badge key={task.id} variant={toneVariant(taskTone(task.status))}>
                    {TASK_KINDS[task.kind] ?? task.kind}:{" "}
                    {TASK_STATUSES[task.status] ?? task.status}
                  </Badge>
                ))}
                {tasks.length > 4 ? (
                  <span className="text-xs text-muted-foreground">+{tasks.length - 4} more</span>
                ) : null}
              </div>
            ) : null}

            {view && total > 0 ? (
              <div className="flex flex-wrap items-center gap-3 border-t border-border pt-4">
                <span className="text-xs text-muted-foreground">
                  Session tokens: in {input} / out {output}
                </span>
                <span className="h-1 max-w-40 flex-1 bg-muted" aria-hidden="true">
                  <span
                    className="block h-full bg-primary"
                    style={{ width: `${Math.round((input / total) * 100)}%` }}
                  />
                </span>
              </div>
            ) : null}
          </div>
        </ScrollArea>
      </div>
    </section>
  );
}
