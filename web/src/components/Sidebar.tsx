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
import { Button, EmptyState } from "./ui";

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
    <aside className="flex h-full w-full flex-col bg-surface" aria-label="Conversations">
      <div className="border-b border-primary px-4 py-4">
        <div className="flex items-start justify-between gap-3">
          <div>
            <p className="label">Workspace</p>
            <h2 className="mt-1 font-display text-xl font-bold tracking-tight">Inbox</h2>
          </div>
          <Button variant="primary" onClick={() => void create()}>
            + New
          </Button>
        </div>
        <div className="mt-4 flex gap-2" role="tablist" aria-label="Conversation scope">
          <button
            type="button"
            role="tab"
            aria-selected={filter === "active"}
            className="tab"
            onClick={() => {
              setFilter("active");
              setPendingRemove(null);
            }}
          >
            Active ({activeCount})
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={filter === "archived"}
            className="tab"
            onClick={() => {
              setFilter("archived");
              setPendingRemove(null);
            }}
          >
            Archived ({archivedCount})
          </button>
        </div>
      </div>

      <nav className="scroll-area flex-1 px-2 py-3" aria-label="Conversations">
        {visible.length === 0 ? (
          <EmptyState title={emptyTitle} hint={emptyHint} />
        ) : (
          <ul className="flex flex-col">
            {visible.map((item) => {
              const isCurrent = item.id === currentId;
              const confirming = pendingRemove === item.id;
              const stamp =
                filter === "archived" ? (item.archivedAt ?? item.createdAt) : item.createdAt;
              return (
                <li
                  key={item.id}
                  className={`border-b border-l-4 border-primary/15 px-2 py-3 ${
                    isCurrent ? "border-l-secondary bg-primary/5" : "border-l-transparent"
                  }`}
                >
                  <button
                    type="button"
                    className="flex w-full items-baseline justify-between gap-3 text-left"
                    onClick={() => onSelect(item.id)}
                  >
                    <span className="truncate font-semibold" title={item.title}>
                      {item.forkOf ? <span className="label mr-1 text-secondary">Fork</span> : null}
                      {item.title || "Untitled conversation"}
                    </span>
                    <span className="label shrink-0 text-primary/70" title={formatTime(stamp)}>
                      {formatListTime(stamp)}
                    </span>
                  </button>
                  {confirming ? (
                    <div className="mt-3 flex flex-wrap items-center gap-2">
                      <span className="label text-primary/70">Removed from the list only.</span>
                      <Button variant="quiet" onClick={() => setPendingRemove(null)}>
                        Cancel
                      </Button>
                      <Button
                        variant="danger"
                        disabled={busyId !== null}
                        onClick={() => void shelf(item.id, "remove")}
                      >
                        Remove
                      </Button>
                    </div>
                  ) : (
                    <div className="mt-3 flex flex-wrap gap-2">
                      <Button
                        variant="quiet"
                        disabled={busyId !== null}
                        onClick={() =>
                          void shelf(item.id, filter === "archived" ? "restore" : "archive")
                        }
                      >
                        {filter === "archived" ? "Restore" : "Archive"}
                      </Button>
                      <Button
                        variant="quiet"
                        disabled={busyId !== null}
                        onClick={() => setPendingRemove(item.id)}
                      >
                        Remove
                      </Button>
                    </div>
                  )}
                </li>
              );
            })}
          </ul>
        )}
      </nav>
    </aside>
  );
}
