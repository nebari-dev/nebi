package pixi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseVersion(t *testing.T) {
	for _, c := range []struct {
		in   string
		want [3]int
	}{
		{"pixi 0.82.1\n", [3]int{0, 82, 1}},
		{"0.84.2", [3]int{0, 84, 2}},
		{"pixi 0.83.0-dev", [3]int{0, 83, 0}},
		{"pixi 1.0.0+build.5", [3]int{1, 0, 0}},
	} {
		got, err := ParseVersion(c.in)
		if err != nil || got != c.want {
			t.Errorf("ParseVersion(%q) = %v, %v; want %v", c.in, got, err, c.want)
		}
	}
	for _, in := range []string{"", "pixi", "pixi 0.82"} {
		if _, err := ParseVersion(in); err == nil {
			t.Errorf("ParseVersion(%q) must fail", in)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	for _, c := range []struct {
		a, b [3]int
		want int
	}{
		{[3]int{0, 58, 0}, [3]int{0, 82, 1}, -1},
		{[3]int{0, 82, 0}, [3]int{0, 82, 1}, -1},
		{[3]int{0, 82, 1}, [3]int{0, 82, 1}, 0},
		{[3]int{0, 100, 0}, [3]int{0, 82, 1}, 1},
		{[3]int{1, 0, 0}, [3]int{0, 99, 99}, 1},
	} {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%v, %v) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// TestPinnedVersionMeetsMinimum keeps the version nebi installs at or
// above the version it requires.
func TestPinnedVersionMeetsMinimum(t *testing.T) {
	pinned, err := ParseVersion(pixiVersion)
	if err != nil {
		t.Fatal(err)
	}
	minimum, err := ParseVersion(MinimumVersion)
	if err != nil {
		t.Fatal(err)
	}
	if compareVersions(pinned, minimum) < 0 {
		t.Fatalf("pixiVersion %s is older than MinimumVersion %s", pixiVersion, MinimumVersion)
	}
}

func TestCheckVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses shell-script stubs")
	}
	stub := func(t *testing.T, script string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "pixi")
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		return path
	}
	ctx := context.Background()

	if v, err := CheckVersion(ctx, stub(t, "echo pixi 0.84.2")); err != nil || v != "0.84.2" {
		t.Fatalf("pixi 0.84.2: got %q, %v", v, err)
	}
	if v, err := CheckVersion(ctx, stub(t, "echo pixi "+MinimumVersion)); err != nil || v != MinimumVersion {
		t.Fatalf("the minimum version itself must pass: got %q, %v", v, err)
	}

	old := stub(t, "echo pixi 0.58.0")
	v, err := CheckVersion(ctx, old)
	var tooOld *VersionTooOldError
	if !errors.As(err, &tooOld) || !errors.Is(err, ErrVersionTooOld) || v != "0.58.0" {
		t.Fatalf("pixi 0.58.0: want a *VersionTooOldError, got %q, %v", v, err)
	}
	if tooOld.Path != old || tooOld.Found != "0.58.0" || tooOld.Minimum != MinimumVersion {
		t.Fatalf("got %+v", tooOld)
	}

	if _, err := CheckVersion(ctx, stub(t, "echo hello")); err == nil || errors.Is(err, ErrVersionTooOld) {
		t.Fatalf("no version: want a plain error, got %v", err)
	}
	_, err = CheckVersion(ctx, stub(t, "echo broken >&2; exit 3"))
	if err == nil || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("a failing binary: want an error with its stderr, got %v", err)
	}
	if _, err := CheckVersion(ctx, filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("a missing binary must fail")
	}
}
