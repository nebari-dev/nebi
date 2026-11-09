//go:build e2e

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseProjectPackageArg(t *testing.T) {
	cases := map[string][2]string{
		"numpy":           {"numpy", "*"},
		"python=3.12.*":   {"python", "3.12.*"},
		"numpy>=2.0":      {"numpy", ">=2.0"},
		"numpy=>=2.0":     {"numpy", ">=2.0"},
		"rich==13.7.1":    {"rich", "==13.7.1"},
		" scipy ~= 1.11 ": {"scipy", "~=1.11"},
		"pandas=":         {"pandas", "*"},
	}
	for in, want := range cases {
		name, constraint := parseProjectPackageArg(in)
		if name != want[0] || constraint != want[1] {
			t.Errorf("%q: got (%q, %q), want %v", in, name, constraint, want)
		}
	}
}

// hostPlatform is this machine's pixi platform.
func hostPlatform() string {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "darwin/arm64":
		return "osx-arm64"
	case "darwin/amd64":
		return "osx-64"
	case "linux/arm64":
		return "linux-aarch64"
	case "windows/amd64":
		return "win-64"
	}
	return "linux-64"
}

// TestE2E_ProjectCreateShowListDelete runs the projects commands against
// the local SDK with real pixi (an empty environment, so the solve is
// quick).
func TestE2E_ProjectCreateShowListDelete(t *testing.T) {
	if _, err := exec.LookPath("pixi"); err != nil {
		t.Skip("pixi not on PATH")
	}
	setupLocalStore(t)
	dir := t.TempDir()

	res := runCLI(t, dir, "project", "create", "cli-proj", "--platform", hostPlatform(), "--tag", "baseline")
	if res.ExitCode != 0 {
		t.Fatalf("create failed:\nstdout: %s\nstderr: %s", res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "Created cli-proj") {
		t.Errorf("expected create confirmation, got stderr: %s", res.Stderr)
	}
	projectDir := filepath.Join(os.Getenv("NEBI_DATA_DIR"), "projects", "cli-proj")
	if _, err := os.Stat(filepath.Join(projectDir, "pixi.lock")); err != nil {
		t.Fatalf("pixi.lock not written: %v", err)
	}

	res = runCLI(t, dir, "project", "create", "CLI-PROJ", "--platform", hostPlatform())
	if res.ExitCode == 0 || !strings.Contains(res.Stderr, "A project with that name already exists.") {
		t.Errorf("duplicate name should fail, got exit %d stderr: %s", res.ExitCode, res.Stderr)
	}

	res = runCLI(t, dir, "project", "show", "cli-proj")
	if res.ExitCode != 0 || !strings.Contains(res.Stdout, "not installed") || !strings.Contains(res.Stdout, "v1") {
		t.Errorf("show: exit %d\nstdout: %s\nstderr: %s", res.ExitCode, res.Stdout, res.Stderr)
	}

	res = runCLI(t, dir, "project", "list", "--json")
	var rows []map[string]any
	if err := json.Unmarshal([]byte(res.Stdout), &rows); err != nil || len(rows) != 1 {
		t.Fatalf("list --json: %v\n%s", err, res.Stdout)
	}
	if rows[0]["name"] != "cli-proj" || rows[0]["latest_version"] != float64(1) || rows[0]["missing"] != false {
		t.Errorf("list row: %v", rows[0])
	}

	res = runCLI(t, dir, "project", "delete", "cli-proj", "--yes")
	if res.ExitCode != 0 {
		t.Fatalf("delete failed:\nstdout: %s\nstderr: %s", res.Stdout, res.Stderr)
	}
	if _, err := os.Stat(projectDir); !os.IsNotExist(err) {
		t.Errorf("project folder should be removed, stat err %v", err)
	}
	res = runCLI(t, dir, "project", "list", "--json")
	if strings.TrimSpace(res.Stdout) != "[]" {
		t.Errorf("list after delete: %s", res.Stdout)
	}
}
