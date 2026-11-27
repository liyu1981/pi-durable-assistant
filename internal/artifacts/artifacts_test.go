package artifacts

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreLifecycle(t *testing.T) {
	ctx := context.Background()
	store := NewStore(t.TempDir())

	created, err := store.CreateArtifact(ctx)
	if err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	if created.ID == "" {
		t.Fatal("create artifact returned empty id")
	}
	if created.SizeBytes != 0 {
		t.Fatalf("new artifact size = %d, want 0", created.SizeBytes)
	}

	if _, ok, err := store.ReadFile(ctx, created.ID, "notes.md"); err != nil || ok {
		t.Fatalf("read missing file: ok=%v err=%v", ok, err)
	}

	updated, err := store.WriteFile(ctx, created.ID, "notes.md", []byte("hello artifact"))
	if err != nil {
		t.Fatalf("write file: %v", err)
	}
	if updated.SizeBytes != int64(len("hello artifact")) {
		t.Fatalf("artifact size = %d, want %d", updated.SizeBytes, len("hello artifact"))
	}

	data, ok, err := store.ReadFile(ctx, created.ID, "notes.md")
	if err != nil || !ok || string(data) != "hello artifact" {
		t.Fatalf("read file: ok=%v data=%q err=%v", ok, data, err)
	}

	files, err := store.ListFiles(ctx, created.ID)
	if err != nil || len(files) != 1 {
		t.Fatalf("list files: len=%d err=%v", len(files), err)
	}
	if files[0].Path != "notes.md" || string(files[0].Content) != "hello artifact" {
		t.Fatalf("unexpected file: %+v", files[0])
	}

	if err := store.SetPinned(ctx, created.ID, true); err != nil {
		t.Fatalf("pin artifact: %v", err)
	}
	if size, err := store.Size(ctx, created.ID); err != nil || size != int64(len("hello artifact")) {
		t.Fatalf("artifact size: size=%d err=%v", size, err)
	}
	total, err := store.Total(ctx)
	if err != nil || total != 0 {
		t.Fatalf("pinned artifact must be excluded from the total: total=%d err=%v", total, err)
	}

	if err := store.DeleteArtifact(ctx, created.ID); err != nil {
		t.Fatalf("delete artifact: %v", err)
	}
	if _, ok, err := store.ReadFile(ctx, created.ID, "notes.md"); err != nil || ok {
		t.Fatalf("read after delete: ok=%v err=%v", ok, err)
	}
	if _, err := os.Stat(filepath.Join(store.root, created.ID)); !os.IsNotExist(err) {
		t.Fatalf("content dir still exists after delete: %v", err)
	}
}

func TestStoreMoveFile(t *testing.T) {
	ctx := context.Background()
	store := NewStore(t.TempDir())
	created, err := store.CreateArtifact(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.WriteFile(ctx, created.ID, "draft/note.md", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	updated, err := store.MoveFile(ctx, created.ID, "draft/note.md", "final/note.md")
	if err != nil {
		t.Fatal(err)
	}
	if updated.SizeBytes != 5 {
		t.Fatalf("size after move = %d", updated.SizeBytes)
	}
	if _, ok, err := store.ReadFile(ctx, created.ID, "draft/note.md"); err != nil || ok {
		t.Fatalf("source still exists after move: ok=%v err=%v", ok, err)
	}
	data, ok, err := store.ReadFile(ctx, created.ID, "final/note.md")
	if err != nil || !ok || string(data) != "hello" {
		t.Fatalf("destination missing: ok=%v data=%q err=%v", ok, data, err)
	}
	if _, err := store.WriteFile(ctx, created.ID, "final/other.md", []byte("replace me")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MoveFile(ctx, created.ID, "final/note.md", "final/other.md"); err != nil {
		t.Fatalf("move over existing destination: %v", err)
	}
	data, _, err = store.ReadFile(ctx, created.ID, "final/other.md")
	if err != nil || string(data) != "hello" {
		t.Fatalf("destination was not overwritten: %q err=%v", data, err)
	}
	if _, err := store.MoveFile(ctx, created.ID, "missing", "other"); err == nil {
		t.Fatal("moving a missing source must fail")
	}
}

func TestStoreRejectsUnsafePaths(t *testing.T) {
	ctx := context.Background()
	store := NewStore(t.TempDir())
	created, err := store.CreateArtifact(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../escape.txt", "/absolute.txt", "a/../../escape.txt", "", ".", "back\\slash.txt"} {
		if _, err := store.WriteFile(ctx, created.ID, name, []byte("x")); err == nil {
			t.Fatalf("write accepted unsafe path %q", name)
		}
		if _, err := store.MoveFile(ctx, created.ID, "safe.txt", name); err == nil {
			t.Fatalf("move accepted unsafe destination %q", name)
		}
	}
	if err := store.DeleteArtifact(ctx, "../outside"); err == nil {
		t.Fatal("delete accepted unsafe artifact id")
	}
}
