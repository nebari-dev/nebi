package local

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/artifact"
	"github.com/nebari-dev/nebi/internal/client/jobs"
)

// setPixi changes the fake under its lock, so a job reading it does not race.
func (h *harness) setPixi(change func(f *fakePixi)) {
	h.pixi.mu.Lock()
	defer h.pixi.mu.Unlock()
	change(h.pixi)
}

// install calls InstallVersion and waits for its job.
func (h *harness) install(req InstallRequest) jobs.Job {
	h.t.Helper()
	job, err := h.svc.InstallVersion(context.Background(), req)
	if err != nil {
		h.t.Fatalf("InstallVersion: %v", err)
	}
	if job.Type != jobs.TypeInstall {
		h.t.Fatalf("job type = %s, want install", job.Type)
	}
	return h.wait(job)
}

// uninstall calls Uninstall and waits for its job.
func (h *harness) uninstall(id uuid.UUID) jobs.Job {
	h.t.Helper()
	job, err := h.svc.Uninstall(context.Background(), id)
	if err != nil {
		h.t.Fatalf("Uninstall: %v", err)
	}
	if job.Type != jobs.TypeUninstall {
		h.t.Fatalf("job type = %s, want uninstall", job.Type)
	}
	return h.wait(job)
}

// wantInstalled is wantInstallState for a harness on the fake PixiRunner,
// and also checks that pixi's latest install ran on v's files, so they were
// written before pixi ran.
func (h *harness) wantInstalled(p *Project, v ProjectVersion) {
	h.t.Helper()
	if saw, ok := h.pixi.lastInstallSaw(); !ok || saw.Manifest != v.Manifest || saw.Lock != v.Lock {
		h.t.Fatalf("pixi install must run on version %d's pixi.toml and pixi.lock", v.Number)
	}
	h.wantInstallState(p, v)
}

// wantInstallState checks that version v is installed: its files are in
// the project directory, the environment is on disk, and the project
// reports it as installed and as the last installed version.
func (h *harness) wantInstallState(p *Project, v ProjectVersion) {
	h.t.Helper()
	if got := readFile(h.t, filepath.Join(p.Path, artifact.ManifestFile)); got != v.Manifest {
		h.t.Fatalf("pixi.toml is not version %d's", v.Number)
	}
	if got := readFile(h.t, filepath.Join(p.Path, artifact.LockFile)); got != v.Lock {
		h.t.Fatalf("pixi.lock is not version %d's", v.Number)
	}
	d := h.detail(p.ID)
	if d.State != StateInstalled || d.JobID != nil {
		h.t.Fatalf("state = %s (job %v), want installed", d.State, d.JobID)
	}
	if d.InstalledVersionID == nil || *d.InstalledVersionID != v.ID || d.InstalledVersion == nil || *d.InstalledVersion != v.Number {
		h.t.Fatalf("installed = %v (%v), want version %d", d.InstalledVersionID, deref(d.InstalledVersion), v.Number)
	}
	if d.LastInstalledVersionID == nil || *d.LastInstalledVersionID != v.ID || d.LastInstalledVersion == nil || *d.LastInstalledVersion != v.Number {
		h.t.Fatalf("last installed = %v (%v), want version %d", d.LastInstalledVersionID, deref(d.LastInstalledVersion), v.Number)
	}
	if id := h.installingVersionID(p.ID); id != nil {
		h.t.Fatalf("installing version = %s after a successful install, want none", id)
	}
}

// installingVersionID reads the project's InstallingVersionID from its row.
func (h *harness) installingVersionID(id uuid.UUID) *uuid.UUID {
	h.t.Helper()
	var p Project
	if err := h.db.First(&p, "id = ?", id).Error; err != nil {
		h.t.Fatal(err)
	}
	return p.InstallingVersionID
}

// activeJobs returns the IDs of the project's pending and running jobs.
func (h *harness) activeJobs(id uuid.UUID) []uuid.UUID {
	h.t.Helper()
	list, err := h.runner.ProjectJobs(context.Background(), id)
	if err != nil {
		h.t.Fatal(err)
	}
	var out []uuid.UUID
	for _, j := range list {
		if j.Active() {
			out = append(out, j.ID)
		}
	}
	return out
}

