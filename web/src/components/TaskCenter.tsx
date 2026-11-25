import { useAtom, useAtomValue } from "jotai/react";
import { useCallback, useEffect, useState } from "react";
import { exportUrl, get, post } from "../api/client";
import type { Management } from "../api/types";
import { currentConversationIdAtom, managementAtom } from "../atoms";
import { formatTime, newRequestId } from "../lib/format";
import { useToast } from "../lib/useToast";

export function TaskCenter() {
  const currentId = useAtomValue(currentConversationIdAtom);
  const [management, setManagement] = useAtom(managementAtom);
  const toast = useToast();
  const [feedback, setFeedback] = useState("");
  const [scheduleText, setScheduleText] = useState("");
  const [minutes, setMinutes] = useState("2");
  const [every, setEvery] = useState("0");
  const [backgroundText, setBackgroundText] = useState("");

  const refresh = useCallback(async () => {
    if (currentId === null) {
      setManagement(null);
      return;
    }
    try {
      setManagement(await get<Management>(`/api/conversations/${currentId}/management`));
    } catch (error) {
      toast(`读取任务失败：${error instanceof Error ? error.message : String(error)}`, "warning");
    }
  }, [currentId, setManagement, toast]);

  useEffect(() => {
    let active = true;
    const tick = () => {
      if (active) void refresh();
    };
    tick();
    const timer = setInterval(tick, 5000);
    return () => {
      active = false;
      clearInterval(timer);
    };
  }, [refresh]);

  const run = async (label: string, action: () => Promise<unknown>) => {
    setFeedback("正在处理…");
    try {
      await action();
      setFeedback(`${label}完成`);
      await refresh();
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      setFeedback(`${label}失败：${message}`);
      toast(`${label}失败：${message}`, "warning");
    }
  };

  const base = currentId === null ? "" : `/api/conversations/${currentId}`;

  return (
    <>
      <section>
        <div className="card-title">持久任务中心</div>
        <p className="empty-note">
          {currentId === null
            ? "选择会话后才能管理计划任务和后台任务；记忆与技能不依赖会话。"
            : `当前会话 #${currentId}`}
        </p>
        <p className="feedback" role="status" aria-live="polite">
          {feedback}
        </p>
        <div className="row-between">
          {currentId === null ? (
            <button type="button" className="btn" disabled>
              导出 Markdown
            </button>
          ) : (
            <a className="btn" href={exportUrl(currentId)} download>
              导出 Markdown
            </a>
          )}
          <button type="button" className="btn" onClick={() => void refresh()}>
            刷新
          </button>
        </div>
      </section>

      <section>
        <div className="card-title">创建计划任务</div>
        <form
          onSubmit={(event) => {
            event.preventDefault();
            void run("创建计划任务", async () => {
              await post(`${base}/schedules`, {
                text: scheduleText,
                minutesFromNow: Number(minutes),
                everyMinutes: Number(every),
                requestId: newRequestId(),
              });
              setScheduleText("");
            });
          }}
        >
          <label>
            任务内容
            <input
              name="text"
              maxLength={1000}
              required
              disabled={currentId === null}
              value={scheduleText}
              placeholder="例如：检查项目更新"
              onChange={(event) => setScheduleText(event.target.value)}
            />
          </label>
          <div className="row-between">
            <label>
              多少分钟后开始
              <input
                name="minutesFromNow"
                type="number"
                min="0.25"
                max="525600"
                step="any"
                required
                disabled={currentId === null}
                value={minutes}
                onChange={(event) => setMinutes(event.target.value)}
              />
            </label>
            <label>
              每隔多少分钟（0 单次）
              <input
                name="everyMinutes"
                type="number"
                min="0"
                max="525600"
                step="any"
                required
                disabled={currentId === null}
                value={every}
                onChange={(event) => setEvery(event.target.value)}
              />
            </label>
          </div>
          <button type="submit" className="btn btn-primary" disabled={currentId === null}>
            创建任务
          </button>
        </form>
      </section>

      <section>
        <div className="card-title">后台子 Agent</div>
        <form
          onSubmit={(event) => {
            event.preventDefault();
            void run("创建后台任务", async () => {
              await post(`${base}/background`, { text: backgroundText, requestId: newRequestId() });
              setBackgroundText("");
            });
          }}
        >
          <label>
            任务说明
            <textarea
              name="text"
              maxLength={4000}
              required
              disabled={currentId === null}
              value={backgroundText}
              placeholder="描述需要独立完成的工作"
              onChange={(event) => setBackgroundText(event.target.value)}
            />
          </label>
          <button type="submit" className="btn btn-primary" disabled={currentId === null}>
            后台执行
          </button>
        </form>
      </section>

      <section>
        <div className="card-title">计划任务</div>
        {(management?.schedules ?? []).length === 0 ? (
          <p className="empty-note">暂无计划任务</p>
        ) : (
          (management?.schedules ?? []).map((item) => (
            <div className="todo" key={item.id}>
              <span title={item.id}>
                {item.text} · {item.status} · 已执行 {item.runs} 次
                {item.nextAt ? ` · 下次 ${formatTime(item.nextAt)}` : ""}
              </span>
              <span className="row-between">
                {item.status === "active" ? (
                  <button
                    type="button"
                    className="btn"
                    onClick={() =>
                      void run("暂停", () => post(`${base}/schedules/${item.id}/pause`))
                    }
                  >
                    暂停
                  </button>
                ) : null}
                {item.status === "paused" ? (
                  <button
                    type="button"
                    className="btn"
                    onClick={() =>
                      void run("继续", () => post(`${base}/schedules/${item.id}/resume`))
                    }
                  >
                    继续
                  </button>
                ) : null}
                {item.status === "active" || item.status === "paused" ? (
                  <button
                    type="button"
                    className="btn btn-danger"
                    onClick={() =>
                      void run("取消", () => post(`${base}/schedules/${item.id}/cancel`))
                    }
                  >
                    取消
                  </button>
                ) : null}
              </span>
            </div>
          ))
        )}
      </section>

      <section>
        <div className="card-title">提醒</div>
        {(management?.reminders ?? []).length === 0 ? (
          <p className="empty-note">暂无提醒</p>
        ) : (
          (management?.reminders ?? []).map((item) => (
            <div className="todo" key={item.id}>
              <span>
                ⏰ {item.text} · {formatTime(item.at)} · {item.status}
              </span>
              {item.status === "scheduled" ? (
                <button
                  type="button"
                  className="btn btn-danger"
                  onClick={() =>
                    void run("取消提醒", () => post(`${base}/reminders/${item.id}/cancel`))
                  }
                >
                  取消提醒
                </button>
              ) : null}
            </div>
          ))
        )}
      </section>

      <section>
        <div className="card-title">后台任务</div>
        {(management?.background ?? []).length === 0 ? (
          <p className="empty-note">暂无后台任务</p>
        ) : (
          (management?.background ?? []).map((item) => (
            <div className="todo" key={item.id}>
              <span title={item.id}>
                {item.text} · {item.status}
                {item.result ? ` · ${item.result.slice(0, 200)}` : ""}
              </span>
              <span className="row-between">
                {item.conversationId !== null ? (
                  <a className="btn" href={`#${item.conversationId}`}>
                    查看对话
                  </a>
                ) : null}
                {item.status === "running" ? (
                  <button
                    type="button"
                    className="btn btn-danger"
                    onClick={() =>
                      void run("停止", () => post(`${base}/background/${item.id}/stop`))
                    }
                  >
                    停止
                  </button>
                ) : null}
              </span>
            </div>
          ))
        )}
      </section>
    </>
  );
}
