package local

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client/jobs"
	"gorm.io/gorm"
)

func TestDeleteRefusesWhileBusy(t *testing.T) {
	h := newHarness(t)
	gate := make(chan struct{})
	started := make(chan struct{}, 1)
	h.pixi.lockGate, h.pixi.lockStarted = gate, started

	p, job, err := h.svc.CreateProject(context.Background(), specRequest("busy"))
	if err != nil {
		t.Fatal(err)
	}
	<-started

	_, err = h.svc.DeleteProject(context.Background(), p.ID, DeleteProjectOptions{DeleteFiles: true})
	if !errors.Is(err, ErrProjectBusy) {
		t.Fatalf("want ErrProjectBusy, got %v", err)
	}
	var busy *BusyError
	if !errors.As(err, &busy) || busy.ProjectID != p.ID || len(busy.JobIDs) != 1 || busy.JobIDs[0] != job.ID {
		t.Fatalf("want a BusyError naming job %s, got %#v", job.ID, err)
	}
	if h.projectCount() != 1 || !exists(p.Path) {
		t.Fatal("a refused delete must change nothing")
	}
	if calls := h.versions.deleteCalls; len(calls) != 0 {
		t.Fatal("a refused delete must not touch versions")
	}

	close(gate)
	h.wait(job)
	if _, err := h.svc.DeleteProject(context.Background(), p.ID, DeleteProjectOptions{}); err != nil {
		t.Fatalf("delete after the job finished: %v", err)
	}
}

func TestDeleteManagedProjectWithFiles(t *testing.T) {
	h := newHarness(t)
	p, _ := h.mustCreate(specRequest("gone"))
	if !p.ManagedDir {
		t.Fatal("precondition: project at the default location is managed")
	}
	h.insertSibling(t, p.ID)

	job, err := h.svc.DeleteProject(context.Background(), p.ID, DeleteProjectOptions{DeleteFiles: true})
	if err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	if job == nil || job.Type != jobs.TypeDeleteFiles || job.ProjectID != p.ID {
		t.Fatalf("want a delete_files job, got %+v", job)
	}
	if h.projectCount() != 1 {
		t.Fatal("the row must be deleted at once, before the files; only the sibling must remain")
	}
	if _, err := h.svc.GetProject(context.Background(), p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted project: want ErrNotFound, got %v", err)
	}
	if n := len(h.versions.versionsOf(t, p.ID)); n != 0 {
		t.Fatalf("versions must be deleted with the project, %d left", n)
	}
	if done := h.wait(job); done.Status != jobs.StatusCompleted {
		t.Fatalf("file removal job: %+v", done)
	}
	if exists(p.Path) {
		t.Fatal("the managed directory must be removed")
	}
	if !exists(h.resolved(h.projectsDir)) {
		t.Fatal("only the project directory may be removed, not the projects dir")
	}
	if left := trashEntries(t, filepath.Dir(p.Path)); len(left) != 0 {
		t.Fatalf("the moved-aside directory must be removed: %v", left)
	}
	if h.pendingCount() != 0 {
		t.Fatal("the removal record must be cleared once the directory is gone")
	}
}

// trashEntries lists the moved-aside directories in dir.
func trashEntries(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if strings.Contains(e.Name(), ".deleting-") {
			out = append(out, e.Name())
		}
	}
	return out
}

// gatedDeletes holds delete_files jobs until gate is closed.
type gatedDeletes struct {
	*jobs.Memory
	gate chan struct{}
}

func (g gatedDeletes) Submit(ctx context.Context, id uuid.UUID, typ jobs.Type, run jobs.Func) (*jobs.Job, error) {
	if typ == jobs.TypeDeleteFiles {
		inner := run
		run = func(ctx context.Context, log io.Writer) error {
			<-g.gate
			return inner(ctx, log)
		}
	}
	return g.Memory.Submit(ctx, id, typ, run)
}