// jobCount returns how many jobs the runner keeps for the project.
func (h *harness) jobCount(id uuid.UUID) int {
	h.t.Helper()
	list, err := h.runner.ProjectJobs(context.Background(), id)
	if err != nil {
		h.t.Fatal(err)
	}
	return len(list)
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// waitStarted waits for a gated fake call to start.
func waitStarted(t *testing.T, started <-chan struct{}) {
	t.Helper()
	select {
	case <-started:
	case <-time.After(30 * time.Second):
		t.Fatal("the gated pixi call never started")
	}
}

// TestInstallSpecificLastAndLatest installs a given version, the latest
// one when nothing was installed before, and the last installed one, and
// checks that each install replaces the installed version.
func TestInstallSpecificLastAndLatest(t *testing.T) {
	h := newHarness(t)
	p, v1 := h.newProject("pick", false)
	h.createVersion(CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, Spec: newerSpec()})
	v2 := h.versions.versionsOf(t, p.ID)[1]

	// Nothing installed before: no version means the latest.
	if done := h.install(InstallRequest{ProjectID: p.ID}); done.Status != jobs.StatusCompleted || done.Stage != StageInstall {
		t.Fatalf("install latest: %+v", done)
	}
	h.wantInstalled(p, v2)

	// A specific version replaces the installed one.
	if done := h.install(InstallRequest{ProjectID: p.ID, VersionID: &v1.ID}); done.Status != jobs.StatusCompleted {
		t.Fatalf("install v1: %+v", done)
	}
	h.wantInstalled(p, v1)

	// Version 3 becomes the latest, but no version still means the last
	// installed one.
	h.createVersion(CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v2.ID, AllowOlderBase: true})
	if n := len(h.versions.versionsOf(t, p.ID)); n != 3 {
		t.Fatalf("want 3 versions, got %d", n)
	}
	if done := h.install(InstallRequest{ProjectID: p.ID}); done.Status != jobs.StatusCompleted {
		t.Fatalf("install last installed: %+v", done)
	}
	h.wantInstalled(p, v1)

	// After an uninstall the last installed version is still the default.
	h.uninstall(p.ID)
	if done := h.install(InstallRequest{ProjectID: p.ID}); done.Status != jobs.StatusCompleted {
		t.Fatalf("install after uninstall: %+v", done)
	}
	h.wantInstalled(p, v1)

	// The read model reports both numbers in the list too.
	list, err := h.svc.ListProjects(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || deref(list[0].InstalledVersion) != 1 || deref(list[0].LastInstalledVersion) != 1 || deref(list[0].LatestVersion) != 3 {
		t.Fatalf("ListProjects = %+v; want installed 1, last installed 1, latest 3", list)
	}

	if lock, install := h.pixi.calls(); len(lock) != 2 || len(install) != 4 {
		t.Fatalf("installs must not solve: lock calls %d (want 2, for versions 1 and 2), install calls %d (want 4)", len(lock), len(install))
	}
}

// TestInstallNeverChangesLock checks that an install writes the version's
// pixi.lock byte for byte, never solves, and never changes the version row.
func TestInstallNeverChangesLock(t *testing.T) {
	h := newHarness(t)
	p, v1 := h.newProject("frozen", false)
	h.install(InstallRequest{ProjectID: p.ID, VersionID: &v1.ID})
	if got := readFile(t, filepath.Join(p.Path, artifact.LockFile)); got != v1.Lock {
		t.Fatal("the installed pixi.lock must be the version's")
	}
	after := h.versions.versionsOf(t, p.ID)
	if len(after) != 1 || after[0].Lock != v1.Lock || after[0].ContentHash != v1.ContentHash {
		t.Fatal("installing must not change the version")
	}
	if lock, _ := h.pixi.calls(); len(lock) != 1 {
		t.Fatalf("install must not run pixi lock, lock calls: %v", lock)
	}
}

// TestInstallTheInstalledVersionRunsPixiAgain checks that a reinstall of
// the installed version repairs a damaged environment.
func TestInstallTheInstalledVersionRunsPixiAgain(t *testing.T) {
	h := newHarness(t)
	p, v1 := h.newProject("repair", true)
	marker := filepath.Join(p.Path, ".pixi", "envs", "default", "marker")
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	if done := h.install(InstallRequest{ProjectID: p.ID, VersionID: &v1.ID}); done.Status != jobs.StatusCompleted {
		t.Fatalf("reinstall: %+v", done)
	}
	if _, install := h.pixi.calls(); len(install) != 2 {
		t.Fatalf("pixi install must run again, ran %d times", len(install))
	}
	if !exists(marker) {
		t.Fatal("the reinstall must repair the environment")
	}
	h.wantInstalled(p, v1)
}

