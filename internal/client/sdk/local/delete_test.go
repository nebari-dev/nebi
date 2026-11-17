package local

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
	if done := h.wait(job); done.Status != jobs.StatusSucceeded {
		t.Fatalf("file removal job: %+v", done)
	}
	if exists(p.Path) {
		t.Fatal("the managed directory must be removed")
	}
	if !exists(h.resolved(h.projectsDir)) {
		t.Fatal("only the project directory may be removed, not the projects dir")
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
	if !exists(p.Path) {
		t.Fatal("files must not be removed when the records could not be deleted")
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
