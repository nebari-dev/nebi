//go:build linux

// This file is the acceptance test for nebari-dev/nebi#445: "Security tests
// use a deliberately malicious build backend and verify containment."
//
// Everything here runs a real, confined child process against a real kernel.
// It is therefore Linux-only, and it skips rather than fails when the kernel
// cannot supply the Landlock ABI a given case needs, so that older hosts do
// not turn a missing capability into a red build. Read the skip messages: a
// silently skipping run proves nothing.
//
// The malicious backend is testdata/probe, compiled here into a temp
// directory. See its package comment for why it is a Go program and not a
// shell script.
//
// The sandbox re-exec shim is this test binary itself, via TestMain. Building
// the nebi binary in-test is not an option: a plain "go build ./cmd/nebi"
// shares nothing with the race-instrumented cache the CI run has already
// built, so it is a full cold compile of the whole application (measured at
// 84s of CPU and 1.1GB of compiler RSS) landing while the rest of the suite
// is still running.

package sandbox_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	llsys "github.com/landlock-lsm/go-landlock/landlock/syscall"

	"github.com/nebari-dev/nebi/internal/sandbox"
)

// Fake secrets planted in the parent environment. They stand in for the two
// values config.go can load from the environment that would be catastrophic
// to hand to build code.
const (
	plantedDSN       = "NEBI_DATABASE_DSN=host=db user=nebi password=hunter2"
	plantedJWTSecret = "NEBI_AUTH_JWT_SECRET=supersecret"
	dsnPassword      = "hunter2"
	jwtSecret        = "supersecret"
)

// victimManifest is another tenant's pixi.toml. The canary is what must never
// appear in a confined process's output.
const (
	victimCanary   = "canary-tenant-b-private-manifest"
	victimManifest = "[project]\nname = \"tenant-b\"\n# " + victimCanary + "\n"
)

// Landlock ABI versions the individual cases need. The ladder in
// landlock_linux.go degrades one capability at a time, so a case that needs
// refer or TCP rules has to check for it rather than assume.
const (
	abiRefer   = 2 // Linux 5.19+: cross-directory rename inside the workspace
	abiNetwork = 4 // Linux 6.7+: TCP connect restriction
)

// confinedRunTimeout bounds every confined command. A blocked connect(2) or a
// wedged exec must surface as a named failure, not eat the package's ten
// minute test budget.
const confinedRunTimeout = 60 * time.Second

// confineShimSubcommand is the argv[1] that Runner.Command emits when it
// re-execs. runConfined asserts the two still agree.
const confineShimSubcommand = "sandbox-exec"

// TestMain lets this test binary stand in for the nebi binary as the sandbox
// re-exec shim, so the test never has to compile cmd/nebi.
//
// The switch is argv rather than an environment variable because the sandbox
// deliberately scrubs the environment down to an allowlist before the child
// starts: no test-only variable can survive into it. That scrubbing is the
// property the first subtest asserts, so it must not be weakened to make the
// harness convenient.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == confineShimSubcommand {
		runConfineShim(os.Args[2:]) // never returns
	}
	os.Exit(m.Run())
}

