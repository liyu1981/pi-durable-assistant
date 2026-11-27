import {
  ActivityIcon,
  BookOpen01Icon,
  BrainIcon,
  Calendar01Icon,
  Clock01Icon,
  MagicWand01Icon,
  NotebookIcon,
  ServerIcon,
  SettingsIcon,
  StickyNoteIcon,
  ToolboxIcon,
} from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { useAtomValue, useSetAtom } from "jotai/react";
import { useEffect, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { Switch } from "@/components/ui/switch";
import { cn } from "@/lib/utils";
import { post } from "../api/client";
import type { TaskView } from "../api/types";
import { drawerAtom, serverInfoAtom, sseStatusAtom, viewAtom } from "../atoms";
import { extractLiveInfo, formatDuration, formatTime, usageNumbers } from "../lib/format";
import { useToast } from "../lib/useToast";
import { ConfirmDialog, StatusMessage, toneVariant } from "./ui";

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
    return <p className="text-sm text-muted-foreground">Idle. No active tasks.</p>;
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
    <ul className="flex flex-col text-sm">
      {rows.map(({ task, depth }) => (
        <li
          key={task.id}
          className={`flex items-center justify-between gap-2 border-b border-border py-1 ${indent[Math.min(depth, 2)]}`}
        >
          <span className="truncate" title={task.id}>
            {TASK_KINDS[task.kind] ?? task.kind}
          </span>
          <Badge variant={toneVariant(taskTone(task.status))}>
            {TASK_STATUSES[task.status] ?? task.status}
          </Badge>
        </li>
      ))}
    </ul>
  );
}

