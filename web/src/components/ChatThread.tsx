import { useAtomValue, useSetAtom } from "jotai/react";
import { useEffect, useRef } from "react";
import type { LiveToolSlot, ToolCallView, ToolEntry, ViewPayload } from "../api/types";
import { busyAtom, draftAtom, viewAtom } from "../atoms";
import {
  extractLiveInfo,
  formatFileSize,
  formatToolArgs,
  makeKeyFactory,
  usageNumbers,
} from "../lib/format";
import { Markdown } from "./Markdown";

interface ChatThreadProps {
  onFork: (entryId: number) => void;
  onSelectConversation: (id: number) => void;
}

const EXAMPLES = [
  "帮我看看 https://pi.dev 首页讲了什么，抓完等 20 秒再总结",
  "2 分钟后提醒我站起来活动一下",
  "派一个子 Agent 去看看 https://earendil.com 有什么新文章，再告诉我",
  "记住：我喜欢简洁、带要点的回答",
];

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
  const preview = text.length > 240 ? `${text.slice(0, 240)}…` : text;
  const runningOutput = running?.output?.trim().split(/\r?\n/).filter(Boolean).at(-1) ?? "";
  return (
    <div className={`tool-card${isError ? " is-error" : ""}`}>
      <div className="tool-card-head">
        <span className="tool-name">{name === "delegate" ? "子 Agent" : name || "工具调用"}</span>
        <span className={`badge ${isError ? "badge-error" : result ? "badge-ok" : "badge-wait"}`}>
          {isError ? "执行失败" : result ? "已完成" : "运行中"}
        </span>
      </div>
      {args !== undefined ? <div className="tool-detail">参数：{formatToolArgs(args)}</div> : null}
      {childId !== undefined ? (
        <button
          type="button"
          className="btn btn-quiet"
          onClick={() => onSelectConversation(childId)}
        >
          查看子 Agent 对话 →
        </button>
      ) : null}
      <div className="tool-detail">
        {result ? preview || (isError ? "工具执行出错" : "已完成") : runningOutput || "正在执行…"}
      </div>
    </div>
  );
}

function entryHasContent(view: ViewPayload | null): boolean {
  if (!view) return false;
  return (view.entries ?? []).some((entry) => {
    if (entry.kind === "assistant")
      return !!entry.text?.trim() || !!entry.error || (entry.toolCalls?.length ?? 0) > 0;
    if (entry.kind === "user") return !!entry.text?.trim() || (entry.attachments?.length ?? 0) > 0;
    return true;
  });
}

export function ChatThread({ onFork, onSelectConversation }: ChatThreadProps) {
  const view = useAtomValue(viewAtom);
  const busy = useAtomValue(busyAtom);
  const setDraft = useSetAtom(draftAtom);
  const scrollRef = useRef<HTMLElement | null>(null);
  const nearBottom = useRef(true);
  const keyFor = makeKeyFactory();

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
    if (entry.kind === "user") {
      if (entry.text.startsWith("[定时提醒]")) {
        return (
          <div className="system-card" key={entry.id}>
            ⏰ {entry.text.replace(/^\[定时提醒\]\s*/, "")}
          </div>
        );
      }
      return (
        <div className="message-row user" key={entry.id}>
          {entry.attachments && entry.attachments.length > 0 ? (
            <div className="attachment-grid">
              {entry.attachments.map((attachment) =>
                attachment.kind === "image" && attachment.url ? (
                  <a
                    href={attachment.url}
                    target="_blank"
                    rel="noopener noreferrer"
                    key={keyFor(`image-${attachment.url}`)}
                  >
                    <img src={attachment.url} alt={attachment.name} loading="lazy" />
                  </a>
                ) : (
                  <span
                    className="file-chip"
                    key={keyFor(`file-${attachment.name}`)}
                    title={attachment.name}
                  >
                    📄 {attachment.name} · {formatFileSize(attachment.size)}
                  </span>
                ),
              )}
            </div>
          ) : null}
          {entry.text.trim() ? <div className="user-bubble">{entry.text}</div> : null}
          <button
            type="button"
            className="btn btn-quiet"
            onClick={() => onFork(entry.id)}
            title="从这里分叉"
          >
            ⑂ 从这里分叉
          </button>
        </div>
      );
    }

    if (entry.kind === "assistant") {
      if (entry.error) {
        if (hiddenErrors.has(index)) return null;
        return (
          <div className="system-card error" key={entry.id}>
            模型调用失败：{entry.error}
          </div>
        );
      }
      const calls: ToolCallView[] = entry.toolCalls ?? [];
      return (
        <div className="message-row assistant" key={entry.id}>
          {entry.text.trim() ? (
            <div className="assistant-bubble">
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
      <div className="message-row tool" key={entry.id}>
        <ToolCard
          name={entry.name ?? "工具调用"}
          result={entry}
          running={liveTools.get(String(entry.toolCallId))}
          onSelectConversation={onSelectConversation}
        />
      </div>
    );
  });

  const unrendered = live.runningTools.filter(
    (slot) => slot.callId === undefined || !renderedCalls.has(String(slot.callId)),
  );
  const showLive =
    busy && (!!live.text || !!live.thinking || unrendered.length > 0 || !!view?.live?.generation);
  const { input, output, total } = usageNumbers(view?.usage ?? null);

  return (
    <section
      className="messages"
      ref={scrollRef}
      onScroll={onScroll}
      aria-live="polite"
      aria-label="对话内容"
    >
      {!entryHasContent(view) && !busy ? (
        <div className="empty-chat">
          <h2>开始与你的持久化助手对话</h2>
          <p className="empty-note">支持断点恢复、定时提醒与子 Agent 协同。点击示例填入输入框：</p>
          <div className="example-grid">
            {EXAMPLES.map((example) => (
              <button
                type="button"
                className="example-card"
                key={example}
                onClick={() => setDraft(example)}
              >
                <span>{example}</span>
                <small>点击填入输入框</small>
              </button>
            ))}
          </div>
        </div>
      ) : (
        rendered
      )}

      {showLive ? (
        <div className="message-row assistant">
          {live.text ? (
            <div className="assistant-bubble">
              <Markdown text={live.text} />
              <span className="cursor" aria-hidden="true" />
            </div>
          ) : live.thinking ? (
            <div className="assistant-bubble">💭 正在思考：{live.thinking}</div>
          ) : view?.live?.generation ? (
            <div className="assistant-bubble">
              正在思考
              <span className="cursor" aria-hidden="true" />
            </div>
          ) : null}
          {unrendered.map((slot) => (
            <ToolCard
              key={keyFor(`live-${slot.callId ?? slot.name ?? "tool"}`)}
              name={slot.name ?? "工具"}
              args={slot.args}
              running={slot}
              onSelectConversation={onSelectConversation}
            />
          ))}
        </div>
      ) : null}

      {view && total > 0 ? (
        <div className="stat-row" title="本次会话 Token 用量">
          <span>
            会话 Token：入 {input} · 出 {output}
          </span>
          <span className="usage-bar">
            <span style={{ width: `${Math.round((input / total) * 100)}%` }} />
          </span>
        </div>
      ) : null}
    </section>
  );
}
