import { RefreshIcon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { useAtomValue } from "jotai/react";
import { useCallback, useEffect, useState } from "react";
import { Button } from "@/components/ui/button";
import { getSystemPrompt } from "../api/client";
import type { SystemPrompt } from "../api/types";
import { currentConversationIdAtom } from "../atoms";
import { LoadingState, StatusMessage } from "./ui";

/** Shows the system prompt currently in effect for the open conversation. */
export function SystemPromptPanel() {
  const conversationId = useAtomValue(currentConversationIdAtom);
  const [prompt, setPrompt] = useState<SystemPrompt | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      setPrompt(await getSystemPrompt(conversationId));
      setError(null);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setLoading(false);
    }
  }, [conversationId]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  if (loading && !prompt) return <LoadingState label="Loading system prompt" />;

  return (
    <div className="flex flex-col gap-4 text-sm">
      <div className="flex items-center justify-between gap-2">
        <div>
          <h3 className="font-heading text-lg font-semibold">System prompt</h3>
          <p className="text-xs text-muted-foreground">
            The sections in effect for conversation {prompt?.conversationId ?? "the root agent"}.
          </p>
        </div>
        <Button
          variant="outline"
          size="icon-sm"
          onClick={() => void refresh()}
          aria-label="Refresh system prompt"
        >
          <HugeiconsIcon icon={RefreshIcon} strokeWidth={2} />
        </Button>
      </div>
      <StatusMessage>{error}</StatusMessage>
      {prompt?.sections.map((section) => (
        <section
          key={section.name}
          className="flex flex-col gap-1 rounded-2xl border border-border p-3"
        >
          <h4 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            {section.name}
          </h4>
          <pre className="overflow-x-auto whitespace-pre-wrap break-words font-mono text-xs leading-5 text-foreground">
            {section.value}
          </pre>
        </section>
      ))}
      {prompt && prompt.sections.length === 0 ? (
        <p className="text-xs text-muted-foreground">No system prompt sections are active yet.</p>
      ) : null}
    </div>
  );
}
