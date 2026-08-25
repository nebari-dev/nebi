// Package sandbox constructs subprocess commands for untrusted package and
// build code (pixi). Every command it returns runs with a scrubbed
// environment; when the sandbox is enabled the command is additionally
// re-executed through the "nebi sandbox-exec" shim, which applies a Landlock
// ruleset before exec'ing the real binary.
//
// See https://github.com/nebari-dev/nebi/issues/445 for the threat model.
package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nebari-dev/nebi/internal/limits"
	"github.com/nebari-dev/nebi/internal/process"
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
	// ResourceLimits bounds the build's CPU time and file sizes. The zero
	// value applies no limits.
	ResourceLimits limits.ProcessLimits
}

// Runner builds sandboxed commands.
type Runner struct {
	cfg      Config
	selfPath string // absolute path to the nebi binary, used to re-exec the shim
}

// shimCheckFlag makes "sandbox-exec" exit 0 immediately without applying a
// ruleset or exec'ing anything, so it can be used to prove a binary
// implements the shim.
const shimCheckFlag = "--check"

// shimProbeEnv marks a process spawned by verifyShim. A real shim answers
// shimCheckFlag and exits long before it could construct a Runner, so a
// process that reaches verifyShim with this set is by definition not a shim.
// Refusing there caps an accidental re-exec chain at depth one instead of
// letting it grow without bound.
const shimProbeEnv = "NEBI_SANDBOX_SHIM_PROBE"

// shimProbeTimeout bounds the probe. A binary that is not the shim may not
// fail: a Go test binary handed "sandbox-exec" runs its whole suite, because
// flag parsing stops at the first non-flag argument. Overridden by tests.
var shimProbeTimeout = 10 * time.Second

// shimProbeGrace is how long the probe waits after killing the target before
// giving up on its output. Killing the target does not kill anything it has
// already spawned, and those grandchildren hold the inherited output pipe
// open, so without this the probe blocks for as long as they run and the
// timeout above buys nothing. Any survivors are left orphaned; reaping them
// needs process groups, which are not portable to the Windows build.
const shimProbeGrace = 2 * time.Second

// verifyShim reports whether path implements "nebi sandbox-exec".
//
// Without this check NewRunner trusts os.Executable() blindly, and when the
// running program is not the nebi binary the failure is catastrophic and
// silent rather than loud: every build re-execs that program with
// "sandbox-exec ..." prepended, and a program that ignores those arguments
// simply runs again, spawning another server that does the same. That is a
// fork bomb, and it is what killed CI. One cheap probe at construction turns
// it into an instant, legible startup error.
func verifyShim(path string) error {
	if os.Getenv(shimProbeEnv) == "1" {
		return fmt.Errorf("refusing to probe %s: this process is itself a shim probe, so it cannot be the nebi binary", path)
	}

	ctx, cancel := context.WithTimeout(context.Background(), shimProbeTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, "sandbox-exec", shimCheckFlag)
	cmd.Env = append(os.Environ(), shimProbeEnv+"=1")
	cmd.WaitDelay = shimProbeGrace
	out, err := cmd.CombinedOutput()

	if ctx.Err() != nil {
		return fmt.Errorf("timed out after %s waiting for %q; a binary that ignores the subcommand rather than answering it is not the shim", shimProbeTimeout, "sandbox-exec "+shimCheckFlag)
	}
	if err != nil {
		if trimmed := lastBytes(strings.TrimSpace(string(out)), 300); trimmed != "" {
			return fmt.Errorf("%w (output: %s)", err, trimmed)
		}
		return err
	}
	return nil
}

// lastBytes keeps the tail of s, where a failing command's actual error
// usually is. The probe target is an arbitrary binary that may print
// megabytes, and none of that belongs verbatim in an error string.
func lastBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "..." + s[len(s)-n:]
}

