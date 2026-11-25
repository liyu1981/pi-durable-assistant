import { useAtomValue, useSetAtom } from "jotai/react";
import { useCallback, useEffect, useState } from "react";
import { exportUrl, get, post, subscribeView } from "./api/client";
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
import { Badge, Button } from "./components/ui";
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
      toast(
        `Failed to load conversations: ${error instanceof Error ? error.message : String(error)}`,
        "error",
      );
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
      toast(
        `Failed to create a conversation: ${error instanceof Error ? error.message : String(error)}`,
        "error",
      );
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
        toast(
          `Initialization failed: ${error instanceof Error ? error.message : String(error)}`,
          "error",
        );
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
      toast(`Fork failed: ${error instanceof Error ? error.message : String(error)}`, "warning");
    }
  };

  const current = conversations.find((item) => item.id === currentId);
  const title =
    currentId === null
      ? "No conversation selected"
      : `${current?.forkOf ? "Fork - " : ""}${current?.title || (view?.parentId ? `Subagent session #${currentId}` : "New conversation")}`;
  const connectionTone =
    sseStatus === "open" ? "success" : sseStatus === "reconnecting" ? "warning" : "neutral";
  const connectionLabel =
    sseStatus === "open"
      ? "Connected"
      : sseStatus === "reconnecting"
        ? "Reconnecting"
        : "Connecting";

  return (
    <div className="flex h-dvh flex-col bg-surface text-text">
      <header className="flex shrink-0 items-center justify-between gap-3 border-b border-primary px-4 py-3">
        <div className="flex items-center gap-3">
          <Button
            variant="quiet"
            className="min-[761px]:hidden"
            aria-label="Toggle conversation list"
            aria-expanded={mobileNav}
            onClick={() => setMobileNav((previous) => !previous)}
          >
            Inbox
          </Button>
          <span
            className="grid h-9 w-9 place-items-center rounded-sm bg-primary font-display text-lg font-bold text-surface"
            aria-hidden="true"
          >
            P
          </span>
          <div>
            <p className="label">Pi Durable</p>
            <p className="font-display text-lg font-bold tracking-tight">Assistant</p>
          </div>
        </div>
        <nav className="flex flex-wrap items-center justify-end gap-2" aria-label="Tools">
          <Badge tone={connectionTone}>{connectionLabel}</Badge>
          <Button variant="quiet" onClick={() => setDrawer({ open: true, panel: "tasks" })}>
            Tasks
          </Button>
          <Button variant="quiet" onClick={() => setDrawer({ open: true, panel: "personal" })}>
            Memory
          </Button>
          <Button variant="quiet" onClick={() => setDrawer({ open: true, panel: "knowledge" })}>
            Knowledge
          </Button>
        </nav>
      </header>

      {sseStatus === "reconnecting" ? (
        <div className="shrink-0 border-b border-warning px-4 py-2" role="status">
          <span className="label text-warning">Connection lost. Reconnecting...</span>
        </div>
      ) : null}

      <div className="grid min-h-0 flex-1 grid-cols-[280px_minmax(0,1fr)] max-[1100px]:grid-cols-[260px_minmax(0,1fr)] max-[760px]:grid-cols-[minmax(0,1fr)]">
        {mobileNav ? (
          <button
            type="button"
            className="fixed inset-0 z-40 bg-primary/50 min-[761px]:hidden"
            aria-label="Close conversation list"
            onClick={() => setMobileNav(false)}
          />
        ) : null}
        <div
          className={`${
            mobileNav ? "block" : "hidden"
          } min-[761px]:block max-[760px]:fixed max-[760px]:bottom-0 max-[760px]:left-0 max-[760px]:top-0 max-[760px]:z-50 max-[760px]:w-[min(300px,85vw)] max-[760px]:border-r max-[760px]:border-primary`}
        >
          <Sidebar onSelect={select} onCreated={(id) => select(id)} />
        </div>

        <main
          data-chat-column
          className="relative flex min-w-0 flex-col border-r border-primary bg-surface"
        >
          <header className="flex shrink-0 items-center justify-between gap-3 border-b border-primary px-6 py-4">
            <div className="min-w-0">
              <p className="label">Conversation</p>
              <h2 className="truncate font-display text-2xl font-bold tracking-tight" title={title}>
                {title}
              </h2>
            </div>
            {currentId !== null ? (
              <a className="btn" href={exportUrl(currentId)} download>
                Export
              </a>
            ) : null}
          </header>
          {view?.parentId ? (
            <div className="flex shrink-0 flex-wrap items-center justify-between gap-2 border-b border-secondary px-6 py-2">
              <span className="label text-secondary">Subagent conversation</span>
              <Button variant="quiet" onClick={() => select(view.parentId as number)}>
                Back to parent
              </Button>
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
