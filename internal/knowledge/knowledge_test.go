package knowledge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchAndRead(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "AI学习"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "AI学习", "note.md")
	if err := os.WriteFile(path, []byte("第一行\nPi Durable 很好用\n第三行\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	roots := AllowedRoots(root, []string{"AI学习"})
	hits, err := Search(roots, "Durable", false)
	if err != nil || len(hits) != 1 || hits[0].Line != 2 {
		t.Fatalf("unexpected hits: %+v %v", hits, err)
	}
	page, err := Read(roots, "note.md", 2)
	if err != nil || !strings.HasPrefix(page, "2\t") {
		t.Fatalf("unexpected page: %q %v", page, err)
	}
	if _, err := Read(roots, "../secret.txt", 1); err == nil {
		t.Fatal("path traversal must be rejected")
	}
}

func TestDisabledKnowledge(t *testing.T) {
	if roots := AllowedRoots("", nil); roots != nil {
		t.Fatalf("expected no roots, got %v", roots)
	}
	if Status("")["enabled"] != false {
		t.Fatal("status should report disabled")
	}
}
