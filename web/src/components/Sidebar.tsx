import { useAtom } from "jotai/react";
import { useState } from "react";
import { get, post } from "../api/client";
import type { Conversation, ServerInfo } from "../api/types";
import {
  conversationFilterAtom,
  conversationsAtom,
  currentConversationIdAtom,
  pendingRemoveAtom,
  serverInfoAtom,
  viewAtom,
} from "../atoms";
import { formatListTime, formatTime } from "../lib/format";
import { useToast } from "../lib/useToast";

interface SidebarProps {
  onSelect: (id: number) => void;
  onCreated: (id: number) => void;
}

export function Sidebar({ onSelect, onCreated }: SidebarProps) {
  const [conversations, setConversations] = useAtom(conversationsAtom);
  const [filter, setFilter] = useAtom(conversationFilterAtom);
  const [currentId, setCurrentId] = useAtom(currentConversationIdAtom);
  const [pendingRemove, setPendingRemove] = useAtom(pendingRemoveAtom);
  const [, setView] = useAtom(viewAtom);
  const [, setServerInfo] = useAtom(serverInfoAtom);
  const [busyId, setBusyId] = useState<number | null>(null);
  const toast = useToast();

  const refresh = async () => {
    try {
      setConversations(await get<Conversation[]>("/api/conversations"));
    } catch (error) {
      toast(`拉取会话列表失败：${error instanceof Error ? error.message : String(error)}`, "error");
    }
  };

  const create = async () => {
    try {
      const created = await post<{ id: number }>("/api/conversations", {});
      setFilter("active");
      setPendingRemove(null);
      onCreated(created.id);
      await refresh();
    } catch (error) {
      toast(`新建会话失败：${error instanceof Error ? error.message : String(error)}`, "error");
    }
  };

  const shelf = async (id: number, action: "archive" | "restore" | "remove") => {
    if (busyId !== null) return;
    setBusyId(id);
    try {
      await post(`/api/conversations/${id}/${action}`);
      setPendingRemove(null);
      if (action === "remove" && currentId === id) {
        setCurrentId(null);
        setView(null);
      }
      if (action === "restore" && currentId === id) setFilter("active");
      await refresh();
      try {
        setServerInfo(await get<ServerInfo>("/api/info"));
      } catch {
        // Server info is optional for this action.
      }
    } catch (error) {
      toast(`操作失败：${error instanceof Error ? error.message : String(error)}`, "warning");
    } finally {
      setBusyId(null);
    }
  };

  const visible = conversations.filter((item) =>
    filter === "archived" ? !!item.archivedAt : !item.archivedAt,
  );
  const activeCount = conversations.filter((item) => !item.archivedAt).length;

  return (
    <aside className="sidebar" aria-label="会话列表">
      <div className="sidebar-header">
        <div className="brand">
          <span className="brand-mark" aria-hidden="true">
            P
          </span>
          <span>Pi Durable 助手</span>
        </div>
        <button type="button" className="btn btn-primary" onClick={() => void create()}>
          ＋ 新对话
        </button>
        <div className="tabs" role="tablist" aria-label="会话范围">
          <button
            type="button"
            className="tab"
            role="tab"
            aria-selected={filter === "active"}
            onClick={() => {
              setFilter("active");
              setPendingRemove(null);
            }}
          >
            当前 {activeCount}
          </button>
          <button
            type="button"
            className="tab"
            role="tab"
            aria-selected={filter === "archived"}
            onClick={() => {
              setFilter("archived");
              setPendingRemove(null);
            }}
          >
            归档 {conversations.length - activeCount}
          </button>
        </div>
      </div>
      <nav className="conversation-list" aria-label="对话">
        {visible.length === 0 ? (
          <p className="empty-note">
            {filter === "archived"
              ? "还没有归档的对话。"
              : activeCount === 0 && conversations.length > 0
                ? "当前没有对话，归档里还有记录。"
                : "当前没有对话。点击“新对话”开始。"}
          </p>
        ) : (
          visible.map((item) => {
            const isCurrent = item.id === currentId;
            const confirming = pendingRemove === item.id;
            const isFork = !!item.forkOf || (item.title ?? "").startsWith("分叉");
            const stamp =
              filter === "archived" ? (item.archivedAt ?? item.createdAt) : item.createdAt;
            return (
              <article
                className={`conversation-item${isCurrent ? " is-current" : ""}`}
                key={item.id}
              >
                <button
                  type="button"
                  className="conversation-open"
                  onClick={() => onSelect(item.id)}
                >
                  <span className="conversation-title" title={item.title}>
                    {isFork ? "⑂ " : ""}
                    {item.title || "新对话"}
                  </span>
                  <span className="conversation-time" title={formatTime(stamp)}>
                    {formatListTime(stamp)}
                  </span>
                </button>
                {confirming ? (
                  <div className="conversation-actions">
                    <span className="empty-note">只从列表移除，记录仍在。</span>
                    <button
                      type="button"
                      className="btn btn-quiet"
                      onClick={() => setPendingRemove(null)}
                    >
                      取消
                    </button>
                    <button
                      type="button"
                      className="btn btn-danger"
                      disabled={busyId !== null}
                      onClick={() => void shelf(item.id, "remove")}
                    >
                      删除
                    </button>
                  </div>
                ) : (
                  <div className="conversation-actions">
                    <button
                      type="button"
                      className="btn"
                      disabled={busyId !== null}
                      onClick={() =>
                        void shelf(item.id, filter === "archived" ? "restore" : "archive")
                      }
                    >
                      {filter === "archived" ? "恢复" : "归档"}
                    </button>
                    <button
                      type="button"
                      className="btn btn-danger"
                      disabled={busyId !== null}
                      onClick={() => setPendingRemove(item.id)}
                    >
                      删除
                    </button>
                  </div>
                )}
              </article>
            );
          })
        )}
      </nav>
    </aside>
  );
}
