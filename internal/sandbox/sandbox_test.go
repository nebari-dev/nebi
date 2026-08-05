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

func TestReadOnlyPaths_GrantsSystemDirsButNotEtc(t *testing.T) {
	r := newTestRunner(t, ModeStrict)
	ws := t.TempDir()

	// Use a binary path whose directory is guaranteed to exist so the
	// "directory holding the package manager" entry is exercised.
	binDir := t.TempDir()
	binary := filepath.Join(binDir, "pixi")

	cmd, err := r.Command(context.Background(), Spec{
		WorkspaceDir: ws,
		Argv:         []string{binary, "install"},
		ParentEnv:    []string{"PATH=/bin"},
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}

	var roDirs []string
	for _, arg := range cmd.Args {
		if dir, ok := strings.CutPrefix(arg, "--allow-ro="); ok {
			roDirs = append(roDirs, dir)
		}
	}
	if len(roDirs) == 0 {
		t.Fatalf("expected at least one --allow-ro flag, got %v", cmd.Args)
	}
	if !slices.Contains(roDirs, binDir) {
		t.Fatalf("expected the binary's directory %q to be readable, got %v", binDir, roDirs)
	}
	// Granting /etc wholesale would expose /etc/nebi/config.yaml, which can
	// hold the database DSN and the JWT secret.
	if slices.Contains(roDirs, "/etc") {
		t.Fatalf("/etc must not be granted wholesale, got %v", roDirs)
	}
}

func TestReadOnlyFiles_GrantsResolverFilesOnly(t *testing.T) {
	for _, f := range readOnlyFiles() {
		if !filepath.IsAbs(f) {
			t.Fatalf("expected absolute paths, got %q", f)
		}
		if !strings.HasPrefix(f, "/etc/") {
			t.Fatalf("unexpected read-only file %q", f)
		}
		if f == "/etc/nebi/config.yaml" {
			t.Fatal("the nebi config file must never be readable by a build")
		}
	}
}

func TestShimArgv_EmitsReadOnlyFileFlags(t *testing.T) {
	files := readOnlyFiles()
	if len(files) == 0 {
		t.Skip("no candidate read-only files exist on this host")
	}

	r := newTestRunner(t, ModeStrict)
	cmd, err := r.Command(context.Background(), Spec{
		WorkspaceDir: t.TempDir(),
		Argv:         []string{"/usr/bin/pixi", "install"},
		ParentEnv:    []string{"PATH=/bin"},
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	for _, f := range files {
		if !slices.Contains(cmd.Args, "--allow-ro-file="+f) {
			t.Fatalf("expected --allow-ro-file=%s, got %v", f, cmd.Args)
		}
	}
}

func TestRestrictions_Validate(t *testing.T) {
	if err := (Restrictions{}).Validate(); err == nil {
		t.Fatal("expected an error when no RW path is given")
	}
	if err := (Restrictions{RW: []string{"/ws"}}).Validate(); err != nil {
		t.Fatalf("expected a single RW path to be valid, got %v", err)
	}
	if err := (Restrictions{RW: []string{"relative"}}).Validate(); err == nil {
		t.Fatal("expected an error for a relative RW path")
	}
	if err := (Restrictions{RW: []string{"/ws"}, RO: []string{"rel"}}).Validate(); err == nil {
		t.Fatal("expected an error for a relative RO path")
	}
	if err := (Restrictions{RW: []string{"/ws"}, ROFiles: []string{"rel"}}).Validate(); err == nil {
		t.Fatal("expected an error for a relative RO file")
	}
	if err := (Restrictions{RW: []string{"/ws"}, RWFiles: []string{"rel"}}).Validate(); err == nil {
		t.Fatal("expected an error for a relative RW file")
	}
	// 70000 would wrap to 4464 in the uint16 conversion, silently opening a
	// port nobody asked for.
	if err := (Restrictions{RW: []string{"/ws"}, TCPConnectPorts: []int{70000}}).Validate(); err == nil {
		t.Fatal("expected an error for an out-of-range TCP port")
	}
	if err := (Restrictions{RW: []string{"/ws"}, TCPConnectPorts: []int{0}}).Validate(); err == nil {
		t.Fatal("expected an error for TCP port 0")
	}
	if err := (Restrictions{RW: []string{"/ws"}, TCPConnectPorts: []int{443}}).Validate(); err != nil {
		t.Fatalf("expected port 443 to be valid, got %v", err)
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