func TestRecreateWhileFilesAreBeingRemoved(t *testing.T) {
	gate := make(chan struct{})
	var runner *jobs.Memory
	h := newHarness(t, func(o *Options) {
		runner = o.Jobs.(*jobs.Memory)
		o.Jobs = gatedDeletes{Memory: runner, gate: gate}
	})
	old, _ := h.mustCreate(specRequest("reuse"))
	delJob, err := h.svc.DeleteProject(context.Background(), old.ID, DeleteProjectOptions{DeleteFiles: true})
	if err != nil {
		t.Fatal(err)
	}
	if exists(old.Path) {
		t.Fatal("the path must be free as soon as DeleteProject returns")
	}

	// The removal job has not run yet; create the same project again.
	p, job := h.mustCreate(specRequest("reuse"))
	if job.Status != jobs.StatusCompleted || p.Path != old.Path || !p.ManagedDir {
		t.Fatalf("recreate: job %+v, project %+v", job, p)
	}

	close(gate)
	if done := h.wait(delJob); done.Status != jobs.StatusCompleted {
		t.Fatalf("removal job: %+v", done)
	}
	if !exists(filepath.Join(p.Path, "pixi.toml")) {
		t.Fatal("the old project's removal must not touch the new project's files")
	}
	if left := trashEntries(t, filepath.Dir(p.Path)); len(left) != 0 {
		t.Fatalf("the moved-aside directory must be removed: %v", left)
	}
}

func TestDeleteFilesWhenDirectoryIsAlreadyGone(t *testing.T) {
	h := newHarness(t)
	p, _ := h.mustCreate(specRequest("vanished"))
	if err := os.RemoveAll(p.Path); err != nil {
		t.Fatal(err)
	}
	job, err := h.svc.DeleteProject(context.Background(), p.ID, DeleteProjectOptions{DeleteFiles: true})
	if err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	if done := h.wait(job); done.Status != jobs.StatusCompleted {
		t.Fatalf("removal job: %+v", done)
	}
	if h.projectCount() != 0 {
		t.Fatal("the row must be deleted")
	}
	if h.pendingCount() != 0 {
		t.Fatal("no removal record may be left when there was nothing to move")
	}
}

// insertSibling adds an unrelated project so tests can check that a delete
// only touches its own rows.
func (h *harness) insertSibling(t *testing.T, notID uuid.UUID) {
	t.Helper()
	sib := &Project{Name: "sibling", Path: filepath.Join(t.TempDir(), "sibling")}
	h.insertRow(sib)
	if sib.ID == notID {
		t.Fatal("sibling collides")
	}
}

