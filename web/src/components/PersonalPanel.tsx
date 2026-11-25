import { useAtom } from "jotai/react";
import { useCallback, useEffect, useState } from "react";
import { get, post } from "../api/client";
import type { PersonalData } from "../api/types";
import { personalAtom } from "../atoms";
import { newRequestId } from "../lib/format";
import { useToast } from "../lib/useToast";

const STATUS_LABEL: Record<string, string> = {
  pending: "待批准",
  approved: "已生效",
  rejected: "已撤回",
};

export function PersonalPanel() {
  const [personal, setPersonal] = useAtom(personalAtom);
  const toast = useToast();
  const [feedback, setFeedback] = useState("");
  const [memoryText, setMemoryText] = useState("");
  const [skillName, setSkillName] = useState("");
  const [skillDescription, setSkillDescription] = useState("");
  const [skillBody, setSkillBody] = useState("");

  const refresh = useCallback(async () => {
    try {
      setPersonal(await get<PersonalData>("/api/personal"));
    } catch (error) {
      toast(
        `读取记忆和技能失败：${error instanceof Error ? error.message : String(error)}`,
        "warning",
      );
    }
  }, [setPersonal, toast]);

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

  const run = async (label: string, action: () => Promise<unknown>, reset?: () => void) => {
    setFeedback("正在处理…");
    try {
      await action();
      reset?.();
      setFeedback(`${label}完成`);
      await refresh();
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      setFeedback(`${label}失败：${message}`);
      toast(`${label}失败：${message}`, "warning");
    }
  };

  const knowledge = personal?.knowledge;

  return (
    <>
      <section>
        <div className="card-title">记忆与技能</div>
        <p className="empty-note">
          {knowledge?.enabled
            ? `知识库只读：${(knowledge.allow ?? []).join("、") || "全部"}`
            : `知识库未启用：${knowledge?.reason ?? "未配置"}`}
        </p>
        <p className="feedback" role="status" aria-live="polite">
          {feedback}
        </p>
      </section>

      <section>
        <div className="card-title">长期记忆</div>
        <p className="empty-note">跨会话生效。你写下的会直接生效；助手提交的需要先批准。</p>
        <form
          onSubmit={(event) => {
            event.preventDefault();
            void run(
              "保存记忆",
              async () => {
                await post("/api/personal/memory", { text: memoryText, requestId: newRequestId() });
              },
              () => setMemoryText(""),
            );
          }}
        >
          <label>
            要记住的事实
            <input
              name="text"
              maxLength={280}
              required
              value={memoryText}
              placeholder="例如：回复用简体中文"
              onChange={(event) => setMemoryText(event.target.value)}
            />
          </label>
          <button type="submit" className="btn btn-primary">
            记住
          </button>
        </form>
        {(personal?.memory ?? []).length === 0 ? (
          <p className="empty-note">还没有跨会话记忆</p>
        ) : (
          (personal?.memory ?? []).map((item) => (
            <div className="todo" key={item.id}>
              <span title={item.id}>
                {item.text} · {STATUS_LABEL[item.status] ?? item.status} ·{" "}
                {item.source === "user" ? "你写下的" : "助手提交"}
              </span>
              <span className="row-between">
                {item.status === "pending" ? (
                  <button
                    type="button"
                    className="btn"
                    onClick={() =>
                      void run("批准", () => post(`/api/personal/memory/${item.id}/approve`, {}))
                    }
                  >
                    批准
                  </button>
                ) : null}
                {item.status === "pending" || item.status === "approved" ? (
                  <button
                    type="button"
                    className="btn btn-danger"
                    onClick={() =>
                      void run("撤回", () => post(`/api/personal/memory/${item.id}/reject`, {}))
                    }
                  >
                    {item.status === "approved" ? "撤回" : "驳回"}
                  </button>
                ) : null}
              </span>
            </div>
          ))
        )}
      </section>

      <section>
        <div className="card-title">技能</div>
        <p className="empty-note">提示里只放名称；助手需要时才读取全文。</p>
        <form
          onSubmit={(event) => {
            event.preventDefault();
            void run(
              "保存技能",
              async () => {
                await post("/api/personal/skills", {
                  name: skillName,
                  description: skillDescription,
                  body: skillBody,
                  requestId: newRequestId(),
                });
              },
              () => {
                setSkillName("");
                setSkillDescription("");
                setSkillBody("");
              },
            );
          }}
        >
          <label>
            名称
            <input
              name="name"
              maxLength={40}
              required
              value={skillName}
              onChange={(event) => setSkillName(event.target.value)}
            />
          </label>
          <label>
            什么时候用
            <input
              name="description"
              maxLength={180}
              required
              value={skillDescription}
              onChange={(event) => setSkillDescription(event.target.value)}
            />
          </label>
          <label>
            步骤
            <textarea
              name="body"
              maxLength={4000}
              required
              value={skillBody}
              onChange={(event) => setSkillBody(event.target.value)}
            />
          </label>
          <button type="submit" className="btn btn-primary">
            保存技能
          </button>
        </form>
        {(personal?.skills ?? []).length === 0 ? (
          <p className="empty-note">还没有技能</p>
        ) : (
          (personal?.skills ?? []).map((item) => (
            <div className="todo" key={item.id}>
              <div>
                <span title={item.id}>
                  {item.name}：{item.description} · {STATUS_LABEL[item.status] ?? item.status}
                </span>
                <details>
                  <summary>查看步骤</summary>
                  <pre>{item.body}</pre>
                </details>
              </div>
              <span className="row-between">
                {item.status === "pending" ? (
                  <button
                    type="button"
                    className="btn"
                    onClick={() =>
                      void run("批准", () => post(`/api/personal/skills/${item.id}/approve`, {}))
                    }
                  >
                    批准
                  </button>
                ) : null}
                {item.status === "pending" || item.status === "approved" ? (
                  <button
                    type="button"
                    className="btn btn-danger"
                    onClick={() =>
                      void run("撤回", () => post(`/api/personal/skills/${item.id}/reject`, {}))
                    }
                  >
                    {item.status === "approved" ? "撤回" : "驳回"}
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
