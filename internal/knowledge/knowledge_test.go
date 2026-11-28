package knowledge

import (
	"strings"
	"testing"
	"sort"
)

func TestCreateAndListVaults(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)

	// Create a vault
	vault, err := store.CreateVault("Test Vault", "A test knowledge vault")
	if err != nil {
		t.Fatal(err)
	}
	if vault.Name != "Test Vault" {
		t.Fatalf("vault name %q, want %q", vault.Name, "Test Vault")
	}
	if vault.Description != "A test knowledge vault" {
		t.Fatalf("vault description %q, want %q", vault.Description, "A test knowledge vault")
	}

	// List vaults
	vaults, err := store.ListVaults()
	if err != nil {
		t.Fatal(err)
	}
	if len(vaults) != 1 {
		t.Fatalf("expected 1 vault, got %d", len(vaults))
	}
	if vaults[0].ID != vault.ID {
		t.Fatalf("vault ID %q, want %q", vaults[0].ID, vault.ID)
	}

	// Write a file to the vault
	_, err = store.WriteFile(vault.ID, "note.txt", "Hello Knowledge")
	if err != nil {
		t.Fatal(err)
	}

	// List files
	files, err := store.ListFiles(vault.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Path != "note.txt" {
		t.Fatalf("file path %q, want %q", files[0].Path, "note.txt")
	}

	// Read file
	content, ok, err := store.ReadFile(vault.ID, "note.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("file not found")
	}
	if content != "Hello Knowledge" {
		t.Fatalf("content %q, want %q", content, "Hello Knowledge")
	}

	// Delete vault
	err = store.DeleteVault(vault.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Verify vault is deleted
	vaults, err = store.ListVaults()
	if err != nil {
		t.Fatal(err)
	}
	if len(vaults) != 0 {
		t.Fatalf("expected 0 vaults after deletion, got %d", len(vaults))
	}
}

func TestCreateAndListFiles(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)

	// Create a vault
	vault, err := store.CreateVault("My Vault", "")
	if err != nil {
		t.Fatal(err)
	}

	// Write multiple files
	_, err = store.WriteFile(vault.ID, "doc1.md", "# Document 1\nContent here")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.WriteFile(vault.ID, "doc2.md", "# Document 2\nMore content")
	if err != nil {
		t.Fatal(err)
	}

	// List files
	files, err := store.ListFiles(vault.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}

	// Sort check
	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.Path
	}
	sort.Strings(paths)
	if paths[0] != "doc1.md" || paths[1] != "doc2.md" {
		t.Fatalf("unexpected file order: %v", paths)
	}

	// Read a file
	content, ok, err := store.ReadFile(vault.ID, "doc1.md")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("file not found")
	}
	if !strings.HasPrefix(content, "# Document 1") {
		t.Fatalf("unexpected content: %q", content)
	}

	// Delete vault
	err = store.DeleteVault(vault.ID)
	if err != nil {
		t.Fatal(err)
	}
}