import { useCallback } from "react";
import { toast } from "sonner";
import type { Toast } from "../atoms";

/** Stable bridge from the app's toast kinds to the Sonner toaster. */
export function useToast() {
  return useCallback((message: string, kind: Toast["kind"] = "info") => {
    toast[kind](message);
  }, []);
}
