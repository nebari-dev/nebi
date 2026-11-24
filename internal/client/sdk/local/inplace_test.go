package local

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nebari-dev/nebi/internal/artifact"
	"github.com/nebari-dev/nebi/internal/client/jobs"
)

func (h *harness) pendingRestores() int64 {
	h.t.Helper()
	var n int64
	if err := h.db.Model(&pendingRestore{}).Count(&n).Error; err != nil {
		h.t.Fatal(err)
	}
	return n
}

func TestInPlaceSolveKeepsEditsMadeDuringTheLock(t *testing.T) {
	h := newHarness(t)
	p, _ := h.newProject("edit-during-lock", true)
	gate := make(chan struct{})
	started := make(chan struct{}, 1)
	h.pixi.mu.Lock()
	h.pixi.lockGate, h.pixi.lockStarted = gate, started
	h.pixi.mu.Unlock()

	res, err := h.svc.CreateVersion(context.Background(), CreateVersionRequest{ProjectID: p.ID, BaseVersionID: h.versions.versionsOf(t, p.ID)[0].ID, Manifest: pathDepManifest})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	// The user saves pixi.toml while pixi lock runs in the project.
	manifestPath := filepath.Join(p.Path, artifact.ManifestFile)
	edited := pathDepManifest + "# saved during the lock\n"
	if err := os.WriteFile(manifestPath, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	close(gate)

	done := h.wait(res.Job)
	if done.Status != jobs.StatusFailed || !strings.Contains(done.Error, "changed") {
		t.Fatalf("want the solve to fail because pixi.toml changed, got %+v", done)
	}
	if got := readFile(t, manifestPath); got != edited {
		t.Fatalf("the edit must be kept, pixi.toml is:\n%s", got)
	}
	if n := len(h.versions.versionsOf(t, p.ID)); n != 1 {
		t.Fatalf("a solve whose pixi.toml changed must record nothing, %d versions", n)
	}
	if n := h.pendingRestores(); n != 0 {
		t.Fatalf("the in-place record must be dropped, %d left", n)
	}
}

func TestFileWritesKeepSymlinksAndPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks and unix permissions")
	}
	h := newHarness(t)
	p, v1 := h.newProject("linked", true)
	shared := filepath.Join(t.TempDir(), "shared-pixi.toml")
	if err := os.WriteFile(shared, []byte(v1.Manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(p.Path, artifact.ManifestFile)
	if err := os.Remove(manifestPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(shared, manifestPath); err != nil {
		t.Fatal(err)
	}
	check := func(want string) {
		t.Helper()
		info, err := os.Lstat(manifestPath)
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("pixi.toml must stay a symlink: %v, %v", info, err)
		}
		target, err := os.Stat(shared)
		if err != nil || target.Mode().Perm() != 0o600 {
			t.Fatalf("the linked file must keep mode 0600: %v, %v", target, err)
		}
		if got := readFile(t, shared); got != want {
			t.Fatalf("linked file:\n%s\nwant:\n%s", got, want)
		}
	}

	// An in-place solve puts the files back through the link.
	h.createVersion(CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, Manifest: pathDepManifest})
	check(v1.Manifest)

	// An install writes through the link.
	v2 := h.versions.versionsOf(t, p.ID)[1]
	h.createVersion(CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v2.ID, Install: true})
	check(v2.Manifest)
}

func TestRecoverInPlaceSolvesAfterCrash(t *testing.T) {
	ctx := context.Background()
	for _, edited := range []bool{false, true} {
		t.Run(map[bool]string{false: "put back", true: "edited since"}[edited], func(t *testing.T) {
			h := newHarness(t)
			p, v1 := h.newProject("crashed", true)
			j := versionJob{projectID: p.ID, dir: p.Path, base: &v1, manifest: pathDepManifest}
			if _, _, err := h.svc.beginInPlace(ctx, j); err != nil {
				t.Fatal(err)
			}
			// The process stops here, with the solve's files in place.
			manifestPath := filepath.Join(p.Path, artifact.ManifestFile)
			if readFile(t, manifestPath) != pathDepManifest {
				t.Fatal("precondition: the new pixi.toml is in place")
			}
			userEdit := pathDepManifest + "# kept\n"
			if edited {
				if err := os.WriteFile(manifestPath, []byte(userEdit), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			r := h.restarted()
			if err := r.svc.RecoverInPlaceSolves(ctx); err != nil {
				t.Fatalf("RecoverInPlaceSolves: %v", err)
			}
			if edited {
				if readFile(t, manifestPath) != userEdit {
					t.Fatal("a pixi.toml changed after the crash must be left as it is")
				}
			} else if readFile(t, manifestPath) != v1.Manifest || readFile(t, filepath.Join(p.Path, artifact.LockFile)) != v1.Lock {
				t.Fatal("recovery must put the version's files back")
			}
			if n := h.pendingRestores(); n != 0 {
				t.Fatalf("recovery must drop the record, %d left", n)
			}
		})
	}
}

func TestDeleteProjectDropsInPlaceRecord(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	p, v1 := h.newProject("crashed-then-deleted", true)
	if _, _, err := h.svc.beginInPlace(ctx, versionJob{projectID: p.ID, dir: p.Path, base: &v1, manifest: pathDepManifest}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.DeleteProject(ctx, p.ID, DeleteProjectOptions{}); err != nil {
		t.Fatal(err)
	}
	if n := h.pendingRestores(); n != 0 {
		t.Fatalf("delete must drop the project's in-place record, %d left", n)
	}
}
