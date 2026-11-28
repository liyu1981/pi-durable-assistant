import { PencilIcon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { renameConversation } from "../api/client";
import { useToast } from "../lib/useToast";

/** Inline rename control shown in the conversation header and sidebar. */
export function ConversationRename({
  id,
  title,
  onRenamed,
}: {
  id: number;
  title: string;
  onRenamed: (title: string) => void;
}) {
  const toast = useToast();
  const [editing, setEditing] = useState(false);
  const [value, setValue] = useState(title);
  const [busy, setBusy] = useState(false);

  const save = async () => {
    const next = value.trim();
    if (!next || busy) return;
    setBusy(true);
    try {
      const result = await renameConversation(id, next);
      onRenamed(result.title);
      setEditing(false);
    } catch (cause) {
      toast(`Rename failed: ${cause instanceof Error ? cause.message : String(cause)}`, "warning");
    } finally {
      setBusy(false);
    }
  };

  if (editing) {
    return (
      <div className="flex min-w-0 items-center gap-1">
        <Input
          autoFocus
          value={value}
          maxLength={80}
          onChange={(event) => setValue(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter") void save();
            if (event.key === "Escape") setEditing(false);
          }}
          className="h-8 max-w-64"
          aria-label="Conversation title"
        />
        <Button size="sm" onClick={() => void save()} disabled={busy}>
          Save
        </Button>
        <Button variant="ghost" size="sm" onClick={() => setEditing(false)} disabled={busy}>
          Cancel
        </Button>
      </div>
    );
  }

  return (
    <Button
      variant="ghost"
      size="icon-sm"
      aria-label="Rename conversation"
      title="Rename conversation"
      onClick={() => {
        setValue(title);
        setEditing(true);
      }}
    >
      <HugeiconsIcon icon={PencilIcon} strokeWidth={2} />
    </Button>
  );
}
