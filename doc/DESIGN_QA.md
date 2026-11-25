# `minimal` design QA checklist — completed audit

Source: `liyu1981/awesome-design-skills/skills/minimal` (DESIGN.md + SKILL.md). Tokens are implemented in `web/src/styles/tokens.css` as Tailwind v4 `@theme` variables and self-hosted `@fontsource/*` imports.

Audit date: 2026-11-25. Method: source inspection of `web/src/**` plus `npm run typecheck/check/build`. Where an item requires a human at a browser, it is marked explicitly as **manual follow-up** rather than claimed as verified.

## Tokens (must)

- [x] Colors: primary `#0C0C09`, secondary `#312C85`, success `#16A34A`, warning `#D97706`, danger `#DC2626`, surface `#F4F4F1`, text `#0C0C09`, neutral `#F4F4F1` — verified in `tokens.css:13-20`.
- [x] Typography: Inter display / h1 3rem, Open Sans body / 1rem, Inconsolata label / 0.75rem uppercase with tracking — `tokens.css:22-29`, `.label` at `tokens.css:92`.
- [x] Radius: 4px small, 8px medium only — `tokens.css:31-32`.
- [x] Spacing: 4/8/12/16/24/32 — `tokens.css:34-39`.
- [x] Status colors reserved for status; no gradients/heavy shadows in styles — reviewed `web/src/styles/tokens.css` and component classes.

## Component states (must)

- [x] Visible focus ring for every interactive element: `:where(button, a, input, textarea, summary, [tabindex]):focus-visible` with 2px secondary outline and offset — `tokens.css:87-91`.
- [x] Hover/active/disabled states defined for buttons (`:hover:not(:disabled)`, `:active:not(:disabled)`, `button:disabled`) — `tokens.css:80-85,125-151`.
- [x] Loading/empty/error states present: `aria-live` feedback in `TaskCenter.tsx`, `PersonalPanel.tsx`, `ChatThread.tsx`, `ToastContainer.tsx`; disabled while busy in `KnowledgePanel.tsx`, `TaskCenter.tsx`, `Composer.tsx`.
- [x] Keyboard interactions: Enter to send (IME-aware) in `Composer.tsx:458`; Escape to close drawer in `Drawer.tsx`; Tab focus trap and focus restoration in `Drawer.tsx` (added during this audit).
- [x] Long labels / overflow: conversation rows truncate via CSS; toasts and panels scroll; empty conversation state has explicit "新建会话" action (`Sidebar.tsx`).
- [ ] **Manual follow-up:** visually exercise hover/active/loading/empty/overflow states in a browser at desktop and narrow widths.

## Accessibility (WCAG 2.2 AA)

- [x] Contrast: body text uses `#0C0C09` on `#F4F4F1` (high contrast); `#312C85` is used only for display text/focus/borders, not small body text — token audit.
- [x] Accessible names/roles: `aria-label` on sidebar, nav, composer, attachment buttons, drawer backdrop/dialog; `role="tablist"/"tab"/"aria-selected"` on sidebar and drawer tabs; `role="status" aria-live="polite"` on toasts and status panels — `Sidebar.tsx`, `Drawer.tsx`, `Composer.tsx`, `ToastContainer.tsx`, `TaskCenter.tsx`, `PersonalPanel.tsx`.
- [x] Focus order: drawer focuses its close button on open, traps Tab inside `role="dialog"`, restores the previously focused element on close — `Drawer.tsx`.
- [x] No color-only status: tool errors show text plus `isError` styling; SSE reconnect shows a `role="status"` banner (`App.tsx:191`).
- [ ] **Manual follow-up:** keyboard-only browser pass through sidebar → composer → task center → memory/skills → knowledge; verify focus visibility and focus return with a real browser. No GUI browser was available in this environment, so this is **not claimed as signed off**.

## Content and tone

- [x] Buttons use action labels in Chinese ("新建会话", "取消计划", "添加附件", "归档", "恢复", "删除") — reviewed component JSX.
- [x] Errors/toasts are concise and actionable; no ambiguous standalone "确定"/"提交" labels found in the reviewed components.

## Anti-patterns

- [x] No gradients, heavy shadows, mixed radii, raw hex in components, hidden focus outlines, or color-only meaning found in `web/src/**`.

## QA gate

- [x] `cd web && npm run typecheck` — clean.
- [x] `cd web && npm run check` — Biome clean after `npm run format`.
- [x] `cd web && npm run build` — Vite 8 build emits hashed assets into `web/dist`.

## Sign-off

Code-level `minimal` compliance: **signed off** (2026-11-25). Interactive WCAG keyboard/visual pass: **not signed off** — requires a human with a browser; see the two manual follow-up boxes above.
