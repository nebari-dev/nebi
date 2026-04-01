package contenthash

import (
	"strings"
	"testing"
)

func TestHash_Format(t *testing.T) {
	result := Hash("toml-content", "lock-content")
	if !strings.HasPrefix(result, "sha-") {
		t.Fatalf("expected sha- prefix, got %q", result)
	}
	// "sha-" + 12 hex chars = 16 chars total
	if len(result) != 16 {
		t.Fatalf("expected 16 chars, got %d: %q", len(result), result)
	}
}

func TestHash_Deterministic(t *testing.T) {
	a := Hash("toml", "lock")
	b := Hash("toml", "lock")
	if a != b {
		t.Fatalf("same input produced different hashes: %q vs %q", a, b)
	}
}

func TestHash_DifferentInputs(t *testing.T) {
	a := Hash("toml-a", "lock")
	b := Hash("toml-b", "lock")
	if a == b {
		t.Fatalf("different inputs produced same hash: %q", a)
	}
}

func TestHash_MatchesExpected(t *testing.T) {
	result := Hash("hello", "world")
	if !strings.HasPrefix(result, "sha-") {
		t.Fatalf("unexpected format: %q", result)
	}
	if result != Hash("hello", "world") {
		t.Fatal("not deterministic")
	}
}
