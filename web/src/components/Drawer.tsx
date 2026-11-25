import { useAtom } from "jotai/react";
import { useEffect, useRef } from "react";
import { drawerAtom } from "../atoms";
import { KnowledgePanel } from "./KnowledgePanel";
import { PersonalPanel } from "./PersonalPanel";
import { TaskCenter } from "./TaskCenter";
import { Button } from "./ui";

const TABS = [
  { key: "tasks", label: "Tasks" },
  { key: "personal", label: "Memory & skills" },
  { key: "knowledge", label: "Knowledge" },
] as const;

export function Drawer() {
  const [drawer, setDrawer] = useAtom(drawerAtom);
  const closeRef = useRef<HTMLButtonElement>(null);
  const drawerRef = useRef<HTMLElement>(null);
  const previousFocus = useRef<HTMLElement | null>(null);

  useEffect(() => {
    if (drawer.open) {
      previousFocus.current = document.activeElement as HTMLElement | null;
      closeRef.current?.focus();
    } else if (previousFocus.current) {
      previousFocus.current.focus();
      previousFocus.current = null;
    }
  }, [drawer.open]);

  useEffect(() => {
    if (!drawer.open) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setDrawer((previous) => ({ ...previous, open: false }));
        return;
      }
      if (event.key !== "Tab" || !drawerRef.current) return;
      const focusable = drawerRef.current.querySelectorAll<HTMLElement>(
        'button:not([disabled]), a[href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), summary, [tabindex]:not([tabindex="-1"])',
      );
      if (focusable.length === 0) return;
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [drawer.open, setDrawer]);

  if (!drawer.open) return null;
  const close = () => setDrawer((previous) => ({ ...previous, open: false }));

  return (
    <>
      <button
        type="button"
        className="fixed inset-0 z-40 bg-primary/50"
        aria-label="Close panel"
        onClick={close}
      />
      <aside
        ref={drawerRef}
        className="fixed bottom-0 right-0 top-0 z-50 flex w-full max-w-[480px] flex-col border-l border-primary bg-surface shadow-none"
        role="dialog"
        aria-modal="true"
        aria-label="Management panel"
      >
        <header className="flex items-start justify-between gap-3 border-b border-primary px-4 py-4">
          <div>
            <p className="label">Tools</p>
            <h2 className="font-display text-xl font-bold tracking-tight">Management</h2>
          </div>
          <Button ref={closeRef} variant="quiet" onClick={close}>
            Close
          </Button>
        </header>
        <div
          className="flex gap-2 border-b border-primary/20 px-4 py-3"
          role="tablist"
          aria-label="Management panel"
        >
          {TABS.map((tab) => (
            <button
              type="button"
              role="tab"
              aria-selected={drawer.panel === tab.key}
              className="tab"
              key={tab.key}
              onClick={() => setDrawer({ open: true, panel: tab.key })}
            >
              {tab.label}
            </button>
          ))}
        </div>
        <div className="scroll-area flex-1 space-y-8 px-4 py-6">
          {drawer.panel === "tasks" ? <TaskCenter /> : null}
          {drawer.panel === "personal" ? <PersonalPanel /> : null}
          {drawer.panel === "knowledge" ? <KnowledgePanel /> : null}
        </div>
      </aside>
    </>
  );
}