func TestInstallLogsPixiOutput(t *testing.T) {
	h := newHarness(t)
	p, _ := h.newProject("logged-install", false)
	done := h.install(InstallRequest{ProjectID: p.ID})
	out, err := h.runner.Logs(done.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := fakeInstallOutput(p.Path); string(out) != want {
		t.Fatalf("job log = %q, want pixi's output %q", out, want)
	}
}

// TestInstallProtectsUnsavedEdits checks that install refuses a working
// copy that matches no recorded version, and overwrites it only with Force.
func TestInstallProtectsUnsavedEdits(t *testing.T) {
	h := newHarness(t)
	p, v1 := h.newProject("edited", true)
	manifestPath := filepath.Join(p.Path, artifact.ManifestFile)
	lockPath := filepath.Join(p.Path, artifact.LockFile)
	edited := v1.Manifest + "\n# a hand edit\n"
	writeTestFile(t, manifestPath, edited)
	jobsBefore := h.jobCount(p.ID)

	_, err := h.svc.InstallVersion(context.Background(), InstallRequest{ProjectID: p.ID, VersionID: &v1.ID})
	if !errors.Is(err, ErrWorkingCopyModified) {
		t.Fatalf("want ErrWorkingCopyModified, got %v", err)
	}
	if got := readFile(t, manifestPath); got != edited {
		t.Fatal("a refused install must leave the edit alone")
	}
	if h.jobCount(p.ID) != jobsBefore {
		t.Fatal("a refused install must submit no job")
	}

	// A pixi.lock without a pixi.toml may hold a lock that is in no version.
	if err := os.Remove(manifestPath); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, lockPath, "version: 6\n# hand-made\n")
	if _, err := h.svc.InstallVersion(context.Background(), InstallRequest{ProjectID: p.ID}); !errors.Is(err, ErrWorkingCopyModified) {
		t.Fatalf("a lone pixi.lock: want ErrWorkingCopyModified, got %v", err)
	}

	// Force overwrites the changes.
	writeTestFile(t, manifestPath, edited)
	if done := h.install(InstallRequest{ProjectID: p.ID, VersionID: &v1.ID, Force: true}); done.Status != jobs.StatusCompleted {
		t.Fatalf("forced install: %+v", done)
	}
	h.wantInstalled(p, v1)
}

// TestInstallAcceptsRecordedOrMissingFiles checks the working copies an
// install may overwrite without Force: one holding any recorded version of
// the project, not only the installed one, and none at all.
func TestInstallAcceptsRecordedOrMissingFiles(t *testing.T) {
	h := newHarness(t)
	p, v1 := h.newProject("replaceable", false)
	h.createVersion(CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, Spec: newerSpec()})
	v2 := h.versions.versionsOf(t, p.ID)[1]

	// The directory holds version 1, which is not installed.
	h.install(InstallRequest{ProjectID: p.ID, VersionID: &v2.ID})
	h.wantInstalled(p, v2)

	// Neither file is there.
	for _, f := range []string{artifact.ManifestFile, artifact.LockFile} {
		if err := os.Remove(filepath.Join(p.Path, f)); err != nil {
			t.Fatal(err)
		}
	}
	h.install(InstallRequest{ProjectID: p.ID, VersionID: &v1.ID})
	h.wantInstalled(p, v1)
}

// TestInstallRechecksTheWorkingCopyInTheJob edits the working copy while
// the install job is pending: the job must refuse to overwrite the edit,
// unless the install was forced. A refused job touches nothing, so the
// version installed before stays installed and is reported so.
func TestInstallRechecksTheWorkingCopyInTheJob(t *testing.T) {
	for _, force := range []bool{false, true} {
		name := "unforced"
		if force {
			name = "forced"
		}
		t.Run(name, func(t *testing.T) {
			h := newHarnessWithRunner(t, jobs.Options{MaxRunning: 1})
			blocker, _ := h.newProject("blocker", false)
			p, v1 := h.newProject("waiting", true)

			gate := make(chan struct{})
			started := make(chan struct{}, 2)
			h.setPixi(func(f *fakePixi) { f.installGate, f.installStarted = gate, started })
			blocking, err := h.svc.InstallVersion(context.Background(), InstallRequest{ProjectID: blocker.ID})
			if err != nil {
				t.Fatal(err)
			}
			waitStarted(t, started)

			pending, err := h.svc.InstallVersion(context.Background(), InstallRequest{ProjectID: p.ID, VersionID: &v1.ID, Force: force})
			if err != nil {
				t.Fatal(err)
			}
			if pending.Status != jobs.StatusPending {
				t.Fatalf("with one slot the install must wait, got %+v", pending)
			}
			manifestPath := filepath.Join(p.Path, artifact.ManifestFile)
			edited := v1.Manifest + "\n# edited while pending\n"
			writeTestFile(t, manifestPath, edited)
			close(gate)
			if done := h.wait(blocking); done.Status != jobs.StatusCompleted {
				t.Fatalf("blocking install: %+v", done)
			}

			done := h.wait(pending)
			if !force {
				if done.Status != jobs.StatusFailed || !strings.Contains(done.Error, ErrWorkingCopyModified.Error()) {
					t.Fatalf("want the job to refuse the edit, got %+v", done)
				}
				if got := readFile(t, manifestPath); got != edited {
					t.Fatal("the edit must be kept")
				}
				if d := h.detail(p.ID); d.State != StateInstalled || d.InstalledVersionID == nil || *d.InstalledVersionID != v1.ID {
					t.Fatalf("version 1 must stay installed and be reported so: %+v", d)
				}
				return
			}
			if done.Status != jobs.StatusCompleted {
				t.Fatalf("forced install: %+v", done)
			}
			h.wantInstalled(p, v1)
		})
	}
}

