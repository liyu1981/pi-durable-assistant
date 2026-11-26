import { useAtom } from "jotai/react";
import { useCallback, useEffect, useState } from "react";
import { get } from "../api/client";
import type { PersonalData } from "../api/types";
import { personalAtom } from "../atoms";
import type { Tone } from "../components/ui";
import { useToast } from "./useToast";

/** Badge labels for approval-gated personal items. */
export const STATUS_LABEL: Record<string, string> = {
  pending: "Pending",
  approved: "Active",
  rejected: "Rejected",
};

/** Map an approval status onto the shared badge tone. */
export function statusTone(status: string): Tone {
  if (status === "pending") return "warning";
  if (status === "approved") return "success";
  return "danger";
}

/**
 * Shared personal-data state for the memory and skills panels: polls
 * `/api/personal` while mounted and runs labelled actions with feedback.
 */
export function usePersonal() {
  const [personal, setPersonal] = useAtom(personalAtom);
  const [feedback, setFeedback] = useState("");
  const toast = useToast();

  const refresh = useCallback(async () => {
    try {
      setPersonal(await get<PersonalData>("/api/personal"));
    } catch (error) {
      toast(
        `Failed to load memory and skills: ${error instanceof Error ? error.message : String(error)}`,
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

  const run = useCallback(
    async (label: string, action: () => Promise<unknown>, reset?: () => void) => {
      setFeedback("Working...");
      try {
        await action();
        reset?.();
        setFeedback(`${label} complete`);
        await refresh();
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        setFeedback(`${label} failed: ${message}`);
        toast(`${label} failed: ${message}`, "warning");
      }
    },
    [refresh, toast],
  );

  return { personal, feedback, run };
}
