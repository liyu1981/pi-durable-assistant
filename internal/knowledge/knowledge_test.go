package knowledge

import (
	"context"
	"strings"
	"testing"
)

func TestVaultLifecycle(t *testing.T) {
	ctx := context.Background()
	store := NewStore(t.TempDir())

	vault, err := store.CreateVault(ctx, "  Research  ", " Papers I read ")
	if err != nil {
		t.Fatal(err)
	}
	if vault.Name != "Research" || vault.Description != "Papers I read" {
		t.Fatalf("fields not trimmed: %+v", vault)
	}

	vaults, err := store.ListVaults(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(vaults) != 1 || vaults[0].ID != vault.ID {
		t.Fatalf("unexpected vaults: %+v", vaults)
	}

	updated, err := store.UpdateVault(ctx, vault.ID, "Reading", "Updated description")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Reading" || updated.Description != "Updated description" {
		t.Fatalf("update not applied: %+v", updated)
	}

	if err := store.DeleteVault(ctx, vault.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetVault(ctx, vault.ID); err == nil {
		t.Fatal("deleted vault must be gone")
	}
	if err := store.DeleteVault(ctx, vault.ID); err == nil {
		t.Fatal("deleting a missing vault must fail")
	}
}

func TestVaultValidation(t *testing.T) {
	ctx := context.Background()
	store := NewStore(t.TempDir())
	if _, err := store.CreateVault(ctx, "   ", "x"); err == nil {
		t.Fatal("empty name must be rejected")
	}
	if _, err := store.CreateVault(ctx, strings.Repeat("a", maxNameLen+1), ""); err == nil {
		t.Fatal("over-long name must be rejected")
	}
	if _, err := store.CreateVault(ctx, "ok", strings.Repeat("d", maxDescriptionLen+1)); err == nil {
		t.Fatal("over-long description must be rejected")
	}
	if _, err := store.UpdateVault(ctx, "kv_missing", "name", ""); err != ErrVaultNotFound {
		t.Fatalf("err = %v, want ErrVaultNotFound", err)
	}
}

func TestWriteListReadDeleteFiles(t *testing.T) {
	ctx := context.Background()
	store := NewStore(t.TempDir())
	vault, err := store.CreateVault(ctx, "Docs", "")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.WriteFile(ctx, vault.ID, "notes/todo.md", []byte("# Todo\n- one")); err != nil {
		t.Fatal(err)
	}
	// Arbitrary binary content is stored byte for byte.
	binary := []byte{0x89, 0x50, 0x4e, 0x47, 0x00, 0xff}
	if _, err := store.WriteFile(ctx, vault.ID, "logo.png", binary); err != nil {
		t.Fatal(err)
	}

	files, err := store.ListFiles(ctx, vault.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0].Path != "logo.png" || files[1].Path != "notes/todo.md" {
		t.Fatalf("unexpected files: %+v", files)
	}
	if files[0].MIMEType != "image/png" {
		t.Fatalf("mime = %q, want image/png", files[0].MIMEType)
	}

	data, ok, err := store.ReadFile(ctx, vault.ID, "notes/todo.md")
	if err != nil || !ok || string(data) != "# Todo\n- one" {
		t.Fatalf("read = %q ok=%v err=%v", data, ok, err)
	}
	if !IsText(data) {
		t.Fatal("markdown should be detected as text")
	}
	raw, ok, err := store.ReadFile(ctx, vault.ID, "logo.png")
	if err != nil || !ok || string(raw) != string(binary) {
		t.Fatalf("binary read lost bytes: %v %v", ok, err)
	}
	if IsText(raw) {
		t.Fatal("png bytes must not be detected as text")
	}
	if _, ok, _ := store.ReadFile(ctx, vault.ID, "missing.txt"); ok {
		t.Fatal("missing file must report ok=false")
	}

	// Totals are recomputed for display.
	after, err := store.GetVault(ctx, vault.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.FileCount != 2 || after.SizeBytes != int64(len(data)+len(binary)) {
		t.Fatalf("totals = %d files, %d bytes", after.FileCount, after.SizeBytes)
	}

	if err := store.DeleteFile(ctx, vault.ID, "logo.png"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := store.ReadFile(ctx, vault.ID, "logo.png"); ok {
		t.Fatal("deleted file still readable")
	}
	if err := store.DeleteFile(ctx, vault.ID, "logo.png"); err == nil {
		t.Fatal("deleting a missing file must fail")
	}
}

func TestFilePathsCannotEscapeTheVault(t *testing.T) {
	ctx := context.Background()
	store := NewStore(t.TempDir())
	vault, err := store.CreateVault(ctx, "Safe", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"", ".", "..", "../escape.txt", "a/../../escape.txt",
		"/etc/passwd", "a\\b.txt", "dir/../../escape.txt",
	} {
		if _, err := store.WriteFile(ctx, vault.ID, name, []byte("x")); err == nil {
			t.Fatalf("write %q must be rejected", name)
		}
		if _, ok, err := store.ReadFile(ctx, vault.ID, name); err == nil || ok {
			t.Fatalf("read %q must be rejected", name)
		}
	}
	if _, err := store.WriteFile(ctx, "kv_unknown", "a.txt", []byte("x")); err != ErrVaultNotFound {
		t.Fatalf("err = %v, want ErrVaultNotFound", err)
	}
}