func TestInstallErrors(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	p, _ := h.newProject("errors", false)
	other, otherV1 := h.newProject("other", false)
	missing := uuid.New()

	for _, c := range []struct {
		name string
		req  InstallRequest
		want error
	}{
		{"unknown project", InstallRequest{ProjectID: missing}, ErrNotFound},
		{"unknown version", InstallRequest{ProjectID: p.ID, VersionID: &missing}, ErrNotFound},
		{"another project's version", InstallRequest{ProjectID: p.ID, VersionID: &otherV1.ID}, ErrNotFound},
	} {
		if _, err := h.svc.InstallVersion(ctx, c.req); !errors.Is(err, c.want) {
			t.Errorf("%s: want %v, got %v", c.name, c.want, err)
		}
	}
	if _, err := h.svc.Uninstall(ctx, missing); !errors.Is(err, ErrNotFound) {
		t.Errorf("Uninstall of an unknown project: want ErrNotFound, got %v", err)
	}

	// A project whose create failed has no versions.
	h.setPixi(func(f *fakePixi) { f.lockErr = errors.New("cannot solve") })
	empty, _ := h.mustCreate(specRequest("empty"))
	h.setPixi(func(f *fakePixi) { f.lockErr = nil })
	if _, err := h.svc.InstallVersion(ctx, InstallRequest{ProjectID: empty.ID}); !errors.Is(err, ErrNoVersions) {
		t.Errorf("no versions: want ErrNoVersions, got %v", err)
	}

	// The project directory is gone, with and without Force.
	if err := os.RemoveAll(other.Path); err != nil {
		t.Fatal(err)
	}
	for _, force := range []bool{false, true} {
		if _, err := h.svc.InstallVersion(ctx, InstallRequest{ProjectID: other.ID, Force: force}); !errors.Is(err, ErrWorkingCopyMissing) {
			t.Errorf("missing directory (force %v): want ErrWorkingCopyMissing, got %v", force, err)
		}
	}
	if exists(other.Path) {
		t.Error("a refused install must not create the project directory")
	}
	if _, install := h.pixi.calls(); len(install) != 0 {
		t.Errorf("no refused install may run pixi, ran %v", install)
	}
}

// TestInstallAndUninstallWhileBusy checks that install and uninstall are
// refused, with the job IDs, while a job for the project is active, and
// that cancelling a running install ends it as cancelled and leaves the
// project install_failed with its last installed version unchanged.
func TestInstallAndUninstallWhileBusy(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	p, v1 := h.newProject("busy", true)
	h.createVersion(CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, Spec: newerSpec()})
	v2 := h.versions.versionsOf(t, p.ID)[1]

	gate := make(chan struct{})
	defer close(gate)
	started := make(chan struct{}, 1)
	h.setPixi(func(f *fakePixi) { f.installGate, f.installStarted = gate, started })
	running, err := h.svc.InstallVersion(ctx, InstallRequest{ProjectID: p.ID, VersionID: &v2.ID})
	if err != nil {
		t.Fatal(err)
	}
	waitStarted(t, started)
	if d := h.detail(p.ID); d.State != StateInstalling || d.JobID == nil || *d.JobID != running.ID {
		t.Fatalf("want installing with the job, got %+v", d.ProjectSummary)
	}

	wantBusy := func(op string, err error) {
		t.Helper()
		var busy *BusyError
		if !errors.Is(err, ErrProjectBusy) || !errors.As(err, &busy) {
			t.Fatalf("%s: want a *BusyError, got %v", op, err)
		}
		if len(busy.JobIDs) != 1 || busy.JobIDs[0] != running.ID || busy.ProjectID != p.ID {
			t.Fatalf("%s: BusyError = %+v, want the running install %s", op, busy, running.ID)
		}
	}
	_, err = h.svc.InstallVersion(ctx, InstallRequest{ProjectID: p.ID, VersionID: &v2.ID})
	wantBusy("second install", err)
	_, err = h.svc.Uninstall(ctx, p.ID)
	wantBusy("uninstall", err)
	if got := h.activeJobs(p.ID); len(got) != 1 {
		t.Fatalf("a refused call must not queue a job, active: %v", got)
	}

	if err := h.runner.Cancel(ctx, running.ID); err != nil {
		t.Fatal(err)
	}
	done := h.wait(running)
	if done.Status != jobs.StatusCancelled {
		t.Fatalf("a cancelled install ends as cancelled, got %+v", done)
	}
	d := h.detail(p.ID)
	if d.State != StateInstallFailed || d.JobID == nil || *d.JobID != running.ID {
		t.Fatalf("want install_failed linked to the cancelled job, got %+v", d.ProjectSummary)
	}
	if d.InstalledVersionID != nil || d.InstalledVersion != nil {
		t.Fatalf("a cancelled install leaves nothing installed, got %v", d.InstalledVersionID)
	}
	if d.LastInstalledVersionID == nil || *d.LastInstalledVersionID != v1.ID || deref(d.LastInstalledVersion) != 1 {
		t.Fatalf("the last installed version must stay version 1, got %v", deref(d.LastInstalledVersion))
	}
	if got := readFile(t, filepath.Join(p.Path, artifact.LockFile)); got != v2.Lock {
		t.Fatal("the target version's files are left in place")
	}
}

