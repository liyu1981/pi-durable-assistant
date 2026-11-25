# `minimal` design audit

Source: `liyu1981/awesome-design-skills/skills/minimal` (DESIGN.md + SKILL.md). The UI was rebuilt from scratch on these tokens; no legacy visual language remains. Tokens live only in `web/src/styles/tokens.css` (Tailwind v4 `@theme` + self-hosted `@fontsource/*`).

Audit date: 2026-11-25. Method: source inspection of `web/src/**` plus `pnpm check`. Items needing a human at a browser are marked as manual follow-up, not claimed as verified.

## Tokens

- [x] Exact palette in `tokens.css`: `#0C0C09`, `#312C85`, `#16A34A`, `#D97706`, `#DC2626`, `#F4F4F1`, `#0C0C09`, `#F4F4F1`.
- [x] Typography: Inter display (`h1` 3rem), Open Sans body (1rem), Inconsolata labels (0.75rem uppercase/tracked), self-hosted.
- [x] Spacing: only 4/8/12/16/24/32. Radii: 4px/8px only.
- [x] No raw hex values outside `tokens.css`; no gradients, shadows, or text shadows (`grep` confirmed none).
- [x] Flat surface panels with thin borders and a restrained indigo accent; no legacy dark/light app chrome, emoji decoration, or pill-heavy styling.

## Components and interaction

- [x] Shared primitives in `src/components/ui.tsx` (section headings, buttons, badges, empty states) keep the visual language consistent.
- [x] Sidebar: active/archived tabs, new conversation, fork, archive/restore/remove with confirmation, archive counts, explicit empty states.
- [x] Chat thread: user/assistant/tool entries, tool-call badges, live streaming text, usage and task badges, attachment chips, system notices for `[Reminder]`, `[Scheduled task]`, and `[Background task result]`.
- [x] Composer: 5-file limit, PNG/JPEG/WebP/GIF and text files, base64, drag/drop, paste, image compression, size caps, steering, stop, IME-aware Enter-to-send.
- [x] Status panel, task center drawer (schedules/background/reminders with pause/resume/cancel/stop and Markdown export), memory/skills panel, knowledge search/read, SSE reconnect, toasts.
- [x] Hover/active/disabled/loading/error states defined through token-based utilities; no color-only status (each badge carries text).

## Accessibility

- [x] Global 2px `--color-secondary` focus ring on all interactive elements; reduced-motion support.
- [x] Keyboard: Enter sends, Shift+Enter newline, Escape closes the drawer, drawer traps Tab and restores focus on close.
- [x] `aria-label`/`role` on sidebar, navigation, composer, attachments, dialog, tabs, toasts, live chat region; `aria-live` for feedback.
- [x] Empty, loading and error messages present and written in concise, confident English.
- [ ] **Manual follow-up:** interactive keyboard-only and contrast pass in a real browser. No GUI browser was available here, so this is not signed off.

## Content

- [x] All UI text and comments in English. A repository-wide CJK scan (excluding `tmp/`, `node_modules`, `dist`) returned zero matches.
- [x] Buttons use action labels ("New conversation", "Cancel schedule", "Add attachment", "Archive", "Restore", "Delete"), not ambiguous standalone confirmations.

## Quality gate

- [x] `pnpm check` — Go tests/build, `tsc --noEmit`, Biome check, and Vite build all pass.
- [x] End-to-end chat flow with the dev-time faux provider returns English todos, tool results, and the final `Demo task complete.` answer.

**Sign-off:** code-level `minimal` compliance is complete. Interactive browser keyboard/visual pass remains a manual follow-up.
