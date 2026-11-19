package local

import (
	"context"
	"errors"
	"fmt"
	"io"
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
		j := jobs.Job{ID: uuid.New(), Type: typ, Status: status, Stage: stage}
		if status != jobs.StatusPending {
			started := time.Now()
			j.StartedAt = &started
		}
		return j
	}
	// neverStarted is j cancelled while pending: its body never ran.
	neverStarted := func(j jobs.Job) jobs.Job {
		j.StartedAt = nil
		return j
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
	createCancelledPending := neverStarted(job(jobs.TypeCreate, jobs.StatusCancelled, ""))
	createCancelledInstall := job(jobs.TypeCreate, jobs.StatusCancelled, StageInstall)
	installPending := job(jobs.TypeInstall, jobs.StatusPending, "")
	installCancelled := job(jobs.TypeInstall, jobs.StatusCancelled, "")
	uninstallCancelled := job(jobs.TypeUninstall, jobs.StatusCancelled, "")
	installCancelledPending := neverStarted(job(jobs.TypeInstall, jobs.StatusCancelled, ""))

	cases := []struct {
		name     string
		versions int
		jobs     []jobs.Job
		// recorded is whether InstalledVersionID is set; onDisk is whether
		// .pixi/envs exists.
		recorded, onDisk bool
		want             State
		wantJob          *jobs.Job
	}{
		{"create running", 0, []jobs.Job{createRunning}, false, false, StateCreating, &createRunning},
		{"create pending", 0, []jobs.Job{createPending}, false, false, StateCreating, &createPending},
		{"create running at install stage", 1, []jobs.Job{job(jobs.TypeCreate, jobs.StatusRunning, StageInstall)}, false, true, StateCreating, nil},
		{"lock failed", 0, []jobs.Job{createFailedLock}, false, false, StateLockFailed, &createFailedLock},
		{"no versions and no job (lost on restart)", 0, nil, false, false, StateLockFailed, nil},
		{"no versions even if create completed", 0, []jobs.Job{createOK}, false, false, StateLockFailed, &createOK},
		{"created, not installed", 1, []jobs.Job{createOK}, false, false, StateNotInstalled, nil},
		{"created and installed", 1, []jobs.Job{createOKInstalled}, true, true, StateInstalled, nil},
		{"install failed in create", 1, []jobs.Job{createFailedInstall}, false, true, StateInstallFailed, &createFailedInstall},
		{"lock failed then fixed by a new version", 1, []jobs.Job{createFailedLock}, false, false, StateNotInstalled, nil},
		{"installing", 2, []jobs.Job{createOK, installRunning}, false, false, StateInstalling, &installRunning},
		{"uninstalling", 2, []jobs.Job{createOK, installOK, uninstallPending}, true, true, StateUninstalling, &uninstallPending},
		{"install failed wins over disk", 1, []jobs.Job{createOK, installFailed}, true, true, StateInstallFailed, &installFailed},
		{"later install completed", 1, []jobs.Job{installFailed, installOK}, true, true, StateInstalled, nil},
		{"failed uninstall falls back to disk", 1, []jobs.Job{installOK, uninstallFailed}, true, true, StateInstalled, nil},
		{"other jobs are ignored", 1, []jobs.Job{createOK, deleteFiles}, false, false, StateNotInstalled, nil},
		{"installed, no jobs (after restart)", 1, nil, true, true, StateInstalled, nil},
		// An env on disk with nothing recorded: a failed or interrupted
		// install whose job is gone after a restart left a partial
		// .pixi/envs, or pixi install was run outside nebi. Either way nebi
		// does not know which version, if any, is on disk, so it is
		// deliberately not reported as installed.
		{"env without a recorded install (job lost on restart, or installed outside nebi)", 1, nil, false, true, StateNotInstalled, nil},
		{"partial env, create job lost on restart", 1, []jobs.Job{createOK}, false, true, StateNotInstalled, nil},
		{"installed version recorded but env removed", 1, nil, true, false, StateNotInstalled, nil},
		{"create cancelled while locking", 0, []jobs.Job{createCancelledLock}, false, false, StateLockFailed, &createCancelledLock},
		{"create cancelled while pending", 0, []jobs.Job{createCancelledPending}, false, false, StateLockFailed, &createCancelledPending},
		{"create cancelled while installing", 1, []jobs.Job{createCancelledInstall}, false, true, StateInstallFailed, &createCancelledInstall},
		{"install pending", 1, []jobs.Job{createOK, installPending}, false, false, StateInstalling, &installPending},
		{"install cancelled wins over a partial env", 1, []jobs.Job{createOK, installCancelled}, false, true, StateInstallFailed, &installCancelled},
		{"cancelled uninstall falls back to disk", 1, []jobs.Job{installOK, uninstallCancelled}, true, true, StateInstalled, nil},
		{"install cancelled while pending is ignored", 1, []jobs.Job{createOKInstalled, installCancelledPending}, true, true, StateInstalled, nil},
		{"install cancelled while pending does not hide a failed one", 1, []jobs.Job{createOK, installFailed, installCancelledPending}, false, true, StateInstallFailed, &installFailed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, gotJob := deriveState(c.versions, c.jobs, c.recorded, c.onDisk)
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

// restarted returns a copy of h whose service and job runner are new, as
// after the client restarts (see restart). The old ones are left as they
// are, like a process that was killed, and closed at cleanup.
func (h *harness) restarted() *harness {
	h.t.Helper()
	r := *h
	r.svc, r.runner = h.restart()
	return &r
}

// checkAfterRestart checks GetProject and ListProjects for the project.
// want is the expected state; wantInstalled is the expected installed
// version number (nil: none). The state must not link to a job, since the
// new runner has none.
func checkAfterRestart(t *testing.T, h *harness, id uuid.UUID, want State, wantInstalled *int) {
	t.Helper()
	if js, err := h.runner.ProjectJobs(context.Background(), id); err != nil || len(js) != 0 {
		t.Fatalf("precondition: the restarted runner must know no jobs of the project, got %v, %v", js, err)
	}
	d := h.detail(id)
	list, err := h.svc.ListProjects(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var s *ProjectSummary
	for i := range list {
		if list[i].ID == id {
			s = &list[i]
		}
	}
	if s == nil {
		t.Fatalf("ListProjects does not list project %s", id)
	}
	for _, got := range []struct {
		from string
		ProjectSummary
	}{{"GetProject", d.ProjectSummary}, {"ListProjects", *s}} {
		if got.State != want {
			t.Errorf("%s: state = %s, want %s", got.from, got.State, want)
		}
		if got.JobID != nil {
			t.Errorf("%s: JobID = %s, want none", got.from, got.JobID)
		}
		if !reflect.DeepEqual(got.InstalledVersion, wantInstalled) {
			t.Errorf("%s: InstalledVersion = %v, want %v", got.from, deref(got.InstalledVersion), deref(wantInstalled))
		}
		if got.LatestVersion == nil || *got.LatestVersion != 1 {
			t.Errorf("%s: LatestVersion = %v, want 1", got.from, deref(got.LatestVersion))
		}
	}
	if d.VersionCount != 1 {
		t.Errorf("VersionCount = %d, want 1", d.VersionCount)
	}
}

func deref(n *int) any {
	if n == nil {
		return nil
	}
	return *n
}

func intPtr(n int) *int { return &n }

// A failed install leaves a partial .pixi/envs. After a restart its job is
// gone, and the project must not be reported as installed (#670).
func TestStateAfterRestartFollowingFailedInstall(t *testing.T) {
	h := newHarness(t)
	h.pixi.installErr = errors.New("download failed")
	h.pixi.partialEnv = true
	req := specRequest("failed-install")
	req.Install = true
	p, job := h.mustCreate(req)
	if job.Status != jobs.StatusFailed || job.Stage != StageInstall {
		t.Fatalf("precondition: want the create job failed at the install stage, got %+v", job)
	}
	if !envInstalled(p.Path) {
		t.Fatal("precondition: the failed install must leave .pixi/envs behind")
	}
	if d := h.detail(p.ID); d.State != StateInstallFailed || d.InstalledVersionID != nil {
		t.Fatalf("precondition: before the restart want install_failed and no installed version, got %+v", d)
	}

	r := h.restarted()
	checkAfterRestart(t, r, p.ID, StateNotInstalled, nil)
	if d := r.detail(p.ID); d.InstalledVersionID != nil {
		t.Fatalf("InstalledVersionID = %s, want nil", d.InstalledVersionID)
	}
}

// Quitting nebi during a long install leaves a partial .pixi/envs and no
// job. The old process is not shut down cleanly here: the new service
// starts while the old install is still blocked, as after a kill.
func TestStateAfterRestartDuringInstall(t *testing.T) {
	h := newHarness(t)
	started := make(chan struct{}, 1)
	h.pixi.installGate = make(chan struct{}) // never opened: the install never finishes
	h.pixi.installStarted = started
	req := specRequest("interrupted")
	req.Install = true
	p, job, err := h.svc.CreateProject(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(30 * time.Second):
		t.Fatal("the install never started")
	}
	if !envInstalled(p.Path) {
		t.Fatal("precondition: the interrupted install must have created .pixi/envs")
	}
	if d := h.detail(p.ID); d.State != StateCreating || d.JobID == nil || *d.JobID != job.ID {
		t.Fatalf("precondition: before the restart want creating with the job, got %+v", d.ProjectSummary)
	}

	r := h.restarted()
	checkAfterRestart(t, r, p.ID, StateNotInstalled, nil)
}

// A successful install is still reported after a restart, with its version.
func TestStateAfterRestartKeepsSuccessfulInstall(t *testing.T) {
	h := newHarness(t)
	req := specRequest("installed")
	req.Install = true
	p, job := h.mustCreate(req)
	if job.Status != jobs.StatusCompleted {
		t.Fatalf("precondition: create job: %+v", job)
	}

	r := h.restarted()
	checkAfterRestart(t, r, p.ID, StateInstalled, intPtr(1))
}

// An environment created by running pixi outside nebi has .pixi/envs but no
// InstalledVersionID. nebi does not know which version is on disk, so the
// project is deliberately reported as not_installed (#670).
func TestStateOfEnvInstalledOutsideNebi(t *testing.T) {
	h := newHarness(t)
	p, _ := h.mustCreate(specRequest("by-hand"))
	if err := os.MkdirAll(filepath.Join(p.Path, ".pixi", "envs", "default"), 0o755); err != nil {
		t.Fatal(err)
	}
	d := h.detail(p.ID)
	if d.State != StateNotInstalled || d.InstalledVersion != nil || d.InstalledVersionID != nil || d.JobID != nil {
		t.Fatalf("want not_installed with no installed version, got %+v", d)
	}
}

// Removing an installed environment by hand makes the project
// not_installed even though InstalledVersionID is still set. The summary
// keeps reporting that version as the one nebi last installed.
func TestStateOfInstalledEnvRemovedFromDisk(t *testing.T) {
	h := newHarness(t)
	req := specRequest("removed")
	req.Install = true
	p, _ := h.mustCreate(req)
	if d := h.detail(p.ID); d.State != StateInstalled {
		t.Fatalf("precondition: want installed, got %s", d.State)
	}
	if err := os.RemoveAll(filepath.Join(p.Path, ".pixi")); err != nil {
		t.Fatal(err)
	}
	d := h.detail(p.ID)
	if d.State != StateNotInstalled {
		t.Fatalf("want not_installed once .pixi is removed, got %s", d.State)
	}
	if d.InstalledVersion == nil || *d.InstalledVersion != 1 || d.InstalledVersionID == nil {
		t.Fatalf("InstalledVersion = %v, want 1, the version nebi last recorded as installed", deref(d.InstalledVersion))
	}
}

// hookOnRead is a JobRunner that runs an armed hook once, right after its
// after-th ProjectJobs call has read the list (after=0: before the first
// call reads it). A GetProject or a ListProjects of one project reads the
// project's jobs before and after its row, so after=1 runs the hook
// between the first jobs read and the row read, and after=2 once the read
// has read everything it uses.
type hookOnRead struct {
	*jobs.Memory
	mu    sync.Mutex
	calls int
	after int
	hook  func()
}

func (f *hookOnRead) arm(after int, hook func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls, f.after, f.hook = 0, after, hook
}

// due returns the hook when it is due now, and disarms it.
func (f *hookOnRead) due() func() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.hook == nil || f.calls != f.after {
		return nil
	}
	hook := f.hook
	f.hook = nil
	return hook
}

func (f *hookOnRead) ProjectJobs(ctx context.Context, id uuid.UUID) ([]jobs.Job, error) {
	if hook := f.due(); hook != nil {
		hook()
	}
	list, err := f.Memory.ProjectJobs(ctx, id)
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if hook := f.due(); hook != nil {
		hook()
	}
	return list, err
}

// readers are the two ways to read one project's summary.
var readers = map[string]func(h *harness, id uuid.UUID) ProjectSummary{
	"GetProject": func(h *harness, id uuid.UUID) ProjectSummary { return h.detail(id).ProjectSummary },
	"ListProjects": func(h *harness, id uuid.UUID) ProjectSummary {
		h.t.Helper()
		list, err := h.svc.ListProjects(context.Background())
		if err != nil || len(list) != 1 || list[0].ID != id {
			h.t.Fatalf("ListProjects = %v, %v", list, err)
		}
		return list[0]
	},
}

// readMoments are the points in a read where the hooks below run.
var readMoments = map[int]string{
	0: "before the first jobs read",
	1: "between the jobs read and the row read",
	2: "after the second jobs read",
}

// TestStateWhenInstallFinishesDuringRead covers a create-with-install job
// that finishes while a project is being read. The read must never pair
// the finished job with a row read before the job recorded the install,
// which would report not_installed for a project that just installed: it
// reports creating when the whole read saw the job running, and installed
// otherwise.
func TestStateWhenInstallFinishesDuringRead(t *testing.T) {
	for name, readState := range readers {
		for after, moment := range readMoments {
			t.Run(name+"/"+moment, func(t *testing.T) {
				runner := &hookOnRead{}
				h := newHarness(t, func(o *Options) {
					runner.Memory = o.Jobs.(*jobs.Memory)
					o.Jobs = runner
				})
				gate := make(chan struct{})
				started := make(chan struct{}, 1)
				h.pixi.installGate, h.pixi.installStarted = gate, started
				req := specRequest("straddle")
				req.Install = true
				p, job, err := h.svc.CreateProject(context.Background(), req)
				if err != nil {
					t.Fatal(err)
				}
				<-started
				runner.arm(after, func() {
					close(gate)
					h.wait(job)
				})

				got := readState(h, p.ID)
				want, wantJob := StateInstalled, (*uuid.UUID)(nil)
				if after == 2 {
					want, wantJob = StateCreating, &job.ID
				}
				if got.State != want {
					t.Fatalf("state = %s, want %s", got.State, want)
				}
				if !reflect.DeepEqual(got.JobID, wantJob) {
					t.Fatalf("JobID = %v, want %v", got.JobID, wantJob)
				}
				if d := h.detail(p.ID); d.State != StateInstalled {
					t.Fatalf("the next read must be installed, got %s", d.State)
				}
			})
		}
	}
}

// TestStateWhenInstallStartsDuringRead is the mirror image: a reinstall of
// an installed project starts while the project is being read, and clears
// InstalledVersionID before touching .pixi as its job must. The read must
// never pair the cleared row with jobs read before the job was submitted,
// which would report not_installed, with no job, for a project being
// installed: it reports installed when the whole read came before the job,
// and installing with the job otherwise.
func TestStateWhenInstallStartsDuringRead(t *testing.T) {
	for name, readState := range readers {
		for after, moment := range readMoments {
			t.Run(name+"/"+moment, func(t *testing.T) {
				runner := &hookOnRead{}
				h := newHarness(t, func(o *Options) {
					runner.Memory = o.Jobs.(*jobs.Memory)
					o.Jobs = runner
				})
				req := specRequest("restart")
				req.Install = true
				p, _ := h.mustCreate(req)
				installed := h.detail(p.ID)
				if installed.State != StateInstalled || installed.InstalledVersionID == nil {
					t.Fatalf("setup: want installed, got %+v", installed)
				}
				versionID := *installed.InstalledVersionID

				gate := make(chan struct{})
				var reinstall *jobs.Job
				runner.arm(after, func() {
					cleared := make(chan struct{})
					var err error
					reinstall, err = runner.Memory.Submit(context.Background(), p.ID, jobs.TypeInstall, func(ctx context.Context, _ io.Writer) error {
						err := h.db.Model(&Project{}).Where("id = ?", p.ID).Update("installed_version_id", nil).Error
						close(cleared)
						if err != nil {
							return err
						}
						<-gate
						return h.db.Model(&Project{}).Where("id = ?", p.ID).Update("installed_version_id", versionID).Error
					})
					if err != nil {
						t.Error(err)
						close(cleared)
					}
					<-cleared
				})
				t.Cleanup(func() {
					close(gate)
					if reinstall != nil {
						h.wait(reinstall)
					}
				})

				got := readState(h, p.ID)
				if reinstall == nil {
					t.Fatal("the reinstall was not submitted")
				}
				want, wantJob := StateInstalling, &reinstall.ID
				if after == 2 {
					want, wantJob = StateInstalled, nil
				}
				if got.State != want {
					t.Fatalf("state = %s, want %s", got.State, want)
				}
				if !reflect.DeepEqual(got.JobID, wantJob) {
					t.Fatalf("JobID = %v, want %v", got.JobID, wantJob)
				}
				if d := h.detail(p.ID); d.State != StateInstalling || d.JobID == nil || *d.JobID != reinstall.ID {
					t.Fatalf("the next read must be installing with the reinstall, got %s %v", d.State, d.JobID)
				}
			})
		}
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

func TestInstallCancelledWhilePendingKeepsState(t *testing.T) {
	h := newHarnessWithRunner(t, jobs.Options{MaxRunning: 1})
	ctx := context.Background()
	req := specRequest("installed")
	req.Install = true
	p, _ := h.mustCreate(req)

	// The install waits for the only slot and is cancelled before it runs.
	gate := make(chan struct{})
	blocker, err := h.runner.Submit(ctx, uuid.New(), jobs.TypeInstall, func(context.Context, io.Writer) error { <-gate; return nil })
	if err != nil {
		t.Fatal(err)
	}
	install, err := h.runner.Submit(ctx, p.ID, jobs.TypeInstall, func(context.Context, io.Writer) error { return nil })
	if err != nil || install.Status != jobs.StatusPending {
		t.Fatalf("precondition: a pending install, got %+v, %v", install, err)
	}
	if err := h.runner.Cancel(ctx, install.ID); err != nil {
		t.Fatal(err)
	}
	close(gate)
	h.wait(blocker)

	if d := h.detail(p.ID); d.State != StateInstalled || d.JobID != nil {
		t.Fatalf("an install that never ran must not change the state, got %+v", d.ProjectSummary)
	}
}
