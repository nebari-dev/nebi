# Sandboxed Environment Builds Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Run every untrusted pixi build with a scrubbed environment and, on Linux, confined by Landlock to its own workspace directory, so build code cannot read the database password or touch another workspace's files (https://github.com/nebari-dev/nebi/issues/445).

**Architecture:** A new `internal/sandbox` package is the single place that constructs pixi subprocess commands. It always replaces the inherited environment with an allowlist. When sandboxing is active it rewrites the argv to `nebi sandbox-exec --allow-rw=<workspace> ... -- <real argv>`; that hidden subcommand applies a Landlock ruleset to itself and then `syscall.Exec`s the real command, so the confinement is inherited by pixi and everything it spawns. The executor and the pixi package call the sandbox runner instead of `exec.CommandContext` directly.

**Tech Stack:** Go 1.24, cobra, viper, `github.com/landlock-lsm/go-landlock` (pure Go, no cgo), Landlock LSM (Linux 5.13+ for filesystem, 6.7+ for TCP).

**Spec:** `docs/superpowers/specs/2026-08-05-build-sandbox-design.md`

---

## File Structure

| File | Responsibility |
|---|---|
| `internal/config/config.go` (modify) | Add `SandboxConfig`, defaults, `BindEnv`, mode resolution |
| `internal/config/config_test.go` (modify) | Mode-resolution tests |
| `internal/sandbox/sandbox.go` (create) | `Runner`, `Spec`, `Command`, exit-code helpers |
| `internal/sandbox/env.go` (create) | Environment allowlist construction |
| `internal/sandbox/env_test.go` (create) | Allowlist unit tests |
| `internal/sandbox/sandbox_test.go` (create) | Argv/wrapping unit tests |
| `internal/sandbox/landlock_linux.go` (create) | Landlock ruleset application (build tag `linux`) |
| `internal/sandbox/landlock_other.go` (create) | Non-Linux stub returning `ErrUnsupported` |
| `internal/sandbox/confine_test.go` (create) | Linux-only containment integration test (the #445 acceptance test) |
| `cmd/nebi/sandbox_exec.go` (create) | Hidden `sandbox-exec` cobra subcommand |
| `cmd/nebi/main.go` (modify) | Register the subcommand |
| `internal/executor/local.go` (modify) | Hold a `*sandbox.Runner`; use it at the two exec sites; inject into pixi manager |
| `internal/pkgmgr/pixi/pixi.go` (modify) | Optional `Runner`; use it at the six exec sites |
| `internal/worker/worker.go` (modify) | Per-job build timeout + setup-failure error mapping |
| `config.yaml.example` (modify) | Document the `sandbox` block |
| `docs/docs/server-setup.md` (modify) | Operator documentation |

---

### Task 1: Sandbox configuration

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/config/config_test.go`:

```go
func TestLoad_SandboxDefaultsToStrictInTeamMode(t *testing.T) {
	isolate(t)
	t.Setenv("NEBI_MODE", "team")
	t.Setenv("NEBI_AUTH_JWT_SECRET", strings.Repeat("s", 32))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Sandbox.Mode != "strict" {
		t.Fatalf("expected sandbox mode strict in team mode, got %q", cfg.Sandbox.Mode)
	}
}

func TestLoad_SandboxDefaultsToOffInLocalMode(t *testing.T) {
	isolate(t)
	t.Setenv("NEBI_MODE", "local")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Sandbox.Mode != "off" {
		t.Fatalf("expected sandbox mode off in local mode, got %q", cfg.Sandbox.Mode)
	}
}

func TestLoad_SandboxModeExplicitOverridesDefault(t *testing.T) {
	isolate(t)
	t.Setenv("NEBI_MODE", "team")
	t.Setenv("NEBI_AUTH_JWT_SECRET", strings.Repeat("s", 32))
	t.Setenv("NEBI_SANDBOX_MODE", "permissive")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Sandbox.Mode != "permissive" {
		t.Fatalf("expected explicit permissive mode to win, got %q", cfg.Sandbox.Mode)
	}
}

func TestLoad_SandboxModeRejectsUnknownValue(t *testing.T) {
	isolate(t)
	t.Setenv("NEBI_MODE", "local")
	t.Setenv("NEBI_SANDBOX_MODE", "banana")

	if _, err := Load(); err == nil {
		t.Fatal("expected error for unknown sandbox mode")
	}
}

func TestLoad_SandboxDefaultPortsAndTimeout(t *testing.T) {
	isolate(t)
	t.Setenv("NEBI_MODE", "local")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Sandbox.AllowedPorts) != 2 || cfg.Sandbox.AllowedPorts[0] != 80 || cfg.Sandbox.AllowedPorts[1] != 443 {
		t.Fatalf("expected default allowed ports [80 443], got %v", cfg.Sandbox.AllowedPorts)
	}
	if cfg.Sandbox.BuildTimeout != 30*time.Minute {
		t.Fatalf("expected default build timeout 30m, got %v", cfg.Sandbox.BuildTimeout)
	}
}
```

Add `"time"` to that file's imports.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/config/ -run TestLoad_Sandbox -v`
Expected: FAIL, compile error `cfg.Sandbox undefined`.

- [ ] **Step 3: Implement the config**

In `internal/config/config.go`, add the field to `Config` (after `Storage`):

```go
	Storage        StorageConfig        `mapstructure:"storage"`
	Sandbox        SandboxConfig        `mapstructure:"sandbox"`
```

Add the type after `StorageConfig`:

```go
// SandboxConfig controls isolation of untrusted package/build subprocesses.
//
// Mode semantics:
//   strict     — builds fail if the kernel cannot enforce filesystem confinement
//   permissive — confine when possible, warn and continue when not
//   off        — no confinement (the environment allowlist still applies)
//
// Mode defaults to "strict" in team mode and "off" in local mode.
type SandboxConfig struct {
	Mode         string        `mapstructure:"mode"`
	AllowedPorts []int         `mapstructure:"allowed_ports"` // TCP ports build code may connect to
	BuildTimeout time.Duration `mapstructure:"build_timeout"` // Wall-clock ceiling for a build job
}
```

Add `"time"` to the imports. Add defaults next to the other `SetDefault` calls:

```go
	v.SetDefault("sandbox.mode", "")
	v.SetDefault("sandbox.allowed_ports", []int{80, 443})
	v.SetDefault("sandbox.build_timeout", 30*time.Minute)
```

Add the env bindings next to the other `BindEnv` calls:

```go
	_ = v.BindEnv("sandbox.mode", "NEBI_SANDBOX_MODE")
	_ = v.BindEnv("sandbox.allowed_ports", "NEBI_SANDBOX_ALLOWED_PORTS")
	_ = v.BindEnv("sandbox.build_timeout", "NEBI_SANDBOX_BUILD_TIMEOUT")
```

