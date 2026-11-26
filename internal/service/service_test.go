package service

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestUnitRunsServeWithDataDir(t *testing.T) {
	unit := Unit("/home/u/.local/bin/pida", "/home/u/.pida")
	for _, want := range []string{
		"ExecStart=/home/u/.local/bin/pida serve",
		"Environment=DATA_DIR=/home/u/.pida",
		"WantedBy=default.target",
		"Restart=on-failure",
	} {
		if !strings.Contains(unit, want) {
			t.Fatalf("unit missing %q:\n%s", want, unit)
		}
	}
}

func TestPathUsesXDGConfigHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg")
	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("/tmp/xdg", "systemd", "user", UnitName)
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}

func TestDefaultBinary(t *testing.T) {
	t.Setenv("HOME", "/home/u")
	binary, err := DefaultBinary()
	if err != nil {
		t.Fatal(err)
	}
	if binary != filepath.Join("/home/u", ".local", "bin", "pida") {
		t.Fatalf("binary = %q", binary)
	}
}
