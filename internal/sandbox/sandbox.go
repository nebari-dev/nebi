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
