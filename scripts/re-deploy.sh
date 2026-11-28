set -x

pnpm build
systemctl --user stop pida.service
cp ~/pi-durable-assistant/dist/pida ~/.local/bin/
systemctl --user start pida.service