// TestInstallAndUninstallWhilePending checks that a pending job, which
// has not started yet, keeps the project busy too, and that cancelling it
// leaves the project as it was.
func TestInstallAndUninstallWhilePending(t *testing.T) {
	h := newHarnessWithRunner(t, jobs.Options{MaxRunning: 1})
	ctx := context.Background()
	blocker, _ := h.newProject("blocker", false)
	p, v1 := h.newProject("queued", true)

	gate := make(chan struct{})
	started := make(chan struct{}, 2)
	h.setPixi(func(f *fakePixi) { f.installGate, f.installStarted = gate, started })
	blocking, err := h.svc.InstallVersion(ctx, InstallRequest{ProjectID: blocker.ID})
	if err != nil {
		t.Fatal(err)
	}
	waitStarted(t, started)
	pending, err := h.svc.InstallVersion(ctx, InstallRequest{ProjectID: p.ID, VersionID: &v1.ID})
	if err != nil {
		t.Fatal(err)
	}
	if pending.Status != jobs.StatusPending {
		t.Fatalf("with one slot the install must wait, got %+v", pending)
	}

	for op, call := range map[string]func() error{
		"install":   func() error { _, err := h.svc.InstallVersion(ctx, InstallRequest{ProjectID: p.ID}); return err },
		"uninstall": func() error { _, err := h.svc.Uninstall(ctx, p.ID); return err },
	} {
		var busy *BusyError
		if err := call(); !errors.As(err, &busy) || !errors.Is(err, ErrProjectBusy) || len(busy.JobIDs) != 1 || busy.JobIDs[0] != pending.ID {
			t.Fatalf("%s while a job is pending: want a *BusyError with %s, got %v", op, pending.ID, err)
		}
	}

	if err := h.runner.Cancel(ctx, pending.ID); err != nil {
		t.Fatal(err)
	}
	if done := h.wait(pending); done.Status != jobs.StatusCancelled || done.Started() {
		t.Fatalf("want cancelled without starting, got %+v", done)
	}
	close(gate)
	h.wait(blocking)
	h.wantInstallState(p, v1)
	installs := 0
	for _, dir := range func() []string { _, in := h.pixi.calls(); return in }() {
		if dir == p.Path {
			installs++
		}
	}
	if installs != 1 {
		t.Fatalf("the cancelled pending install must not run pixi: %d installs in %s, want only the create's", installs, p.Path)
	}
}

// TestInstallFailureKeepsLastInstalled checks the state after a failed
// install, and that the previous version can be installed again.
func TestInstallFailureKeepsLastInstalled(t *testing.T) {
	h := newHarness(t)
	p, v1 := h.newProject("failing", true)
	h.createVersion(CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, Spec: newerSpec()})
	v2 := h.versions.versionsOf(t, p.ID)[1]

	h.setPixi(func(f *fakePixi) { f.installErr, f.partialEnv = errors.New("download failed"), true })
	done := h.install(InstallRequest{ProjectID: p.ID, VersionID: &v2.ID})
	if done.Status != jobs.StatusFailed || done.Stage != StageInstall || !strings.Contains(done.Error, "download failed") {
		t.Fatalf("want a failed install, got %+v", done)
	}
	d := h.detail(p.ID)
	if d.State != StateInstallFailed || d.JobID == nil || *d.JobID != done.ID {
		t.Fatalf("want install_failed linked to the job, got %+v", d.ProjectSummary)
	}
	if d.InstalledVersionID != nil {
		t.Fatal("a failed install leaves nothing installed")
	}
	if d.LastInstalledVersionID == nil || *d.LastInstalledVersionID != v1.ID || deref(d.LastInstalledVersion) != 1 {
		t.Fatalf("the last installed version must stay version 1, got %v", deref(d.LastInstalledVersion))
	}
	if got := readFile(t, filepath.Join(p.Path, artifact.ManifestFile)); got != v2.Manifest {
		t.Fatal("the target version's files are left in place, with no automatic return")
	}

	// The user installs the previous version again.
	h.setPixi(func(f *fakePixi) { f.installErr, f.partialEnv = nil, false })
	if done := h.install(InstallRequest{ProjectID: p.ID}); done.Status != jobs.StatusCompleted {
		t.Fatalf("reinstall of the last installed version: %+v", done)
	}
	h.wantInstalled(p, v1)
}

