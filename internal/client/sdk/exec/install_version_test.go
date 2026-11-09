package exec_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/auth"
	"github.com/nebari-dev/nebi/internal/client/sdk/exec"
	"github.com/nebari-dev/nebi/internal/client/sdk/local"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
)

// fakeExecutor records install calls and materializes .pixi/envs on
// success. failOn lists manifests (by their "# vN" marker) whose install
// fails.
type fakeExecutor struct {
	root     string
	frozen   []string // version markers installed with --frozen
	plain    []string // version markers installed without --frozen
	failOn   map[string]error
	noFrozen bool
}

func (f *fakeExecutor) GetProjectPath(p *models.Project) string {
	if p.Path != "" {
		return p.Path
	}
	return filepath.Join(f.root, p.ID.String())
}

func (f *fakeExecutor) marker(p *models.Project) string {
	b, _ := os.ReadFile(filepath.Join(f.GetProjectPath(p), "pixi.toml"))
	first, _, _ := strings.Cut(string(b), "\n")
	return strings.TrimPrefix(first, "# ")
}

func (f *fakeExecutor) install(ctx context.Context, p *models.Project, frozen bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m := f.marker(p)
	if frozen {
		f.frozen = append(f.frozen, m)
	} else {
		f.plain = append(f.plain, m)
	}
	if err := f.failOn[m]; err != nil {
		return err
	}
	envs := filepath.Join(f.GetProjectPath(p), ".pixi", "envs", "default")
	if err := os.MkdirAll(envs, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(envs, "marker"), []byte(m), 0o644)
}

func (f *fakeExecutor) InstallEnvironment(ctx context.Context, p *models.Project, _ io.Writer) error {
	return f.install(ctx, p, false)
}

func (f *fakeExecutor) UninstallEnvironment(_ context.Context, p *models.Project, _ io.Writer) error {
	return os.RemoveAll(filepath.Join(f.GetProjectPath(p), ".pixi", "envs"))
}

func (f *fakeExecutor) IsEnvInstalled(p *models.Project) bool {
	info, err := os.Stat(filepath.Join(f.GetProjectPath(p), ".pixi", "envs"))
	return err == nil && info.IsDir()
}

func (f *fakeExecutor) CreateProject(context.Context, *models.Project, io.Writer, executor.CreateProjectOptions) error {
	return nil
}
func (f *fakeExecutor) InstallPackages(context.Context, *models.Project, []string, io.Writer) error {
	return nil
}
func (f *fakeExecutor) RemovePackages(context.Context, *models.Project, []string, io.Writer) error {
	return nil
}
func (f *fakeExecutor) DeleteProject(context.Context, *models.Project, io.Writer) error { return nil }
func (f *fakeExecutor) SolveEnvironment(context.Context, *models.Project, io.Writer) error {
	return nil
}
func (f *fakeExecutor) CleanupJobArtifacts(context.Context, *models.Project, models.JobType, io.Writer) error {
	return nil
}
func (f *fakeExecutor) StagingRoot() string { return f.root }

// frozenExecutor adds `pixi install --frozen`.
type frozenExecutor struct{ *fakeExecutor }

func (f frozenExecutor) InstallEnvironmentFrozen(ctx context.Context, p *models.Project, _ io.Writer) error {
	return f.install(ctx, p, true)
}

// recordingJobs stores submitted jobs as pending rows.
type recordingJobs struct {
	db   *gorm.DB
	reqs []shared.JobRequest
}

func (r *recordingJobs) Submit(ctx context.Context, req shared.JobRequest) (*models.Job, error) {
	r.reqs = append(r.reqs, req)
	job := &models.Job{ProjectID: req.ProjectID, Type: req.Type, Status: models.JobStatusPending, VersionNumber: req.VersionNumber, Metadata: req.Metadata}
	return job, r.db.WithContext(ctx).Create(job).Error
}

type installFixture struct {
	svc     *exec.Service
	db      *gorm.DB
	exec    *fakeExecutor
	jobs    *recordingJobs
	project models.Project
}

