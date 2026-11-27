package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSkipsCommentsAndUnquotes(t *testing.T) {
	text := `# comment
PORT=8080
export DATA_DIR="/tmp/pida data"
LLM_MODEL='gpt test'
NOTIFY_MODE=errors # not a comment
OBSIDIAN_ALLOW=AI Learning, X Ideas
`
	values, order := Parse(text)
	if values["PORT"] != "8080" {
		t.Fatalf("PORT = %q", values["PORT"])
	}
	if values["DATA_DIR"] != "/tmp/pida data" {
		t.Fatalf("DATA_DIR = %q", values["DATA_DIR"])
	}
	if values["LLM_MODEL"] != "gpt test" {
		t.Fatalf("LLM_MODEL = %q", values["LLM_MODEL"])
	}
	if values["NOTIFY_MODE"] != "errors # not a comment" {
		t.Fatalf("NOTIFY_MODE = %q", values["NOTIFY_MODE"])
	}
	if values["OBSIDIAN_ALLOW"] != "AI Learning, X Ideas" {
		t.Fatalf("OBSIDIAN_ALLOW = %q", values["OBSIDIAN_ALLOW"])
	}
	if len(order) != 5 {
		t.Fatalf("order = %v", order)
	}
}

func TestMergePreservesValuesAndCustomKeys(t *testing.T) {
	existing := `PORT=8080
DEMO_PASSWORD="hunter two"
LLM_MODEL=local-model
MY_CUSTOM=kept
`
	merged, report := Merge(existing)

	values, _ := Parse(merged)
	if values["PORT"] != "8080" || values["LLM_MODEL"] != "local-model" {
		t.Fatalf("values not preserved: %v", values)
	}
	if values["DEMO_PASSWORD"] != "hunter two" {
		t.Fatalf("quoted value not preserved: %q", values["DEMO_PASSWORD"])
	}
	if values["MY_CUSTOM"] != "kept" {
		t.Fatalf("custom key not preserved")
	}
	if report.Preserved != 2 {
		t.Fatalf("Preserved = %d", report.Preserved)
	}
	if report.Custom != 2 {
		t.Fatalf("Custom = %d", report.Custom)
	}
	if !strings.Contains(merged, "# pida configuration file, version 1") {
		t.Fatalf("missing template header")
	}
	if !strings.Contains(merged, "# ---------------------------------------------------------------- custom") {
		t.Fatalf("missing custom section")
	}
}

func TestMergeUncommentsDocumentedOptionalKeys(t *testing.T) {
	merged, report := Merge("DEV=1\nNODE_ENV=production\n")
	values, _ := Parse(merged)
	if values["DEV"] != "1" || values["NODE_ENV"] != "production" {
		t.Fatalf("optional keys not activated: %v", values)
	}
	if report.Preserved != 2 {
		t.Fatalf("Preserved = %d", report.Preserved)
	}
}

func TestMergeAddsMissingKeys(t *testing.T) {
	merged, report := Merge("PORT=9000\n")
	values, _ := Parse(merged)
	if values["HTTP_MODE"] != "http" {
		t.Fatalf("HTTP_MODE = %q", values["HTTP_MODE"])
	}
	if values["ARTIFACT_MAX_TOTAL_BYTES"] != "4GB" {
		t.Fatalf("ARTIFACT_MAX_TOTAL_BYTES = %q", values["LLM_CONTEXT_WINDOW"])
	}
	if report.Added == 0 {
		t.Fatalf("expected added keys, report = %+v", report)
	}
}

func TestWriteExampleRefusesOverwriteWithoutUpdate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pida.env")
	if err := os.WriteFile(path, []byte("PORT=7000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteExample(path, false); !errors.Is(err, ErrConfigExists) {
		t.Fatalf("err = %v", err)
	}
	report, err := WriteExample(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Updated || report.Path != path {
		t.Fatalf("report = %+v", report)
	}
	values, _ := Parse(readFile(t, path))
	if values["PORT"] != "7000" {
		t.Fatalf("PORT = %q", values["PORT"])
	}
}

func TestWriteExampleCreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "pida.env")
	report, err := WriteExample(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if report.Updated {
		t.Fatalf("report = %+v", report)
	}
	values, _ := Parse(readFile(t, path))
	if values["PORT"] != "3000" {
		t.Fatalf("PORT = %q", values["PORT"])
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