// TestInstallFailureOutlivesARefusedInstall checks that an install that
// refuses before installing does not let the runner drop the earlier
// failed install, which still decides the state.
func TestInstallFailureOutlivesARefusedInstall(t *testing.T) {
	h := newHarnessWithRunner(t, jobs.Options{MaxRunning: 1, KeepFinished: 1})
	blocker, _ := h.newProject("blocker", false)
	p, v1 := h.newProject("failing", true)
	h.createVersion(CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, Spec: newerSpec()})
	v2 := h.versions.versionsOf(t, p.ID)[1]

	h.setPixi(func(f *fakePixi) { f.installErr, f.partialEnv = errors.New("download failed"), true })
	failed := h.install(InstallRequest{ProjectID: p.ID, VersionID: &v2.ID})
	if failed.Status != jobs.StatusFailed || failed.Stage != StageInstall {
		t.Fatalf("want a failed install, got %+v", failed)
	}
	h.setPixi(func(f *fakePixi) { f.installErr, f.partialEnv = nil, false })

	// An install waits behind another project's install, and the files
	// change while it waits, so it refuses before installing.
	gate := make(chan struct{})
	started := make(chan struct{}, 2)
	h.setPixi(func(f *fakePixi) { f.installGate, f.installStarted = gate, started })
	blocking, err := h.svc.InstallVersion(context.Background(), InstallRequest{ProjectID: blocker.ID})
	if err != nil {
		t.Fatal(err)
	}
	waitStarted(t, started)
	pending, err := h.svc.InstallVersion(context.Background(), InstallRequest{ProjectID: p.ID, VersionID: &v1.ID})
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(p.Path, artifact.ManifestFile), v2.Manifest+"\n# edited while pending\n")
	close(gate)
	h.wait(blocking)
	if refused := h.wait(pending); refused.Status != jobs.StatusFailed || refused.Stage != "" {
		t.Fatalf("want the install refused before installing, got %+v", refused)
	}
	h.setPixi(func(f *fakePixi) { f.installGate, f.installStarted = nil, nil })

	// Later jobs of another project push old finished jobs out.
	for range 2 {
		h.install(InstallRequest{ProjectID: blocker.ID})
	}
	if d := h.detail(p.ID); d.State != StateInstallFailed || d.JobID == nil || *d.JobID != failed.ID {
		t.Fatalf("want install_failed linked to the failed install %s, got %+v", failed.ID, d.ProjectSummary)
	}
}

// TestInstallInterruptedByRestart quits nebi during an install: after the
// restart the project is install_failed with no job, nothing is installed,
// and the last installed version is the one before. Installing again
// recovers.
func TestInstallInterruptedByRestart(t *testing.T) {
	h := newHarness(t)
	p, v1 := h.newProject("interrupted-install", true)
	h.createVersion(CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, Spec: newerSpec()})
	v2 := h.versions.versionsOf(t, p.ID)[1]

	started := make(chan struct{}, 1)
	h.setPixi(func(f *fakePixi) { f.installGate, f.installStarted = make(chan struct{}), started }) // never opened
	if _, err := h.svc.InstallVersion(context.Background(), InstallRequest{ProjectID: p.ID, VersionID: &v2.ID}); err != nil {
		t.Fatal(err)
	}
	waitStarted(t, started)

	r := h.restarted()
	d := r.detail(p.ID)
	if d.State != StateInstallFailed || d.JobID != nil || d.InstalledVersionID != nil {
		t.Fatalf("after the restart want install_failed with no job and nothing installed, got %+v", d)
	}
	if d.LastInstalledVersionID == nil || *d.LastInstalledVersionID != v1.ID || deref(d.LastInstalledVersion) != 1 {
		t.Fatalf("the last installed version must stay version 1, got %v", deref(d.LastInstalledVersion))
	}
	if id := r.installingVersionID(p.ID); id == nil || *id != v2.ID {
		t.Fatalf("installing version = %v, want version 2", id)
	}

	r.setPixi(func(f *fakePixi) { f.installGate, f.installStarted = nil, nil })
	if done := r.install(InstallRequest{ProjectID: p.ID}); done.Status != jobs.StatusCompleted {
		t.Fatalf("install after the restart: %+v", done)
	}
	r.wantInstalled(p, v1)
}

