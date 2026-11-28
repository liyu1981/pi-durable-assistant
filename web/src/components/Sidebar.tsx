import { ArchiveIcon, ArchiveRestoreIcon, PlusIcon, TrashIcon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { useAtom } from "jotai/react";
import { useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { get, post } from "../api/client";
import type { Conversation, ServerInfo } from "../api/types";
import {
  type ConversationFilter,
  conversationFilterAtom,
  conversationsAtom,
  currentConversationIdAtom,
  pendingRemoveAtom,
  serverInfoAtom,
  viewAtom,
} from "../atoms";
import { formatListTime, formatTime } from "../lib/format";
import { useToast } from "../lib/useToast";
import { ConfirmDialog, EmptyState } from "./ui";

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
      toast(
        `Failed to load conversations: ${error instanceof Error ? error.message : String(error)}`,
        "error",
      );
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
      toast(
        `Failed to create a conversation: ${error instanceof Error ? error.message : String(error)}`,
        "error",
      );
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
        // Server information is optional for this action.
      }
    } catch (error) {
      toast(`Action failed: ${error instanceof Error ? error.message : String(error)}`, "warning");
    } finally {
      setBusyId(null);
    }
  };

  const visible = conversations.filter((item) =>
    filter === "archived" ? Boolean(item.archivedAt) : !item.archivedAt,
  );
  const activeCount = conversations.filter((item) => !item.archivedAt).length;
  const archivedCount = conversations.length - activeCount;

  const emptyTitle =
    filter === "archived"
      ? "No archived conversations"
      : activeCount === 0 && archivedCount > 0
        ? "No active conversations"
        : "No conversations yet";
  const emptyHint =
    filter === "archived"
      ? "Conversations you archive will appear here."
      : archivedCount > 0
        ? "Open the Archived tab to find older conversations."
        : "Create a conversation to begin a durable session.";

  return (
    <aside
      className="flex h-full w-full flex-col border-r border-border bg-muted"
      aria-label="Conversations"
    >
      <div className="shrink-0 border-b border-border px-4 py-4">
        <div className="flex items-start justify-between gap-3">
          <div>
            <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
              Workspace
            </p>
            <h2 className="mt-1 font-heading text-xl font-bold tracking-tight">Inbox</h2>
          </div>
          <Button size="sm" onClick={() => void create()}>
            <HugeiconsIcon icon={PlusIcon} strokeWidth={2} />
            New
          </Button>
        </div>
        <Tabs
          value={filter}
          onValueChange={(value) => {
            setFilter((value ?? "active") as ConversationFilter);
            setPendingRemove(null);
          }}
          className="mt-4"
        >
          <TabsList className="w-full">
            <TabsTrigger value="active">Active ({activeCount})</TabsTrigger>
            <TabsTrigger value="archived">Archived ({archivedCount})</TabsTrigger>
          </TabsList>
        </Tabs>
      </div>

      <ScrollArea className="min-h-0 flex-1">
        <nav className="flex flex-col gap-2 px-2 py-3" aria-label="Conversations">
          {visible.length === 0 ? (
            <EmptyState title={emptyTitle} hint={emptyHint} />
          ) : (
            visible.map((item) => {
              const isCurrent = item.id === currentId;
              const stamp =
                filter === "archived" ? (item.archivedAt ?? item.createdAt) : item.createdAt;
              return (
                <Card
                  key={item.id}
                  size="sm"
                  className={`gap-2 py-3 ${isCurrent ? "ring-1 ring-primary" : ""}`}
                >
                  <CardContent className="flex flex-col gap-2">
                    <button
                      type="button"
                      className="flex w-full items-baseline justify-between gap-3 text-left"
                      onClick={() => onSelect(item.id)}
                    >
                      <span className="truncate text-sm font-medium" title={item.title}>
                        {item.forkOf ? (
                          <Badge variant="secondary" className="mr-1">
                            Fork
                          </Badge>
                        ) : null}
                        {item.title || "Untitled conversation"}
                      </span>
                      <span
                        className="shrink-0 text-xs text-muted-foreground"
                        title={formatTime(stamp)}
                      >
                        {formatListTime(stamp)}
                      </span>
                    </button>
                    <div className="flex flex-wrap gap-2">
                      <Button
                        variant="ghost"
                        size="xs"
                        disabled={busyId !== null}
                        onClick={() =>
                          void shelf(item.id, filter === "archived" ? "restore" : "archive")
                        }
                      >
                        <HugeiconsIcon
                          icon={filter === "archived" ? ArchiveRestoreIcon : ArchiveIcon}
                          strokeWidth={2}
                        />
                        {filter === "archived" ? "Restore" : "Archive"}
                      </Button>
                      <Button
                        variant="ghost"
                        size="xs"
                        className="text-destructive"
                        disabled={busyId !== null}
                        onClick={() => setPendingRemove(item.id)}
                      >
                        <HugeiconsIcon icon={TrashIcon} strokeWidth={2} />
                        Remove
                      </Button>
                    </div>
                  </CardContent>
                </Card>
              );
            })
          )}
        </nav>
      </ScrollArea>

      <ConfirmDialog
        open={pendingRemove !== null}
        onOpenChange={(open) => {
          if (!open) setPendingRemove(null);
        }}
        title="Remove conversation?"
        description="This removes the conversation from the list only. The stored session stays on disk."
        confirmLabel="Remove"
        destructive
        onConfirm={() => {
          if (pendingRemove !== null) void shelf(pendingRemove, "remove");
        }}
      />
    </aside>
  );
}
