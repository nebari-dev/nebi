package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/nebari-dev/nebi/internal/limits"
	"time"
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

// Both sides of the HOME scoping branch are pinned: strict/permissive redirect
// it into the workspace, off leaves the parent's alone. TMPDIR is scoped in
// every mode, so it is checked here too.
func TestCommand_ActiveModesCreateJobScopedHomeAndTmp(t *testing.T) {
	for _, mode := range []Mode{ModeStrict, ModePermissive} {
		t.Run(string(mode), func(t *testing.T) {
			r := newTestRunner(t, mode)
			ws := t.TempDir()

			cmd, err := r.Command(context.Background(), Spec{
				WorkspaceDir: ws,
				Argv:         []string{"/usr/bin/pixi", "lock"},
				ParentEnv:    []string{"PATH=/bin", "HOME=/home/dev", "TMPDIR=/var/tmp"},
			})
			if err != nil {
				t.Fatalf("Command: %v", err)
			}

			wantHome := "HOME=" + filepath.Join(ws, ".nebi", "home")
			wantTmp := "TMPDIR=" + filepath.Join(ws, ".nebi", "tmp")
			if !slices.Contains(cmd.Env, wantHome) || !slices.Contains(cmd.Env, wantTmp) {
				t.Fatalf("expected %q and %q in env, got %v", wantHome, wantTmp, cmd.Env)
			}
			if slices.Contains(cmd.Env, "HOME=/home/dev") {
				t.Fatalf("parent HOME must not survive an active sandbox: %v", cmd.Env)
			}
			for _, dir := range []string{filepath.Join(ws, ".nebi", "home"), filepath.Join(ws, ".nebi", "tmp")} {
				if _, err := statDir(dir); err != nil {
					t.Fatalf("expected %q to exist: %v", dir, err)
				}
			}
		})
	}
}

// In off mode the build is unconfined anyway, so redirecting HOME buys no
// security and costs real things: for local workspaces the workspace dir is
// the user's own project folder, and pixi/rattler lose
// $HOME/.rattler/credentials.json.
//
// TMPDIR is a separate matter. internal/process scopes it into the workspace
// for every build, sandboxed or not, so off mode keeps that rather than
// inheriting the parent's.
func TestCommand_OffModeLeavesParentHomeAlone(t *testing.T) {
	r := newTestRunner(t, ModeOff)
	ws := t.TempDir()

	cmd, err := r.Command(context.Background(), Spec{
		WorkspaceDir: ws,
		Argv:         []string{"/usr/bin/pixi", "lock"},
		ParentEnv:    []string{"PATH=/bin", "HOME=/home/dev", "TMPDIR=/var/tmp"},
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}

	if !slices.Contains(cmd.Env, "HOME=/home/dev") {
		t.Fatalf("expected HOME to pass through in off mode, got %v", cmd.Env)
	}
	if slices.Contains(cmd.Env, "TMPDIR=/var/tmp") {
		t.Fatalf("expected TMPDIR to be scoped to the workspace, got %v", cmd.Env)
	}
	if want := "TMPDIR=" + filepath.Join(ws, ".nebi", "tmp"); !slices.Contains(cmd.Env, want) {
		t.Fatalf("expected %q, got %v", want, cmd.Env)
	}
	// Off mode must not create the sandbox-only HOME.
	if _, err := os.Stat(filepath.Join(ws, ".nebi", "home")); !os.IsNotExist(err) {
		t.Fatalf("off mode must not create .nebi/home in the workspace (err=%v)", err)
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

// In off mode the shim never runs, so exit code 125 can only have come from
// the build tool itself and must not be reported as a sandbox failure.
func TestRunner_IsSetupFailure_OffModeNeverClaimsSetupFailure(t *testing.T) {
	err := &exitCodeErr{code: SetupFailureExitCode}

	if newTestRunner(t, ModeOff).IsSetupFailure(err) {
		t.Fatal("off mode never runs the shim, so 125 is the build's own exit code")
	}
	for _, mode := range []Mode{ModeStrict, ModePermissive} {
		if !newTestRunner(t, mode).IsSetupFailure(err) {
			t.Fatalf("%s mode must recognise 125 as a setup failure", mode)
		}
		if newTestRunner(t, mode).IsSetupFailure(&exitCodeErr{code: 1}) {
			t.Fatalf("%s mode must not treat exit 1 as a setup failure", mode)
		}
		if newTestRunner(t, mode).IsSetupFailure(nil) {
			t.Fatalf("%s mode must not treat nil as a setup failure", mode)
		}
	}
}

// selfPath is only used to build the shim argv, which off mode never does,
// so a failed os.Executable lookup must not stop the server from booting.
func TestNewRunner_OffModeSkipsExecutableLookup(t *testing.T) {
	r, err := NewRunner(Config{Mode: ModeOff}, "")
	if err != nil {
		t.Fatalf("off mode must not need the nebi binary path: %v", err)
	}
	if r.selfPath != "" {
		t.Fatalf("expected no self path to be resolved in off mode, got %q", r.selfPath)
	}
}

// The regression test for the CI fork bomb: a Go test binary that enables an
// active sandbox mode resolves os.Executable() to itself, and re-exec'ing it
// runs the whole test suite again instead of the shim. NewRunner must refuse
// to construct rather than hand back a Runner that will do that.
//
// shimProbeEnv is set here so the check short-circuits without spawning
// anything; the probe mechanism itself is covered by the verifyShim tests.
func TestNewRunner_ActiveModeRejectsAnExecutableThatIsNotTheShim(t *testing.T) {
	t.Setenv(shimProbeEnv, "1")

	for _, mode := range []Mode{ModeStrict, ModePermissive} {
		_, err := NewRunner(Config{Mode: mode}, "")
		if err == nil {
			t.Fatalf("%s mode must refuse an executable that does not implement the shim", mode)
		}
		self, _ := os.Executable()
		if self != "" && !strings.Contains(err.Error(), self) {
			t.Fatalf("error must name the resolved path %q so an operator can see it, got: %v", self, err)
		}
	}
}

func writeFakeBinary(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell-script probe targets are not runnable on windows")
	}
	p := filepath.Join(t.TempDir(), "fakebin")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	return p
}

func TestVerifyShim_AcceptsABinaryThatAnswersCheck(t *testing.T) {
	// Also pins the probe argv: a target that is handed anything other than
	// "sandbox-exec --check" fails loudly rather than passing by accident.
	bin := writeFakeBinary(t, `[ "$1" = "sandbox-exec" ] && [ "$2" = "--check" ] && exit 0; exit 9`)

	if err := verifyShim(bin); err != nil {
		t.Fatalf("expected a shim that answers --check to be accepted, got %v", err)
	}
}

func TestVerifyShim_RejectsABinaryThatDoesNotAnswerCheck(t *testing.T) {
	bin := writeFakeBinary(t, `echo "I am a test binary, not the shim"; exit 1`)

	err := verifyShim(bin)
	if err == nil {
		t.Fatal("expected a non-shim binary to be rejected")
	}
	// The target's own output is the most useful diagnostic, so keep it.
	if !strings.Contains(err.Error(), "not the shim") {
		t.Fatalf("expected the probe output in the error, got %v", err)
	}
}

// The probe target is an arbitrary binary and may print a great deal. None
// of it belongs verbatim in an error string.
func TestVerifyShim_TruncatesALoudFailure(t *testing.T) {
	bin := writeFakeBinary(t, `head -c 200000 /dev/zero | tr '\0' 'x'; exit 1`)

	err := verifyShim(bin)
	if err == nil {
		t.Fatal("expected a failing probe target to be rejected")
	}
	if len(err.Error()) > 1000 {
		t.Fatalf("probe output was not truncated: error is %d bytes", len(err.Error()))
	}
}

// A Go test binary handed "sandbox-exec" does not fail: flag parsing stops at
// the first non-flag argument, so it runs its whole suite. The probe must
// time out on that rather than wait forever.
func TestVerifyShim_RejectsABinaryThatHangs(t *testing.T) {
	bin := writeFakeBinary(t, `sleep 30`)

	restore := shimProbeTimeout
	shimProbeTimeout = 250 * time.Millisecond
	defer func() { shimProbeTimeout = restore }()

	start := time.Now()
	err := verifyShim(bin)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected a hanging probe target to be rejected")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected a timeout error, got %v", err)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("probe did not honour its timeout, took %s", elapsed)
	}
}