export function StatusPanel({ className }: { className?: string }) {
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

  const connectionVariant =
    sseStatus === "open" ? "default" : sseStatus === "reconnecting" ? "secondary" : "outline";

  return (
    <aside
      className={cn(
        "h-full w-[480px] shrink-0 overflow-y-auto border-l border-border bg-muted px-4 py-6",
        className,
      )}
      aria-label="Status and notes"
    >
      <div className="flex flex-col gap-4">
        <Card size="sm">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <HugeiconsIcon icon={ActivityIcon} strokeWidth={2} />
              Session status
            </CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-2">
            <Badge variant={connectionVariant} className="w-fit">
              {sseStatus === "open"
                ? "Connected"
                : sseStatus === "reconnecting"
                  ? "Reconnecting"
                  : "Connecting"}
            </Badge>
            <StatusMessage>
              {live.runningTools.length > 0
                ? "Tools running"
                : usage.total > 0
                  ? "Usage recorded"
                  : "Idle"}
            </StatusMessage>
          </CardContent>
        </Card>

        <Card size="sm">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <HugeiconsIcon icon={NotebookIcon} strokeWidth={2} />
              Notebook
            </CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-2">
            <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
              To-dos
            </p>
            {todos.length > 0 ? (
              <ul>
                {todos.map((todo) => (
                  <li key={todo.text} className="flex items-center gap-2 py-1 text-sm">
                    <Switch
                      size="sm"
                      checked={todo.done}
                      disabled
                      onCheckedChange={() => {}}
                      aria-label={todo.text}
                    />
                    <Label className={`font-normal ${todo.done ? textClassDone : ""}`}>
                      {todo.text}
                    </Label>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="text-sm text-muted-foreground">No to-dos yet.</p>
            )}
            <Separator className="my-2" />
            <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
              Memos
            </p>
            {memos.length > 0 ? (
              <ul>
                {memos.map((memo) => {
                  const text = typeof memo === "string" ? memo : memo.text;
                  return (
                    <li key={text} className="border-l-2 border-primary/60 py-1 pl-2 text-sm">
                      {text}
                    </li>
                  );
                })}
              </ul>
            ) : (
              <p className="text-sm text-muted-foreground">No memos yet.</p>
            )}
          </CardContent>
        </Card>

        <Card size="sm">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <HugeiconsIcon icon={Clock01Icon} strokeWidth={2} />
              Reminders
            </CardTitle>
          </CardHeader>
          <CardContent>
            {reminders.length > 0 ? (
              <ul className="flex flex-col text-sm">
                {reminders.map((reminder) => (
                  <li
                    key={reminder.id}
                    className="flex items-center justify-between gap-2 border-b border-border py-1"
                  >
                    <span className="truncate" title={formatTime(reminder.at)}>
                      {reminder.text}
                    </span>
                    <Badge variant={reminder.status === "scheduled" ? "secondary" : "default"}>
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
              <p className="text-sm text-muted-foreground">No reminders yet.</p>
            )}
          </CardContent>
        </Card>

        <Card size="sm">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <HugeiconsIcon icon={Calendar01Icon} strokeWidth={2} />
              Durable tasks
            </CardTitle>
          </CardHeader>
          <CardContent>
            <TaskTree tasks={view?.tasks ?? []} />
          </CardContent>
        </Card>

        <Card size="sm">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <HugeiconsIcon icon={ServerIcon} strokeWidth={2} />
              Runtime
            </CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-2">
            <dl className="flex flex-col text-sm">
              <RuntimeRow label="Boots" value={serverInfo?.boots ?? "-"} />
              <RuntimeRow label="PID" value={serverInfo?.pid ?? "-"} />
              <RuntimeRow label="Started" value={formatTime(serverInfo?.startedAt)} />
              <RuntimeRow
                label="Model"
                value={serverInfo?.model || serverInfo?.api || "-"}
                truncate
              />
              <RuntimeRow label="Tokens" value={`in ${usage.input} / out ${usage.output}`} />
            </dl>
            <div className="h-1 bg-muted" aria-hidden="true">
              <div
                className="h-full bg-primary"
                style={{
                  width: `${usage.total ? Math.round((usage.input / usage.total) * 100) : 0}%`,
                }}
              />
            </div>
          </CardContent>
        </Card>

        <Card size="sm">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <HugeiconsIcon icon={SettingsIcon} strokeWidth={2} />
              Management
            </CardTitle>
          </CardHeader>
          <CardContent className="flex flex-wrap gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => setDrawer({ open: true, panel: "tasks" })}
            >
              <HugeiconsIcon icon={ActivityIcon} strokeWidth={2} />
              Task center
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setDrawer({ open: true, panel: "memory" })}
            >
              <HugeiconsIcon icon={BrainIcon} strokeWidth={2} />
              Memory
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setDrawer({ open: true, panel: "skills" })}
            >
              <HugeiconsIcon icon={MagicWand01Icon} strokeWidth={2} />
              Skills
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setDrawer({ open: true, panel: "tools" })}
            >
              <HugeiconsIcon icon={ToolboxIcon} strokeWidth={2} />
              Tools
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setDrawer({ open: true, panel: "knowledge" })}
            >
              <HugeiconsIcon icon={BookOpen01Icon} strokeWidth={2} />
              Knowledge
            </Button>
          </CardContent>
        </Card>

        {serverInfo?.dev ? (
          <Card size="sm">
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <HugeiconsIcon icon={StickyNoteIcon} strokeWidth={2} />
                Demo
              </CardTitle>
            </CardHeader>
            <CardContent>
              <Button variant="destructive" size="sm" onClick={() => setConfirmCrash(true)}>
                Simulate crash
              </Button>
            </CardContent>
          </Card>
        ) : null}
      </div>

      <ConfirmDialog
        open={confirmCrash}
        onOpenChange={setConfirmCrash}
        title="Simulate a crash?"
        description="The supervisor restarts the exited process. This is a demo action."
        confirmLabel="Simulate crash"
        destructive
        onConfirm={() => void crash()}
      />
    </aside>
  );
}

const textClassDone = "line-through text-muted-foreground";

function RuntimeRow({
  label,
  value,
  truncate = false,
}: {
  label: string;
  value: string | number;
  truncate?: boolean;
}) {
  return (
    <div className="flex justify-between gap-2 border-b border-border py-1">
      <dt className="text-xs uppercase tracking-wide text-muted-foreground">{label}</dt>
      <dd className={truncate ? "truncate" : ""} title={String(value)}>
        {value}
      </dd>
    </div>
  );
}
