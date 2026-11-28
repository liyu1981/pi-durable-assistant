package tasks

import (
	"testing"
)

func TestNormalizeTitle(t *testing.T) {
	cases := map[string]string{
		"  `Solar System`  ": "Solar System",
		"**Plan**: research": "Plan: research",
		"line one\nline two": "line one line two",
		"":                   "",
	}
	for input, want := range cases {
		if got := NormalizeTitle(input); got != want {
			t.Fatalf("NormalizeTitle(%q) = %q, want %q", input, got, want)
		}
	}
	long := ""
	for i := 0; i < 80; i++ {
		long += "a"
	}
	if got := NormalizeTitle(long); len([]rune(got)) != 50 {
		t.Fatalf("long title length = %d, want 50", len([]rune(got)))
	}
}