// TestInstallFailureSurvivesRestart checks that a failed install, which
// left no environment, is still install_failed after a restart, and that
// uninstall clears it.
func TestInstallFailureSurvivesRestart(t *testing.T) {
	h := newHarness(t)
	p, v1 := h.newProject("failed-then-restarted", true)
	h.uninstall(p.ID)
	h.setPixi(func(f *fakePixi) { f.installErr = errors.New("solve failed") })
	if done := h.install(InstallRequest{ProjectID: p.ID, VersionID: &v1.ID}); done.Status != jobs.StatusFailed || done.Stage != StageInstall {
		t.Fatalf("want a failed install, got %+v", done)
	}
	if exists(envsDir(p.Path)) {
		t.Fatal("precondition: the failed install left no .pixi/envs")
	}

	r := h.restarted()
	checkAfterRestart(t, r, p.ID, StateInstallFailed, nil)

	done := r.uninstall(p.ID)
	if done.Status != jobs.StatusCompleted {
		t.Fatalf("uninstall after a failed install with no environment: %+v", done)
	}
	if d := r.detail(p.ID); d.State != StateNotInstalled || d.JobID != nil {
		t.Fatalf("after uninstall want not_installed, got %+v", d.ProjectSummary)
	}
	if id := r.installingVersionID(p.ID); id != nil {
		t.Fatalf("installing version = %s after uninstall, want none", id)
	}
	if _, err := r.svc.Uninstall(context.Background(), p.ID); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("a second uninstall: got %v, want ErrNotInstalled", err)
	}
}

// TestUninstall checks that uninstall removes .pixi/envs and clears the
// installed version, and keeps the files and the last installed version.
func TestUninstall(t *testing.T) {
	h := newHarness(t)
	p, v1 := h.newProject("removed", true)
	other := filepath.Join(p.Path, ".pixi", "config.toml")
	writeTestFile(t, other, "# kept\n")

	done := h.uninstall(p.ID)
	if done.Status != jobs.StatusCompleted {
		t.Fatalf("uninstall: %+v", done)
	}
	if exists(envsDir(p.Path)) {
		t.Fatal(".pixi/envs must be removed")
	}
	if !exists(other) {
		t.Fatal("only .pixi/envs is removed")
	}
	if readFile(t, filepath.Join(p.Path, artifact.ManifestFile)) != v1.Manifest || readFile(t, filepath.Join(p.Path, artifact.LockFile)) != v1.Lock {
		t.Fatal("pixi.toml and pixi.lock must be kept")
	}
	d := h.detail(p.ID)
	if d.State != StateNotInstalled || d.InstalledVersionID != nil || d.InstalledVersion != nil {
		t.Fatalf("want not_installed with nothing installed, got %+v", d)
	}
	if d.LastInstalledVersionID == nil || *d.LastInstalledVersionID != v1.ID || deref(d.LastInstalledVersion) != 1 {
		t.Fatalf("the last installed version must be kept, got %v", deref(d.LastInstalledVersion))
	}
	if out, _ := h.runner.Logs(done.ID); !strings.Contains(string(out), envsDir(p.Path)) {
		t.Fatalf("the log must say what is removed, got %q", out)
	}

	jobsBefore := h.jobCount(p.ID)
	if _, err := h.svc.Uninstall(context.Background(), p.ID); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("a second uninstall: want ErrNotInstalled, got %v", err)
	}
	if h.jobCount(p.ID) != jobsBefore {
		t.Fatal("a refused uninstall must submit no job")
	}
}

// TestUninstallLeftoverEnvironment checks that uninstall removes a
// .pixi/envs that nebi did not record as installed: one a failed install
// left behind, and one made by running pixi outside nebi.
func TestUninstallLeftoverEnvironment(t *testing.T) {
	h := newHarness(t)
	p, _ := h.newProject("leftover", false)
	h.setPixi(func(f *fakePixi) { f.installErr, f.partialEnv = errors.New("download failed"), true })
	if done := h.install(InstallRequest{ProjectID: p.ID}); done.Status != jobs.StatusFailed {
		t.Fatalf("precondition: want a failed install, got %+v", done)
	}
	if !exists(envsDir(p.Path)) || h.detail(p.ID).InstalledVersionID != nil {
		t.Fatal("precondition: a partial environment and no installed version")
	}
	if done := h.uninstall(p.ID); done.Status != jobs.StatusCompleted {
		t.Fatalf("uninstall: %+v", done)
	}
	if exists(envsDir(p.Path)) {
		t.Fatal("the leftover environment must be removed")
	}
	if d := h.detail(p.ID); d.State != StateNotInstalled || d.LastInstalledVersionID != nil {
		t.Fatalf("want not_installed and no last installed version, got %+v", d)
	}

	// An environment pixi made outside nebi.
	if err := os.MkdirAll(filepath.Join(envsDir(p.Path), "default"), 0o755); err != nil {
		t.Fatal(err)
	}
	if done := h.uninstall(p.ID); done.Status != jobs.StatusCompleted || exists(envsDir(p.Path)) {
		t.Fatalf("uninstall of an outside environment: %+v", done)
	}
}