// runConfineShim mirrors cmd/nebi/sandbox_exec.go: parse the ruleset off the
// command line, apply it to this process, then execve the real command so the
// confinement is inherited across the exec.
//
// Being a second implementation of that wrapper is a drift risk, so it keeps
// the parts that carry meaning identical: it calls sandbox.Confine, which is
// the code actually under test, and it exits with sandbox.SetupFailureExitCode
// when the sandbox cannot be established, which is the contract
// Runner.IsSetupFailure depends on. An unrecognised flag is a hard error
// rather than something to ignore, so a change to Runner.shimArgv shows up
// here as a loud failure instead of a silently weaker ruleset.
func runConfineShim(args []string) {
	var (
		restrictions sandbox.Restrictions
		mode         = sandbox.ModeStrict
		argv         []string
	)

	for i, arg := range args {
		if arg == "--" {
			argv = args[i+1:]
			break
		}
		// Mirrors cmd/nebi: --check is the probe sandbox.NewRunner uses to
		// prove a re-exec target implements the shim. It must exit 0
		// immediately, without applying a ruleset or exec'ing anything.
		if arg == "--check" {
			os.Exit(0)
		}
		key, value, ok := strings.Cut(arg, "=")
		if !ok {
			shimFail(fmt.Errorf("malformed shim flag %q", arg))
		}
		switch key {
		case "--mode":
			mode = sandbox.Mode(value)
		case "--allow-rw":
			restrictions.RW = append(restrictions.RW, value)
		case "--allow-ro":
			restrictions.RO = append(restrictions.RO, value)
		case "--allow-ro-file":
			restrictions.ROFiles = append(restrictions.ROFiles, value)
		case "--allow-port":
			p, err := strconv.Atoi(value)
			if err != nil {
				shimFail(fmt.Errorf("bad port %q: %w", value, err))
			}
			restrictions.TCPConnectPorts = append(restrictions.TCPConnectPorts, p)
		default:
			shimFail(fmt.Errorf("unknown shim flag %q: Runner.shimArgv has changed and this harness needs updating to match", arg))
		}
	}
	restrictions.RWFiles = shimDevFiles()

	if len(argv) == 0 {
		shimFail(errors.New("no command after --"))
	}
	switch mode {
	case sandbox.ModeStrict, sandbox.ModePermissive:
	default:
		shimFail(fmt.Errorf("shim requires strict or permissive mode, got %q", mode))
	}

	err := sandbox.Confine(restrictions)
	switch {
	case err == nil:
	case errors.Is(err, sandbox.ErrNetworkUnrestricted):
		// Filesystem confinement held; only the TCP restriction is missing.
		fmt.Fprintf(os.Stderr, "[nebi] WARNING: %v\n", err)
	case mode == sandbox.ModePermissive:
		fmt.Fprintf(os.Stderr, "[nebi] WARNING: build is running UNCONFINED: %v\n", err)
	default:
		shimFail(err)
	}

	// syscall.Exec replaces this process image on success and so never
	// returns; any return at all is a failure to launch.
	shimFail(fmt.Errorf("exec %s: %w", argv[0], syscall.Exec(argv[0], argv, os.Environ())))
}

// shimDevFiles mirrors devFiles in cmd/nebi/sandbox_exec.go.
func shimDevFiles() []string {
	var out []string
	for _, f := range []string{"/dev/null", "/dev/urandom", "/dev/random", "/dev/zero"} {
		if _, err := os.Stat(f); err == nil {
			out = append(out, f)
		}
	}
	return out
}

func shimFail(err error) {
	fmt.Fprintf(os.Stderr, "[nebi] sandbox setup failed: %v\n", err)
	os.Exit(sandbox.SetupFailureExitCode)
}

