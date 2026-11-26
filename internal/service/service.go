// Package service renders and locates the systemd user unit for pida.
package service

import (
	"fmt"
	"os"
	"path/filepath"
)

// UnitName is the systemd unit file name.
const UnitName = "pida.service"

// Unit renders a systemd user unit that runs the given binary and data dir.
func Unit(binary, dataDir string) string {
	return fmt.Sprintf(`[Unit]
Description=Pi Durable Assistant
Documentation=https://github.com/liyu1981/pi-durable-assistant
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s serve
Environment=DATA_DIR=%s
Restart=on-failure
RestartSec=3

[Install]
WantedBy=default.target
`, binary, dataDir)
}

// Path returns the systemd user unit path, honoring XDG_CONFIG_HOME.
func Path() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "systemd", "user", UnitName), nil
}

// DefaultBinary returns ~/.local/bin/pida, the path the unit expects.
func DefaultBinary() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "bin", "pida"), nil
}
