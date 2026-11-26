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
