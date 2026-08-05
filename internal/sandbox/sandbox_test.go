package sandbox

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func newTestRunner(t *testing.T, mode Mode) *Runner {
	t.Helper()
	r, err := NewRunner(Config{Mode: mode, AllowedPorts: []int{443}}, "/opt/nebi/nebi")
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	return r
}

func TestCommand_OffModeRunsRealArgvWithScrubbedEnv(t *testing.T) {
	r := newTestRunner(t, ModeOff)
	ws := t.TempDir()

	cmd, err := r.Command(context.Background(), Spec{
		WorkspaceDir: ws,
		Argv:         []string{"/usr/bin/pixi", "install", "-v"},
		ParentEnv:    []string{"PATH=/bin", "NEBI_DATABASE_DSN=secret"},
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}

	if cmd.Path != "/usr/bin/pixi" {
		t.Fatalf("expected the real binary, got %q", cmd.Path)
	}
	if !slices.Equal(cmd.Args, []string{"/usr/bin/pixi", "install", "-v"}) {
		t.Fatalf("argv should be unchanged in off mode, got %v", cmd.Args)
	}
	for _, kv := range cmd.Env {
		if strings.HasPrefix(kv, "NEBI_DATABASE_DSN=") {
			t.Fatal("off mode must still scrub the environment")
		}
	}
	if cmd.Dir != ws {
		t.Fatalf("expected Dir=%q, got %q", ws, cmd.Dir)
	}
}

func TestCommand_StrictModeWrapsInShim(t *testing.T) {
	r := newTestRunner(t, ModeStrict)
	ws := t.TempDir()

	cmd, err := r.Command(context.Background(), Spec{
		WorkspaceDir: ws,
		Argv:         []string{"/usr/bin/pixi", "install", "-v"},
		ParentEnv:    []string{"PATH=/bin"},
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}

	if cmd.Args[0] != "/opt/nebi/nebi" || cmd.Args[1] != "sandbox-exec" {
		t.Fatalf("expected the shim to lead the argv, got %v", cmd.Args)
	}
	if !slices.Contains(cmd.Args, "--allow-rw="+ws) {
		t.Fatalf("expected the workspace as an RW root, got %v", cmd.Args)
	}
	if !slices.Contains(cmd.Args, "--allow-port=443") {
		t.Fatalf("expected the configured port, got %v", cmd.Args)
	}
	if !slices.Contains(cmd.Args, "--mode=strict") {
		t.Fatalf("expected the mode to be forwarded, got %v", cmd.Args)
	}

	sep := slices.Index(cmd.Args, "--")
	if sep < 0 {
		t.Fatalf("expected a -- separator, got %v", cmd.Args)
	}
	if !slices.Equal(cmd.Args[sep+1:], []string{"/usr/bin/pixi", "install", "-v"}) {
		t.Fatalf("expected the real argv after --, got %v", cmd.Args[sep+1:])
	}
}

func TestCommand_CreatesJobScopedHomeAndTmp(t *testing.T) {
	r := newTestRunner(t, ModeOff)
	ws := t.TempDir()

	cmd, err := r.Command(context.Background(), Spec{
		WorkspaceDir: ws,
		Argv:         []string{"/usr/bin/pixi", "lock"},
		ParentEnv:    []string{"PATH=/bin"},
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}

	wantHome := "HOME=" + filepath.Join(ws, ".nebi-home")
	wantTmp := "TMPDIR=" + filepath.Join(ws, ".nebi-tmp")
	if !slices.Contains(cmd.Env, wantHome) || !slices.Contains(cmd.Env, wantTmp) {
		t.Fatalf("expected %q and %q in env, got %v", wantHome, wantTmp, cmd.Env)
	}
	for _, dir := range []string{filepath.Join(ws, ".nebi-home"), filepath.Join(ws, ".nebi-tmp")} {
		if _, err := statDir(dir); err != nil {
			t.Fatalf("expected %q to exist: %v", dir, err)
		}
	}
}

func TestCommand_RejectsRelativeWorkspaceDir(t *testing.T) {
	r := newTestRunner(t, ModeStrict)

	_, err := r.Command(context.Background(), Spec{
		WorkspaceDir: "relative/path",
		Argv:         []string{"/usr/bin/pixi", "lock"},
	})
	if err == nil {
		t.Fatal("expected an error for a relative workspace dir")
	}
}

func TestCommand_RejectsEmptyArgv(t *testing.T) {
	r := newTestRunner(t, ModeStrict)

	_, err := r.Command(context.Background(), Spec{WorkspaceDir: t.TempDir()})
	if err == nil {
		t.Fatal("expected an error for empty argv")
	}
}

func TestIsSetupFailure(t *testing.T) {
	if !IsSetupFailure(&exitCodeErr{code: SetupFailureExitCode}) {
		t.Fatal("expected exit code 125 to be a setup failure")
	}
	if IsSetupFailure(&exitCodeErr{code: 1}) {
		t.Fatal("exit code 1 is a build failure, not a setup failure")
	}
	if IsSetupFailure(nil) {
		t.Fatal("nil is not a setup failure")
	}
}

type exitCodeErr struct{ code int }

func (e *exitCodeErr) Error() string { return "exit status" }
func (e *exitCodeErr) ExitCode() int { return e.code }
