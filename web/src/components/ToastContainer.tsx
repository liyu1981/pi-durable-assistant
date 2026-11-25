import { useAtom } from "jotai/react";
import { useEffect } from "react";
import { type Toast, toastsAtom } from "../atoms";

const EDGE: Record<Toast["kind"], string> = {
  info: "border-l-secondary",
  success: "border-l-success",
  warning: "border-l-warning",
  error: "border-l-danger",
};

const LABEL: Record<Toast["kind"], string> = {
  info: "Info",
  success: "Success",
  warning: "Warning",
  error: "Error",
};

export function ToastContainer() {
  const [toasts, setToasts] = useAtom(toastsAtom);

  useEffect(() => {
    const timers = toasts.map((toast) =>
      setTimeout(
        () => setToasts((previous) => previous.filter((item) => item.id !== toast.id)),
        4000,
      ),
    );
    return () => {
      for (const timer of timers) clearTimeout(timer);
    };
  }, [toasts, setToasts]);

  if (toasts.length === 0) return null;
  return (
    <div
      className="fixed bottom-4 right-4 z-[60] flex w-[min(380px,calc(100vw-32px))] flex-col gap-2"
      role="status"
      aria-live="polite"
    >
      {toasts.map((toast) => (
        <div
          key={toast.id}
          className={`flex items-start gap-3 rounded-sm border border-primary/20 border-l-4 bg-surface px-3 py-2 text-sm ${EDGE[toast.kind]}`}
        >
          <span className="label mt-0.5 shrink-0 text-primary/70">{LABEL[toast.kind]}</span>
          <span className="min-w-0 flex-1 break-words">{toast.message}</span>
          <button
            type="button"
            className="btn btn-quiet min-h-6 px-1"
            aria-label="Dismiss notification"
            onClick={() => setToasts((previous) => previous.filter((item) => item.id !== toast.id))}
          >
            ×
          </button>
        </div>
      ))}
    </div>
  );
}