// NewRunner returns a Runner.
//
// selfPath may be empty, in which case the current executable is resolved via
// os.Executable and then verified to actually implement the shim. Both steps
// are skipped in off mode, where selfPath is never read: a failure there
// would stop the executor from constructing, and so the server from booting,
// over a value nothing uses.
//
// A non-empty selfPath is taken on trust. It is an in-package escape hatch
// for tests that never re-exec; production passes "".
func NewRunner(cfg Config, selfPath string) (*Runner, error) {
	switch cfg.Mode {
	case ModeStrict, ModePermissive, ModeOff:
	default:
		return nil, fmt.Errorf("invalid sandbox mode %q", cfg.Mode)
	}
	if selfPath == "" && cfg.Mode != ModeOff {
		p, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("resolve nebi binary for sandbox re-exec: %w", err)
		}
		if err := verifyShim(p); err != nil {
			return nil, fmt.Errorf(
				"sandbox mode %q re-execs the running binary as %q, but %s does not implement it: %w; "+
					"this usually means an active sandbox mode was enabled in a process that is not the nebi binary, "+
					"such as a Go test binary, where NEBI_SANDBOX_MODE should be set to \"off\"",
				cfg.Mode, "nebi sandbox-exec", p, err)
		}
		selfPath = p
	}
	return &Runner{cfg: cfg, selfPath: selfPath}, nil
}

// Mode reports the configured mode.
func (r *Runner) Mode() Mode { return r.cfg.Mode }

