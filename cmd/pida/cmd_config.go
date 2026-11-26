package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/liyu1981/pi-durable-assistant/internal/config"
)

func printConfigUsage(fs *flag.FlagSet) {
	fmt.Println(`Usage: pida config [options]

Write an example configuration file with comments for every option. The file
is created as pida.env in the current directory.

Options:`)
	fs.SetOutput(os.Stdout)
	fs.PrintDefaults()
	fs.SetOutput(io.Discard)
}

func runConfig(args []string) error {
	fs := flag.NewFlagSet("config", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	update := fs.Bool("update", false, "upgrade an existing file, preserving configured values")
	path := fs.String("path", "pida.env", "file to write")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printConfigUsage(fs)
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("config: unexpected argument %q", fs.Arg(0))
	}

	report, err := config.WriteExample(*path, *update)
	if err != nil {
		return err
	}
	if report.Updated {
		fmt.Printf("Upgraded %s to config version %d: %d value(s) preserved, %d new setting(s) added.\n",
			report.Path, report.Version, report.Preserved, report.Added)
	} else {
		fmt.Printf("Wrote %s (config version %d).\n", report.Path, report.Version)
	}
	if report.Custom > 0 {
		fmt.Printf("Kept %d custom setting(s) at the end of the file.\n", report.Custom)
	}

	absolute, err := filepath.Abs(report.Path)
	if err != nil {
		absolute = report.Path
	}
	defaultPath, defaultErr := config.DefaultFile()
	fmt.Println()
	if defaultErr == nil && absolute == defaultPath {
		fmt.Println("Edit the file, then start the server with:")
		fmt.Println("  pida serve")
		return nil
	}
	fmt.Println("Edit the file, then use it with:")
	fmt.Printf("  pida serve --config %s\n", absolute)
	if defaultErr == nil {
		fmt.Println("Or install it as the default:")
		fmt.Printf("  mkdir -p %s && cp %s %s\n", filepath.Dir(defaultPath), absolute, defaultPath)
	}
	return nil
}
