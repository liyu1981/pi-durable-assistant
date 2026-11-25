# Collaboration rules (Go rewrite)

- Current implementation and run instructions: `README.md`. Rewrite plan and decisions: `doc/REWRITE_PLAN.md`.
- Backend APIs are defined by the pinned `github.com/MichaelKinsy/PiG` v0.7.0 source in the Go module cache (`go doc`, `pkg.go.dev`). Verify signatures against the pinned version; do not invent APIs.
- Frontend: React 19 + TypeScript 7 + Vite 8 + Tailwind 4 + Jotai 3 + Biome 2. Use `pnpm` only; do not add ESLint, Prettier, or typescript-eslint.
- The frontend uses shadcn/ui components with the Luma preset (code `b2D0wqNxT`). Its theme lives in `web/src/styles/tokens.css`; use shadcn primitives and semantic tokens, and do not reintroduce the retired `minimal` design. No Chinese text in project files or UI.
- Never read or print `.env` files or secrets. Never access remote servers except dependency downloads and the explicit dev-time faux provider on loopback.
- Do not run `git commit` or `git push` without an explicit user request; commit once when asked, then stop and report status.
- Before finishing: run `pnpm check` (`go test ./...`, `go build`, web typecheck/Biome/build) and report what was and was not verified. CI passing is not production validation.