// Command returns an *exec.Cmd for spec. Stdout/Stderr are left for the
// caller to wire to the job's log writer.
//
// Job-scoped directories live under the workspace's .nebi/ tree, whose layout
// internal/process owns. Reusing that layout rather than inventing a parallel
// one keeps these directories covered by process.WorkspaceTransientDirs,
// which failed-job cleanup walks.
func (r *Runner) Command(ctx context.Context, spec Spec) (*exec.Cmd, error) {
	if len(spec.Argv) == 0 {
		return nil, errors.New("sandbox: argv must not be empty")
	}
	if !filepath.IsAbs(spec.WorkspaceDir) {
		return nil, fmt.Errorf("sandbox: workspace dir %q must be absolute", spec.WorkspaceDir)
	}

	// TMPDIR and the language caches are scoped to the workspace in every
	// mode, matching what internal/process does for unsandboxed builds.
	if err := process.PrepareWorkspaceDirs(spec.WorkspaceDir); err != nil {
		return nil, fmt.Errorf("sandbox: %w", err)
	}
	tmp := process.WorkspaceTmpDir(spec.WorkspaceDir)

	// Scoping HOME into the workspace only makes sense when the sandbox is
	// active. In off mode the build is unconfined anyway, so the redirection
	// buys no isolation while breaking pixi's credential lookup under $HOME.
	var home string
	argv := spec.Argv
	if r.cfg.Mode != ModeOff {
		home = process.WorkspaceHomeDir(spec.WorkspaceDir)
		if err := os.MkdirAll(home, 0o700); err != nil {
			return nil, fmt.Errorf("sandbox: create %s: %w", home, err)
		}
		argv = r.shimArgv(spec)
	}

	parent := spec.ParentEnv
	if parent == nil {
		parent = os.Environ()
	}

	// The resource-limit wrapper shells out to /bin/sh, which applies the
	// ulimits and then execs argv. Both rlimits and the Landlock ruleset the
	// shim installs survive that exec, so the two layers compose: sh limits,
	// shim confines, build runs.
	cmd := process.CommandContext(ctx, argv[0], argv[1:], spec.ResourceLimits)
	cmd.Dir = spec.WorkspaceDir
	// Allowlist first so no parent secret survives, then layer the
	// workspace-scoped overrides on top; last occurrence wins.
	cmd.Env = dedupEnv(append(buildEnv(parent, home, tmp), process.WorkspaceEnvOverrides(spec.WorkspaceDir)...))
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
	for _, file := range readOnlyFiles() {
		argv = append(argv, "--allow-ro-file="+file)
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
//
// /etc is deliberately NOT granted wholesale. config.go searches
// /etc/nebi/ for config.yaml, and both database.dsn and auth.jwt_secret are
// file-loadable, so a readable /etc would hand build code exactly the
// credentials the environment allowlist exists to strip. Only the TLS trust
// stores under /etc are granted here; the handful of individual files a
// build genuinely needs come from readOnlyFiles.
func readOnlyPaths(binary string) []string {
	candidates := []string{
		"/usr", "/lib", "/lib64", "/bin", "/sbin", "/opt",
		"/etc/ssl", "/etc/pki", "/etc/ca-certificates",
	}
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

// readOnlyFiles returns the individual files under /etc a build needs in
// order to resolve DNS names and look up its own user. None of them carry
// secrets, unlike /etc as a whole. Nonexistent files are dropped.
func readOnlyFiles() []string {
	candidates := []string{
		"/etc/resolv.conf",
		"/etc/hosts",
		"/etc/nsswitch.conf",
		"/etc/localtime",
		"/etc/passwd",
		"/etc/group",
		// The dynamic loader opens this on every exec of a dynamically
		// linked binary. glibc falls back to its compiled-in search paths
		// when it is unreadable, so denying it costs a failed open per
		// exec for no benefit. It holds no secrets.
		"/etc/ld.so.cache",
	}

	out := make([]string, 0, len(candidates))
	for _, p := range candidates {
		info, err := os.Stat(p)
		if err != nil || info.IsDir() {
			continue
		}
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

// ErrNetworkUnrestricted reports that Confine established filesystem
// confinement but the kernel is too old to restrict TCP (Landlock ABI v4,
// Linux 6.7+). Callers should warn and continue, including in strict mode:
// the build is confined, just not on the network.
var ErrNetworkUnrestricted = errors.New("sandbox: kernel does not support Landlock network restriction; build network access is unrestricted")

// Restrictions is the ruleset the shim applies to itself before exec'ing the
// build command.
type Restrictions struct {
	RW              []string // directories the build may read and write
	RO              []string // directories the build may read
	ROFiles         []string // individual files the build may read
	RWFiles         []string // individual files the build may read and write
	TCPConnectPorts []int    // TCP ports the build may connect to
}

// Validate rejects rulesets that would be useless or accidentally empty.
func (r Restrictions) Validate() error {
	if len(r.RW) == 0 {
		return errors.New("sandbox: at least one read-write path is required")
	}
	for _, set := range [][]string{r.RW, r.RO, r.ROFiles, r.RWFiles} {
		for _, p := range set {
			if !filepath.IsAbs(p) {
				return fmt.Errorf("sandbox: path %q must be absolute", p)
			}
		}
	}
	// Ports are narrowed to uint16 when handed to the kernel. Config
	// validation already rejects out-of-range values, but the shim parses
	// its own flags, so re-check here rather than let 70000 wrap to 4464.
	for _, p := range r.TCPConnectPorts {
		if p < 1 || p > 65535 {
			return fmt.Errorf("sandbox: TCP port %d out of range 1-65535", p)
		}
	}
	return nil
}

// IsSetupFailure reports whether err came from this Runner's sandbox failing
// to start, rather than from the build command failing on its own merits.
//
// Off mode never runs the shim, so nothing in that path can produce the
// reserved exit code; a build tool that legitimately exits 125 would
// otherwise be mislabelled as a sandbox failure. Prefer this over the
// package-level IsSetupFailure wherever a Runner is in scope.
func (r *Runner) IsSetupFailure(err error) bool {
	return r.cfg.Mode != ModeOff && IsSetupFailure(err)
}

// IsSetupFailure reports whether err carries the reserved sandbox
// setup-failure exit code, so callers can produce an actionable
// operator-facing message. Callers holding a Runner should use its method
// instead, which also accounts for the mode.
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
