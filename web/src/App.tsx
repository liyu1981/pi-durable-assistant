import { useAtomValue, useSetAtom } from "jotai/react";
import { useCallback, useEffect, useState } from "react";
import { get, post, subscribeView } from "./api/client";
import type { Conversation, ServerInfo } from "./api/types";
import {
  conversationFilterAtom,
  conversationsAtom,
  currentConversationIdAtom,
  drawerAtom,
  pendingRemoveAtom,
  serverInfoAtom,
  sseStatusAtom,
  viewAtom,
} from "./atoms";
import { ChatThread } from "./components/ChatThread";
import { Composer } from "./components/Composer";
import { Drawer } from "./components/Drawer";
import { Sidebar } from "./components/Sidebar";
import { StatusPanel } from "./components/StatusPanel";
import { ToastContainer } from "./components/ToastContainer";
import { useToast } from "./lib/useToast";

function hashConversationId(): number | null {
  const raw = window.location.hash.replace(/^#/, "").trim();
  return /^\d+$/.test(raw) ? Number(raw) : null;
}

export function App() {
  const currentId = useAtomValue(currentConversationIdAtom);
  const setCurrentId = useSetAtom(currentConversationIdAtom);
  const setConversations = useSetAtom(conversationsAtom);
  const setView = useSetAtom(viewAtom);
  const setServerInfo = useSetAtom(serverInfoAtom);
  const setSseStatus = useSetAtom(sseStatusAtom);
  const setFilter = useSetAtom(conversationFilterAtom);
  const setPendingRemove = useSetAtom(pendingRemoveAtom);
  const setDrawer = useSetAtom(drawerAtom);
  const conversations = useAtomValue(conversationsAtom);
  const view = useAtomValue(viewAtom);
  const sseStatus = useAtomValue(sseStatusAtom);
  const toast = useToast();
  const [mobileNav, setMobileNav] = useState(false);

  const refreshConversations = useCallback(async () => {
    try {
      setConversations(await get<Conversation[]>("/api/conversations"));
    } catch (error) {
      toast(`拉取会话列表失败：${error instanceof Error ? error.message : String(error)}`, "error");
    }
  }, [setConversations, toast]);

  const refreshInfo = useCallback(async () => {
    try {
      setServerInfo(await get<ServerInfo>("/api/info"));
    } catch {
      // Server information is non-critical for the chat UI.
    }
  }, [setServerInfo]);

  const select = useCallback(
    (id: number) => {
      if (id === currentId) return;
      setCurrentId(id);
      setView(null);
      setPendingRemove(null);
      setMobileNav(false);
      window.location.hash = String(id);
    },
    [currentId, setCurrentId, setView, setPendingRemove],
  );

  const createConversation = useCallback(async (): Promise<number | null> => {
    try {
      const created = await post<{ id: number }>("/api/conversations", {});
      setFilter("active");
      setPendingRemove(null);
      setMobileNav(false);
      setCurrentId(created.id);
      setView(null);
      window.location.hash = String(created.id);
      await refreshConversations();
      return created.id;
    } catch (error) {
      toast(`新建会话失败：${error instanceof Error ? error.message : String(error)}`, "error");
      return null;
    }
  }, [setFilter, setPendingRemove, setCurrentId, setView, refreshConversations, toast]);

  useEffect(() => {
    let cancelled = false;
    const initialise = async () => {
      try {
        const list = await get<Conversation[]>("/api/conversations");
        if (cancelled) return;
        setConversations(list);
        const hashId = hashConversationId();
        let chosen = hashId ?? list.find((item) => !item.archivedAt)?.id ?? null;
        if (chosen === null && list.length === 0) {
          const created = await post<{ id: number }>("/api/conversations", {});
          if (cancelled) return;
          chosen = created.id;
          setConversations(await get<Conversation[]>("/api/conversations"));
        }
        if (chosen !== null) {
          setCurrentId(chosen);
          window.location.hash = String(chosen);
        }
      } catch (error) {
        toast(`初始化失败：${error instanceof Error ? error.message : String(error)}`, "error");
      }
      await refreshInfo();
    };
    void initialise();
    return () => {
      cancelled = true;
    };
  }, [setConversations, setCurrentId, refreshInfo, toast]);

  useEffect(() => {
    if (currentId === null) {
      setSseStatus("closed");
      setView(null);
      return;
    }
    setView(null);
    setSseStatus("connecting");
    const subscription = subscribeView(
      currentId,
      (payload) => setView(payload),
      (status) => {
        setSseStatus(status);
        if (status === "open") {
          void refreshConversations();
          void refreshInfo();
        }
      },
    );
    return () => {
      subscription.close();
      setView(null);
    };
  }, [currentId, setView, setSseStatus, refreshConversations, refreshInfo]);

  useEffect(() => {
    if (currentId !== null && conversations.length > 0 && !view?.parentId) {
      if (!conversations.some((item) => item.id === currentId)) {
        setCurrentId(null);
        setView(null);
      }
    }
  }, [conversations, currentId, view?.parentId, setCurrentId, setView]);

  useEffect(() => {
    const timer = setInterval(() => void refreshConversations(), 10_000);
    const onHashChange = () => {
      const id = hashConversationId();
      if (id !== null && id !== currentId) {
        setCurrentId(id);
        setView(null);
      }
    };
    window.addEventListener("hashchange", onHashChange);
    return () => {
      clearInterval(timer);
      window.removeEventListener("hashchange", onHashChange);
    };
  }, [currentId, refreshConversations, setCurrentId, setView]);

  const fork = async (entryId: number) => {
    if (currentId === null) return;
    try {
      const created = await post<{ id: number }>(`/api/conversations/${currentId}/fork`, {
        entryId,
      });
      await refreshConversations();
      select(created.id);
    } catch (error) {
      toast(`分叉失败：${error instanceof Error ? error.message : String(error)}`, "warning");
    }
  };

  const current = conversations.find((item) => item.id === currentId);
  const title =
    currentId === null
      ? "未选择对话"
      : `${current?.forkOf ? "⑂ " : ""}${current?.title || (view?.parentId ? `子 Agent 会话 #${currentId}` : "新对话")}`;

  return (
    <div className="app-root">
      {sseStatus === "reconnecting" ? (
        <div className="reconnect-banner" role="status">
          与服务器断开，正在重连…
        </div>
      ) : null}
      <div className="app-shell">
        <div className={mobileNav ? "sidebar-wrap open" : "sidebar-wrap"}>
          <Sidebar onSelect={select} onCreated={(id) => select(id)} />
        </div>
        <main className="chat-column">
          <header className="chat-header">
            <div className="row-between">
              <button
                type="button"
                className="btn mobile-only"
                aria-label="打开会话列表"
                onClick={() => setMobileNav((previous) => !previous)}
              >
                会话
              </button>
              <h2 title={title}>{title}</h2>
              <div className="row-between">
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
                  记忆
                </button>
                <button
                  type="button"
                  className="btn"
                  onClick={() => setDrawer({ open: true, panel: "knowledge" })}
                >
                  知识
                </button>
              </div>
            </div>
          </header>
          {view?.parentId ? (
            <div className="parent-banner">
              <span>这是子 Agent 的对话</span>
              <button
                type="button"
                className="btn btn-quiet"
                onClick={() => select(view.parentId as number)}
              >
                ← 返回主对话
              </button>
            </div>
          ) : null}
          <ChatThread onFork={(entryId) => void fork(entryId)} onSelectConversation={select} />
          <Composer
            onEnsureConversation={createConversation}
            onSent={() => void refreshConversations()}
          />
        </main>
        <StatusPanel />
      </div>
      <Drawer />
      <ToastContainer />
    </div>
  );
}
