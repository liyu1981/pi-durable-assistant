# Production deployment (systemd user service)

This project deploys as one static Go binary with the web UI embedded. The
production instance runs as the `pida.service` systemd **user** service.

## Paths

| Item | Path |
| --- | --- |
| Source checkout | `/home/yli/pi-durable-assistant` |
| Built binary | `/home/yli/pi-durable-assistant/dist/pida` |
| Installed binary | `/home/yli/.local/bin/pida` |
| Service unit | `/home/yli/.config/systemd/user/pida.service` |
| State directory | `/home/yli/.pida` (`DATA_DIR=/home/yli/.pida`) |
| Config file | `/home/yli/.pida/pida.env` |
| SQLite database | `/home/yli/.pida/agent.sqlite` |
| Artifacts | `/home/yli/.pida/artifacts` |

Model providers are configured in the app and stored in
`agent.sqlite` as named profiles. There is no `LLM_*` environment config path.

## Deploy steps

1. Review the target commit and working tree:

   ```bash
   cd /home/yli/pi-durable-assistant
   git status --short
   git log --oneline -5
   ```

2. Run checks before building:

   ```bash
   pnpm check
   ```

   This runs `go test ./...`, `go build`, web typecheck, Biome, and the Vite
   build.

3. Stop the service **before** replacing the binary:

   ```bash
   systemctl --user stop pida.service
   systemctl --user is-active pida.service   # inactive
   ```

4. Build the release binary:

   ```bash
   pnpm build
   ```

   `pnpm build` installs web dependencies, builds the web assets, builds the
   Go binary into `dist/pida`, and embeds `web/dist` into it. No separate copy
   of the frontend is needed on the server.

5. Replace the installed binary:

   ```bash
   install -m 755 dist/pida /home/yli/.local/bin/pida
   sha256sum dist/pida /home/yli/.local/bin/pida   # hashes must match
   /home/yli/.local/bin/pida version
   ```

6. Start the service:

   ```bash
   systemctl --user start pida.service
   systemctl --user status pida.service --no-pager
   ```

7. Verify startup from the journal:

   ```bash
   journalctl --user -u pida.service --no-pager -n 20
   ```

   A healthy start logs:

   ```text
   pida started: http://127.0.0.1:8444, model <active model>, PID <pid>
   ```

## Service management

```bash
systemctl --user status pida.service --no-pager
systemctl --user stop pida.service
systemctl --user start pida.service
systemctl --user restart pida.service
journalctl --user -u pida.service -f
```

The unit runs:

```ini
ExecStart=/home/yli/.local/bin/pida serve
Environment=DATA_DIR=/home/yli/.pida
Restart=on-failure
RestartSec=3
```

Do not edit the unit unless changing the binary path or data directory.

## Configuration

`pida serve` reads `/home/yli/.pida/pida.env` when it exists. Precedence is
command-line flags, process environment, config file, then built-in defaults.
Useful keys included in the generated template are `PORT`, `HTTP_MODE`,
`HTTPS_PORT`, `TLS_CERT_FILE`, `TLS_KEY_FILE`, `DEMO_PASSWORD`, and
`ARTIFACT_MAX_TOTAL_BYTES`.

Generate or refresh the documented template with:

```bash
/home/yli/.local/bin/pida config --update --path /home/yli/.pida/pida.env
```

Review the resulting file before restarting; never commit or print secrets.

Model provider profiles are managed in the app under System settings → Model
providers, or through `/api/settings/llm/profiles`. After deploying a new
binary, verify the active profile survives the restart (state is in SQLite).

## Rollback

If the new binary fails to start:

1. Restore the previous `~/.local/bin/pida` binary if a backup was kept.
2. Start the service with `systemctl --user start pida.service`.
3. Check `journalctl --user -u pida.service --no-pager -n 50`.

This deployment process does not create an automatic binary backup; keep one
outside `dist/` if rollback is required.

## Notes

- The deployed UI is embedded in the binary; rebuilding only the Go package
  without `pnpm web:build` would serve stale frontend assets.
- `dist/` is gitignored and disposable.
- Never run the development server (`pnpm dev`) to validate production. Build
  and service checks above are the deployment validation.
- CI passing is not production validation.
