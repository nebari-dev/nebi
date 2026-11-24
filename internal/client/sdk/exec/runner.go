// Package exec runs pixi for the client SDK (issue #684). It owns every
// pixi process the reworked client starts and nothing else: it imports no
// database package and knows nothing about projects or versions.
// *Runner implements local.PixiRunner; the install and uninstall
// operations in internal/client/sdk/local reach pixi through it.
package exec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/nebari-dev/nebi/internal/artifact"
	"github.com/nebari-dev/nebi/internal/limits"
	"github.com/nebari-dev/nebi/internal/pixi"
	"github.com/nebari-dev/nebi/internal/process"
)

// Options configures a Runner.
type Options struct {
	// PixiPath is the pixi binary. When it is set, that binary and no
	// other is used, and nothing is downloaded (#630). A bare name such as
	// "pixi" is looked up in PATH once, by New.
	//
	// Empty means the binary internal/pixi resolves for the old client:
	// ~/.local/bin/pixi, then pixi in PATH, and as a last resort the
	// pinned release, downloaded to ~/.local/bin.
	PixiPath string
}

// Runner runs pixi in project directories. It implements local.PixiRunner
// and is safe for concurrent use.
type Runner struct {
	path    string
	version string
}

// New resolves the pixi binary once and returns a Runner that runs it for
// every command. It refuses a pixi older than pixi.MinimumVersion with a
// *pixi.VersionTooOldError, and a binary that does not run.
func New(ctx context.Context, opts Options) (*Runner, error) {
	path := opts.PixiPath
	if path == "" {
		m, err := pixi.NewWithPathContext(ctx, "")
		if err != nil {
			return nil, fmt.Errorf("finding pixi: %w", err)
		}
		path = m.BinaryPath()
	}
	// Pin a bare name or a relative path to one file now, so a later
	// change to PATH or the working directory cannot swap the binary.
	resolved, err := osexec.LookPath(path)
	if err != nil {
		return nil, fmt.Errorf("finding pixi: %w", err)
	}
	if resolved, err = filepath.Abs(resolved); err != nil {
		return nil, fmt.Errorf("finding pixi: %w", err)
	}
	version, err := pixi.CheckVersion(ctx, resolved)
	if err != nil {
		return nil, err
	}
	return &Runner{path: resolved, version: version}, nil
}

// Path returns the absolute path of the pixi binary every command runs.
func (r *Runner) Path() string { return r.path }

// Version returns the version of that binary, such as "0.82.1".
func (r *Runner) Version() string { return r.version }

// Lock runs `pixi lock`: it solves dir/pixi.toml and writes dir/pixi.lock.
func (r *Runner) Lock(ctx context.Context, dir string, out io.Writer) error {
	_, err := r.run(ctx, dir, out, "lock")
	return err
}

// Install runs `pixi install --frozen`: it installs exactly what
// dir/pixi.lock records into dir/.pixi, never solves, and never changes
// pixi.lock, even when it no longer satisfies pixi.toml. Without a
// pixi.lock it fails before starting pixi.
func (r *Runner) Install(ctx context.Context, dir string, out io.Writer) error {
	if err := requireFile(dir, artifact.LockFile); err != nil {
		return fmt.Errorf("pixi install --frozen: %w; run pixi lock first", err)
	}
	_, err := r.run(ctx, dir, out, "install", "--frozen")
	return err
}

// CheckLock runs `pixi lock --check`, which reports whether dir/pixi.lock
// satisfies dir/pixi.toml without writing either file. It returns nil for
// a current lock and a *StaleLockError for a stale or missing one. Any
// other error means the check could not run. A current lock is checked
// offline; a stale one is solved to report what changed, so with no
// network it can fail with an ordinary error instead.
func (r *Runner) CheckLock(ctx context.Context, dir string, out io.Writer) error {
	if err := requireFile(dir, artifact.ManifestFile); err != nil {
		return fmt.Errorf("pixi lock --check: %w", err)
	}
	if err := requireFile(dir, artifact.LockFile); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &StaleLockError{Dir: dir, Detail: artifact.LockFile + " is missing"}
		}
		return fmt.Errorf("pixi lock --check: %w", err)
	}
	stderr, err := r.run(ctx, dir, out, "lock", "--check")
	var cmdErr *CommandError
	if errors.As(err, &cmdErr) && isStaleLockOutput(stderr) {
		return &StaleLockError{Dir: dir, Detail: cmdErr.Line}
	}
	return err
}

