package artifacts

import (
	"context"
	"path/filepath"
	"testing"
)

func TestConfirmationStoreLifecycle(t *testing.T) {
	ctx := context.Background()
	store := NewConfirmationStore(filepath.Join(t.TempDir(), "confirmations.json"))

	proposed, err := store.Propose(ctx, "art_1", "pin", "keep this report")
	if err != nil {
		t.Fatal(err)
	}
	if proposed.Status != "pending" || proposed.Action != "pin" {
		t.Fatalf("unexpected proposal: %+v", proposed)
	}

	// A duplicate request for the same artifact and action dedupes.
	again, err := store.Propose(ctx, "art_1", "pin", "duplicate")
	if err != nil || again.ID != proposed.ID {
		t.Fatalf("duplicate proposal: %+v err=%v", again, err)
	}

	listed, err := store.List(ctx)
	if err != nil || len(listed) != 1 {
		t.Fatalf("list: %d err=%v", len(listed), err)
	}

	resolved, err := store.Resolve(ctx, proposed.ID, "approved", "tester")
	if err != nil || resolved.Status != "approved" {
		t.Fatalf("resolve: %+v err=%v", resolved, err)
	}
	if _, err := store.Resolve(ctx, proposed.ID, "rejected", "tester"); err == nil {
		t.Fatal("resolving twice should fail")
	}
	if _, err := store.Resolve(ctx, proposed.ID, "invalid", "tester"); err == nil {
		t.Fatal("invalid status should fail")
	}
	if _, err := store.Propose(ctx, "art_1", "bogus", ""); err == nil {
		t.Fatal("invalid action should fail")
	}
}