func newInstallFixture(t *testing.T, versions int, frozen bool) *installFixture {
	t.Helper()
	gdb, err := local.OpenDB(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := gdb.DB(); err == nil {
			sqlDB.Close()
		}
	})
	if err := local.Migrate(gdb); err != nil {
		t.Fatal(err)
	}
	la, err := auth.NewLocalAuthenticator(gdb)
	if err != nil {
		t.Fatal(err)
	}
	fe := &fakeExecutor{root: t.TempDir(), failOn: map[string]error{}}
	var ex executor.Executor = fe
	if frozen {
		ex = frozenExecutor{fe}
	}
	rt := &shared.Runtime{}
	jobs := &recordingJobs{db: gdb}
	rt.SetJobs(jobs)
	deps := shared.Deps{DB: gdb, Executor: ex, DataDir: t.TempDir(), LocalUserID: la.User().ID, Runtime: rt}
	l := local.New(deps)
	f := &installFixture{svc: exec.New(deps, l), db: gdb, exec: fe, jobs: jobs}
	f.project = models.Project{Name: "demo", OwnerID: la.User().ID, Status: models.ProjectStatusReady}
	if err := gdb.Create(&f.project).Error; err != nil {
		t.Fatal(err)
	}
	for n := 1; n <= versions; n++ {
		v := models.ProjectVersion{
			ProjectID:       f.project.ID,
			VersionNumber:   n,
			ManifestContent: fmt.Sprintf("# v%d\n[workspace]\nname = \"demo\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n", n),
			LockFileContent: fmt.Sprintf("version: 6\n# lock v%d\n", n),
			PackageMetadata: "[]",
			CreatedBy:       la.User().ID,
		}
		if err := gdb.Create(&v).Error; err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func (f *installFixture) reload(t *testing.T) models.Project {
	t.Helper()
	var p models.Project
	if err := f.db.First(&p, "id = ?", f.project.ID).Error; err != nil {
		t.Fatal(err)
	}
	return p
}

func (f *installFixture) run(t *testing.T, version *int) (*models.Job, error) {
	t.Helper()
	job := &models.Job{ProjectID: f.project.ID, Type: models.JobTypeInstallVersion, VersionNumber: version}
	var log strings.Builder
	err := f.svc.RunInstallVersion(context.Background(), job, &log)
	return job, err
}

func intp(n int) *int { return &n }

func TestInstallVersionResolvesAndSubmits(t *testing.T) {
	f := newInstallFixture(t, 3, true)
	ctx := context.Background()

	job, err := f.svc.InstallVersion(ctx, f.project.ID, shared.InstallRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if job.Type != models.JobTypeInstallVersion || job.VersionNumber == nil || *job.VersionNumber != 3 || job.ProjectName != "demo" {
		t.Fatalf("no version = latest: %+v", job)
	}
	if job.TypeLabel != "Install version" {
		t.Fatalf("label = %q", job.TypeLabel)
	}

	// An active job blocks another install.
	var cerr *shared.ConflictError
	if _, err := f.svc.InstallVersion(ctx, f.project.ID, shared.InstallRequest{Version: intp(1)}); !errors.As(err, &cerr) || cerr.Code != "job_active" {
		t.Fatalf("active job: %v", err)
	}
	f.db.Model(&models.Job{}).Where("project_id = ?", f.project.ID).Update("status", models.JobStatusCompleted)

	// Last installed wins when no version is given.
	f.db.Model(&models.Project{}).Where("id = ?", f.project.ID).Update("last_installed_version", 2)
	job, err = f.svc.InstallVersion(ctx, f.project.ID, shared.InstallRequest{})
	if err != nil || *job.VersionNumber != 2 {
		t.Fatalf("last installed: %+v, %v", job, err)
	}
	f.db.Model(&models.Job{}).Where("project_id = ?", f.project.ID).Update("status", models.JobStatusCompleted)

	var verr *shared.ValidationError
	if _, err := f.svc.InstallVersion(ctx, f.project.ID, shared.InstallRequest{Version: intp(9)}); !errors.As(err, &verr) || !strings.Contains(verr.Message, "Pull version 9 first") {
		t.Fatalf("missing version: %v", err)
	}
	if _, err := f.svc.InstallVersion(ctx, uuid.New(), shared.InstallRequest{}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("missing project: %v", err)
	}
	f.db.Model(&models.Project{}).Where("id = ?", f.project.ID).Update("status", models.ProjectStatusDeleting)
	if _, err := f.svc.InstallVersion(ctx, f.project.ID, shared.InstallRequest{}); !errors.As(err, &cerr) {
		t.Fatalf("deleting project: %v", err)
	}
}

func TestInstallVersionWithoutVersions(t *testing.T) {
	f := newInstallFixture(t, 0, true)
	var verr *shared.ValidationError
	if _, err := f.svc.InstallVersion(context.Background(), f.project.ID, shared.InstallRequest{}); !errors.As(err, &verr) {
		t.Fatalf("no versions: %v", err)
	}
}

func TestRunInstallVersionWritesFilesAndRecordsInstalled(t *testing.T) {
	f := newInstallFixture(t, 3, true)
	job, err := f.run(t, intp(2))
	if err != nil {
		t.Fatal(err)
	}
	if *job.VersionNumber != 2 {
		t.Fatal("job.VersionNumber")
	}
	dir := f.exec.GetProjectPath(&f.project)
	toml, _ := os.ReadFile(filepath.Join(dir, "pixi.toml"))
	lock, _ := os.ReadFile(filepath.Join(dir, "pixi.lock"))
	if !strings.HasPrefix(string(toml), "# v2\n") || !strings.Contains(string(lock), "lock v2") {
		t.Fatalf("files not written: %q %q", toml, lock)
	}
	if len(f.exec.frozen) != 1 || f.exec.frozen[0] != "v2" || len(f.exec.plain) != 0 {
		t.Fatalf("install must be frozen: frozen=%v plain=%v", f.exec.frozen, f.exec.plain)
	}
	p := f.reload(t)
	if p.InstalledVersion == nil || *p.InstalledVersion != 2 || p.LastInstalledVersion == nil || *p.LastInstalledVersion != 2 {
		t.Fatalf("installed = %v last = %v", p.InstalledVersion, p.LastInstalledVersion)
	}
	if p.SizeBytes <= 0 {
		t.Fatal("size on disk recorded")
	}

	// Installing another version replaces it.
	if _, err := f.run(t, intp(3)); err != nil {
		t.Fatal(err)
	}
	p = f.reload(t)
	if *p.InstalledVersion != 3 || *p.LastInstalledVersion != 3 {
		t.Fatalf("replace: installed = %v", *p.InstalledVersion)
	}
	toml, _ = os.ReadFile(filepath.Join(dir, "pixi.toml"))
	if !strings.HasPrefix(string(toml), "# v3\n") {
		t.Fatal("v3 files")
	}
}

func TestRunInstallVersionWithoutFrozenSupportOrLock(t *testing.T) {
	f := newInstallFixture(t, 1, false)
	if _, err := f.run(t, nil); err != nil {
		t.Fatal(err)
	}
	if len(f.exec.plain) != 1 {
		t.Fatalf("executors without --frozen use a plain install: %v", f.exec.plain)
	}

	g := newInstallFixture(t, 1, true)
	g.db.Model(&models.ProjectVersion{}).Where("project_id = ?", g.project.ID).Update("lock_file_content", "")
	dir := g.exec.GetProjectPath(&g.project)
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "pixi.lock"), []byte("stale"), 0o644)
	if _, err := g.run(t, intp(1)); err != nil {
		t.Fatal(err)
	}
	if len(g.exec.plain) != 1 || len(g.exec.frozen) != 0 {
		t.Fatal("a version without a lock cannot be installed frozen")
	}
	if _, err := os.Stat(filepath.Join(dir, "pixi.lock")); !os.IsNotExist(err) {
		t.Fatal("stale lock must be removed")
	}
}

