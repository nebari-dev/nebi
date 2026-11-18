package local

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client/jobs"
)

func TestDeriveState(t *testing.T) {
	job := func(typ jobs.Type, status jobs.Status, stage string) jobs.Job {
		return jobs.Job{ID: uuid.New(), Type: typ, Status: status, Stage: stage}
	}
	createRunning := job(jobs.TypeCreate, jobs.StatusRunning, StageLock)
	createPending := job(jobs.TypeCreate, jobs.StatusPending, "")
	createFailedLock := job(jobs.TypeCreate, jobs.StatusFailed, StageLock)
	createFailedInstall := job(jobs.TypeCreate, jobs.StatusFailed, StageInstall)
	createOK := job(jobs.TypeCreate, jobs.StatusCompleted, StageLock)
	createOKInstalled := job(jobs.TypeCreate, jobs.StatusCompleted, StageInstall)
	installRunning := job(jobs.TypeInstall, jobs.StatusRunning, "")
	installFailed := job(jobs.TypeInstall, jobs.StatusFailed, "")
	installOK := job(jobs.TypeInstall, jobs.StatusCompleted, "")
	uninstallPending := job(jobs.TypeUninstall, jobs.StatusPending, "")
	uninstallFailed := job(jobs.TypeUninstall, jobs.StatusFailed, "")
	deleteFiles := job(jobs.TypeDeleteFiles, jobs.StatusRunning, "")
	createCancelledLock := job(jobs.TypeCreate, jobs.StatusCancelled, StageLock)
	createCancelledPending := job(jobs.TypeCreate, jobs.StatusCancelled, "")
	createCancelledInstall := job(jobs.TypeCreate, jobs.StatusCancelled, StageInstall)
	installPending := job(jobs.TypeInstall, jobs.StatusPending, "")
	installCancelled := job(jobs.TypeInstall, jobs.StatusCancelled, "")
	uninstallCancelled := job(jobs.TypeUninstall, jobs.StatusCancelled, "")

	cases := []struct {
		name      string
		versions  int
		jobs      []jobs.Job
		installed bool
		want      State
		wantJob   *jobs.Job
	}{
		{"create running", 0, []jobs.Job{createRunning}, false, StateCreating, &createRunning},
		{"create pending", 0, []jobs.Job{createPending}, false, StateCreating, &createPending},
		{"create running at install stage", 1, []jobs.Job{job(jobs.TypeCreate, jobs.StatusRunning, StageInstall)}, false, StateCreating, nil},
		{"lock failed", 0, []jobs.Job{createFailedLock}, false, StateLockFailed, &createFailedLock},
		{"no versions and no job (lost on restart)", 0, nil, false, StateLockFailed, nil},
		{"no versions even if create completed", 0, []jobs.Job{createOK}, false, StateLockFailed, &createOK},
		{"created, not installed", 1, []jobs.Job{createOK}, false, StateNotInstalled, nil},
		{"created and installed", 1, []jobs.Job{createOKInstalled}, true, StateInstalled, nil},
		{"install failed in create", 1, []jobs.Job{createFailedInstall}, true, StateInstallFailed, &createFailedInstall},
		{"lock failed then fixed by a new version", 1, []jobs.Job{createFailedLock}, false, StateNotInstalled, nil},
		{"installing", 2, []jobs.Job{createOK, installRunning}, false, StateInstalling, &installRunning},
		{"uninstalling", 2, []jobs.Job{createOK, installOK, uninstallPending}, true, StateUninstalling, &uninstallPending},
		{"install failed wins over disk", 1, []jobs.Job{createOK, installFailed}, true, StateInstallFailed, &installFailed},
		{"later install completed", 1, []jobs.Job{installFailed, installOK}, true, StateInstalled, nil},
		{"failed uninstall falls back to disk", 1, []jobs.Job{installOK, uninstallFailed}, true, StateInstalled, nil},
		{"other jobs are ignored", 1, []jobs.Job{createOK, deleteFiles}, false, StateNotInstalled, nil},
		{"installed by hand", 1, nil, true, StateInstalled, nil},
		{"create cancelled while locking", 0, []jobs.Job{createCancelledLock}, false, StateLockFailed, &createCancelledLock},
		{"create cancelled while pending", 0, []jobs.Job{createCancelledPending}, false, StateLockFailed, &createCancelledPending},
		{"create cancelled while installing", 1, []jobs.Job{createCancelledInstall}, true, StateInstallFailed, &createCancelledInstall},
		{"install pending", 1, []jobs.Job{createOK, installPending}, false, StateInstalling, &installPending},
		{"install cancelled wins over a partial env", 1, []jobs.Job{createOK, installCancelled}, true, StateInstallFailed, &installCancelled},
		{"cancelled uninstall falls back to disk", 1, []jobs.Job{installOK, uninstallCancelled}, true, StateInstalled, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, gotJob := deriveState(c.versions, c.jobs, c.installed)
			if got != c.want {
				t.Fatalf("state = %s, want %s", got, c.want)
			}
			if c.wantJob != nil && (gotJob == nil || *gotJob != c.wantJob.ID) {
				t.Fatalf("job = %v, want %s", gotJob, c.wantJob.ID)
			}
			if c.wantJob == nil && c.want != StateCreating && gotJob != nil {
				t.Fatalf("job = %v, want none", gotJob)
			}
		})
	}
}

