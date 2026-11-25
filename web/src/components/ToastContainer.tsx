import { useAtom } from "jotai/react";
import { useEffect } from "react";
import { toastsAtom } from "../atoms";

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
    <div className="toast-stack" role="status" aria-live="polite">
      {toasts.map((toast) => (
        <div className={`toast ${toast.kind}`} key={toast.id}>
          <span>{toast.message}</span>
          <button
            type="button"
            className="btn btn-quiet"
            aria-label="关闭提示"
            onClick={() => setToasts((previous) => previous.filter((item) => item.id !== toast.id))}
          >
            ×
          </button>
        </div>
      ))}
    </div>
  );
}