After the existing mode validation block (the `switch cfg.Mode` that ends with the `invalid mode` error), add:

```go
	// Sandbox mode defaults by deployment mode: team servers run untrusted
	// build code for multiple tenants and fail closed; local/desktop is a
	// single user on their own machine.
	if cfg.Sandbox.Mode == "" {
		if cfg.IsLocalMode() {
			cfg.Sandbox.Mode = "off"
		} else {
			cfg.Sandbox.Mode = "strict"
		}
	}
	switch cfg.Sandbox.Mode {
	case "strict", "permissive", "off":
	default:
		return nil, fmt.Errorf("invalid sandbox.mode %q: must be \"strict\", \"permissive\", or \"off\"", cfg.Sandbox.Mode)
	}
	if cfg.Sandbox.BuildTimeout <= 0 {
		return nil, fmt.Errorf("sandbox.build_timeout must be positive, got %s", cfg.Sandbox.BuildTimeout)
	}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/config/ -v`
Expected: PASS, including the pre-existing JWT secret tests.

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): add sandbox mode, allowed ports, and build timeout"
```

---

### Task 2: Environment allowlist

**Files:**
- Create: `internal/sandbox/env.go`
- Test: `internal/sandbox/env_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/sandbox/env_test.go`:

```go
package sandbox

import (
	"slices"
	"strings"
	"testing"
)

func TestBuildEnv_DropsSecretsKeepsAllowlisted(t *testing.T) {
	parent := []string{
		"PATH=/usr/bin:/bin",
		"NEBI_DATABASE_DSN=host=db user=nebi password=hunter2",
		"NEBI_AUTH_JWT_SECRET=supersecret",
		"NEBI_QUEUE_VALKEY_ADDR=valkey:6379",
		"AWS_SECRET_ACCESS_KEY=leakme",
		"LANG=en_US.UTF-8",
		"SSL_CERT_FILE=/etc/ssl/certs/org-ca.crt",
		"HTTPS_PROXY=http://proxy:3128",
	}

	got := buildEnv(parent, "/ws/home", "/ws/tmp")

	for _, banned := range []string{"NEBI_DATABASE_DSN", "NEBI_AUTH_JWT_SECRET", "NEBI_QUEUE_VALKEY_ADDR", "AWS_SECRET_ACCESS_KEY"} {
		for _, kv := range got {
			if strings.HasPrefix(kv, banned+"=") {
				t.Fatalf("secret %q leaked into sandbox env: %v", banned, got)
			}
		}
	}

	for _, want := range []string{
		"PATH=/usr/bin:/bin",
		"LANG=en_US.UTF-8",
		"SSL_CERT_FILE=/etc/ssl/certs/org-ca.crt",
		"HTTPS_PROXY=http://proxy:3128",
		"HOME=/ws/home",
		"TMPDIR=/ws/tmp",
	} {
		if !slices.Contains(got, want) {
			t.Fatalf("expected %q in sandbox env, got %v", want, got)
		}
	}
}

func TestBuildEnv_AlwaysSetsPathEvenWhenParentHasNone(t *testing.T) {
	got := buildEnv([]string{"NEBI_AUTH_JWT_SECRET=x"}, "/ws/home", "/ws/tmp")

	var path string
	for _, kv := range got {
		if strings.HasPrefix(kv, "PATH=") {
			path = kv
		}
	}
	if path == "" {
		t.Fatalf("expected a PATH entry, got %v", got)
	}
}