func TestListProjects(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	// "beta": created normally. "alpha": its lock is held so it stays in
	// the creating state while we list.
	beta, _ := h.mustCreate(specRequest("beta"))

	gate := make(chan struct{})
	started := make(chan struct{}, 1)
	h.pixi.mu.Lock()
	h.pixi.lockGate, h.pixi.lockStarted = gate, started
	h.pixi.mu.Unlock()
	alpha, alphaJob, err := h.svc.CreateProject(ctx, specRequest("alpha"))
	if err != nil {
		t.Fatal(err)
	}
	<-started

	list, err := h.svc.ListProjects(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Name != "alpha" || list[1].Name != "beta" {
		t.Fatalf("want [alpha beta] ordered by name, got %+v", list)
	}
	if list[0].State != StateCreating || list[0].JobID == nil || *list[0].JobID != alphaJob.ID {
		t.Fatalf("alpha: want creating with its job, got %+v", list[0])
	}
	b := list[1]
	if b.ID != beta.ID || b.Path != beta.Path || b.State != StateNotInstalled ||
		b.LatestVersion == nil || *b.LatestVersion != 1 || b.InstalledVersion != nil {
		t.Fatalf("beta: unexpected summary %+v", b)
	}
	for _, s := range list {
		if s.Remotes == nil || len(s.Remotes) != 0 {
			t.Fatalf("%s: Remotes must be an empty, non-nil list until sdk/remote lands, got %#v", s.Name, s.Remotes)
		}
	}

	close(gate)
	h.wait(alphaJob)
	if d := h.detail(alpha.ID); d.State != StateNotInstalled {
		t.Fatalf("alpha after its job: %s", d.State)
	}
}

func TestListDoesNotBlockOnSizeWalk(t *testing.T) {
	h := newHarness(t)
	p, _ := h.mustCreate(specRequest("big"))

	walking := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	h.svc.measureDir = func(ctx context.Context, dir string) (int64, error) {
		once.Do(func() { close(walking) })
		select {
		case <-release:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
		return 4242, nil
	}

	listed := make(chan []ProjectSummary, 1)
	go func() {
		list, err := h.svc.ListProjects(context.Background())
		if err != nil {
			t.Error(err)
		}
		listed <- list
	}()
	var first []ProjectSummary
	select {
	case first = <-listed:
	case <-time.After(10 * time.Second):
		t.Fatal("ListProjects blocked on the size walk")
	}
	if first[0].SizeBytes != nil {
		t.Fatalf("first listing must have no size yet, got %d", *first[0].SizeBytes)
	}
	<-walking // the walk runs in the background

	close(release)
	h.svc.bg.Wait()
	second, err := h.svc.ListProjects(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second[0].ID != p.ID || second[0].SizeBytes == nil || *second[0].SizeBytes != 4242 {
		t.Fatalf("second listing must show the cached size, got %+v", second[0])
	}
}

func TestSizeCacheIsRefreshedWhenStale(t *testing.T) {
	h := newHarness(t, func(o *Options) { o.SizeTTL = time.Minute })
	h.mustCreate(specRequest("sized"))

	var mu sync.Mutex
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	h.svc.now = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return clock
	}
	var walks atomic.Int32
	h.svc.measureDir = func(context.Context, string) (int64, error) {
		return int64(100 * walks.Add(1)), nil
	}
	list := func() *int64 {
		t.Helper()
		got, err := h.svc.ListProjects(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		h.svc.bg.Wait()
		return got[0].SizeBytes
	}

	if s := list(); s != nil {
		t.Fatalf("first listing: size %d, want none", *s)
	}
	if s := list(); s == nil || *s != 100 || walks.Load() != 1 {
		t.Fatalf("second listing: size %v after %d walks, want 100 after 1", s, walks.Load())
	}
	if s := list(); *s != 100 || walks.Load() != 1 {
		t.Fatalf("within the TTL the cache is reused: size %d after %d walks", *s, walks.Load())
	}

	mu.Lock()
	clock = clock.Add(2 * time.Minute)
	mu.Unlock()
	if s := list(); *s != 100 {
		t.Fatalf("a stale size is still returned at once, got %d", *s)
	}
	if walks.Load() != 2 {
		t.Fatalf("a stale size must be recomputed in the background: %d walks", walks.Load())
	}
	if s := list(); *s != 200 {
		t.Fatalf("after the refresh the new size is listed, got %d", *s)
	}
}

func TestListAndGetDoNotWriteToDisk(t *testing.T) {
	h := newHarness(t)
	req := specRequest("readonly")
	req.Install = true
	p, _ := h.mustCreate(req)
	snapshot := func() map[string]string {
		t.Helper()
		out := map[string]string{}
		err := filepath.WalkDir(h.projectsDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			out[path] = fmt.Sprintf("%s %s %d", info.Mode(), info.ModTime().Format(time.RFC3339Nano), info.Size())
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	before := snapshot()
	for i := 0; i < 3; i++ {
		if _, err := h.svc.ListProjects(context.Background()); err != nil {
			t.Fatal(err)
		}
		h.detail(p.ID)
		h.svc.bg.Wait()
	}
	if after := snapshot(); !reflect.DeepEqual(before, after) {
		t.Fatal("List and Get must not write anything to disk")
	}
}

func TestGetProject(t *testing.T) {
	h := newHarness(t)
	p, _ := h.mustCreate(specRequest("detail"))
	d := h.detail(p.ID)
	if d.ID != p.ID || d.Name != "detail" || !d.ManagedDir || d.VersionCount != 1 || d.CreatedAt.IsZero() {
		t.Fatalf("unexpected detail %+v", d)
	}
	if _, err := h.svc.GetProject(context.Background(), uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown project: want ErrNotFound, got %v", err)
	}
}

func TestProjectSize(t *testing.T) {
	h := newHarness(t)
	dir := filepath.Join(t.TempDir(), "sized")
	if err := os.MkdirAll(filepath.Join(dir, ".pixi", "envs", "default"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(rel string, n int) {
		if err := os.WriteFile(filepath.Join(dir, rel), make([]byte, n), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("data.bin", 1000)
	write(filepath.Join(".pixi", "envs", "default", "lib.so"), 5000)
	outside := filepath.Join(t.TempDir(), "outside.bin")
	if err := os.WriteFile(outside, make([]byte, 1<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	symlinked := os.Symlink(outside, filepath.Join(dir, "link.bin")) == nil

	req := specRequest("sized")
	req.Path = dir
	p, _ := h.mustCreate(req)
	manifest := int64(len(readFile(t, filepath.Join(dir, "pixi.toml"))))
	want := 1000 + 5000 + manifest + int64(len(fakeLockContent))

	got, err := h.svc.ProjectSize(context.Background(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("size = %d, want %d (symlinks not followed: %v)", got, want, symlinked)
	}
	if d := h.detail(p.ID); d.SizeBytes == nil || *d.SizeBytes != want {
		t.Fatalf("ProjectSize must fill the cache, got %v", d.SizeBytes)
	}

	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if got, err := h.svc.ProjectSize(context.Background(), p.ID); err != nil || got != 0 {
		t.Fatalf("missing directory: got %d, %v; want 0", got, err)
	}
	if _, err := h.svc.ProjectSize(context.Background(), uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown project: want ErrNotFound, got %v", err)
	}
}
