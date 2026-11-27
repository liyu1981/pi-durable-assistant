package sqlutil

import "testing"

func TestFormatAcceptsValidFragments(t *testing.T) {
	got, err := Format("PRAGMA user_version = %d", Int(3))
	if err != nil || got != "PRAGMA user_version = 3" {
		t.Fatalf("format: %q, %v", got, err)
	}
	got, err = Format("SELECT %s FROM %s", Ident("id"), Ident("app_llm_profiles"))
	if err != nil || got != "SELECT id FROM app_llm_profiles" {
		t.Fatalf("format identifiers: %q, %v", got, err)
	}
}

func TestFormatRejectsUnsafeValues(t *testing.T) {
	if _, err := Format("SELECT %s", Ident("id; DROP TABLE users")); err == nil {
		t.Fatal("unsafe identifier accepted")
	}
	if _, err := Format("SELECT %s", "id"); err == nil {
		t.Fatal("raw string accepted")
	}
}