func TestRunInstallVersionFailureRestoresPreviousVersion(t *testing.T) {
	f := newInstallFixture(t, 3, true)
	if _, err := f.run(t, intp(1)); err != nil {
		t.Fatal(err)
	}
	f.exec.failOn["v3"] = errors.New("download failed")
	_, err := f.run(t, intp(3))
	if err == nil || !strings.Contains(err.Error(), "version 3 couldn't install") {
		t.Fatalf("err = %v", err)
	}
	p := f.reload(t)
	if p.InstalledVersion == nil || *p.InstalledVersion != 1 {
		t.Fatalf("v1 must stay installed: %v", p.InstalledVersion)
	}
	toml, _ := os.ReadFile(filepath.Join(f.exec.GetProjectPath(&f.project), "pixi.toml"))
	if !strings.HasPrefix(string(toml), "# v1\n") {
		t.Fatal("v1 files restored")
	}
	if got := strings.Join(f.exec.frozen, ","); got != "v1,v3,v1" {
		t.Fatalf("installs = %s (v1 reinstalled after the failure)", got)
	}

	// When the restore fails too, nothing counts as installed.
	f.exec.failOn["v1"] = errors.New("still offline")
	if _, err := f.run(t, intp(3)); err == nil {
		t.Fatal("want failure")
	}
	p = f.reload(t)
	if p.InstalledVersion != nil || p.LastInstalledVersion == nil || *p.LastInstalledVersion != 1 {
		t.Fatalf("installed = %v last = %v", p.InstalledVersion, p.LastInstalledVersion)
	}
}

