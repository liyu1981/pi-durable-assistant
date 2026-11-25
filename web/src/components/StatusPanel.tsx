import { useAtomValue, useSetAtom } from "jotai/react";
import { useEffect, useState } from "react";
import { post } from "../api/client";
import type { TaskView } from "../api/types";
import { drawerAtom, serverInfoAtom, sseStatusAtom, viewAtom } from "../atoms";
import { extractLiveInfo, formatDuration, formatTime, usageNumbers } from "../lib/format";
import { useToast } from "../lib/useToast";
import { Badge, Button, SectionTitle, StatusMessage } from "./ui";

const TASK_KINDS: Record<string, string> = {
  "pi.generation": "Model call",
  "pi.tool": "Tool call",
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

function TaskTree({ tasks }: { tasks: TaskView[] }) {
  if (tasks.length === 0) {
    return <p className="text-sm text-primary/70">Idle. No active tasks.</p>;
  }
  const ids = new Set(tasks.map((task) => task.id));
  const children = new Map<string, TaskView[]>();
  const roots: TaskView[] = [];
  for (const task of tasks) {
    if (task.owner && ids.has(task.owner)) {
      const list = children.get(task.owner) ?? [];
      list.push(task);
      children.set(task.owner, list);
    } else {
      roots.push(task);
    }
  }
  const rows: Array<{ task: TaskView; depth: number }> = [];
  const walk = (task: TaskView, depth: number) => {
    rows.push({ task, depth });
    for (const child of children.get(task.id) ?? []) walk(child, depth + 1);
  };
  for (const root of roots) walk(root, 0);
  const indent = ["pl-0", "pl-3", "pl-6"];
  return (
    <ul className="flex flex-col">
      {rows.map(({ task, depth }) => (
        <li
          key={task.id}
          className={`flex items-center justify-between gap-2 border-b border-primary/15 py-1 text-sm ${indent[Math.min(depth, 2)]}`}
        >
          <span className="truncate" title={task.id}>
            {TASK_KINDS[task.kind] ?? task.kind}
          </span>
          <Badge tone={taskTone(task.status)}>{TASK_STATUSES[task.status] ?? task.status}</Badge>
        </li>
      ))}
    </ul>
  );
}

export function StatusPanel() {
  const view = useAtomValue(viewAtom);
  const serverInfo = useAtomValue(serverInfoAtom);
  const sseStatus = useAtomValue(sseStatusAtom);
  const setDrawer = useSetAtom(drawerAtom);
  const toast = useToast();
  const [now, setNow] = useState(Date.now());
  const [confirmCrash, setConfirmCrash] = useState(false);

  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, []);

  const live = extractLiveInfo(view?.live);
  const usage = usageNumbers(view?.usage ?? null);
  const todos = view?.notes.todos ?? [];
  const memos = view?.notes.memos ?? [];
  const reminders = [...(view?.reminders ?? [])];

  const crash = async () => {
    setConfirmCrash(false);
    try {
      await post("/api/crash");
      toast("Crash requested. Waiting for the service to restart.", "warning");
    } catch (error) {
      toast(
        `Crash request failed: ${error instanceof Error ? error.message : String(error)}`,
        "warning",
      );
    }
  };

  return (
    <aside
      className="scroll-area hidden w-[300px] shrink-0 border-l border-primary bg-surface px-4 py-6 min-[1101px]:block"
      aria-label="Status and notes"
    >
      <div className="flex items-center justify-between gap-2">
        <div>
          <p className="label">Pi Durable</p>
          <h2 className="font-display text-lg font-bold tracking-tight">Session status</h2>
        </div>
        <Badge tone={sseStatus === "open" ? "success" : "warning"}>
          {sseStatus === "open"
            ? "Connected"
            : sseStatus === "reconnecting"
              ? "Reconnecting"
              : "Connecting"}
        </Badge>
      </div>
      <StatusMessage>
        {live.runningTools.length > 0
          ? "Tools running"
          : usage.total > 0
            ? "Usage recorded"
            : "Idle"}
      </StatusMessage>

      <div className="mt-6 border-t border-primary pt-4">
        <SectionTitle>Notebook</SectionTitle>
        <p className="label text-primary/70">To-dos</p>
        {todos.length > 0 ? (
          <ul className="mt-2">
            {todos.map((todo) => (
              <li
                key={todo.text}
                className="flex items-baseline gap-2 border-b border-primary/15 py-1 text-sm"
              >
                <input
                  type="checkbox"
                  checked={todo.done}
                  disabled
                  aria-label={todo.text}
                  readOnly
                />
                <span className={todo.done ? "text-primary/70 line-through" : ""}>{todo.text}</span>
              </li>
            ))}
          </ul>
        ) : (
          <p className="mt-2 text-sm text-primary/70">No to-dos yet.</p>
        )}
        <p className="label mt-4 text-primary/70">Memos</p>
        {memos.length > 0 ? (
          <ul className="mt-2">
            {memos.map((memo) => {
              const text = typeof memo === "string" ? memo : memo.text;
              return (
                <li key={text} className="border-l-2 border-secondary py-1 pl-2 text-sm">
                  {text}
                </li>
              );
            })}
          </ul>
        ) : (
          <p className="mt-2 text-sm text-primary/70">No memos yet.</p>
        )}
      </div>

      <div className="mt-6 border-t border-primary pt-4">
        <SectionTitle>Reminders</SectionTitle>
        {reminders.length > 0 ? (
          <ul className="flex flex-col">
            {reminders.map((reminder) => (
              <li
                key={reminder.id}
                className="flex items-center justify-between gap-2 border-b border-primary/15 py-1 text-sm"
              >
                <span className="truncate" title={formatTime(reminder.at)}>
                  {reminder.text}
                </span>
                <Badge tone={reminder.status === "scheduled" ? "warning" : "success"}>
                  {reminder.status === "scheduled"
                    ? reminder.at > now
                      ? formatDuration(reminder.at - now)
                      : "Due"
                    : "Sent"}
                </Badge>
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-sm text-primary/70">No reminders yet.</p>
        )}
      </div>

      <div className="mt-6 border-t border-primary pt-4">
        <SectionTitle>Durable tasks</SectionTitle>
        <TaskTree tasks={view?.tasks ?? []} />
      </div>

      <div className="mt-6 border-t border-primary pt-4">
        <SectionTitle>Runtime</SectionTitle>
        <dl className="flex flex-col text-sm">
          <div className="flex justify-between gap-2 border-b border-primary/15 py-1">
            <dt className="label text-primary/70">Boots</dt>
            <dd>{serverInfo?.boots ?? "-"}</dd>
          </div>
          <div className="flex justify-between gap-2 border-b border-primary/15 py-1">
            <dt className="label text-primary/70">PID</dt>
            <dd>{serverInfo?.pid ?? "-"}</dd>
          </div>
          <div className="flex justify-between gap-2 border-b border-primary/15 py-1">
            <dt className="label text-primary/70">Started</dt>
            <dd>{formatTime(serverInfo?.startedAt)}</dd>
          </div>
          <div className="flex justify-between gap-2 border-b border-primary/15 py-1">
            <dt className="label text-primary/70">Model</dt>
            <dd className="truncate" title={serverInfo?.model}>
              {serverInfo?.model || serverInfo?.api || "-"}
            </dd>
          </div>
          <div className="flex justify-between gap-2 border-b border-primary/15 py-1">
            <dt className="label text-primary/70">Tokens</dt>
            <dd>
              in {usage.input} / out {usage.output}
            </dd>
          </div>
        </dl>
        <div className="mt-2 h-1 bg-primary/10" aria-hidden="true">
          <div
            className="h-full bg-secondary"
            style={{ width: `${usage.total ? Math.round((usage.input / usage.total) * 100) : 0}%` }}
          />
        </div>
      </div>

      <div className="mt-6 border-t border-primary pt-4">
        <SectionTitle>Management</SectionTitle>
        <div className="flex flex-wrap gap-2">
          <Button variant="quiet" onClick={() => setDrawer({ open: true, panel: "tasks" })}>
            Task center
          </Button>
          <Button variant="quiet" onClick={() => setDrawer({ open: true, panel: "personal" })}>
            Memory &amp; skills
          </Button>
          <Button variant="quiet" onClick={() => setDrawer({ open: true, panel: "knowledge" })}>
            Knowledge
          </Button>
        </div>
      </div>

      <div className="mt-6 border-t border-primary pt-4">
        <SectionTitle>Demo</SectionTitle>
        {confirmCrash ? (
          <div className="flex flex-col gap-2">
            <p className="text-sm text-primary/70">The supervisor restarts the exited process.</p>
            <div className="flex gap-2">
              <Button variant="danger" onClick={() => void crash()}>
                Confirm
              </Button>
              <Button variant="quiet" onClick={() => setConfirmCrash(false)}>
                Cancel
              </Button>
            </div>
          </div>
        ) : (
          <Button variant="danger" onClick={() => setConfirmCrash(true)}>
            Simulate crash
          </Button>
        )}
      </div>
    </aside>
  );
}
