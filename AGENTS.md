# Collaboration rules (Go rewrite)

- The current implementation and run instructions live in `README.md`; the rewrite plan and decisions live in `doc/REWRITE_PLAN.md`.
- Backend APIs are defined by the pinned `github.com/MichaelKinsy/PiG` v0.7.0 source in the Go module cache (`go doc`, `pkg.go.dev`). Do not invent APIs from memory; verify signatures against the pinned version.
- The frontend is React 19 + TypeScript 7 + Vite 8 + Tailwind 4 + Jotai 3 + Biome 2. Do not add ESLint, Prettier, or typescript-eslint.
- The `minimal` design tokens in `web/src/styles/tokens.css` are authoritative; prefer semantic tokens over raw values.
- Never read or print `.env` files or secrets. Never access remote servers except dependency downloads and the explicit dev-time faux provider on loopback.
- Before finishing: run `make check` (`go test ./...`, `go build`), `cd web && npm run typecheck && npm run check && npm run build`, and report what was and was not verified. CI passing is not production validation.