// TestUninstallKeepsWhatASymlinkPointsAt checks that a .pixi/envs symlink
// is removed without touching its target.
func TestUninstallKeepsWhatASymlinkPointsAt(t *testing.T) {
	h := newHarness(t)
	p, _ := h.newProject("linked", true)
	target := t.TempDir()
	writeTestFile(t, filepath.Join(target, "keep"), "x")
	if err := os.RemoveAll(envsDir(p.Path)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, envsDir(p.Path)); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if done := h.uninstall(p.ID); done.Status != jobs.StatusCompleted {
		t.Fatalf("uninstall: %+v", done)
	}
	if _, err := os.Lstat(envsDir(p.Path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the symlink must be removed: %v", err)
	}
	if !exists(filepath.Join(target, "keep")) {
		t.Fatal("what the symlink points at must be kept")
	}
}

// TestUninstallThroughASymlinkedPixiDir checks that when .pixi is a
// symlink, uninstall removes the envs inside its target, where pixi
// installed it, and keeps the symlink and the target's other files.
func TestUninstallThroughASymlinkedPixiDir(t *testing.T) {
	h := newHarness(t)
	p, _ := h.newProject("linked-pixi", true)
	target := filepath.Join(t.TempDir(), "pixi")
	pixiDir := filepath.Join(p.Path, ".pixi")
	if err := os.Rename(pixiDir, target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, pixiDir); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	writeTestFile(t, filepath.Join(target, "keep"), "x")
	if !exists(filepath.Join(target, "envs")) {
		t.Fatal("precondition: the environment is inside the symlink's target")
	}
	if done := h.uninstall(p.ID); done.Status != jobs.StatusCompleted {
		t.Fatalf("uninstall: %+v", done)
	}
	if exists(filepath.Join(target, "envs")) {
		t.Fatal("the envs inside the symlink's target must be removed")
	}
	if info, err := os.Lstat(pixiDir); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the .pixi symlink must be kept: %v", err)
	}
	if !exists(filepath.Join(target, "keep")) {
		t.Fatal("the target's other files must be kept")
	}
}

// TestUninstallFailureLeavesNotInstalled checks that a removal that fails
// part way does not leave the project reported as installed.
func TestUninstallFailureLeavesNotInstalled(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can remove from a read-only directory")
	}
	h := newHarness(t)
	p, _ := h.newProject("stuck", true)
	locked := filepath.Join(envsDir(p.Path), "default")
	if err := os.Chmod(locked, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	done := h.uninstall(p.ID)
	if done.Status != jobs.StatusFailed {
		t.Skipf("the file system allowed the removal: %+v", done)
	}
	if d := h.detail(p.ID); d.State != StateNotInstalled || d.InstalledVersionID != nil {
		t.Fatalf("a failed uninstall must not look installed, got %+v", d)
	}
	if err := os.Chmod(locked, 0o755); err != nil {
		t.Fatal(err)
	}
	if done := h.uninstall(p.ID); done.Status != jobs.StatusCompleted || exists(envsDir(p.Path)) {
		t.Fatalf("a second uninstall removes the rest: %+v", done)
	}
}

// TestEveryInstallSetsLastInstalled checks that the create job, a
// create_version job and an install job run the one install step, which
// records the last installed version.
func TestEveryInstallSetsLastInstalled(t *testing.T) {
	h := newHarness(t)
	p, v1 := h.newProject("shared-step", true)
	h.wantInstalled(p, v1)

	h.createVersion(CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, Spec: newerSpec(), Install: true})
	v2 := h.versions.versionsOf(t, p.ID)[1]
	h.wantInstalled(p, v2)

	res, _ := h.createVersion(CreateVersionRequest{ProjectID: p.ID, BaseVersionID: v1.ID, AllowOlderBase: true, Install: true})
	h.wait(res.Job)
	h.wantInstalled(p, *res.Version)

	h.install(InstallRequest{ProjectID: p.ID, VersionID: &v1.ID})
	h.wantInstalled(p, v1)
}