func TestConfinement_MaliciousBuildCannotEscape(t *testing.T) {
	abi := requireLandlock(t)

	// This binary is the re-exec shim (see TestMain), so the only thing that
	// has to be compiled is the malicious build backend, which is a few
	// hundred lines and builds in seconds.
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve this test binary for the sandbox re-exec: %v", err)
	}
	probeBin := goBuild(t, t.TempDir(), "probe", "./testdata/probe")

	// Two sibling workspaces under a shared root. Only "attacker" is ever
	// granted to the confined process.
	root := t.TempDir()
	attacker := mkdir(t, filepath.Join(root, "attacker"))
	victim := mkdir(t, filepath.Join(root, "victim"))
	victimToml := filepath.Join(victim, "pixi.toml")
	if err := os.WriteFile(victimToml, []byte(victimManifest), 0o600); err != nil {
		t.Fatalf("plant the victim manifest: %v", err)
	}
	// Non-vacuity: the manifest must genuinely be readable from outside the
	// sandbox, otherwise the denial below would prove nothing.
	if b, err := os.ReadFile(victimToml); err != nil || !bytes.Contains(b, []byte(victimCanary)) {
		t.Fatalf("the victim manifest must be readable outside the sandbox (err=%v)", err)
	}

	runner := newRunner(t, self, 443)

	// Prove the shim can actually establish a ruleset on this host before
	// asserting anything about what it blocks. Exit code 125 is the shim
	// telling us the sandbox itself could not be set up, which is a skip;
	// anything else means the probe is broken and must be loud.
	probe := runConfined(t, runner, attacker, probeBin, "env")
	switch {
	case probe.exitCode == sandbox.SetupFailureExitCode:
		t.Skipf("the sandbox could not be established on this host, so containment cannot be tested: %s", strings.TrimSpace(probe.stderr))
	case probe.err != nil:
		t.Fatalf("the confinement probe failed for an unrelated reason (exit %d): %v\nstdout:\n%s\nstderr:\n%s",
			probe.exitCode, probe.err, probe.stdout, probe.stderr)
	}

	t.Run("cannot read the server environment", func(t *testing.T) {
		res := runConfined(t, runner, attacker, probeBin, "env")
		assertSucceeded(t, res)

		// Guard against a vacuous pass: an empty or truncated dump would
		// contain no secrets either.
		if !strings.Contains(res.stdout, "PATH=") {
			t.Fatalf("the environment dump looks empty, so the absence of secrets proves nothing:\n%s", res.stdout)
		}
		assertNoLeak(t, res, dsnPassword, jwtSecret)

		// Repeat through a real, dynamically linked system binary. This also
		// proves the read-only grants are sufficient to exec one at all.
		sysEnv := firstExisting("/usr/bin/env", "/bin/env")
		if sysEnv == "" {
			t.Log("no env(1) on this host; skipping the system-binary half of this case")
			return
		}
		res = runConfined(t, runner, attacker, sysEnv)
		if res.err != nil {
			t.Fatalf("a confined build must still be able to run %s: exit %d, %v\nstderr:\n%s", sysEnv, res.exitCode, res.err, res.stderr)
		}
		if !strings.Contains(res.stdout, "PATH=") {
			t.Fatalf("%s produced no environment, so this proves nothing:\n%s", sysEnv, res.stdout)
		}
		assertNoLeak(t, res, dsnPassword, jwtSecret)
	})

	t.Run("cannot read another workspace's manifest", func(t *testing.T) {
		res := runConfined(t, runner, attacker, probeBin, "read", victimToml)
		assertDenied(t, res, "EACCES", "EPERM")
		assertNoLeak(t, res, victimCanary)
	})

	t.Run("cannot write into another workspace", func(t *testing.T) {
		backdoor := filepath.Join(victim, "backdoor")
		res := runConfined(t, runner, attacker, probeBin, "write", backdoor, "pwned")
		assertDenied(t, res, "EACCES", "EPERM", "EROFS")

		if _, err := os.Stat(backdoor); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("expected %s not to exist, stat returned %v", backdoor, err)
		}
	})

	t.Run("can write inside its own workspace", func(t *testing.T) {
		target := filepath.Join(attacker, "build-output.txt")
		res := runConfined(t, runner, attacker, probeBin, "write", target, "legitimate build output")
		assertSucceeded(t, res)

		got, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("a legitimate build write must land on disk: %v", err)
		}
		if string(got) != "legitimate build output" {
			t.Fatalf("unexpected content %q", got)
		}
	})

	// Landlock denies reparenting a file across directories in every domain
	// unless the refer right is granted. Package managers stage a download
	// and then rename it into their cache constantly, so losing this breaks
	// every build; it is the regression the single-ruleset construction in
	// Confine exists to prevent.
	t.Run("can rename across its own subdirectories", func(t *testing.T) {
		if abi < abiRefer {
			t.Skipf("Landlock ABI %d cannot grant the refer right (needs %d, Linux 5.19+), so cross-directory rename is expected to fail here", abi, abiRefer)
		}
		src := filepath.Join(mkdir(t, filepath.Join(attacker, "a")), "staged.tar")
		dst := filepath.Join(mkdir(t, filepath.Join(attacker, "b")), "staged.tar")
		if err := os.WriteFile(src, []byte("package"), 0o600); err != nil {
			t.Fatalf("stage the file: %v", err)
		}

		res := runConfined(t, runner, attacker, probeBin, "rename", src, dst)
		if res.err != nil && strings.Contains(res.stdout, "errno=EXDEV") {
			t.Fatalf("cross-directory rename inside the workspace was blocked; the refer right is missing from the ruleset:\n%s", res.stdout)
		}
		assertSucceeded(t, res)

		if _, err := os.Stat(dst); err != nil {
			t.Fatalf("expected the renamed file at %s: %v", dst, err)
		}
		if _, err := os.Stat(src); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("expected %s to be gone after a rename, stat returned %v", src, err)
		}
	})

	// /etc is not granted wholesale precisely because config.go searches
	// /etc/nebi/ for config.yaml, and both database.dsn and auth.jwt_secret
	// are file-loadable from there.
	t.Run("cannot read the nebi config directory", func(t *testing.T) {
		// Non-vacuity first: an allowlisted file under /etc must still be
		// readable, so a denial below is about the path and not about /etc
		// having become unreachable altogether.
		if _, err := os.Stat("/etc/hosts"); err == nil {
			// /etc/hosts is on the read-only file allowlist, so it must stay
			// readable. Landlock does not restrict directory traversal, only
			// the access itself, which is why granting a single file inside
			// an otherwise ungranted /etc works.
			assertSucceeded(t, runConfined(t, runner, attacker, probeBin, "read", "/etc/hosts"))
		}

		path, canary := pickUnallowlistedEtcFile()
		if path == "" {
			t.Skip("no readable, non-allowlisted regular file found under /etc on this host, so the /etc denial cannot be demonstrated")
		}
		t.Logf("using %s as a stand-in for /etc/nebi/config.yaml", path)

		res := runConfined(t, runner, attacker, probeBin, "read", path)
		assertDenied(t, res, "EACCES", "EPERM")
		assertNoLeak(t, res, canary)
	})

	t.Run("network", func(t *testing.T) {
		if abi < abiNetwork {
			t.Skipf("Landlock ABI %d cannot restrict TCP (needs %d, Linux 6.7+), so build network access is unrestricted on this kernel", abi, abiNetwork)
		}

		// Two loopback listeners on ephemeral ports. Only one of them is put
		// on the allowlist. Using the real ephemeral port rather than 443
		// keeps the positive case unprivileged: binding 443 needs
		// CAP_NET_BIND_SERVICE, which no CI test runner has.
		allowedAddr := listen(t)
		deniedAddr := listen(t)
		netRunner := newRunner(t, self, port(t, allowedAddr))

		t.Run("cannot connect to the database port", func(t *testing.T) {
			res := runConfined(t, netRunner, attacker, probeBin, "connect", deniedAddr)
			if strings.Contains(res.stderr, sandbox.ErrNetworkUnrestricted.Error()) {
				t.Skipf("the kernel established filesystem confinement only: %s", strings.TrimSpace(res.stderr))
			}
			assertDenied(t, res, "EACCES", "EPERM")
		})

		// Without this the case above could pass because the allowlist denies
		// everything, which would hide a sandbox that has broken all builds.
		t.Run("can connect to an allowed port", func(t *testing.T) {
			res := runConfined(t, netRunner, attacker, probeBin, "connect", allowedAddr)
			assertSucceeded(t, res)
		})
	})
}