func TestRunInstallVersionFailureWithNothingInstalled(t *testing.T) {
	f := newInstallFixture(t, 1, true)
	dir := f.exec.GetProjectPath(&f.project)
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "pixi.toml"), []byte("# original\n"), 0o644)
	f.exec.failOn["v1"] = errors.New("boom")
	if _, err := f.run(t, intp(1)); err == nil {
		t.Fatal("want failure")
	}
	if f.exec.IsEnvInstalled(&f.project) {
		t.Fatal("partial environment removed")
	}
	toml, _ := os.ReadFile(filepath.Join(dir, "pixi.toml"))
	if string(toml) != "# original\n" {
		t.Fatalf("original files restored: %q", toml)
	}
	if _, err := os.Stat(filepath.Join(dir, "pixi.lock")); !os.IsNotExist(err) {
		t.Fatal("lock did not exist before")
	}
	if p := f.reload(t); p.InstalledVersion != nil {
		t.Fatal("nothing installed")
	}
}

func TestRunInstallVersionCancelled(t *testing.T) {
	f := newInstallFixture(t, 2, true)
	if _, err := f.run(t, intp(1)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	job := &models.Job{ProjectID: f.project.ID, VersionNumber: intp(2)}
	if err := f.svc.RunInstallVersion(ctx, job, io.Discard); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if p := f.reload(t); p.InstalledVersion == nil || *p.InstalledVersion != 1 {
		t.Fatal("cancelled install keeps v1")
	}
}

func TestUninstall(t *testing.T) {
	f := newInstallFixture(t, 2, true)
	ctx := context.Background()
	var cerr *shared.ConflictError
	if _, err := f.svc.Uninstall(ctx, f.project.ID); !errors.As(err, &cerr) || cerr.Code != "not_installed" {
		t.Fatalf("nothing installed: %v", err)
	}
	if _, err := f.run(t, intp(2)); err != nil {
		t.Fatal(err)
	}
	job, err := f.svc.Uninstall(ctx, f.project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if job.Type != models.JobTypeUninstall || job.VersionNumber == nil || *job.VersionNumber != 2 {
		t.Fatalf("job = %+v", job)
	}
	run := &models.Job{ProjectID: f.project.ID, Type: models.JobTypeUninstall}
	if err := f.svc.RunUninstall(ctx, run, io.Discard); err != nil {
		t.Fatal(err)
	}
	if run.VersionNumber == nil || *run.VersionNumber != 2 {
		t.Fatal("uninstall job names the version")
	}
	p := f.reload(t)
	if p.InstalledVersion != nil || p.LastInstalledVersion == nil || *p.LastInstalledVersion != 2 || p.SizeBytes != 0 {
		t.Fatalf("after uninstall: installed=%v last=%v size=%d", p.InstalledVersion, p.LastInstalledVersion, p.SizeBytes)
	}
	if f.exec.IsEnvInstalled(&f.project) {
		t.Fatal("env removed")
	}
	if _, err := os.Stat(filepath.Join(f.exec.GetProjectPath(&f.project), "pixi.toml")); err != nil {
		t.Fatal("project files stay")
	}
}
