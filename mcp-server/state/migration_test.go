package state

import "testing"

func TestMigrate_V0ToV2(t *testing.T) {
	s := &State{Version: 0}
	result := Migrate(s)
	if result != s {
		t.Fatal("Migrate must return the same pointer")
	}
	if result.Version != 2 {
		t.Fatalf("expected Version == 2 after migrating from v0, got %d", result.Version)
	}
}

func TestMigrate_V1ToV2(t *testing.T) {
	s := &State{Version: 1}
	result := Migrate(s)
	if result != s {
		t.Fatal("Migrate must return the same pointer")
	}
	if result.Version != 2 {
		t.Fatalf("expected Version == 2 after migrating from v1, got %d", result.Version)
	}
}

func TestMigrate_V2Idempotent(t *testing.T) {
	s := &State{Version: 2, SpecName: "test-spec"}
	result := Migrate(s)
	if result != s {
		t.Fatal("Migrate must return the same pointer")
	}
	if result.Version != 2 {
		t.Fatalf("expected Version == 2 to remain unchanged, got %d", result.Version)
	}
	if result.SpecName != "test-spec" {
		t.Fatalf("Migrate must not alter other fields; SpecName changed to %q", result.SpecName)
	}
}