func TestBuildEnv_JobScopedHomeOverridesParentHome(t *testing.T) {
	got := buildEnv([]string{"HOME=/root", "PATH=/bin"}, "/ws/home", "/ws/tmp")

	if slices.Contains(got, "HOME=/root") {
		t.Fatalf("parent HOME must not survive: %v", got)
	}
	if !slices.Contains(got, "HOME=/ws/home") {
		t.Fatalf("expected job-scoped HOME, got %v", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/sandbox/ -v`
Expected: FAIL, `no required module provides package` / `buildEnv undefined`.

- [ ] **Step 3: Implement the allowlist**

Create `internal/sandbox/env.go`:

```go
package sandbox

import (
	"strings"
)

// defaultPath is used when the parent process has no PATH. Without it pixi
// cannot find the tools it shells out to.
const defaultPath = "/usr/local/bin:/usr/bin:/bin"

// envAllowlist names the parent environment variables a build may inherit.
// Everything else is dropped, which is what keeps NEBI_DATABASE_DSN,
// NEBI_AUTH_JWT_SECRET, and registry credentials out of build code's reach.
var envAllowlist = []string{
	"PATH",
	"LANG",
	"LC_ALL",
	"SSL_CERT_FILE",
	"SSL_CERT_DIR",
	"HTTP_PROXY",
	"HTTPS_PROXY",
	"NO_PROXY",
	"http_proxy",
	"https_proxy",
	"no_proxy",
}

// buildEnv returns the environment for a build subprocess: the allowlisted
// subset of parent, plus a job-scoped HOME and TMPDIR. HOME is job-scoped so
// the pixi/rattler package cache lands inside the workspace rather than in a
// location shared across tenants.
func buildEnv(parent []string, home, tmpDir string) []string {
	allowed := make(map[string]bool, len(envAllowlist))
	for _, k := range envAllowlist {
		allowed[k] = true
	}

	out := make([]string, 0, len(envAllowlist)+2)
	havePath := false
	for _, kv := range parent {
		key, _, ok := strings.Cut(kv, "=")
		if !ok || !allowed[key] {
			continue
		}
		if key == "PATH" {
			havePath = true
		}
		out = append(out, kv)
	}
	if !havePath {
		out = append(out, "PATH="+defaultPath)
	}
	out = append(out, "HOME="+home, "TMPDIR="+tmpDir)
	return out
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/sandbox/ -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/sandbox/env.go internal/sandbox/env_test.go
git commit -m "feat(sandbox): add environment allowlist for build subprocesses"
```

---

### Task 3: Runner and command construction

**Files:**
- Create: `internal/sandbox/sandbox.go`
- Test: `internal/sandbox/sandbox_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/sandbox/sandbox_test.go`:

```go
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
```

Add this test helper at the bottom of the same file:

```go
type exitCodeErr struct{ code int }

func (e *exitCodeErr) Error() string { return "exit status" }
func (e *exitCodeErr) ExitCode() int { return e.code }
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/sandbox/ -run TestCommand -v`
Expected: FAIL, `undefined: NewRunner`.

- [ ] **Step 3: Implement the runner**

Create `internal/sandbox/sandbox.go`:

```go
// Package sandbox constructs subprocess commands for untrusted package and
// build code (pixi). Every command it returns runs with a scrubbed
// environment; when the sandbox is enabled the command is additionally
// re-executed through the "nebi sandbox-exec" shim, which applies a Landlock
// ruleset before exec'ing the real binary.
//
// See docs/superpowers/specs/2026-08-05-build-sandbox-design.md.
package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

// Mode controls how failures to confine a build are handled.
type Mode string

const (
	// ModeStrict fails the build when confinement cannot be applied.
	ModeStrict Mode = "strict"
	// ModePermissive warns and runs the build unconfined.
	ModePermissive Mode = "permissive"
	// ModeOff skips confinement entirely (the env allowlist still applies).
	ModeOff Mode = "off"
)

// SetupFailureExitCode is returned by the shim when the sandbox itself could
// not be established, as distinct from the build failing on its own merits.
const SetupFailureExitCode = 125

// Job-scoped subdirectories created inside the workspace. HOME is scoped so
// the package cache stays per-workspace; a shared cache would let one tenant
// poison packages for another.
const (
	homeDirName = ".nebi-home"
	tmpDirName  = ".nebi-tmp"
)

// Config is the sandbox slice of application configuration.
type Config struct {
	Mode         Mode
	AllowedPorts []int
}

// Spec describes one build invocation.
type Spec struct {
	// WorkspaceDir is the build's working directory and its only writable
	// root. Must be absolute.
	WorkspaceDir string
	// Argv is the real command, argv[0] being the absolute binary path.
	Argv []string
	// ParentEnv defaults to os.Environ() when nil.
	ParentEnv []string
}

// Runner builds sandboxed commands.
type Runner struct {
	cfg      Config
	selfPath string // absolute path to the nebi binary, used to re-exec the shim
}

// NewRunner returns a Runner. selfPath may be empty, in which case the
// current executable is resolved via os.Executable.
func NewRunner(cfg Config, selfPath string) (*Runner, error) {
	switch cfg.Mode {
	case ModeStrict, ModePermissive, ModeOff:
	default:
		return nil, fmt.Errorf("invalid sandbox mode %q", cfg.Mode)
	}
	if selfPath == "" {
		p, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("resolve nebi binary for sandbox re-exec: %w", err)
		}
		selfPath = p
	}
	return &Runner{cfg: cfg, selfPath: selfPath}, nil
}

// Mode reports the configured mode.
func (r *Runner) Mode() Mode { return r.cfg.Mode }

// Command returns an *exec.Cmd for spec. Stdout/Stderr are left for the
// caller to wire to the job's log writer.
func (r *Runner) Command(ctx context.Context, spec Spec) (*exec.Cmd, error) {
	if len(spec.Argv) == 0 {
		return nil, errors.New("sandbox: argv must not be empty")
	}
	if !filepath.IsAbs(spec.WorkspaceDir) {
		return nil, fmt.Errorf("sandbox: workspace dir %q must be absolute", spec.WorkspaceDir)
	}

	home := filepath.Join(spec.WorkspaceDir, homeDirName)
	tmp := filepath.Join(spec.WorkspaceDir, tmpDirName)
	for _, dir := range []string{home, tmp} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("sandbox: create %s: %w", dir, err)
		}
	}

	parent := spec.ParentEnv
	if parent == nil {
		parent = os.Environ()
	}

	argv := spec.Argv
	if r.cfg.Mode != ModeOff {
		argv = r.shimArgv(spec)
	}

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = spec.WorkspaceDir
	cmd.Env = buildEnv(parent, home, tmp)
	return cmd, nil
}

// shimArgv builds "nebi sandbox-exec <flags> -- <real argv>".
func (r *Runner) shimArgv(spec Spec) []string {
	argv := []string{
		r.selfPath,
		"sandbox-exec",
		"--mode=" + string(r.cfg.Mode),
		"--allow-rw=" + spec.WorkspaceDir,
	}
	for _, dir := range readOnlyPaths(spec.Argv[0]) {
		argv = append(argv, "--allow-ro="+dir)
	}
	for _, port := range r.cfg.AllowedPorts {
		argv = append(argv, "--allow-port="+strconv.Itoa(port))
	}
	argv = append(argv, "--")
	return append(argv, spec.Argv...)
}

// readOnlyPaths returns the system directories a build needs to read:
// interpreters, shared libraries, TLS trust stores, and the directory
// holding the package-manager binary itself. Nonexistent paths are dropped
// so the ruleset stays valid across images.
func readOnlyPaths(binary string) []string {
	candidates := []string{"/usr", "/lib", "/lib64", "/bin", "/sbin", "/etc", "/opt"}
	if dir := filepath.Dir(binary); dir != "" && dir != "." && dir != "/" {
		candidates = append(candidates, dir)
	}

	seen := make(map[string]bool, len(candidates))
	out := make([]string, 0, len(candidates))
	for _, p := range candidates {
		if seen[p] {
			continue
		}
		if _, err := statDir(p); err != nil {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

func statDir(path string) (os.FileInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", path)
	}
	return info, nil
}

// IsSetupFailure reports whether err is a sandbox setup failure (as opposed
// to the build command failing), so callers can produce an actionable
// operator-facing message.
func IsSetupFailure(err error) bool {
	if err == nil {
		return false
	}
	var coder interface{ ExitCode() int }
	if errors.As(err, &coder) {
		return coder.ExitCode() == SetupFailureExitCode
	}
	return false
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/sandbox/ -v`
Expected: PASS (all tests from Tasks 2 and 3).

- [ ] **Step 5: Commit**

```bash
git add internal/sandbox/sandbox.go internal/sandbox/sandbox_test.go
git commit -m "feat(sandbox): add runner that wraps build commands in the shim"
```

---

### Task 4: Landlock application

**Files:**
- Create: `internal/sandbox/landlock_linux.go`
- Create: `internal/sandbox/landlock_other.go`
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Add the dependency**

Run:

```bash
go get github.com/landlock-lsm/go-landlock@latest
```

Expected: `go.mod` gains a `github.com/landlock-lsm/go-landlock` require line.

- [ ] **Step 2: Write the failing test**

Create `internal/sandbox/landlock_other.go` first so the package still builds on macOS:

```go
//go:build !linux

package sandbox

import "errors"

// ErrUnsupported reports that this platform cannot confine builds.
var ErrUnsupported = errors.New("sandbox: filesystem confinement requires Linux with Landlock")

// Confine is a no-op stub on non-Linux platforms.
func Confine(Restrictions) error { return ErrUnsupported }
```

Then add to `internal/sandbox/sandbox_test.go`:

```go
func TestRestrictions_Validate(t *testing.T) {
	if err := (Restrictions{}).Validate(); err == nil {
		t.Fatal("expected an error when no RW path is given")
	}
	if err := (Restrictions{RW: []string{"/ws"}}).Validate(); err != nil {
		t.Fatalf("expected a single RW path to be valid, got %v", err)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/sandbox/ -run TestRestrictions -v`
Expected: FAIL, `undefined: Restrictions`.

- [ ] **Step 4: Implement the Linux path**

Create `internal/sandbox/landlock_linux.go`:

```go
//go:build linux

package sandbox

import (
	"errors"
	"fmt"

	"github.com/landlock-lsm/go-landlock/landlock"
)

// ErrUnsupported reports that the running kernel cannot confine builds.
var ErrUnsupported = errors.New("sandbox: kernel does not support Landlock filesystem confinement")

// Confine applies a Landlock ruleset to the calling process. It is
// irreversible and inherited across execve, which is what confines pixi and
// everything it spawns.
//
// Filesystem restriction requires Landlock ABI v1 (Linux 5.13+) and is
// mandatory: an error here means the caller must decide whether to fail
// (strict) or continue (permissive). Network restriction requires ABI v4
// (Linux 6.7+) and is best-effort, since 6.7 is not yet universal.
func Confine(r Restrictions) error {
	if err := r.Validate(); err != nil {
		return err
	}

	rules := []landlock.Rule{landlock.RWDirs(r.RW...)}
	if len(r.RO) > 0 {
		rules = append(rules, landlock.RODirs(r.RO...))
	}
	if len(r.RWFiles) > 0 {
		rules = append(rules, landlock.RWFiles(r.RWFiles...))
	}

	if err := landlock.V1.RestrictPaths(rules...); err != nil {
		return fmt.Errorf("%w: %v", ErrUnsupported, err)
	}

	if len(r.TCPConnectPorts) > 0 {
		netRules := make([]landlock.Rule, 0, len(r.TCPConnectPorts))
		for _, port := range r.TCPConnectPorts {
			netRules = append(netRules, landlock.ConnectTCP(uint16(port)))
		}
		// Best-effort: on kernels below 6.7 this downgrades to a no-op
		// rather than failing the build.
		if err := landlock.V4.BestEffort().RestrictNet(netRules...); err != nil {
			return fmt.Errorf("restrict network: %w", err)
		}
	}

	return nil
}
```

Add the shared type to `internal/sandbox/sandbox.go` (platform-independent, so it lives in the main file):

```go
// Restrictions is the ruleset the shim applies to itself before exec'ing the
// build command.
type Restrictions struct {
	RW              []string // directories the build may read and write
	RO              []string // directories the build may read
	RWFiles         []string // individual files the build may read and write
	TCPConnectPorts []int    // TCP ports the build may connect to
}

// Validate rejects rulesets that would be useless or accidentally empty.
func (r Restrictions) Validate() error {
	if len(r.RW) == 0 {
		return errors.New("sandbox: at least one read-write path is required")
	}
	for _, p := range append(append([]string{}, r.RW...), r.RO...) {
		if !filepath.IsAbs(p) {
			return fmt.Errorf("sandbox: path %q must be absolute", p)
		}
	}
	return nil
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/sandbox/ -v && GOOS=linux go build ./... && GOOS=darwin go build ./...`
Expected: PASS, and both cross-compiles succeed.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/sandbox/landlock_linux.go internal/sandbox/landlock_other.go internal/sandbox/sandbox.go internal/sandbox/sandbox_test.go
git commit -m "feat(sandbox): apply Landlock filesystem and TCP restrictions"
```

---

### Task 5: The `sandbox-exec` shim command

**Files:**
- Create: `cmd/nebi/sandbox_exec.go`
- Modify: `cmd/nebi/main.go:78` (after `rootCmd.AddCommand(infoCmd)`)

- [ ] **Step 1: Write the shim**

Create `cmd/nebi/sandbox_exec.go`:

```go
package main

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"syscall"

	"github.com/nebari-dev/nebi/internal/sandbox"
	"github.com/spf13/cobra"
)

var (
	sandboxExecMode  string
	sandboxExecRW    []string
	sandboxExecRO    []string
	sandboxExecPorts []int
)

// sandboxExecCmd is an internal re-exec shim, not a user-facing command. The
// server invokes it as:
//
//	nebi sandbox-exec --mode=strict --allow-rw=/ws [--allow-ro=/usr ...] \
//	    [--allow-port=443 ...] -- /path/to/pixi install -v
//
// It applies a Landlock ruleset to itself and then execve's the real command,
// which inherits the confinement.
var sandboxExecCmd = &cobra.Command{
	Use:                   "sandbox-exec [flags] -- COMMAND [ARGS...]",
	Short:                 "Internal: run a command under filesystem and network confinement",
	Hidden:                true,
	DisableFlagsInUseLine: true,
	Args:                  cobra.MinimumNArgs(1),
	// The shim exits via syscall.Exec or os.Exit, so silence cobra's own
	// error decoration.
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		mode := sandbox.Mode(sandboxExecMode)
		switch mode {
		case sandbox.ModeStrict, sandbox.ModePermissive:
		default:
			failSetup(fmt.Errorf("sandbox-exec requires --mode=strict or --mode=permissive, got %q", sandboxExecMode))
		}

		err := sandbox.Confine(sandbox.Restrictions{
			RW:              sandboxExecRW,
			RO:              sandboxExecRO,
			RWFiles:         devFiles(),
			TCPConnectPorts: sandboxExecPorts,
		})
		switch {
		case err == nil:
		case mode == sandbox.ModePermissive:
			fmt.Fprintf(os.Stderr, "[nebi] WARNING: build is running UNCONFINED: %v\n", err)
		default:
			failSetup(err)
		}

		if err := syscall.Exec(args[0], args, os.Environ()); err != nil {
			failSetup(fmt.Errorf("exec %s: %w", args[0], err))
		}
		return nil // unreachable: syscall.Exec replaces the process image
	},
}

// devFiles returns the device nodes a build legitimately needs. Missing
// nodes are skipped so the ruleset stays valid in minimal images.
func devFiles() []string {
	var out []string
	for _, f := range []string{"/dev/null", "/dev/urandom", "/dev/random", "/dev/zero"} {
		if _, err := os.Stat(f); err == nil {
			out = append(out, f)
		}
	}
	return out
}

// failSetup exits with the reserved code so the parent can distinguish a
// broken sandbox from a failed build.
func failSetup(err error) {
	hint := ""
	if errors.Is(err, sandbox.ErrUnsupported) {
		hint = fmt.Sprintf(" (kernel %s/%s lacks Landlock support; set NEBI_SANDBOX_MODE=permissive to run builds unconfined, or NEBI_SANDBOX_MODE=off to disable the sandbox)", runtime.GOOS, runtime.GOARCH)
	}
	fmt.Fprintf(os.Stderr, "[nebi] sandbox setup failed: %v%s\n", err, hint)
	os.Exit(sandbox.SetupFailureExitCode)
}

func init() {
	sandboxExecCmd.Flags().StringVar(&sandboxExecMode, "mode", "strict", "strict or permissive")
	sandboxExecCmd.Flags().StringArrayVar(&sandboxExecRW, "allow-rw", nil, "directory the command may read and write (repeatable)")
	sandboxExecCmd.Flags().StringArrayVar(&sandboxExecRO, "allow-ro", nil, "directory the command may read (repeatable)")
	sandboxExecCmd.Flags().IntSliceVar(&sandboxExecPorts, "allow-port", nil, "TCP port the command may connect to (repeatable)")
}
```

- [ ] **Step 2: Register the command**

In `cmd/nebi/main.go`, after `rootCmd.AddCommand(infoCmd)`:

```go
	rootCmd.AddCommand(sandboxExecCmd)
```

- [ ] **Step 3: Verify it builds and is hidden**

Run:

```bash
go build -o /tmp/nebi ./cmd/nebi && /tmp/nebi --help | grep -c sandbox-exec
```

Expected: `0` (the command is hidden from help).

Run: `/tmp/nebi sandbox-exec --help`
Expected: usage text for the hidden command, exit 0.

- [ ] **Step 4: Commit**

```bash
git add cmd/nebi/sandbox_exec.go cmd/nebi/main.go
git commit -m "feat(cli): add hidden sandbox-exec re-exec shim"
```

---

### Task 6: Route the executor's pixi calls through the sandbox

**Files:**
- Modify: `internal/executor/local.go` (struct, constructor, `runPixiLock`, `InstallEnvironment`)
- Test: `internal/executor/local_sandbox_test.go` (create)

- [ ] **Step 1: Write the failing test**

Create `internal/executor/local_sandbox_test.go`:

```go
package executor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/config"
	"github.com/nebari-dev/nebi/internal/models"
)

// fakePixi writes a stub pixi that dumps its environment to dumpPath. The
// path is baked into the script rather than passed via an env var, because
// the allowlist would strip such a var before the child could read it.
func fakePixi(t *testing.T, dir, dumpPath string) string {
	t.Helper()
	path := filepath.Join(dir, "pixi")
	script := strings.ReplaceAll(`#!/bin/sh
if [ "$1" = "--version" ]; then echo "pixi 0.0.0-test"; exit 0; fi
env > DUMPPATH
exit 0
`, "DUMPPATH", dumpPath)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake pixi: %v", err)
	}
	return path
}

func TestInstallEnvironment_ScrubsSecretsFromBuildEnv(t *testing.T) {
	baseDir := t.TempDir()
	dump := filepath.Join(t.TempDir(), "env.txt")
	pixiPath := fakePixi(t, t.TempDir(), dump)

	t.Setenv("NEBI_DATABASE_DSN", "host=db password=hunter2")
	t.Setenv("NEBI_AUTH_JWT_SECRET", "supersecret")

	cfg := &config.Config{
		Mode:           "local",
		Storage:        config.StorageConfig{WorkspacesDir: baseDir},
		PackageManager: config.PackageManagerConfig{DefaultType: "pixi", PixiPath: pixiPath},
		Sandbox:        config.SandboxConfig{Mode: "off", BuildTimeout: 0},
	}

	e, err := NewLocalExecutor(cfg)
	if err != nil {
		t.Fatalf("NewLocalExecutor: %v", err)
	}

	ws := &models.Workspace{ID: uuid.New(), Name: "test-ws", PackageManager: "pixi"}
	if err := os.MkdirAll(e.GetWorkspacePath(ws), 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}

	var log strings.Builder
	if err := e.InstallEnvironment(context.Background(), ws, &log); err != nil {
		t.Fatalf("InstallEnvironment: %v (log: %s)", err, log.String())
	}

	data, err := os.ReadFile(dump)
	if err != nil {
		t.Fatalf("read env dump: %v (log: %s)", err, log.String())
	}
	got := string(data)
	for _, banned := range []string{"NEBI_DATABASE_DSN", "NEBI_AUTH_JWT_SECRET"} {
		if strings.Contains(got, banned) {
			t.Fatalf("secret %q reached the build subprocess:\n%s", banned, got)
		}
	}
	if !strings.Contains(got, "HOME="+filepath.Join(e.GetWorkspacePath(ws), ".nebi-home")) {
		t.Fatalf("expected job-scoped HOME in build env:\n%s", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/executor/ -run TestInstallEnvironment_Scrubs -v`
Expected: FAIL, `unknown field Sandbox in struct literal` or the secret appearing in the dump.

- [ ] **Step 3: Wire the runner into the executor**

In `internal/executor/local.go`, add the import:

```go
	"github.com/nebari-dev/nebi/internal/sandbox"
```

Extend the struct:

```go
// LocalExecutor runs operations on the local machine
type LocalExecutor struct {
	baseDir string // Base directory for workspaces (e.g., /var/lib/nebi/environments)
	config  *config.Config
	sandbox *sandbox.Runner
}
```

In `NewLocalExecutor`, after the base directory is created and before the return:

```go
	sb, err := sandbox.NewRunner(sandbox.Config{
		Mode:         sandbox.Mode(cfg.Sandbox.Mode),
		AllowedPorts: cfg.Sandbox.AllowedPorts,
	}, "")
	if err != nil {
		return nil, fmt.Errorf("failed to initialize build sandbox: %w", err)
	}

	return &LocalExecutor{
		baseDir: baseDir,
		config:  cfg,
		sandbox: sb,
	}, nil
```

`config.SandboxConfig.Mode` is empty in hand-built test configs, which `NewRunner` rejects. Default it in the constructor, just above the `sandbox.NewRunner` call:

```go
	sandboxMode := cfg.Sandbox.Mode
	if sandboxMode == "" {
		sandboxMode = "off"
	}
```

and pass `sandbox.Mode(sandboxMode)`.

Replace `runPixiLock` so it takes the runner (update both call sites in `CreateWorkspace` and `SolveEnvironment` to pass `e.sandbox`):

```go
// runPixiLock runs `pixi lock` in envPath. It resolves the dependency
// graph and writes pixi.lock without downloading or extracting packages;
// installing is a separate, explicit step (see InstallEnvironment).
func runPixiLock(ctx context.Context, sb *sandbox.Runner, pm pkgmgr.PackageManager, envPath string, logWriter io.Writer) error {
	pixiBinary := "pixi"
	if pixiMgr, ok := pm.(*pixi.PixiManager); ok {
		pixiBinary = pixiMgr.BinaryPath()
	}
	lockCmd, err := sb.Command(ctx, sandbox.Spec{
		WorkspaceDir: envPath,
		Argv:         []string{pixiBinary, "lock"},
	})
	if err != nil {
		return fmt.Errorf("failed to prepare sandboxed pixi lock: %w", err)
	}
	lockCmd.Stdout = logWriter
	lockCmd.Stderr = logWriter
	fmt.Fprintf(logWriter, "Running: %s lock\n", pixiBinary)
	if err := lockCmd.Run(); err != nil {
		if sandbox.IsSetupFailure(err) {
			return fmt.Errorf("build sandbox setup failed: %w", err)
		}
		return fmt.Errorf("failed to lock pixi environment: %w", err)
	}
	fmt.Fprintf(logWriter, "Lockfile resolved successfully\n")
	return nil
}
```

The three call sites become `runPixiLock(ctx, e.sandbox, pm, envPath, logWriter)` (two in `CreateWorkspace`, one in `SolveEnvironment`).

Replace the body of `InstallEnvironment` between the `pixiBinary` resolution and the success log:

```go
	cmd, err := e.sandbox.Command(ctx, sandbox.Spec{
		WorkspaceDir: envPath,
		Argv:         []string{pixiBinary, "install", "-v"},
	})
	if err != nil {
		return fmt.Errorf("failed to prepare sandboxed pixi install: %w", err)
	}
	cmd.Stdout = logWriter
	cmd.Stderr = logWriter
	fmt.Fprintf(logWriter, "Running: %s install -v\n", pixiBinary)
	if err := cmd.Run(); err != nil {
		if sandbox.IsSetupFailure(err) {
			return fmt.Errorf("build sandbox setup failed: %w", err)
		}
		return fmt.Errorf("pixi install failed: %w", err)
	}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/executor/ ./internal/sandbox/ ./internal/config/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/executor/local.go internal/executor/local_sandbox_test.go
git commit -m "feat(executor): run pixi lock and install through the build sandbox"
```

---

### Task 7: Route the pixi package's calls through the sandbox

**Files:**
- Modify: `internal/pkgmgr/pixi/pixi.go` (struct + six exec sites)
- Modify: `internal/executor/local.go` (`packageManagerFor` injects the runner)
- Test: `internal/pkgmgr/pixi/pixi_test.go` (append)

A nil runner means "behave exactly as before". That keeps the CLI paths
(`nebi shell`, `nebi run`) and the existing tests unchanged, while the
server and worker always inject a runner.

- [ ] **Step 1: Write the failing test**

Append to `internal/pkgmgr/pixi/pixi_test.go`:

```go
func TestInstall_UsesSandboxRunnerWhenSet(t *testing.T) {
	pm, argsPath, envPath := newRecordingPixi(t)

	sb, err := sandbox.NewRunner(sandbox.Config{Mode: sandbox.ModeOff}, "")
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	pm.SetSandbox(sb)

	if err := pm.Install(context.Background(), pkgmgr.InstallOptions{
		EnvPath:  envPath,
		Packages: []string{"python=3.11"},
	}); err != nil {
		t.Fatalf("Install: %v", err)
	}

	got := readRecordedArgs(t, argsPath)
	want := []string{"add", "-v", "--", "python=3.11"}
	if !slices.Equal(got, want) {
		t.Fatalf("recorded args = %#v, want %#v", got, want)
	}
}

func TestInstall_WithoutSandboxRunnerBehavesAsBefore(t *testing.T) {
	pm, argsPath, envPath := newRecordingPixi(t)

	if err := pm.Install(context.Background(), pkgmgr.InstallOptions{
		EnvPath:  envPath,
		Packages: []string{"python=3.11"},
	}); err != nil {
		t.Fatalf("Install: %v", err)
	}

	got := readRecordedArgs(t, argsPath)
	want := []string{"add", "-v", "--", "python=3.11"}
	if !slices.Equal(got, want) {
		t.Fatalf("recorded args = %#v, want %#v", got, want)
	}
}
```

Add `"github.com/nebari-dev/nebi/internal/sandbox"` to that file's imports.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/pkgmgr/pixi/ -run TestInstall_UsesSandbox -v`
Expected: FAIL, `pm.SetSandbox undefined`.

- [ ] **Step 3: Implement the hook**

In `internal/pkgmgr/pixi/pixi.go`, add the import and extend the struct:

```go
// PixiManager implements the PackageManager interface for pixi
type PixiManager struct {
	pixiPath string          // Path to pixi binary
	sandbox  *sandbox.Runner // Optional; nil means run unsandboxed (CLI paths)
}

// SetSandbox makes every pixi subprocess run through the build sandbox.
// Server and worker processes set this; the CLI, which already runs as the
// invoking user, leaves it nil.
func (p *PixiManager) SetSandbox(s *sandbox.Runner) { p.sandbox = s }

// command builds the exec.Cmd for a pixi invocation in workDir, routing
// through the sandbox when one is configured.
func (p *PixiManager) command(ctx context.Context, workDir string, args ...string) (*exec.Cmd, error) {
	if p.sandbox == nil {
		cmd := exec.CommandContext(ctx, p.pixiPath, args...)
		cmd.Dir = workDir
		return cmd, nil
	}
	return p.sandbox.Command(ctx, sandbox.Spec{
		WorkspaceDir: workDir,
		Argv:         append([]string{p.pixiPath}, args...),
	})
}
```

Replace each of the six exec sites. `Init` (was line 135):

```go
	cmd, err := p.command(ctx, opts.EnvPath, args...)
	if err != nil {
		return fmt.Errorf("failed to prepare pixi init: %w", err)
	}
```

`Install` (was line 210), `Remove` (was line 285): same shape with `pixi add` / `pixi remove` in the error string. `List` (was line 347):

```go
	cmd, err := p.command(ctx, opts.EnvPath, "list")
	if err != nil {
		return nil, fmt.Errorf("failed to prepare pixi list: %w", err)
	}
```

`Update` (was line 414) and `executeCommand` (was line 500) follow the same pattern, returning `fmt.Errorf("failed to prepare pixi command: %w", err)` in the latter.

Delete the now-redundant `cmd.Dir = ...` line at each site; `command` sets it.

In `internal/executor/local.go`, make `packageManagerFor` inject the runner. Replace its body's returns so every path passes through:

```go
func (e *LocalExecutor) packageManagerFor(ws *models.Workspace) (pkgmgr.PackageManager, error) {
	pmType := ws.PackageManager
	if pmType == "" {
		pmType = e.config.PackageManager.DefaultType
	}

	var (
		pm  pkgmgr.PackageManager
		err error
	)
	switch {
	case pmType == "pixi" && e.config.PackageManager.PixiPath != "":
		pm, err = pkgmgr.NewWithPath(pmType, e.config.PackageManager.PixiPath)
	case pmType == "uv" && e.config.PackageManager.UvPath != "":
		pm, err = pkgmgr.NewWithPath(pmType, e.config.PackageManager.UvPath)
	default:
		pm, err = pkgmgr.New(pmType)
	}
	if err != nil {
		return nil, err
	}

	// Server-side builds are untrusted; confine them.
	if pixiMgr, ok := pm.(*pixi.PixiManager); ok {
		pixiMgr.SetSandbox(e.sandbox)
	}
	return pm, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/pkgmgr/... ./internal/executor/ -v`
Expected: PASS, including the pre-existing `TestPackageCommandsSeparateUserArgs`.

- [ ] **Step 5: Commit**

```bash
git add internal/pkgmgr/pixi/pixi.go internal/pkgmgr/pixi/pixi_test.go internal/executor/local.go
git commit -m "feat(pkgmgr): route pixi subprocesses through the build sandbox"
```

---

### Task 8: Build timeout and setup-failure reporting in the worker

**Files:**
- Modify: `internal/worker/worker.go` (`New`, `processJob`)
- Modify: `internal/server/server.go:179` and `app.go:183` (pass the timeout)

- [ ] **Step 1: Add the timeout to the worker**

In `internal/worker/worker.go`, add a field to the struct:

```go
	buildTimeout time.Duration
```

Change the constructor signature and body:

```go
// New creates a worker. buildTimeout bounds a single job's execution; pass 0
// to disable the ceiling.
func New(q queue.Queue, exec executor.Executor, svc *service.WorkspaceService, jobSvc *service.JobService, logger *slog.Logger, valkeyClient valkey.Client, buildTimeout time.Duration) *Worker {
```

and set `buildTimeout: buildTimeout,` in the returned struct literal.

In `processJob`, wrap the execution context immediately before the `executeJob` call:

```go
	// Bound the build so a hung or deliberately slow build cannot occupy a
	// worker slot forever.
	execCtx := ctx
	if w.buildTimeout > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(ctx, w.buildTimeout)
		defer cancel()
	}

	// Execute the job with streaming logs
	err := w.executeJob(execCtx, job, logWriter)
	if errors.Is(err, context.DeadlineExceeded) {
		err = fmt.Errorf("build exceeded the %s time limit: %w", w.buildTimeout, err)
	}
```

Add `"errors"` and `"time"` to the imports if absent.

- [ ] **Step 2: Update both construction sites**

`internal/server/server.go`:

```go
		w = worker.New(jobQueue, exec, workerSvc, workerJobSvc, slog.Default(), valkeyClient, appCfg.Sandbox.BuildTimeout)
```

`app.go` (desktop, local mode; `cfg` is in scope there):

```go
	w := worker.New(jobQueue, exec, svc, jobSvc, slog.Default(), nil, cfg.Sandbox.BuildTimeout)
```

- [ ] **Step 3: Verify the build and full test suite**

Run: `go build ./... && go test ./... 2>&1 | grep -v "^ok" | head -20`
Expected: no compile errors, no failures.

- [ ] **Step 4: Commit**

```bash
git add internal/worker/worker.go internal/server/server.go app.go
git commit -m "feat(worker): bound build execution with the configured timeout"
```

---

### Task 9: Containment integration test (the #445 acceptance test)

**Files:**
- Create: `internal/sandbox/confine_test.go`

- [ ] **Step 1: Write the test**

Create `internal/sandbox/confine_test.go`:

```go
//go:build linux

package sandbox

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildNebiBinary compiles the nebi binary once so the runner has a real
// sandbox-exec shim to re-exec.
func buildNebiBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "nebi")
	cmd := exec.Command("go", "build", "-o", bin, "../../cmd/nebi")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build nebi: %v\n%s", err, out)
	}
	return bin
}

// requireLandlock skips when the kernel cannot enforce filesystem rules. The
// read-only grants are required for the probe itself: without them the
// confined process could not read and execute /bin/true.
func requireLandlock(t *testing.T, nebiBin, dir string) {
	t.Helper()
	cmd := exec.Command(nebiBin, "sandbox-exec", "--mode=strict",
		"--allow-rw="+dir, "--allow-ro=/usr", "--allow-ro=/bin", "--allow-ro=/lib",
		"--", "/bin/true")
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == SetupFailureExitCode {
			t.Skipf("kernel lacks Landlock support: %v", err)
		}
		t.Fatalf("probe failed unexpectedly: %v", err)
	}
}

// runConfined runs a shell snippet inside the sandbox and returns its output
// and error.
func runConfined(t *testing.T, nebiBin, workspace, script string) (string, error) {
	t.Helper()
	r, err := NewRunner(Config{Mode: ModeStrict, AllowedPorts: []int{443}}, nebiBin)
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	cmd, err := r.Command(context.Background(), Spec{
		WorkspaceDir: workspace,
		Argv:         []string{"/bin/sh", "-c", script},
		ParentEnv: []string{
			"PATH=/usr/bin:/bin",
			"NEBI_DATABASE_DSN=host=db user=nebi password=hunter2",
			"NEBI_AUTH_JWT_SECRET=supersecret",
		},
	})
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestConfinement_MaliciousBuildCannotEscape(t *testing.T) {
	nebiBin := buildNebiBinary(t)

	base := t.TempDir()
	victim := filepath.Join(base, "victim-workspace")
	attacker := filepath.Join(base, "attacker-workspace")
	for _, d := range []string{victim, attacker} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	secretPath := filepath.Join(victim, "pixi.toml")
	if err := os.WriteFile(secretPath, []byte("[project]\nname = \"victim\"\n"), 0o644); err != nil {
		t.Fatalf("write victim manifest: %v", err)
	}

	requireLandlock(t, nebiBin, attacker)

	t.Run("cannot read the database password from the environment", func(t *testing.T) {
		out, _ := runConfined(t, nebiBin, attacker, "env")
		if strings.Contains(out, "hunter2") || strings.Contains(out, "supersecret") {
			t.Fatalf("build saw parent secrets:\n%s", out)
		}
	})

	t.Run("cannot read another workspace's manifest", func(t *testing.T) {
		out, err := runConfined(t, nebiBin, attacker, "cat "+secretPath)
		if err == nil {
			t.Fatalf("expected the read to fail, got output:\n%s", out)
		}
		if strings.Contains(out, "victim") {
			t.Fatalf("build read the victim manifest:\n%s", out)
		}
	})

	t.Run("cannot write into another workspace", func(t *testing.T) {
		target := filepath.Join(victim, "backdoor")
		if _, err := runConfined(t, nebiBin, attacker, "echo pwned > "+target); err == nil {
			t.Fatal("expected the write to fail")
		}
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Fatalf("attacker created %s", target)
		}
	})

	t.Run("can write inside its own workspace", func(t *testing.T) {
		own := filepath.Join(attacker, "artifact")
		if out, err := runConfined(t, nebiBin, attacker, "echo ok > "+own); err != nil {
			t.Fatalf("legitimate write failed: %v\n%s", err, out)
		}
		data, err := os.ReadFile(own)
		if err != nil || strings.TrimSpace(string(data)) != "ok" {
			t.Fatalf("expected the artifact to exist with content ok, got %q err %v", data, err)
		}
	})

	t.Run("cannot connect to the database port", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		defer ln.Close()
		port := ln.Addr().(*net.TCPAddr).Port
		go func() {
			for {
				conn, err := ln.Accept()
				if err != nil {
					return
				}
				conn.Close()
			}
		}()

		probe := fmt.Sprintf("exec 3<>/dev/tcp/127.0.0.1/%d", port)
		out, err := runConfined(t, nebiBin, attacker, probe)
		if err == nil {
			t.Skipf("kernel allowed the connection (Landlock TCP needs 6.7+); output: %s", out)
		}
	})
}
```

- [ ] **Step 2: Run the test**

Run: `go test ./internal/sandbox/ -run TestConfinement -v`
Expected on Linux 5.13+: PASS (the TCP subtest may skip on kernels below 6.7).
Expected on macOS: the file is excluded by the build tag, so no tests run.

- [ ] **Step 3: Commit**

```bash
git add internal/sandbox/confine_test.go
git commit -m "test(sandbox): verify a malicious build cannot escape confinement"
```

---

### Task 10: Documentation

**Files:**
- Modify: `config.yaml.example`
- Modify: `docs/docs/server-setup.md`

- [ ] **Step 1: Document the config block**

Append to `config.yaml.example`:

```yaml
# =============================================================================
# Build sandbox
# =============================================================================
# Environment builds run package code (pixi solve/install), which is
# untrusted: a manifest can specify source or build backends that execute
# arbitrary code. Nebi always strips secrets from the build environment and,
# on Linux, confines the build to its own workspace directory with Landlock.
sandbox:
  # strict     — fail the build if the kernel cannot confine it (default in team mode)
  # permissive — warn and run unconfined when the kernel cannot confine
  # off        — no confinement; the environment allowlist still applies
  #              (default in local/desktop mode)
  mode: strict

  # TCP ports build code may connect to. Package downloads need 443 (and 80
  # for plain-HTTP mirrors). The database and queue ports are unreachable by
  # omission. Requires Linux 6.7+; ignored with a warning on older kernels.
  allowed_ports: [80, 443]

  # Wall-clock ceiling for a single build job.
  build_timeout: 30m
```

- [ ] **Step 2: Document for operators**

Append a section to `docs/docs/server-setup.md`:

```markdown
## Build sandbox

Environment builds execute untrusted code: a `pixi.toml` can declare source or
build backends that run arbitrary commands during a solve or install. Nebi
isolates those builds in two ways.

**Environment scrubbing** applies everywhere, including local mode and macOS.
Build subprocesses receive only `PATH`, `HOME`, `TMPDIR`, locale, TLS trust,
and proxy variables. `NEBI_DATABASE_DSN`, `NEBI_AUTH_JWT_SECRET`, queue
addresses, and registry credentials are never passed through.

**Filesystem and network confinement** uses [Landlock](https://landlock.io/),
available on Linux 5.13+ (network rules need 6.7+). A build can read and write
only its own workspace directory, read system directories, and open TCP
connections only to `sandbox.allowed_ports`. The database and queue are
unreachable from build code even when they share a network with the server.

Confinement requires no extra privileges: it works with `runAsNonRoot`,
dropped capabilities, and the default seccomp profile.

| `sandbox.mode` | Behavior when the kernel cannot confine |
|---|---|
| `strict` (team default) | The job fails with a message naming `NEBI_SANDBOX_MODE` |
| `permissive` | A warning is written to the job log and the server log; the build runs unconfined |
| `off` (local default) | No confinement is attempted; environment scrubbing still applies |

Set `NEBI_SANDBOX_MODE=permissive` if you run team mode on a kernel older than
5.13 and accept the reduced isolation. `NEBI_SANDBOX_BUILD_TIMEOUT` (default
`30m`) bounds how long a single build may run.

Because `HOME` is scoped to the workspace, the pixi package cache is
per-workspace rather than shared. This costs some download time on the first
build of each workspace and prevents one tenant from poisoning another
tenant's cached packages.
```

- [ ] **Step 3: Verify docs build is untouched and commit**

Run: `go build ./... && go vet ./...`
Expected: clean.

```bash
git add config.yaml.example docs/docs/server-setup.md
git commit -m "docs: document the build sandbox configuration and behavior"
```

---

### Task 11: Full verification and PR

- [ ] **Step 1: Run the full suite**

Run: `go build ./... && go vet ./... && go test ./... 2>&1 | tail -40`
Expected: all packages `ok` or `no test files`; no failures.

- [ ] **Step 2: Verify the cross-platform build**

Run: `GOOS=darwin GOARCH=arm64 go build ./... && GOOS=linux GOARCH=amd64 go build ./...`
Expected: both succeed (the desktop app must still compile on macOS).

- [ ] **Step 3: Confirm the acceptance criteria on Linux**

Run: `go test ./internal/sandbox/ -run TestConfinement -v`
Expected: PASS. If the host is macOS, run it in a Linux container:
`docker run --rm -v "$PWD":/src -w /src golang:1.24 go test ./internal/sandbox/ -run TestConfinement -v`

- [ ] **Step 4: Push and open the PR**

```bash
git push -u origin feat/sandbox-builds
gh pr create --repo nebari-dev/nebi \
  --title "Sandbox untrusted environment builds" \
  --body-file docs/superpowers/specs/2026-08-05-build-sandbox-design.md
```

Then edit the PR body to lead with a summary, reference
`Part of https://github.com/nebari-dev/nebi/issues/445`, and state explicitly
that the K8s Job executor, queue ack/retry, and per-workspace volumes remain
follow-ups.
