package local

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestNameFormat(t *testing.T) {
	valid := []string{
		"a", "0", "demo", "my-analysis", "data.v2_final", "0abc", "a..b", "a-", "x_",
		strings.Repeat("a", 63),
	}
	invalid := []string{
		"", "MyEnv", "Demo", "my env", "-x", ".x", "_x", "a/b", `a\b`, "a:b", "ä", "x\n", " demo", "demo ",
		strings.Repeat("a", 64), "..", ".",
	}
	for _, n := range valid {
		if err := checkNameFormat(n); err != nil {
			t.Errorf("%q should be valid: %v", n, err)
		}
	}
	for _, n := range invalid {
		if err := checkNameFormat(n); !errors.Is(err, ErrInvalidName) {
			t.Errorf("%q: want ErrInvalidName, got %v", n, err)
		}
	}
}

func TestValidateProjectName(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	// A legacy row with an uppercase name, as the migration keeps them.
	legacy := &Project{Name: "Demo", Path: "/legacy/demo"}
	h.insertRow(legacy)

	if err := h.svc.ValidateProjectName(ctx, "demo", nil); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("demo vs existing Demo: want ErrNameTaken, got %v", err)
	}
	if err := h.svc.ValidateProjectName(ctx, "demo", &legacy.ID); err != nil {
		t.Fatalf("excluding the project's own ID: %v", err)
	}
	other := legacy.ID
	other[0] ^= 0xff
	if err := h.svc.ValidateProjectName(ctx, "demo", &other); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("excluding another ID: want ErrNameTaken, got %v", err)
	}
	if err := h.svc.ValidateProjectName(ctx, "demo-2", nil); err != nil {
		t.Fatalf("free name: %v", err)
	}
	if err := h.svc.ValidateProjectName(ctx, "Bad Name", nil); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("want ErrInvalidName, got %v", err)
	}
}
