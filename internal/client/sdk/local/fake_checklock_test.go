package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/nebari-dev/nebi/internal/artifact"
)

// The fake's CheckLock, until the version CRUD PR (#689) lands. That PR
// adds its own fakeStaleLock and fakePixi.CheckLock to harness_test.go;
// when both are merged, delete this file.

// fakeStaleLock is the fake's stale-lock error. Like the real runner's,
// it is its own type with a StaleLock method (see StaleLockError).
type fakeStaleLock struct{ dir string }

func (e fakeStaleLock) Error() string   { return "lock file is not up to date in " + e.dir }
func (e fakeStaleLock) StaleLock() bool { return true }

// CheckLock returns a fakeStaleLock unless pixi.lock is the one Lock
// writes, and an ordinary error without pixi.toml.
func (f *fakePixi) CheckLock(_ context.Context, dir string, _ io.Writer) error {
	if _, err := os.Stat(filepath.Join(dir, artifact.ManifestFile)); err != nil {
		return err
	}
	lock, err := os.ReadFile(filepath.Join(dir, artifact.LockFile))
	if errors.Is(err, os.ErrNotExist) || (err == nil && string(lock) != fakeLockContent) {
		return fmt.Errorf("pixi lock --check: %w", fakeStaleLock{dir})
	}
	return err
}

// TestFakeCheckLock checks the fake's CheckLock: a missing or changed lock
// is stale, the lock Lock writes is current, and a missing manifest is an
// ordinary error.
func TestFakeCheckLock(t *testing.T) {
	ctx := context.Background()
	f := &fakePixi{}
	dir := t.TempDir()
	if err := f.CheckLock(ctx, dir, io.Discard); err == nil || isStaleLock(err) {
		t.Fatalf("no pixi.toml: want an ordinary error, got %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, artifact.ManifestFile), []byte("[workspace]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := f.CheckLock(ctx, dir, io.Discard); !isStaleLock(err) {
		t.Fatalf("no pixi.lock: want a stale lock, got %v", err)
	}
	if err := f.Lock(ctx, dir, io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := f.CheckLock(ctx, dir, io.Discard); err != nil {
		t.Fatalf("after Lock: want a current lock, got %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, artifact.LockFile), []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := f.CheckLock(ctx, dir, io.Discard); !isStaleLock(err) {
		t.Fatalf("edited pixi.lock: want a stale lock, got %v", err)
	}
}
