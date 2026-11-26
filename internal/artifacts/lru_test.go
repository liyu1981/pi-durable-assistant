package artifacts

import (
	"context"
	"testing"
	"time"
)

func TestSweepEvictsLeastRecentlyUsedUnpinned(t *testing.T) {
	ctx := context.Background()
	store := NewStore(t.TempDir())

	old, err := store.CreateArtifact(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.WriteFile(ctx, old.ID, "file.txt", []byte("1234567890")); err != nil {
		t.Fatal(err)
	}
	newer, err := store.CreateArtifact(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.WriteFile(ctx, newer.ID, "file.txt", []byte("1234567890")); err != nil {
		t.Fatal(err)
	}
	// The newer artifact is the LRU: it has not been touched since creation.
	// Touch the old one explicitly so it wins.
	time.Sleep(5 * time.Millisecond)
	if err := store.Touch(ctx, old.ID); err != nil {
		t.Fatal(err)
	}

	removed, err := store.Sweep(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != newer.ID {
		t.Fatalf("evicted %v, want [%s]", removed, newer.ID)
	}
	total, err := store.Total(ctx)
	if err != nil || total != 10 {
		t.Fatalf("total after sweep: %d err=%v", total, err)
	}
}

func TestSweepSkipsPinnedArtifacts(t *testing.T) {
	ctx := context.Background()
	store := NewStore(t.TempDir())

	pinned, err := store.CreateArtifact(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.WriteFile(ctx, pinned.ID, "file.txt", []byte("1234567890")); err != nil {
		t.Fatal(err)
	}
	if err := store.SetPinned(ctx, pinned.ID, true); err != nil {
		t.Fatal(err)
	}
	unpinned, err := store.CreateArtifact(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.WriteFile(ctx, unpinned.ID, "file.txt", []byte("1234567890")); err != nil {
		t.Fatal(err)
	}

	removed, err := store.Sweep(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != unpinned.ID {
		t.Fatalf("evicted %v, want [%s]", removed, unpinned.ID)
	}
	if _, ok, err := store.ReadFile(ctx, pinned.ID, "file.txt"); err != nil || !ok {
		t.Fatalf("pinned artifact was evicted: ok=%v err=%v", ok, err)
	}
	if total, _ := store.Total(ctx); total != 0 {
		t.Fatalf("pinned artifact counted in total: %d", total)
	}
}

func TestMaxTotalBytesFromEnv(t *testing.T) {
	for input, want := range map[string]int64{
		"":       DefaultMaxTotalBytes,
		"1024":   1024,
		"2MB":    2 * 1024 * 1024,
		"1.5GB":  0, // invalid: whole numbers only
		"3gb":    3 * 1024 * 1024 * 1024,
		"10KB":   10 * 1024,
		"-1":     0,
		"broken": 0,
	} {
		got, err := MaxTotalBytesFromEnv(input)
		if want == 0 {
			if err == nil {
				t.Fatalf("%q: expected error, got %d", input, got)
			}
			continue
		}
		if err != nil || got != want {
			t.Fatalf("%q: got %d err=%v, want %d", input, got, err, want)
		}
	}
}
