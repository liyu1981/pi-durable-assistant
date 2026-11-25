import { useAtom } from "jotai/react";
import { useEffect, useRef } from "react";
import { drawerAtom } from "../atoms";
import { KnowledgePanel } from "./KnowledgePanel";
import { PersonalPanel } from "./PersonalPanel";
import { TaskCenter } from "./TaskCenter";

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
        'button:not([disabled]), a[href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
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
  const tabs = [
    { key: "tasks", label: "任务中心" },
    { key: "personal", label: "记忆与技能" },
    { key: "knowledge", label: "知识检索" },
  ] as const;

  return (
    <>
      <button type="button" className="drawer-backdrop" aria-label="关闭面板" onClick={close} />
      <aside
        ref={drawerRef}
        className="drawer"
        role="dialog"
        aria-modal="true"
        aria-label="管理面板"
      >
        <div className="row-between">
          <span className="card-title">管理面板</span>
          <button type="button" className="btn" ref={closeRef} onClick={close}>
            关闭
          </button>
        </div>
        <div className="tabs" role="tablist" aria-label="管理面板">
          {tabs.map((tab) => (
            <button
              type="button"
              className="tab"
              role="tab"
              aria-selected={drawer.panel === tab.key}
              key={tab.key}
              onClick={() => setDrawer({ open: true, panel: tab.key })}
            >
              {tab.label}
            </button>
          ))}
        </div>
        {drawer.panel === "tasks" ? <TaskCenter /> : null}
        {drawer.panel === "personal" ? <PersonalPanel /> : null}
        {drawer.panel === "knowledge" ? <KnowledgePanel /> : null}
      </aside>
    </>
  );
}
