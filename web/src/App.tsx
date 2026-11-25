import {
  ActivityIcon,
  Alert02Icon,
  ArrowLeft01Icon,
  BookOpen01Icon,
  BrainIcon,
  DownloadIcon,
  Menu01Icon,
  PanelLeftCloseIcon,
  PanelLeftOpenIcon,
  PanelRightCloseIcon,
  PanelRightOpenIcon,
  SettingsIcon,
  Wifi01Icon,
} from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { useAtom, useAtomValue, useSetAtom } from "jotai/react";
import { useCallback, useEffect, useState } from "react";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button, buttonVariants } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Separator } from "@/components/ui/separator";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { exportUrl, get, post, subscribeView } from "./api/client";
import type { Conversation, ServerInfo } from "./api/types";
import {
  conversationFilterAtom,
  conversationsAtom,
  currentConversationIdAtom,
  drawerAtom,
  leftPanelCollapsedAtom,
  pendingRemoveAtom,
  rightPanelCollapsedAtom,
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
  const [leftCollapsed, setLeftCollapsed] = useAtom(leftPanelCollapsedAtom);
  const [rightCollapsed, setRightCollapsed] = useAtom(rightPanelCollapsedAtom);
  // Delay the icon swap until the panel and trigger have finished moving together.
  const [leftIconCollapsed, setLeftIconCollapsed] = useState(leftCollapsed);
  const [rightIconCollapsed, setRightIconCollapsed] = useState(rightCollapsed);
  useEffect(() => {
    const timer = setTimeout(() => setLeftIconCollapsed(leftCollapsed), 200);
    return () => clearTimeout(timer);
  }, [leftCollapsed]);
  useEffect(() => {
    const timer = setTimeout(() => setRightIconCollapsed(rightCollapsed), 200);
    return () => clearTimeout(timer);
  }, [rightCollapsed]);
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
  const connectionVariant =
    sseStatus === "open" ? "default" : sseStatus === "reconnecting" ? "secondary" : "outline";
  const connectionLabel =
    sseStatus === "open"
      ? "Connected"
      : sseStatus === "reconnecting"
        ? "Reconnecting"
        : "Connecting";

  return (
    <div className="flex h-dvh flex-col bg-muted text-foreground">
      <header className="sticky top-0 z-30 flex h-16 shrink-0 items-center justify-between gap-3 border-b border-border bg-white app-header-shadow px-4">
        <div className="flex items-center gap-3">
          <Button
            variant="ghost"
            size="icon-sm"
            className="md:hidden"
            aria-label="Toggle conversation list"
            aria-expanded={mobileNav}
            onClick={() => setMobileNav((previous) => !previous)}
          >
            <HugeiconsIcon icon={Menu01Icon} strokeWidth={2} />
          </Button>
          <img
            src="./assets/logo-64.png"
            alt=""
            aria-hidden="true"
            className="size-9 rounded-xl border border-border object-cover"
          />
          <div>
            <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
              Pi Durable
            </p>
            <p className="font-heading text-lg font-bold tracking-tight">Assistant</p>
          </div>
        </div>
        <nav className="flex flex-wrap items-center justify-end gap-2" aria-label="Tools">
          <Badge variant={connectionVariant} className="gap-1">
            <HugeiconsIcon icon={Wifi01Icon} strokeWidth={2} />
            {connectionLabel}
          </Badge>
          <Separator orientation="vertical" className="mx-1 h-6" />
          <DropdownMenu>
            <DropdownMenuTrigger
              render={
                <Button
                  variant="ghost"
                  size="icon-sm"
                  className="md:hidden"
                  aria-label="Open tools menu"
                >
                  <HugeiconsIcon icon={SettingsIcon} strokeWidth={2} />
                </Button>
              }
            />
            <DropdownMenuContent align="end">
              <DropdownMenuItem onClick={() => setDrawer({ open: true, panel: "status" })}>
                <HugeiconsIcon icon={ActivityIcon} strokeWidth={2} />
                Status
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem onClick={() => setDrawer({ open: true, panel: "tasks" })}>
                <HugeiconsIcon icon={ActivityIcon} strokeWidth={2} />
                Tasks
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => setDrawer({ open: true, panel: "personal" })}>
                <HugeiconsIcon icon={BrainIcon} strokeWidth={2} />
                Memory &amp; skills
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => setDrawer({ open: true, panel: "knowledge" })}>
                <HugeiconsIcon icon={BookOpen01Icon} strokeWidth={2} />
                Knowledge
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
          <Button
            variant="ghost"
            size="sm"
            className="hidden md:inline-flex lg:hidden"
            onClick={() => setDrawer({ open: true, panel: "status" })}
          >
            Status
          </Button>
          <Button
            variant="ghost"
            size="sm"
            className="hidden md:inline-flex"
            onClick={() => setDrawer({ open: true, panel: "tasks" })}
          >
            <HugeiconsIcon icon={ActivityIcon} strokeWidth={2} />
            Tasks
          </Button>
          <Button
            variant="ghost"
            size="sm"
            className="hidden md:inline-flex"
            onClick={() => setDrawer({ open: true, panel: "personal" })}
          >
            <HugeiconsIcon icon={BrainIcon} strokeWidth={2} />
            Memory
          </Button>
          <Button
            variant="ghost"
            size="sm"
            className="hidden md:inline-flex"
            onClick={() => setDrawer({ open: true, panel: "knowledge" })}
          >
            <HugeiconsIcon icon={BookOpen01Icon} strokeWidth={2} />
            Knowledge
          </Button>
        </nav>
      </header>

      {sseStatus === "reconnecting" ? (
        <Alert className="shrink-0 rounded-none border-x-0 border-t-0 py-2">
          <HugeiconsIcon icon={Alert02Icon} strokeWidth={2} />
          <AlertDescription>Connection lost. Reconnecting...</AlertDescription>
        </Alert>
      ) : null}

      <div className="relative min-h-0 flex-1">
        {mobileNav ? (
          <button
            type="button"
            className="fixed inset-0 z-40 bg-foreground/40 md:hidden"
            aria-label="Close conversation list"
            onClick={() => setMobileNav(false)}
          />
        ) : null}
        <aside
          className={[
            "fixed bottom-0 left-0 top-16 z-50 flex w-[min(320px,85vw)] flex-col overflow-hidden border-r border-border bg-muted transition-[width,transform] duration-200 md:z-20",
            mobileNav ? "translate-x-0" : "-translate-x-full",
            leftCollapsed
              ? "md:w-0 md:-translate-x-full md:border-r-0 md:pointer-events-none"
              : "md:w-[260px] md:translate-x-0",
          ].join(" ")}
        >
          <div className="relative h-full w-full">
            <Sidebar onSelect={select} onCreated={(id) => select(id)} />
          </div>
        </aside>

        <main
          data-chat-column
          className={[
            "relative flex min-w-0 flex-1 flex-col border-r border-border bg-muted lg:border-r-0",
            leftCollapsed ? "md:ml-0" : "md:ml-[260px]",
            rightCollapsed ? "lg:mr-0" : "lg:mr-[300px]",
          ].join(" ")}
        >
          <header className="flex shrink-0 items-center justify-between gap-3 border-b border-border px-6 py-4">
            <div className="min-w-0">
              <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                Conversation
              </p>
              <h2 className="truncate font-heading text-xl font-bold tracking-tight" title={title}>
                {title}
              </h2>
            </div>
            {currentId !== null ? (
              <Tooltip>
                <TooltipTrigger
                  render={
                    <a
                      href={exportUrl(currentId)}
                      download
                      className={buttonVariants({ variant: "outline", size: "sm" })}
                    />
                  }
                >
                  <HugeiconsIcon icon={DownloadIcon} strokeWidth={2} />
                  Export
                </TooltipTrigger>
                <TooltipContent>Download this conversation as Markdown</TooltipContent>
              </Tooltip>
            ) : null}
          </header>
          {view?.parentId ? (
            <div className="flex shrink-0 flex-wrap items-center justify-between gap-2 border-b border-border bg-muted/50 px-6 py-2">
              <Badge variant="secondary">Subagent conversation</Badge>
              <Button variant="ghost" size="sm" onClick={() => select(view.parentId as number)}>
                <HugeiconsIcon icon={ArrowLeft01Icon} strokeWidth={2} />
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

        <aside
          className={[
            "fixed bottom-0 right-0 top-16 z-20 hidden overflow-hidden border-l border-border bg-muted transition-[width,transform] duration-200 lg:block",
            rightCollapsed
              ? "w-0 translate-x-full border-l-0 pointer-events-none"
              : "w-[300px] translate-x-0",
          ].join(" ")}
        >
          <div className="relative h-full w-full">
            <StatusPanel className="w-[300px]" />
          </div>
        </aside>

        <Button
          variant="outline"
          size="icon-sm"
          className={[
            "fixed top-1/2 z-30 hidden mt-8 -translate-y-1/2 rounded-l-none bg-background transition-all duration-200 md:inline-flex",
            leftCollapsed ? "left-0" : "left-[260px]",
          ].join(" ")}
          aria-label={leftCollapsed ? "Expand conversation list" : "Collapse conversation list"}
          onClick={() => setLeftCollapsed((value) => !value)}
        >
          <HugeiconsIcon
            icon={leftIconCollapsed ? PanelLeftCloseIcon : PanelLeftOpenIcon}
            strokeWidth={2}
          />
        </Button>
        <Button
          variant="outline"
          size="icon-sm"
          className={[
            "fixed top-1/2 z-30 hidden mt-8 -translate-y-1/2 rounded-r-none bg-background transition-all duration-200 lg:inline-flex",
            rightCollapsed ? "right-0" : "right-[300px]",
          ].join(" ")}
          aria-label={rightCollapsed ? "Expand status panel" : "Collapse status panel"}
          onClick={() => setRightCollapsed((value) => !value)}
        >
          <HugeiconsIcon
            icon={rightIconCollapsed ? PanelRightOpenIcon : PanelRightCloseIcon}
            strokeWidth={2}
          />
        </Button>
      </div>

      <Drawer />
      <ToastContainer />
    </div>
  );
}
