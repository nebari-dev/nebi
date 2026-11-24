package exec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// These tests run the real pixi. NEBI_TEST_PIXI lists the binaries to run
// them against, separated like PATH; CI sets it to the pinned version
// (internal/pixi/installer.go) and the latest release. Without it they are
// skipped. They solve against conda-forge, so they need the network.
const testPixiEnv = "NEBI_TEST_PIXI"

func testPixis(t *testing.T) []string {
	t.Helper()
	var paths []string
	for _, p := range filepath.SplitList(os.Getenv(testPixiEnv)) {
		if p = strings.TrimSpace(p); p != "" {
			paths = append(paths, p)
		}
	}
	if len(paths) == 0 {
		t.Skipf("set %s to the pixi binaries to test against", testPixiEnv)
	}
	return paths
}

// condaPlatform is the conda platform of this machine.
func condaPlatform(t *testing.T) string {
	t.Helper()
	p, ok := map[string]string{
		"darwin/arm64":  "osx-arm64",
		"darwin/amd64":  "osx-64",
		"linux/amd64":   "linux-64",
		"linux/arm64":   "linux-aarch64",
		"windows/amd64": "win-64",
		"windows/arm64": "win-arm64",
	}[runtime.GOOS+"/"+runtime.GOARCH]
	if !ok {
		t.Skipf("no conda platform for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	return p
}

func manifest(platform string, deps ...string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[workspace]\nname = \"exec-test\"\nchannels = [\"conda-forge\"]\nplatforms = [%q]\n\n[dependencies]\n", platform)
	for _, d := range deps {
		fmt.Fprintf(&b, "%s = \"*\"\n", d)
	}
	return b.String()
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestRealPixi(t *testing.T) {
	platform := condaPlatform(t)
	for _, bin := range testPixis(t) {
		r, err := New(context.Background(), Options{PixiPath: bin})
		if err != nil {
			t.Fatalf("New(%s): %v", bin, err)
		}
		t.Run(r.Version(), func(t *testing.T) {
			t.Run("LockCheckInstall", func(t *testing.T) { testLockCheckInstall(t, r, platform) })
			t.Run("FailedLock", func(t *testing.T) { testFailedLock(t, r, platform) })
			t.Run("NoParentManifest", func(t *testing.T) { testNoParentManifest(t, r, platform) })
		})
	}
}

func testLockCheckInstall(t *testing.T, r *Runner, platform string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	dir := t.TempDir()
	manifestPath, lockPath := filepath.Join(dir, "pixi.toml"), filepath.Join(dir, "pixi.lock")
	writeFile(t, manifestPath, manifest(platform, "zlib"))

	// Without a lock: install fails before pixi runs, and the check
	// reports a stale lock. Neither writes anything.
	if err := r.Install(ctx, dir, io.Discard); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Install without pixi.lock: want a not-exist error, got %v", err)
	}
	if err := r.CheckLock(ctx, dir, io.Discard); !isStale(err) {
		t.Fatalf("CheckLock without pixi.lock: want a stale lock, got %v", err)
	}
	assertEntries(t, dir, "pixi.toml")

	// Lock writes pixi.lock, and the check passes without changing it.
	var out bytes.Buffer
	if err := r.Lock(ctx, dir, &out); err != nil {
		t.Fatalf("Lock: %v\n%s", err, out.String())
	}
	if out.Len() == 0 {
		t.Error("pixi's output must reach out")
	}
	lock := readFile(t, lockPath)
	if !strings.Contains(lock, "zlib") {
		t.Fatalf("pixi.lock must record zlib:\n%s", lock)
	}
	if err := r.CheckLock(ctx, dir, io.Discard); err != nil {
		t.Fatalf("CheckLock on a fresh lock: %v", err)
	}
	assertEntries(t, dir, "pixi.lock", "pixi.toml")
	if readFile(t, lockPath) != lock {
		t.Fatal("CheckLock changed pixi.lock")
	}

	// Install materializes the environment and leaves pixi.lock as it is.
	out.Reset()
	if err := r.Install(ctx, dir, &out); err != nil {
		t.Fatalf("Install: %v\n%s", err, out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, ".pixi", "envs", "default", "conda-meta")); err != nil {
		t.Fatalf("Install must create the environment: %v", err)
	}
	if readFile(t, lockPath) != lock {
		t.Fatal("Install changed pixi.lock")
	}

	// A new dependency makes the lock stale. The check says so and changes
	// neither file; a frozen install still installs the old lock without
	// solving or rewriting it.
	edited := manifest(platform, "zlib", "bzip2")
	writeFile(t, manifestPath, edited)
	out.Reset()
	err := r.CheckLock(ctx, dir, &out)
	var stale *StaleLockError
	if !errors.As(err, &stale) || !stale.StaleLock() || !strings.Contains(stale.Detail, "not up-to-date") {
		t.Fatalf("CheckLock after editing pixi.toml: want a *StaleLockError with pixi's message, got %T %v\n%s", err, err, out.String())
	}
	if readFile(t, lockPath) != lock || readFile(t, manifestPath) != edited {
		t.Fatal("CheckLock changed pixi.toml or pixi.lock")
	}
	if err := r.Install(ctx, dir, io.Discard); err != nil {
		t.Fatalf("frozen Install with a stale lock: %v", err)
	}
	if readFile(t, lockPath) != lock {
		t.Fatal("a frozen Install changed a stale pixi.lock")
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, ".pixi", "envs", "default", "conda-meta", "bzip2-*.json")); len(matches) != 0 {
		t.Fatalf("a frozen Install must not solve the edited pixi.toml, found %v", matches)
	}
}

func testFailedLock(t *testing.T, r *Runner, platform string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "pixi.toml"), manifest(platform, "nebi-no-such-package-xyz"))
	var out bytes.Buffer
	err := r.Lock(ctx, dir, &out)
	var cmdErr *CommandError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("want a *CommandError, got %T %v", err, err)
	}
	if !strings.Contains(err.Error(), "nebi-no-such-package-xyz") || !strings.Contains(err.Error(), "No candidates were found") {
		t.Fatalf("the error must carry pixi's error line, got %q", err)
	}
	if !strings.Contains(out.String(), "No candidates were found") {
		t.Fatalf("pixi's stderr must reach out, got %q", out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "pixi.lock")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a failed lock must not write pixi.lock: %v", err)
	}
}

// testNoParentManifest checks that a directory without pixi.toml is not
// served by a pixi.toml in a parent directory, which pixi would otherwise
// find by searching upwards.
func testNoParentManifest(t *testing.T, r *Runner, platform string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	parent := t.TempDir()
	writeFile(t, filepath.Join(parent, "pixi.toml"), manifest(platform))
	dir := filepath.Join(parent, "child")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := r.Lock(ctx, dir, io.Discard); err == nil {
		t.Fatal("Lock in a directory without pixi.toml must fail")
	}
	assertEntries(t, parent, "child", "pixi.toml")
	assertEntries(t, dir)
}

func isStale(err error) bool {
	var stale *StaleLockError
	return errors.As(err, &stale) && stale.StaleLock()
}

// assertEntries checks that dir holds exactly the named entries.
func assertEntries(t *testing.T, dir string, want ...string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("%s holds %q, want %q", dir, got, want)
	}
}