func TestVerifyShim_RejectsAMissingBinary(t *testing.T) {
	if err := verifyShim(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Fatal("expected a missing probe target to be rejected")
	}
}

// Caps accidental recursion at depth one: a probe child that reaches this
// code is by definition not a shim, because a real shim exits at --check
// long before it could build a Runner.
func TestVerifyShim_RefusesToProbeFromInsideAProbe(t *testing.T) {
	bin := writeFakeBinary(t, `exit 0`)
	t.Setenv(shimProbeEnv, "1")

	err := verifyShim(bin)
	if err == nil {
		t.Fatal("a probe child must not itself probe")
	}
	if !strings.Contains(err.Error(), "probe") {
		t.Fatalf("expected the error to explain the recursion guard, got %v", err)
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

// TestCommand_AppliesResourceLimits pins the seam between this package and
// internal/process: a Spec carrying resource limits must produce a command that
// actually enforces them. The two features arrived independently (env scrubbing
// and Landlock here, rlimits there) and both wrap the same exec.Cmd, so a
// regression would silently drop one layer while the other kept working.
//
// The limits are applied by a /bin/sh wrapper that ulimits and then execs, so
// the real binary must still be the thing that ends up running, and the
// workspace env must survive the extra hop.
func TestCommand_AppliesResourceLimits(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the resource-limit wrapper is unix-only")
	}
	ws := t.TempDir()
	script := filepath.Join(ws, "show-limits")
	// -H -f reports the hard file-size limit in 512-byte blocks.
	if err := os.WriteFile(script, []byte("#!/bin/sh\nulimit -H -f\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	r := newTestRunner(t, ModeOff)
	cmd, err := r.Command(context.Background(), Spec{
		WorkspaceDir:   ws,
		Argv:           []string{script},
		ParentEnv:      []string{"PATH=/bin:/usr/bin"},
		ResourceLimits: limits.ProcessLimits{FileBytes: 512 * 4096},
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("run: %v (output %q)", err, out)
	}
	if got := strings.TrimSpace(string(out)); got != "4096" {
		t.Fatalf("file-size rlimit not applied: ulimit -H -f reported %q, want %q", got, "4096")
	}
}

// The zero value must mean "no limits", not "a limit of zero", which would
// make every build fail the moment it wrote a byte.
func TestCommand_ZeroResourceLimitsLeaveCommandUnwrapped(t *testing.T) {
	ws := t.TempDir()
	r := newTestRunner(t, ModeOff)
	cmd, err := r.Command(context.Background(), Spec{
		WorkspaceDir: ws,
		Argv:         []string{"/usr/bin/pixi", "lock"},
		ParentEnv:    []string{"PATH=/bin"},
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	if cmd.Path != "/usr/bin/pixi" {
		t.Fatalf("expected the real binary to run unwrapped, got %q with args %v", cmd.Path, cmd.Args)
	}
}