// result is one confined run.
type result struct {
	stdout   string
	stderr   string
	combined string
	exitCode int
	err      error
}

// runConfined runs argv inside the sandbox with a parent environment that
// deliberately carries fake secrets.
//
// It goes through Runner.Command rather than invoking the shim by hand so
// that the read-only grants the production code computes (interpreters,
// shared libraries, the directory holding the binary) are the ones under
// test. Invoking the shim directly would mean hand-writing --allow-ro flags
// and testing those instead.
//
// argv is passed as a real argv rather than a shell script: two of the
// operations under test cannot be expressed in dash, and an argv has no
// quoting hazards.
func runConfined(t *testing.T, runner *sandbox.Runner, workspace string, argv ...string) result {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), confinedRunTimeout)
	defer cancel()

	// Only PATH is expected to survive the allowlist; the two secrets are
	// what a malicious build is trying to reach.
	parentEnv := []string{plantedDSN, plantedJWTSecret}
	if p := os.Getenv("PATH"); p != "" {
		parentEnv = append(parentEnv, "PATH="+p)
	}

	cmd, err := runner.Command(ctx, sandbox.Spec{
		WorkspaceDir: workspace,
		Argv:         argv,
		ParentEnv:    parentEnv,
	})
	if err != nil {
		t.Fatalf("build the confined command: %v", err)
	}
	// TestMain recognises the shim by this exact subcommand. If Runner.shimArgv
	// ever renames it, say so here rather than letting the test binary try to
	// run its own tests with sandbox flags.
	if len(cmd.Args) < 2 || cmd.Args[1] != confineShimSubcommand {
		t.Fatalf("expected the shim subcommand %q at argv[1], got %v; update confineShimSubcommand and TestMain", confineShimSubcommand, cmd.Args)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	if cmd.ProcessState == nil {
		t.Fatalf("the confined command never started: %v", runErr)
	}
	if ctx.Err() != nil {
		t.Fatalf("the confined command %q did not finish within %s and was killed\nstdout:\n%s\nstderr:\n%s",
			strings.Join(argv, " "), confinedRunTimeout, stdout.String(), stderr.String())
	}
	res := result{
		stdout:   stdout.String(),
		stderr:   stderr.String(),
		exitCode: cmd.ProcessState.ExitCode(),
		err:      runErr,
	}
	res.combined = res.stdout + res.stderr
	t.Logf("confined %s -> exit %d", strings.Join(argv, " "), res.exitCode)
	return res
}

