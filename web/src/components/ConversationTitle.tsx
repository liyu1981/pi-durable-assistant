import { PencilIcon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { renameConversation } from "../api/client";
import { useToast } from "../lib/useToast";

/**
 * Conversation title with inline rename. While editing, the heading itself is
 * replaced by an input holding the current title, so nothing shifts sideways.
 */
export function ConversationTitle({
  id,
  title,
  label = title,
  onRenamed,
}: {
  id: number;
  /** Stored title, used to seed the inline editor. */
  title: string;
  /** Heading text when it differs from the stored title, for example a fallback. */
  label?: string;
  onRenamed: (title: string) => void;
}) {
  const toast = useToast();
  const [editing, setEditing] = useState(false);
  const [value, setValue] = useState(title);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    if (!editing) return;
    const input = inputRef.current;
    if (!input) return;
    input.focus();
    input.select();
  }, [editing]);

  const start = () => {
    setValue(title);
    setError(null);
    setEditing(true);
  };

  const save = async () => {
    const next = value.trim();
    if (!next) {
      setError("Title is required.");
      return;
    }
    if (next === title) {
      setEditing(false);
      return;
    }
    if (busy) return;
    setBusy(true);
    try {
      const result = await renameConversation(id, next);
      onRenamed(result.title);
      setEditing(false);
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : String(cause);
      setError(message);
      toast(`Rename failed: ${message}`, "warning");
    } finally {
      setBusy(false);
    }
  };

  if (editing) {
    return (
      <span className="flex min-w-0 flex-col gap-1">
        <Input
          ref={inputRef}
          value={value}
          maxLength={80}
          aria-label="Conversation title"
          aria-invalid={error !== null}
          disabled={busy}
          onChange={(event) => setValue(event.target.value)}
          onBlur={() => void save()}
          onKeyDown={(event) => {
            if (event.key === "Enter") {
              event.preventDefault();
              void save();
            }
            if (event.key === "Escape") {
              event.preventDefault();
              setEditing(false);
              setError(null);
            }
          }}
          className="h-9 w-full max-w-xl font-heading text-xl font-bold"
        />
        {error ? <span className="text-xs text-destructive">{error}</span> : null}
      </span>
    );
  }

  return (
    <span className="flex min-w-0 items-center gap-1">
      <h2 className="truncate font-heading text-xl font-bold tracking-tight" title={label}>
        {label}
      </h2>
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label="Rename conversation"
        title="Rename conversation"
        onClick={start}
      >
        <HugeiconsIcon icon={PencilIcon} strokeWidth={2} />
      </Button>
    </span>
  );
}
