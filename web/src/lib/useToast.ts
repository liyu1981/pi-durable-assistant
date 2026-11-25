import { useSetAtom } from "jotai/react";
import { useCallback } from "react";
import { type Toast, toastsAtom } from "../atoms";

export function useToast() {
  const setToasts = useSetAtom(toastsAtom);
  return useCallback(
    (message: string, kind: Toast["kind"] = "info") => {
      setToasts((previous) => [...previous, { id: Date.now() + Math.random(), message, kind }]);
    },
    [setToasts],
  );
}