// requireFile returns an error wrapping fs.ErrNotExist when dir/name does
// not exist, and an error when it is not a regular file.
func requireFile(dir, name string) error {
	path := filepath.Join(dir, name)
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", path)
	}
	return nil
}

// stderrTail is how much of pixi's stderr is kept to find its error.
const stderrTail = 64 << 10

// run runs pixi with args for the manifest in dir, with dir as the working
// directory. pixi's stdout and stderr go to out. It returns the tail of
// pixi's stderr. A failed command returns a *CommandError carrying pixi's
// error, and a cancelled one an error wrapping ctx's error (see result).
func (r *Runner) run(ctx context.Context, dir string, out io.Writer, args ...string) (string, error) {
	name := "pixi " + strings.Join(args, " ")
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	// pixi runs in dir and resolves --manifest-path from there, so a
	// relative dir is made absolute first or it would be applied twice.
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	// --manifest-path stops pixi from searching the parent directories
	// for a manifest when dir has none.
	args = append(args, "--manifest-path", filepath.Join(dir, artifact.ManifestFile))
	cmd := process.CommandContext(ctx, r.path, args, limits.ProcessLimits{})
	cmd.Dir = dir
	cmd.Env = commandEnv(os.Environ())

	// os/exec copies stdout and stderr on separate goroutines; mu keeps
	// their writes to out from interleaving or racing.
	var mu sync.Mutex
	tail := &tailBuffer{max: stderrTail}
	cmd.Stdout = &lockedWriter{mu: &mu, w: out}
	cmd.Stderr = &lockedWriter{mu: &mu, w: io.MultiWriter(tail, out)}

	err = cmd.Run()
	mu.Lock()
	stderr := tail.String()
	mu.Unlock()
	return stderr, result(ctx, name, stderr, err)
}

// result returns run's error for a pixi command that ended with err. A
// command that succeeded returns nil even when ctx was cancelled after it
// finished: pixi has already written pixi.lock or .pixi, and the caller
// must not take that work as undone. A failure is reported as a cancel
// when ctx is done, since the cancel is what stopped pixi.
func result(ctx context.Context, name, stderr string, err error) error {
	if err == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("%s: %w", name, ctxErr)
	}
	return &CommandError{Command: name, Line: errorLine(stderr), Err: err}
}

// commandEnv returns env for a pixi process. PIXI_LOCKED and PIXI_FROZEN
// are dropped, because they would change what lock and install do, and
// colour is turned off, because the output goes to a log and its error
// line is parsed.
func commandEnv(env []string) []string {
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		key, _, _ := strings.Cut(kv, "=")
		switch strings.ToUpper(key) {
		case "PIXI_LOCKED", "PIXI_FROZEN", "PIXI_COLOR":
			continue
		}
		out = append(out, kv)
	}
	return append(out, "PIXI_COLOR=never")
}

// lockedWriter serializes writes to w with mu.
type lockedWriter struct {
	mu *sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// tailBuffer keeps the last max bytes written to it. It grows as output
// arrives, up to max, and then wraps around, so a write copies only its
// own bytes however full the buffer is (the same ring as the job log in
// internal/client/jobs).
type tailBuffer struct {
	max   int
	buf   []byte // len(buf) <= max
	start int    // index of the oldest byte once len(buf) == max; else 0
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if len(p) >= t.max {
		t.buf = append(t.buf[:0], p[len(p)-t.max:]...)
		t.start = 0
		return n, nil
	}
	if room := t.max - len(t.buf); room > 0 {
		k := min(room, len(p))
		t.buf = append(t.buf, p[:k]...)
		p = p[k:]
	}
	for len(p) > 0 {
		k := copy(t.buf[t.start:], p)
		p = p[k:]
		t.start = (t.start + k) % t.max
	}
	return n, nil
}

func (t *tailBuffer) String() string {
	return string(t.buf[t.start:]) + string(t.buf[:t.start])
}