// assertDenied fails unless the operation was refused by the kernel. It
// insists on one of wantErrnos so that a failure for an unrelated reason
// (a missing file, a broken binary, a sandbox that never started) cannot be
// mistaken for containment working.
func assertDenied(t *testing.T, res result, wantErrnos ...string) {
	t.Helper()

	if res.exitCode == sandbox.SetupFailureExitCode {
		t.Fatalf("the sandbox failed to start, so a blocked operation proves nothing: %s", strings.TrimSpace(res.stderr))
	}
	if res.err == nil {
		t.Fatalf("expected the operation to be blocked, but it succeeded:\nstdout:\n%s", res.stdout)
	}
	for _, want := range wantErrnos {
		if strings.Contains(res.stdout, "errno="+want) {
			return
		}
	}
	t.Fatalf("expected the operation to fail with one of %v (a kernel denial), got exit %d:\nstdout:\n%s\nstderr:\n%s",
		wantErrnos, res.exitCode, res.stdout, res.stderr)
}

func assertSucceeded(t *testing.T, res result) {
	t.Helper()

	if res.exitCode == sandbox.SetupFailureExitCode {
		t.Fatalf("the sandbox failed to start: %s", strings.TrimSpace(res.stderr))
	}
	if res.err != nil {
		t.Fatalf("expected the operation to succeed, got exit %d (%v):\nstdout:\n%s\nstderr:\n%s",
			res.exitCode, res.err, res.stdout, res.stderr)
	}
	if !strings.Contains(res.stdout, " OK") {
		t.Fatalf("expected an OK line from the probe, got:\n%s", res.stdout)
	}
}

// assertNoLeak checks both streams: a leak is a leak wherever it is written.
func assertNoLeak(t *testing.T, res result, secrets ...string) {
	t.Helper()

	for _, secret := range secrets {
		if strings.Contains(res.combined, secret) {
			t.Fatalf("a confined build got hold of %q:\nstdout:\n%s\nstderr:\n%s", secret, res.stdout, res.stderr)
		}
	}
}

