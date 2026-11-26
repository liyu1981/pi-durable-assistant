package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/liyu1981/pi-durable-assistant/internal/config"
	"github.com/liyu1981/pi-durable-assistant/internal/service"
)

func printServiceInstallUsage(fs *flag.FlagSet) {
	fmt.Println(`Usage: pida service-install [options]

Install a systemd user service that runs "pida serve" with the given data
directory. The service expects the binary at ~/.local/bin/pida.

Options:`)
	fs.SetOutput(os.Stdout)
	fs.PrintDefaults()
	fs.SetOutput(io.Discard)
}

func runServiceInstall(args []string) error {
	fs := flag.NewFlagSet("service-install", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	force := fs.Bool("force", false, "overwrite an existing unit file")
	printOnly := fs.Bool("print", false, "print the unit file instead of installing it")
	binary := fs.String("binary", "", "pida binary path (default ~/.local/bin/pida)")
	dataDir := fs.String("data-dir", "", "data directory (default ~/.pida)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printServiceInstallUsage(fs)
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("service-install: unexpected argument %q", fs.Arg(0))
	}

	binPath := *binary
	if binPath == "" {
		var err error
		if binPath, err = service.DefaultBinary(); err != nil {
			return err
		}
	}
	binPath, err := filepath.Abs(binPath)
	if err != nil {
		return err
	}
	dir := *dataDir
	if dir == "" {
		if dir, err = config.DefaultDir(); err != nil {
			return err
		}
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return err
	}

	unit := service.Unit(binPath, dir)
	if *printOnly {
		fmt.Print(unit)
		return nil
	}

	unitPath, err := service.Path()
	if err != nil {
		return err
	}
	if _, statErr := os.Stat(unitPath); statErr == nil && !*force {
		return fmt.Errorf("%s already exists (use --force to overwrite it)", unitPath)
	}
	if err := os.MkdirAll(filepath.Dir(unitPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(unitPath, []byte(unit), 0o644); err != nil {
		return err
	}

	if _, statErr := os.Stat(binPath); statErr != nil {
		fmt.Fprintf(os.Stderr, "warning: %s not found; build it with:\n  CGO_ENABLED=0 go build -o %s ./cmd/pida\n\n", binPath, binPath)
	}

	fmt.Printf("Installed %s\n\n", unitPath)
	fmt.Println("Next steps:")
	fmt.Println("  systemctl --user daemon-reload")
	fmt.Println("  systemctl --user enable --now pida.service")
	fmt.Println("  systemctl --user status pida.service")
	fmt.Println()
	fmt.Println("Logs: journalctl --user -u pida.service -f")
	fmt.Println("To keep it running after logout: loginctl enable-linger $USER")
	return nil
}

func printServiceUninstallUsage(fs *flag.FlagSet) {
	fmt.Println(`Usage: pida service-uninstall [options]

Remove the systemd user service unit installed by pida service-install.

Options:`)
	fs.SetOutput(os.Stdout)
	fs.PrintDefaults()
	fs.SetOutput(io.Discard)
}

func runServiceUninstall(args []string) error {
	fs := flag.NewFlagSet("service-uninstall", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printServiceUninstallUsage(fs)
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("service-uninstall: unexpected argument %q", fs.Arg(0))
	}

	unitPath, err := service.Path()
	if err != nil {
		return err
	}
	if err := os.Remove(unitPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Printf("%s is not installed.\n", unitPath)
			return nil
		}
		return err
	}
	fmt.Printf("Removed %s\n\n", unitPath)
	fmt.Println("Next steps:")
	fmt.Println("  systemctl --user disable --now pida.service")
	fmt.Println("  systemctl --user daemon-reload")
	return nil
}