func TestDeleteWithoutFilesKeepsDirectory(t *testing.T) {
	h := newHarness(t)
	p, _ := h.mustCreate(specRequest("keep-files"))
	job, err := h.svc.DeleteProject(context.Background(), p.ID, DeleteProjectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if job != nil {
		t.Fatalf("no file removal was asked, so no job: %+v", job)
	}
	if h.projectCount() != 0 {
		t.Fatal("the row must be deleted")
	}
	if !exists(filepath.Join(p.Path, "pixi.toml")) {
		t.Fatal("files must be kept when DeleteFiles is false")
	}
}

func TestDeleteFilesOfUnmanagedDirectoryIsRefused(t *testing.T) {
	h := newHarness(t)
	dir := t.TempDir()
	thesis := filepath.Join(dir, "thesis.tex")
	if err := os.WriteFile(thesis, []byte("chapter 1"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := specRequest("thesis")
	req.Path = dir
	p, _ := h.mustCreate(req)

	_, err := h.svc.DeleteProject(context.Background(), p.ID, DeleteProjectOptions{DeleteFiles: true})
	if !errors.Is(err, ErrFilesNotManaged) {
		t.Fatalf("want ErrFilesNotManaged, got %v", err)
	}
	if h.projectCount() != 1 {
		t.Fatal("a refused delete must keep the row")
	}
	if readFile(t, thesis) != "chapter 1" {
		t.Fatal("files in a directory the user chose must never be removed")
	}

	// Without DeleteFiles the record goes and the folder stays.
	if _, err := h.svc.DeleteProject(context.Background(), p.ID, DeleteProjectOptions{}); err != nil {
		t.Fatal(err)
	}
	if readFile(t, thesis) != "chapter 1" {
		t.Fatal("files must be kept")
	}
}

func TestDeleteIsOneTransaction(t *testing.T) {
	var hookCalls []uuid.UUID
	failing := errors.New("publication rows locked")
	h := newHarness(t, func(o *Options) {
		o.DeleteHooks = []DeleteHook{func(tx *gorm.DB, id uuid.UUID) error {
			hookCalls = append(hookCalls, id)
			return failing
		}}
	})
	p, _ := h.mustCreate(specRequest("atomic"))

	_, err := h.svc.DeleteProject(context.Background(), p.ID, DeleteProjectOptions{DeleteFiles: true})
	if !errors.Is(err, failing) {
		t.Fatalf("want the hook error, got %v", err)
	}
	if len(hookCalls) != 1 || hookCalls[0] != p.ID {
		t.Fatalf("hook calls = %v", hookCalls)
	}
	// DeleteProjectVersions ran inside the transaction; the rollback must
	// bring its deletes back together with the project row.
	if len(h.versions.deleteCalls) != 1 {
		t.Fatalf("DeleteProjectVersions calls = %d, want 1", len(h.versions.deleteCalls))
	}
	if n := len(h.versions.versionsOf(t, p.ID)); n != 1 {
		t.Fatalf("versions must be rolled back, %d left", n)
	}
	if h.projectCount() != 1 {
		t.Fatal("the project row must be rolled back")
	}
	if !exists(filepath.Join(p.Path, "pixi.toml")) {
		t.Fatal("files must be moved back when the records could not be deleted")
	}
	if left := trashEntries(t, filepath.Dir(p.Path)); len(left) != 0 {
		t.Fatalf("no moved-aside directory may be left: %v", left)
	}
	if h.pendingCount() != 0 {
		t.Fatal("the removal record must be dropped when the delete is rolled back")
	}
	if list, _ := h.runner.ProjectJobs(context.Background(), p.ID); len(list) != 1 {
		t.Fatalf("no file removal job may be submitted, jobs: %+v", list)
	}
}

func TestDeleteVersionStoreFailureRollsBack(t *testing.T) {
	h := newHarness(t)
	p, _ := h.mustCreate(specRequest("rollback"))
	h.versions.deleteErr = errors.New("tags table locked")
	if _, err := h.svc.DeleteProject(context.Background(), p.ID, DeleteProjectOptions{}); err == nil {
		t.Fatal("want an error")
	}
	if h.projectCount() != 1 || len(h.versions.versionsOf(t, p.ID)) != 1 {
		t.Fatal("project row and versions must be rolled back")
	}
}

func TestDeleteUnknownProject(t *testing.T) {
	h := newHarness(t)
	if _, err := h.svc.DeleteProject(context.Background(), uuid.New(), DeleteProjectOptions{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestRemoveProjectDirGuards(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, dir := range []string{"", "relative/dir", string(filepath.Separator), home} {
		if err := removeProjectDir(dir); err == nil {
			t.Errorf("removeProjectDir(%q) must refuse", dir)
		}
	}
	if !exists(home) {
		t.Fatal("home must survive")
	}
}

func TestDeleteFilesRefusedWhenAnotherProjectIsInside(t *testing.T) {
	h := newHarness(t)
	outer, _ := h.mustCreate(specRequest("outer"))
	// Create refuses nested paths; a migrated row can still nest.
	innerDir := filepath.Join(outer.Path, "inner")
	if err := os.MkdirAll(innerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	inner := &Project{Name: "inner", Path: innerDir, ManagedDir: true}
	h.insertRow(inner)

	_, err := h.svc.DeleteProject(context.Background(), outer.ID, DeleteProjectOptions{DeleteFiles: true})
	if !errors.Is(err, ErrPathTaken) {
		t.Fatalf("want ErrPathTaken, got %v", err)
	}
	if h.projectCount() != 2 || !exists(innerDir) {
		t.Fatal("a refused delete must keep both rows and the inner project's files")
	}

	// Deleting the inner project's files is fine: nothing else is inside it.
	job, err := h.svc.DeleteProject(context.Background(), inner.ID, DeleteProjectOptions{DeleteFiles: true})
	if err != nil {
		t.Fatalf("delete inner: %v", err)
	}
	h.wait(job)
	if exists(innerDir) || !exists(outer.Path) {
		t.Fatal("only the inner directory may be removed")
	}
}

func TestDeleteRemovesLegacyOrigin(t *testing.T) {
	h := newHarness(t)
	p, _ := h.mustCreate(specRequest("synced"))
	sib := &Project{Name: "sibling", Path: filepath.Join(t.TempDir(), "sibling")}
	h.insertRow(sib)
	for _, id := range []uuid.UUID{p.ID, sib.ID} {
		if err := h.db.Create(&LegacyProjectOrigin{ProjectID: id, OriginName: "team/x"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := h.svc.DeleteProject(context.Background(), p.ID, DeleteProjectOptions{}); err != nil {
		t.Fatal(err)
	}
	var left []LegacyProjectOrigin
	if err := h.db.Find(&left).Error; err != nil {
		t.Fatal(err)
	}
	if len(left) != 1 || left[0].ProjectID != sib.ID {
		t.Fatalf("only the deleted project's origin row may go, left %+v", left)
	}
}

func TestDeleteKeepsLegacyOriginOnRollback(t *testing.T) {
	h := newHarness(t, func(o *Options) {
		o.DeleteHooks = []DeleteHook{func(*gorm.DB, uuid.UUID) error { return errors.New("locked") }}
	})
	p, _ := h.mustCreate(specRequest("synced"))
	if err := h.db.Create(&LegacyProjectOrigin{ProjectID: p.ID, OriginName: "team/x"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.DeleteProject(context.Background(), p.ID, DeleteProjectOptions{}); err == nil {
		t.Fatal("want an error")
	}
	var n int64
	if err := h.db.Model(&LegacyProjectOrigin{}).Count(&n).Error; err != nil || n != 1 {
		t.Fatalf("origin row must be rolled back: %d rows, %v", n, err)
	}
}

func TestDeletedProjectAcceptsNoNewJobs(t *testing.T) {
	h := newHarness(t)
	p, _ := h.mustCreate(specRequest("retired"))
	if _, err := h.svc.DeleteProject(context.Background(), p.ID, DeleteProjectOptions{}); err != nil {
		t.Fatal(err)
	}
	noop := func(context.Context, io.Writer) error { return nil }
	if _, err := h.runner.Submit(context.Background(), p.ID, jobs.TypeInstall, noop); !errors.Is(err, jobs.ErrProjectRetired) {
		t.Fatalf("an install for a deleted project must be refused, got %v", err)
	}
}

func TestFailedDeleteAcceptsJobsAgain(t *testing.T) {
	h := newHarness(t)
	p, _ := h.mustCreate(specRequest("kept"))
	h.versions.deleteErr = errors.New("tags table locked")
	if _, err := h.svc.DeleteProject(context.Background(), p.ID, DeleteProjectOptions{DeleteFiles: true}); err == nil {
		t.Fatal("want an error")
	}
	job, err := h.runner.Submit(context.Background(), p.ID, jobs.TypeInstall, func(context.Context, io.Writer) error { return nil })
	if err != nil {
		t.Fatalf("a project whose delete failed must accept jobs again: %v", err)
	}
	h.wait(job)
}

func TestDeleteFilesRefusedForAliasedProjectInside(t *testing.T) {
	h := newHarness(t)
	outer, _ := h.mustCreate(specRequest("outer"))
	innerDir := filepath.Join(outer.Path, "inner")
	if err := os.MkdirAll(innerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	var aliases []string
	// Migrated rows are stored as they were, so a row can reach the
	// directory through a symlink...
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(outer.Path, link); err == nil {
		aliases = append(aliases, filepath.Join(link, "inner"), link)
	}
	// ...or, on a case-insensitive filesystem, in another case.
	if caseInsensitive(t, filepath.Dir(outer.Path)) {
		upper := filepath.Join(filepath.Dir(outer.Path), "OUTER")
		aliases = append(aliases, filepath.Join(upper, "inner"), upper)
	}
	if len(aliases) == 0 {
		t.Skip("no symlinks and a case-sensitive filesystem")
	}

	for i, alias := range aliases {
		other := &Project{Name: fmt.Sprintf("alias-%d", i), Path: alias, ManagedDir: true}
		h.insertRow(other)
		_, err := h.svc.DeleteProject(context.Background(), outer.ID, DeleteProjectOptions{DeleteFiles: true})
		if !errors.Is(err, ErrPathTaken) {
			t.Fatalf("%s: want ErrPathTaken, got %v", alias, err)
		}
		if !exists(innerDir) || h.pendingCount() != 0 {
			t.Fatalf("%s: a refused delete must change nothing", alias)
		}
		if err := h.db.Delete(other).Error; err != nil {
			t.Fatal(err)
		}
	}
}

// TestDeleteFilesRefusedForProjectMovedInsideBehindSymlink is the case
// where a tracked project's stored path is itself a symlink into the
// directory being deleted: an unmanaged project whose directory the user
// moved into a managed one, leaving a symlink at the old path.
func TestDeleteFilesRefusedForProjectMovedInsideBehindSymlink(t *testing.T) {
	h := newHarness(t)
	outer, _ := h.mustCreate(specRequest("outer"))
	req := specRequest("inner")
	req.Path = filepath.Join(t.TempDir(), "inner")
	inner, _ := h.mustCreate(req)
	if inner.ManagedDir {
		t.Fatal("a project outside the projects directory must not be managed")
	}

	moved := filepath.Join(outer.Path, "inner")
	if err := os.Rename(inner.Path, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(moved, inner.Path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	_, err := h.svc.DeleteProject(context.Background(), outer.ID, DeleteProjectOptions{DeleteFiles: true})
	if !errors.Is(err, ErrPathTaken) {
		t.Fatalf("want ErrPathTaken, got %v", err)
	}
	if !exists(filepath.Join(moved, "pixi.toml")) || h.projectCount() != 2 || h.pendingCount() != 0 {
		t.Fatal("a refused delete must keep both rows and the inner project's files")
	}
}

// TestDeleteFilesChecksNestingAfterMovingBack covers a delete that runs
// while an earlier interrupted delete left the directory moved aside. The
// nested-project check looks at the directory on disk, so it must run
// after the directory is moved back, or it sees nothing inside.
func TestDeleteFilesChecksNestingAfterMovingBack(t *testing.T) {
	h := newHarness(t)
	outer, _ := h.mustCreate(specRequest("outer"))
	innerDir := filepath.Join(outer.Path, "inner")
	if err := os.MkdirAll(innerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// The row reaches inner through a symlink to outer, so only the
	// on-disk check can tell it is inside outer.
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(outer.Path, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	h.insertRow(&Project{Name: "inner", Path: filepath.Join(link, "inner"), ManagedDir: true})
	h.crashMidDelete(outer)

	_, err := h.svc.DeleteProject(context.Background(), outer.ID, DeleteProjectOptions{DeleteFiles: true})
	if !errors.Is(err, ErrPathTaken) {
		t.Fatalf("want ErrPathTaken, got %v", err)
	}
	if !exists(innerDir) || h.projectCount() != 2 || h.pendingCount() != 0 {
		t.Fatal("the directory must be moved back and the inner project's files kept")
	}
	if left := trashEntries(t, filepath.Dir(outer.Path)); len(left) != 0 {
		t.Fatalf("no moved-aside directory may be left: %v", left)
	}
}

func (h *harness) pendingCount() int64 {
	h.t.Helper()
	var n int64
	if err := h.db.Model(&pendingRemoval{}).Count(&n).Error; err != nil {
		h.t.Fatal(err)
	}
	return n
}

// failingDeletes is a JobRunner whose Submit fails for file removal jobs.
type failingDeletes struct{ *jobs.Memory }

func (f failingDeletes) Submit(ctx context.Context, id uuid.UUID, typ jobs.Type, run jobs.Func) (*jobs.Job, error) {
	if typ == jobs.TypeDeleteFiles {
		return nil, errors.New("queue unavailable")
	}
	return f.Memory.Submit(ctx, id, typ, run)
}

// restart returns a new Service on the harness database with its own job
// runner, as after the client restarts.
func (h *harness) restart() (*Service, *jobs.Memory) {
	h.t.Helper()
	runner := jobs.NewMemory(jobs.Options{})
	svc, err := New(Options{DB: h.db, Versions: h.versions, Jobs: runner, Pixi: h.pixi, ProjectsDir: h.projectsDir})
	if err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(func() {
		svc.Close()
		runner.Close()
	})
	return svc, runner
}

func TestRecoverDeletesAfterSubmitFailure(t *testing.T) {
	h := newHarness(t, func(o *Options) { o.Jobs = failingDeletes{o.Jobs.(*jobs.Memory)} })
	p, _ := h.mustCreate(specRequest("stranded"))

	_, err := h.svc.DeleteProject(context.Background(), p.ID, DeleteProjectOptions{DeleteFiles: true})
	if err == nil || !strings.Contains(err.Error(), "could not be started") {
		t.Fatalf("want a submit error, got %v", err)
	}
	if h.projectCount() != 0 || exists(p.Path) {
		t.Fatal("the project is deleted and its path is free")
	}
	if len(trashEntries(t, filepath.Dir(p.Path))) != 1 || h.pendingCount() != 1 {
		t.Fatal("the moved-aside directory must be recorded")
	}

	svc, runner := h.restart()
	started, err := svc.RecoverDeletes(context.Background())
	if err != nil || len(started) != 1 {
		t.Fatalf("RecoverDeletes: %v, %d jobs", err, len(started))
	}
	if done, _ := runner.Wait(context.Background(), started[0].ID); done.Status != jobs.StatusCompleted {
		t.Fatalf("removal job: %+v", done)
	}
	if left := trashEntries(t, filepath.Dir(p.Path)); len(left) != 0 || h.pendingCount() != 0 {
		t.Fatalf("the directory and its record must be gone: %v", left)
	}
}

// crashMidDelete leaves p as a delete that stopped after moving its
// directory aside: the record is written and the directory moved.
func (h *harness) crashMidDelete(p *Project) string {
	h.t.Helper()
	r, err := h.svc.moveAside(context.Background(), p)
	if err != nil || r == nil {
		h.t.Fatalf("moveAside: %v", err)
	}
	return r.TrashPath
}

func TestRecoverDeletesAfterCrash(t *testing.T) {
	t.Run("before the commit the directory is moved back", func(t *testing.T) {
		h := newHarness(t)
		p, _ := h.mustCreate(specRequest("interrupted"))
		trash := h.crashMidDelete(p)

		svc, _ := h.restart()
		started, err := svc.RecoverDeletes(context.Background())
		if err != nil || len(started) != 0 {
			t.Fatalf("RecoverDeletes: %v, %d jobs", err, len(started))
		}
		if !exists(filepath.Join(p.Path, "pixi.toml")) || exists(trash) || h.pendingCount() != 0 {
			t.Fatal("the project directory must be back and the record gone")
		}
	})
	t.Run("after the commit the directory is removed", func(t *testing.T) {
		h := newHarness(t)
		p, _ := h.mustCreate(specRequest("committed"))
		trash := h.crashMidDelete(p)
		if err := h.db.Delete(&Project{}, "id = ?", p.ID).Error; err != nil {
			t.Fatal(err)
		}

		svc, runner := h.restart()
		started, err := svc.RecoverDeletes(context.Background())
		if err != nil || len(started) != 1 {
			t.Fatalf("RecoverDeletes: %v, %d jobs", err, len(started))
		}
		runner.Wait(context.Background(), started[0].ID)
		if exists(trash) || h.pendingCount() != 0 {
			t.Fatal("the moved-aside directory and its record must be gone")
		}
	})
	t.Run("a record without a moved directory is dropped", func(t *testing.T) {
		h := newHarness(t)
		p, _ := h.mustCreate(specRequest("unmoved"))
		if err := h.db.Create(&pendingRemoval{ID: uuid.New(), ProjectID: p.ID, Path: p.Path, TrashPath: p.Path + ".gone"}).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := h.svc.RecoverDeletes(context.Background()); err != nil {
			t.Fatal(err)
		}
		if !exists(p.Path) || h.pendingCount() != 0 {
			t.Fatal("the project must be untouched and the record dropped")
		}
	})
}

func TestRecoverDeletesLeavesRunningRemovalAlone(t *testing.T) {
	gate := make(chan struct{})
	h := newHarness(t, func(o *Options) { o.Jobs = gatedDeletes{Memory: o.Jobs.(*jobs.Memory), gate: gate} })
	p, _ := h.mustCreate(specRequest("in-flight"))
	job, err := h.svc.DeleteProject(context.Background(), p.ID, DeleteProjectOptions{DeleteFiles: true})
	if err != nil {
		t.Fatal(err)
	}
	started, err := h.svc.RecoverDeletes(context.Background())
	if err != nil || len(started) != 0 {
		t.Fatalf("RecoverDeletes: %v, %d jobs", err, len(started))
	}
	close(gate)
	h.wait(job)
	if left := trashEntries(t, filepath.Dir(p.Path)); len(left) != 0 || h.pendingCount() != 0 {
		t.Fatalf("the removal must finish and clear its record: %v", left)
	}
}

func TestDeleteMovesBackDirectoryLeftAside(t *testing.T) {
	h := newHarness(t)
	p, _ := h.mustCreate(specRequest("left-aside"))
	h.crashMidDelete(p)

	// The user deletes the record but keeps the files: they must be back
	// at the project path, not left in the hidden directory.
	if _, err := h.svc.DeleteProject(context.Background(), p.ID, DeleteProjectOptions{}); err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(p.Path, "pixi.toml")) || h.pendingCount() != 0 {
		t.Fatal("the directory must be moved back and the record dropped")
	}
	if left := trashEntries(t, filepath.Dir(p.Path)); len(left) != 0 {
		t.Fatalf("no moved-aside directory may be left: %v", left)
	}
}

func TestDeleteMoveAsideFailureChangesNothing(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs a directory the test cannot write to")
	}
	h := newHarness(t)
	p, _ := h.mustCreate(specRequest("stuck"))
	parent := filepath.Dir(p.Path)
	if err := os.Chmod(parent, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })

	if _, err := h.svc.DeleteProject(context.Background(), p.ID, DeleteProjectOptions{DeleteFiles: true}); err == nil {
		t.Fatal("want an error when the directory cannot be moved")
	}
	if h.projectCount() != 1 || !exists(p.Path) || h.pendingCount() != 0 {
		t.Fatal("a failed move must keep the project, its files, and no record")
	}
	job, err := h.runner.Submit(context.Background(), p.ID, jobs.TypeInstall, func(context.Context, io.Writer) error { return nil })
	if err != nil {
		t.Fatalf("the project must accept jobs again: %v", err)
	}
	h.wait(job)
}

func TestPendingJobKeepsProjectBusy(t *testing.T) {
	h := newHarnessWithRunner(t, jobs.Options{MaxRunning: 1})
	gate := make(chan struct{})
	started := make(chan struct{}, 1)
	h.pixi.lockGate, h.pixi.lockStarted = gate, started

	_, firstJob, err := h.svc.CreateProject(context.Background(), specRequest("first"))
	if err != nil {
		t.Fatal(err)
	}
	<-started
	waiting, waitingJob, err := h.svc.CreateProject(context.Background(), specRequest("waiting"))
	if err != nil {
		t.Fatal(err)
	}
	if waitingJob.Status != jobs.StatusPending {
		t.Fatalf("precondition: the second create waits, got %+v", waitingJob)
	}

	// Project state: a pending create is creating, like a running one.
	if d := h.detail(waiting.ID); d.State != StateCreating || d.JobID == nil || *d.JobID != waitingJob.ID {
		t.Fatalf("want creating with the pending job, got %+v", d.ProjectSummary)
	}
	// Delete guard and ErrProjectBusy.
	_, err = h.svc.DeleteProject(context.Background(), waiting.ID, DeleteProjectOptions{DeleteFiles: true})
	var busy *BusyError
	if !errors.Is(err, ErrProjectBusy) || !errors.As(err, &busy) || len(busy.JobIDs) != 1 || busy.JobIDs[0] != waitingJob.ID {
		t.Fatalf("want a BusyError naming the pending job, got %v", err)
	}
	if h.projectCount() != 2 || !exists(waiting.Path) {
		t.Fatal("a refused delete must change nothing")
	}

	close(gate)
	h.wait(firstJob)
	if done := h.wait(waitingJob); done.Status != jobs.StatusCompleted {
		t.Fatalf("the pending create must run once the slot frees: %+v", done)
	}
	if _, err := h.svc.DeleteProject(context.Background(), waiting.ID, DeleteProjectOptions{DeleteFiles: true}); err != nil {
		t.Fatalf("delete after the job finished: %v", err)
	}
}

func TestRemovalJobCannotBeCancelled(t *testing.T) {
	gate := make(chan struct{})
	release := sync.OnceFunc(func() { close(gate) })
	h := newHarness(t, func(o *Options) {
		o.Jobs = gatedDeletes{Memory: o.Jobs.(*jobs.Memory), gate: gate}
	})
	t.Cleanup(release) // runs before the harness closes the runner
	p, _ := h.mustCreate(specRequest("removing"))
	job, err := h.svc.DeleteProject(context.Background(), p.ID, DeleteProjectOptions{DeleteFiles: true})
	if err != nil {
		t.Fatal(err)
	}
	err = h.svc.jobs.Cancel(context.Background(), job.ID)
	var nc *jobs.NotCancellableError
	if !errors.As(err, &nc) || !errors.Is(err, jobs.ErrNotCancellable) || nc.JobID != job.ID {
		t.Fatalf("want a *jobs.NotCancellableError, got %v", err)
	}
	release()
	if done := h.wait(job); done.Status != jobs.StatusCompleted {
		t.Fatalf("the removal must run to the end: %+v", done)
	}
	if left := trashEntries(t, filepath.Dir(p.Path)); len(left) != 0 || h.pendingCount() != 0 {
		t.Fatalf("the directory and its record must be gone: %v", left)
	}
}
