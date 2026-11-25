import { useAtomValue, useSetAtom } from "jotai/react";
import { useEffect, useState } from "react";
import { post } from "../api/client";
import type { TaskView } from "../api/types";
import { drawerAtom, serverInfoAtom, sseStatusAtom, viewAtom } from "../atoms";
import {
  extractLiveInfo,
  formatDuration,
  formatTime,
  makeKeyFactory,
  usageNumbers,
} from "../lib/format";
import { useToast } from "../lib/useToast";

const TASK_NAMES: Record<string, string> = {
  "pi.generation": "调用模型",
  "pi.tool": "执行工具",
  "app.reminder": "定时提醒",
  "pi.compaction": "状态压缩",
};
const STATUS_NAMES: Record<string, string> = {
  pending: "排队",
  running: "运行中",
  waiting: "等待中",
  completing: "收尾",
};

function statusClass(status: string): string {
  if (status === "running") return "badge-info";
  if (status === "waiting" || status === "pending" || status === "completing") return "badge-wait";
  if (status === "failed" || status === "error") return "badge-error";
  return "badge-ok";
}

function TaskTree({ tasks }: { tasks: TaskView[] }) {
  if (tasks.length === 0) return <p className="empty-note">空闲，没有正在运行的任务</p>;
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
  return (
    <div>
      {rows.map(({ task, depth }) => (
        <div className={`task-row indent-${Math.min(depth, 2)}`} key={task.id}>
          <span title={task.id}>{TASK_NAMES[task.kind] ?? task.kind}</span>
          <span className={`badge ${statusClass(task.status)}`}>
            {STATUS_NAMES[task.status] ?? task.status}
          </span>
        </div>
      ))}
    </div>
  );
}

export function StatusPanel() {
  const view = useAtomValue(viewAtom);
  const serverInfo = useAtomValue(serverInfoAtom);
  const sseStatus = useAtomValue(sseStatusAtom);
  const setDrawer = useSetAtom(drawerAtom);
  const toast = useToast();
  const keyFor = makeKeyFactory();
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
      toast("已请求模拟崩溃，正在等待服务恢复…", "warning");
    } catch (error) {
      toast(`模拟崩溃失败：${error instanceof Error ? error.message : String(error)}`, "warning");
    }
  };

  return (
    <aside className="right-column" aria-label="运行状态与记事本">
      <div className="right-header">
        <span className="card-title">Pi Durable · 状态</span>
        <div className="row-between">
          <span className={`badge ${sseStatus === "open" ? "badge-ok" : "badge-wait"}`}>
            {sseStatus === "open" ? "已连接" : sseStatus === "reconnecting" ? "重连中" : "连接中"}
          </span>
          <span className="label">
            {live.runningTools.length ? "工具运行中" : usage.total ? "已用量统计" : "空闲"}
          </span>
        </div>
      </div>

      <section className="card">
        <div className="card-title">记事本</div>
        <div className="label">待办事项</div>
        {todos.length ? (
          todos.map((todo) => (
            <div className={`todo${todo.done ? " done" : ""}`} key={keyFor(`todo-${todo.text}`)}>
              <input type="checkbox" checked={todo.done} disabled aria-label={todo.text} readOnly />
              <span>{todo.text}</span>
            </div>
          ))
        ) : (
          <p className="empty-note">暂无待办事项</p>
        )}
        <div className="label" style={{ marginTop: "var(--spacing-3)" }}>
          备忘录
        </div>
        {memos.length ? (
          memos.map((memo) => (
            <div
              className="memo"
              key={keyFor(`memo-${typeof memo === "string" ? memo : memo.text}`)}
            >
              📌 <span>{typeof memo === "string" ? memo : memo.text}</span>
            </div>
          ))
        ) : (
          <p className="empty-note">暂无备忘录</p>
        )}
      </section>

      <section className="card">
        <div className="card-title">定时提醒</div>
        {reminders.length ? (
          reminders.map((reminder) => (
            <div className="reminder-row" key={reminder.id}>
              <span title={formatTime(reminder.at)}>⏰ {reminder.text}</span>
              <span
                className={`badge ${reminder.status === "scheduled" ? "badge-wait" : "badge-ok"}`}
              >
                {reminder.status === "scheduled"
                  ? reminder.at > now
                    ? formatDuration(reminder.at - now)
                    : "即将触发"
                  : "已提醒"}
              </span>
            </div>
          ))
        ) : (
          <p className="empty-note">暂无定时提醒</p>
        )}
      </section>

      <section className="card">
        <div className="card-title">持久任务</div>
        <TaskTree tasks={view?.tasks ?? []} />
      </section>

      <section className="card">
        <div className="card-title">运行状态</div>
        <div className="stat-list">
          <div className="stat-row">
            <span>启动次数</span>
            <span>第 {serverInfo?.boots ?? "-"} 次</span>
          </div>
          <div className="stat-row">
            <span>进程 PID</span>
            <span>{serverInfo?.pid ?? "-"}</span>
          </div>
          <div className="stat-row">
            <span>启动时间</span>
            <span>{formatTime(serverInfo?.startedAt)}</span>
          </div>
          <div className="stat-row">
            <span>模型</span>
            <span title={serverInfo?.model}>{serverInfo?.model || serverInfo?.api || "-"}</span>
          </div>
          <div className="stat-row">
            <span>会话 Token</span>
            <span>
              入 {usage.input} · 出 {usage.output}
            </span>
          </div>
        </div>
        <span className="usage-bar">
          <span
            style={{ width: `${usage.total ? Math.round((usage.input / usage.total) * 100) : 0}%` }}
          />
        </span>
      </section>

      <section className="card">
        <div className="card-title">管理</div>
        <button
          type="button"
          className="btn"
          onClick={() => setDrawer({ open: true, panel: "tasks" })}
        >
          任务中心
        </button>
        <button
          type="button"
          className="btn"
          onClick={() => setDrawer({ open: true, panel: "personal" })}
        >
          记忆与技能
        </button>
        <button
          type="button"
          className="btn"
          onClick={() => setDrawer({ open: true, panel: "knowledge" })}
        >
          知识检索
        </button>
      </section>

      <section className="card">
        <div className="card-title">演示</div>
        {confirmCrash ? (
          <div className="row-between">
            <span className="empty-note">服务退出后由守护进程重启</span>
            <button type="button" className="btn btn-danger" onClick={() => void crash()}>
              确认
            </button>
            <button type="button" className="btn" onClick={() => setConfirmCrash(false)}>
              取消
            </button>
          </div>
        ) : (
          <button type="button" className="btn btn-danger" onClick={() => setConfirmCrash(true)}>
            💥 模拟进程崩溃
          </button>
        )}
      </section>
    </aside>
  );
}
