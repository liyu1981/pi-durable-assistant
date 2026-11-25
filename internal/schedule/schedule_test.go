package schedule

import "testing"

func TestNormalize(t *testing.T) {
	got, err := Normalize(map[string]any{"text": " 提醒 ", "minutesFromNow": 1, "everyMinutes": 5}, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "提醒" || got.At != 61000 || got.EveryMinutes != 5 {
		t.Fatalf("unexpected schedule: %+v", got)
	}
	for _, input := range []map[string]any{
		{"text": "", "minutesFromNow": 1},
		{"text": "x", "minutesFromNow": 0},
		{"text": "x", "minutesFromNow": 1, "everyMinutes": 0.5},
	} {
		if _, err := Normalize(input, 0); err == nil {
			t.Fatalf("expected error for %v", input)
		}
	}
}

func TestNextOccurrenceAndMatches(t *testing.T) {
	if got := NextOccurrence(1000, 0, 2000); got != nil {
		t.Fatalf("one-off schedule should not advance: %v", *got)
	}
	got := NextOccurrence(0, 1, 150_000)
	if got == nil || *got != 180_000 {
		t.Fatalf("unexpected next occurrence: %v", got)
	}
	if !Matches("active", 100, 2, 100, 2) {
		t.Fatal("expected match")
	}
	if Matches("paused", 100, 2, 100, 2) {
		t.Fatal("paused schedule must not match")
	}
}

func TestNormalizeRequestId(t *testing.T) {
	got, err := NormalizeRequestId("abc")
	if err != nil || got == nil || *got != "abc" {
		t.Fatalf("unexpected result: %v %v", got, err)
	}
	if _, err := NormalizeRequestId("__proto__"); err == nil {
		t.Fatal("reserved name must be rejected")
	}
	if _, err := NormalizeRequestId(42); err == nil {
		t.Fatal("non-string must be rejected")
	}
}