// requireLandlock reports the kernel's Landlock ABI version, skipping when
// there is none. Asking the kernel directly is more precise than inferring
// support from an operation that unexpectedly succeeded: it lets each case
// state the ABI it needs.
func requireLandlock(t *testing.T) int {
	t.Helper()

	abi, err := llsys.LandlockGetABIVersion()
	if err != nil || abi < 1 {
		t.Skipf("this kernel has no Landlock support (abi=%d, err=%v), so build confinement cannot be tested here", abi, err)
	}
	t.Logf("kernel Landlock ABI version %d", abi)
	return abi
}

// goBuildTimeout bounds the one compile this test still does, so a wedged
// toolchain fails by name instead of silently draining the test budget.
const goBuildTimeout = 3 * time.Minute

func goBuild(t *testing.T, outDir, name, pkg string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), goBuildTimeout)
	defer cancel()

	bin := filepath.Join(outDir, name)
	out, err := exec.CommandContext(ctx, "go", "build", "-o", bin, pkg).CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("go build %s did not finish within %s:\n%s", pkg, goBuildTimeout, out)
	}
	if err != nil {
		t.Fatalf("go build %s: %v\n%s", pkg, err, out)
	}
	return bin
}

// newRunner points the Runner at shimBin as the binary to re-exec. In
// production that is the nebi binary; here it is this test binary, which
// answers to the same "sandbox-exec" subcommand.
func newRunner(t *testing.T, shimBin string, allowedPorts ...int) *sandbox.Runner {
	t.Helper()

	r, err := sandbox.NewRunner(sandbox.Config{Mode: sandbox.ModeStrict, AllowedPorts: allowedPorts}, shimBin)
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	return r
}

func mkdir(t *testing.T, path string) string {
	t.Helper()

	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	return path
}

// listen starts a loopback listener that stands in for the database and
// returns its address. It accepts and immediately closes connections so a
// permitted connect completes rather than lingering in the backlog.
func listen(t *testing.T) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	// Accept returns an error as soon as the listener is closed, which is the
	// only way this loop ends. The bounded wait below means that even if some
	// future Go release stopped unblocking Accept on Close, the test would
	// report it rather than hang.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	t.Cleanup(func() {
		_ = ln.Close()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Errorf("the listener accept loop on %s did not stop after Close", ln.Addr())
		}
	})
	return ln.Addr().String()
}

func port(t *testing.T, addr string) int {
	t.Helper()

	_, p, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split %s: %v", addr, err)
	}
	n, err := strconv.Atoi(p)
	if err != nil {
		t.Fatalf("parse port %q: %v", p, err)
	}
	return n
}

func firstExisting(paths ...string) string {
	for _, p := range paths {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}

// pickUnallowlistedEtcFile finds a file under /etc that stands in for
// /etc/nebi/config.yaml: readable outside the sandbox, a regular file that
// really lives under /etc (not a usrmerge symlink into /usr, which is granted
// read-only and so would be readable for a legitimate reason), and absent
// from readOnlyFiles. The real config file is tried first for the hosts that
// have one. It returns the path and a canary line from its contents.
func pickUnallowlistedEtcFile() (path, canary string) {
	candidates := []string{
		"/etc/nebi/config.yaml",
		"/etc/fstab",
		"/etc/login.defs",
		"/etc/shells",
		"/etc/services",
		"/etc/protocols",
		"/etc/debian_version",
		"/etc/machine-id",
	}
	for _, p := range candidates {
		resolved, err := filepath.EvalSymlinks(p)
		if err != nil || !strings.HasPrefix(resolved, "/etc/") {
			continue
		}
		info, err := os.Lstat(resolved)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		line := longestLine(string(b))
		if len(line) < 8 {
			continue
		}
		return p, line
	}
	return "", ""
}

func longestLine(s string) string {
	var longest string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if len(line) > len(longest) {
			longest = line
		}
	}
	if len(longest) > 200 {
		longest = longest[:200]
	}
	return longest
}
