package worker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/pkgmgr"
	"github.com/nebari-dev/nebi/internal/queue"
	"github.com/nebari-dev/nebi/internal/rbac"
	"github.com/nebari-dev/nebi/internal/service"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// testPackageManager is a registered no-op manager used by worker tests.
// Keeping the create flow on its happy path means the test reaches db.Save(ws)
// via UpdateWorkspaceSize regardless of how the worker's error handling around
// SyncPackagesFromWorkspace / CreateVersionSnapshot is refactored later.
const testPackageManager = "test-noop"

func init() {
	pkgmgr.Register(testPackageManager, func(string) (pkgmgr.PackageManager, error) {
		return noopPackageManager{}, nil
	})
}

type noopPackageManager struct{}

func (noopPackageManager) Name() string                                         { return testPackageManager }
func (noopPackageManager) Init(context.Context, pkgmgr.InitOptions) error       { return nil }
func (noopPackageManager) Install(context.Context, pkgmgr.InstallOptions) error { return nil }
func (noopPackageManager) Remove(context.Context, pkgmgr.RemoveOptions) error   { return nil }
func (noopPackageManager) List(context.Context, pkgmgr.ListOptions) ([]pkgmgr.Package, error) {
	return nil, nil
}
func (noopPackageManager) Update(context.Context, pkgmgr.UpdateOptions) error { return nil }
func (noopPackageManager) GetManifest(context.Context, string) (*pkgmgr.Manifest, error) {
	return &pkgmgr.Manifest{}, nil
}

// fakeExecutor is a minimal Executor stub for worker tests. CreateWorkspace
// creates the workspace directory and writes empty manifest/lock files so
// CreateVersionSnapshot reads them successfully; the rest are no-ops.
type fakeExecutor struct {
	rootDir        string
	solveErr       error
	installErr     error
	installCalls   int
	uninstallCalls int
	solveCalls     int
}

func (e *fakeExecutor) CreateWorkspace(ctx context.Context, ws *models.Workspace, w io.Writer, opts executor.CreateWorkspaceOptions) error {
	p := e.GetWorkspacePath(ws)
	if err := os.MkdirAll(p, 0o755); err != nil {
		return err
	}
	for _, name := range []string{"pixi.toml", "pixi.lock"} {
		if err := os.WriteFile(filepath.Join(p, name), nil, 0o644); err != nil {
			return err
		}
	}
	return nil
}
func (e *fakeExecutor) InstallPackages(context.Context, *models.Workspace, []string, io.Writer) error {
	return nil
}
func (e *fakeExecutor) RemovePackages(context.Context, *models.Workspace, []string, io.Writer) error {
	return nil
}
func (e *fakeExecutor) DeleteWorkspace(context.Context, *models.Workspace, io.Writer) error {
	return nil
}
func (e *fakeExecutor) SolveEnvironment(context.Context, *models.Workspace, io.Writer) error {
	e.solveCalls++
	return e.solveErr
}

// InstallEnvironment/UninstallEnvironment/IsEnvInstalled mimic the real
// executor's disk contract: installed means <ws>/.pixi/envs exists.
func (e *fakeExecutor) InstallEnvironment(ctx context.Context, ws *models.Workspace, w io.Writer) error {
	e.installCalls++
	if e.installErr != nil {
		return e.installErr
	}
	return os.MkdirAll(filepath.Join(e.GetWorkspacePath(ws), ".pixi", "envs"), 0o755)
}
func (e *fakeExecutor) UninstallEnvironment(ctx context.Context, ws *models.Workspace, w io.Writer) error {
	e.uninstallCalls++
	return os.RemoveAll(filepath.Join(e.GetWorkspacePath(ws), ".pixi", "envs"))
}
func (e *fakeExecutor) IsEnvInstalled(ws *models.Workspace) bool {
	info, err := os.Stat(filepath.Join(e.GetWorkspacePath(ws), ".pixi", "envs"))
	return err == nil && info.IsDir()
}
func (e *fakeExecutor) GetWorkspacePath(ws *models.Workspace) string {
	return filepath.Join(e.rootDir, ws.Name+"-"+ws.ID.String())
}
func (e *fakeExecutor) StagingRoot() string {
	return filepath.Join(e.rootDir, ".staging")
}

// TestExecuteJob_CreatePersistsWorkspacePath is a regression test for #294.
//
// In the create flow, the worker calls SetWorkspacePath (a targeted UPDATE on
// the path column) and then UpdateWorkspaceSize, which uses db.Save(ws) — a
// full-row write. The ws struct was loaded at job start with Path="", so
// without an in-memory sync the Save call clobbers the path back to empty.
// The fix sets ws.Path = resolvedPath after SetWorkspacePath; this test
// drives executeJob end-to-end with a stale workspace and asserts the path
// is non-empty in the DB after the create flow finishes.
func TestExecuteJob_CreatePersistsWorkspacePath(t *testing.T) {
	db, svc, jobSvc, exec := setupWorkerTest(t)

	user := models.User{Username: "alice", Email: "alice@test.com"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	ws := &models.Workspace{
		Name:           "regr-294",
		OwnerID:        user.ID,
		Status:         models.WsStatusPending,
		PackageManager: testPackageManager,
	}
	if err := db.Create(ws).Error; err != nil {
		t.Fatalf("create ws: %v", err)
	}

	job := &models.Job{
		WorkspaceID: ws.ID,
		Type:        models.JobTypeCreate,
		Status:      models.JobStatusPending,
		Metadata:    map[string]interface{}{},
	}
	if err := db.Create(job).Error; err != nil {
		t.Fatalf("create job: %v", err)
	}

	w := New(queue.NewMemoryQueue(10), exec, svc, jobSvc, slog.Default(), nil)

	if err := w.executeJob(context.Background(), job, &bytes.Buffer{}); err != nil {
		t.Fatalf("executeJob: %v", err)
	}

	want := exec.GetWorkspacePath(ws)
	var stored models.Workspace
	if err := db.First(&stored, "id = ?", ws.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stored.Path != want {
		t.Errorf("workspace path was not persisted: want %q, got %q", want, stored.Path)
	}
}

func TestExecuteJob_UpdateSetsWorkspaceReady(t *testing.T) {
	db, svc, jobSvc, exec := setupWorkerTest(t)

	user := models.User{Username: "alice", Email: "alice@test.com"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	ws := &models.Workspace{
		Name:           "update-ready",
		OwnerID:        user.ID,
		Status:         models.WsStatusPending,
		PackageManager: testPackageManager,
	}
	if err := db.Create(ws).Error; err != nil {
		t.Fatalf("create ws: %v", err)
	}

	job := &models.Job{
		WorkspaceID: ws.ID,
		Type:        models.JobTypeUpdate,
		Status:      models.JobStatusPending,
		Metadata:    map[string]interface{}{},
	}
	if err := db.Create(job).Error; err != nil {
		t.Fatalf("create job: %v", err)
	}

	w := New(queue.NewMemoryQueue(10), exec, svc, jobSvc, slog.Default(), nil)

	if err := w.executeJob(context.Background(), job, &bytes.Buffer{}); err != nil {
		t.Fatalf("executeJob: %v", err)
	}

	var updated models.Workspace
	if err := db.First(&updated, "id = ?", ws.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if updated.Status != models.WsStatusReady {
		t.Errorf("workspace status not updated to ready: got %q", updated.Status)
	}
}

func TestExecuteJob_UpdateSetsWorkspaceFailedOnSolveError(t *testing.T) {
	db, svc, jobSvc, exec := setupWorkerTest(t)
	exec.solveErr = errors.New("solve failed")

	user := models.User{Username: "alice", Email: "alice@test.com"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	ws := &models.Workspace{
		Name:           "update-failed",
		OwnerID:        user.ID,
		Status:         models.WsStatusReady,
		PackageManager: testPackageManager,
	}
	if err := db.Create(ws).Error; err != nil {
		t.Fatalf("create ws: %v", err)
	}

	job := &models.Job{
		WorkspaceID: ws.ID,
		Type:        models.JobTypeUpdate,
		Status:      models.JobStatusPending,
		Metadata:    map[string]interface{}{},
	}
	if err := db.Create(job).Error; err != nil {
		t.Fatalf("create job: %v", err)
	}

	w := New(queue.NewMemoryQueue(10), exec, svc, jobSvc, slog.Default(), nil)

	if err := w.executeJob(context.Background(), job, &bytes.Buffer{}); err == nil {
		t.Fatal("expected executeJob to fail, got nil")
	}

	var updated models.Workspace
	if err := db.First(&updated, "id = ?", ws.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if updated.Status != models.WsStatusFailed {
		t.Errorf("workspace status not updated to failed: got %q", updated.Status)
	}
}

// newTestWorkspace inserts a ready workspace (with backing dir and
// manifest/lock files) plus a job of the given type, returning both.
func newTestWorkspace(t *testing.T, db *gorm.DB, exec *fakeExecutor, name string, jobType models.JobType, metadata map[string]interface{}) (*models.Workspace, *models.Job) {
	t.Helper()

	user := models.User{Username: "alice", Email: "alice@example.com"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	ws := &models.Workspace{
		Name:           name,
		OwnerID:        user.ID,
		Status:         models.WsStatusReady,
		PackageManager: testPackageManager,
	}
	if err := db.Create(ws).Error; err != nil {
		t.Fatalf("create ws: %v", err)
	}

	p := exec.GetWorkspacePath(ws)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"pixi.toml", "pixi.lock"} {
		if err := os.WriteFile(filepath.Join(p, f), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	job := &models.Job{
		WorkspaceID: ws.ID,
		Type:        jobType,
		Status:      models.JobStatusPending,
		Metadata:    metadata,
	}
	if err := db.Create(job).Error; err != nil {
		t.Fatalf("create job: %v", err)
	}
	return ws, job
}

// TestExecuteJob_EnvInstallRunsInstall proves the explicit install job
// materializes the environment via the executor.
func TestExecuteJob_EnvInstallRunsInstall(t *testing.T) {
	db, svc, jobSvc, exec := setupWorkerTest(t)
	_ = db

	ws, job := newTestWorkspace(t, db, exec, "env-install", models.JobTypeEnvInstall, nil)

	w := New(queue.NewMemoryQueue(10), exec, svc, jobSvc, slog.Default(), nil)
	if err := w.executeJob(context.Background(), job, &bytes.Buffer{}); err != nil {
		t.Fatalf("executeJob: %v", err)
	}

	if exec.installCalls != 1 {
		t.Errorf("expected 1 InstallEnvironment call, got %d", exec.installCalls)
	}
	if !exec.IsEnvInstalled(ws) {
		t.Errorf("expected environment installed on disk after env_install job")
	}
}

// TestExecuteJob_EnvUninstallRemovesEnv proves the uninstall job removes
// the installed environment.
func TestExecuteJob_EnvUninstallRemovesEnv(t *testing.T) {
	db, svc, jobSvc, exec := setupWorkerTest(t)

	ws, job := newTestWorkspace(t, db, exec, "env-uninstall", models.JobTypeEnvUninstall, nil)
	if err := os.MkdirAll(filepath.Join(exec.GetWorkspacePath(ws), ".pixi", "envs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(ws).Update("size_bytes", 999).Error; err != nil {
		t.Fatal(err)
	}
	// Leftover files keep a nonzero on-disk size; the job must still
	// report 0 because size tracks the installed environment.
	if err := os.WriteFile(filepath.Join(exec.GetWorkspacePath(ws), "pixi.lock"), []byte("version: 6\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	w := New(queue.NewMemoryQueue(10), exec, svc, jobSvc, slog.Default(), nil)
	if err := w.executeJob(context.Background(), job, &bytes.Buffer{}); err != nil {
		t.Fatalf("executeJob: %v", err)
	}

	if exec.uninstallCalls != 1 {
		t.Errorf("expected 1 UninstallEnvironment call, got %d", exec.uninstallCalls)
	}
	if exec.IsEnvInstalled(ws) {
		t.Errorf("expected environment removed after env_uninstall job")
	}

	var stored models.Workspace
	if err := db.First(&stored, "id = ?", ws.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if stored.SizeBytes != 0 {
		t.Errorf("expected size_bytes reset to 0 after uninstall, got %d", stored.SizeBytes)
	}
}

// TestExecuteJob_UpdateAutoInstallsWhenPreviouslyInstalled proves a manifest
// update keeps an installed environment in sync: solve refreshes the lock,
// then the environment is reinstalled automatically (local mode).
func TestExecuteJob_UpdateAutoInstallsWhenPreviouslyInstalled(t *testing.T) {
	db, svc, jobSvc, exec := setupWorkerTest(t)

	ws, job := newTestWorkspace(t, db, exec, "update-autoinstall", models.JobTypeUpdate, nil)
	if err := os.MkdirAll(filepath.Join(exec.GetWorkspacePath(ws), ".pixi", "envs"), 0o755); err != nil {
		t.Fatal(err)
	}

	w := New(queue.NewMemoryQueue(10), exec, svc, jobSvc, slog.Default(), nil)
	if err := w.executeJob(context.Background(), job, &bytes.Buffer{}); err != nil {
		t.Fatalf("executeJob: %v", err)
	}

	if exec.installCalls != 1 {
		t.Errorf("expected auto-install after update of installed workspace, got %d install calls", exec.installCalls)
	}
}

// TestExecuteJob_UpdateSkipsAutoInstallWhenNotInstalled proves updating a
// never-installed workspace stops at the lockfile.
func TestExecuteJob_UpdateSkipsAutoInstallWhenNotInstalled(t *testing.T) {
	db, svc, jobSvc, exec := setupWorkerTest(t)

	_, job := newTestWorkspace(t, db, exec, "update-noinstall", models.JobTypeUpdate, nil)

	w := New(queue.NewMemoryQueue(10), exec, svc, jobSvc, slog.Default(), nil)
	if err := w.executeJob(context.Background(), job, &bytes.Buffer{}); err != nil {
		t.Fatalf("executeJob: %v", err)
	}

	if exec.installCalls != 0 {
		t.Errorf("expected no auto-install for not-installed workspace, got %d install calls", exec.installCalls)
	}
}

// TestExecuteJob_UpdateNoAutoInstallInTeamMode proves team-mode servers never
// install environments, even for workspaces with leftover .pixi/envs.
func TestExecuteJob_UpdateNoAutoInstallInTeamMode(t *testing.T) {
	db, svc, jobSvc, exec := setupWorkerTestMode(t, false)
	_ = svc

	ws, job := newTestWorkspace(t, db, exec, "update-team", models.JobTypeUpdate, nil)
	if err := os.MkdirAll(filepath.Join(exec.GetWorkspacePath(ws), ".pixi", "envs"), 0o755); err != nil {
		t.Fatal(err)
	}

	w := New(queue.NewMemoryQueue(10), exec, svc, jobSvc, slog.Default(), nil)
	if err := w.executeJob(context.Background(), job, &bytes.Buffer{}); err != nil {
		t.Fatalf("executeJob: %v", err)
	}

	if exec.installCalls != 0 {
		t.Errorf("expected no auto-install in team mode, got %d install calls", exec.installCalls)
	}
}

// TestExecuteJob_PackageOpsAutoInstallWhenPreviouslyInstalled proves
// add/remove package jobs reinstall a previously-installed environment.
func TestExecuteJob_PackageOpsAutoInstallWhenPreviouslyInstalled(t *testing.T) {
	for _, jobType := range []models.JobType{models.JobTypeInstall, models.JobTypeRemove} {
		t.Run(string(jobType), func(t *testing.T) {
			db, svc, jobSvc, exec := setupWorkerTest(t)

			ws, job := newTestWorkspace(t, db, exec, "pkg-"+string(jobType), jobType,
				map[string]interface{}{"packages": []string{"numpy"}})
			if err := os.MkdirAll(filepath.Join(exec.GetWorkspacePath(ws), ".pixi", "envs"), 0o755); err != nil {
				t.Fatal(err)
			}

			w := New(queue.NewMemoryQueue(10), exec, svc, jobSvc, slog.Default(), nil)
			if err := w.executeJob(context.Background(), job, &bytes.Buffer{}); err != nil {
				t.Fatalf("executeJob: %v", err)
			}

			if exec.installCalls != 1 {
				t.Errorf("expected auto-install after %s, got %d install calls", jobType, exec.installCalls)
			}
		})
	}
}

// TestExecuteJob_RollbackLocksAndAutoInstalls proves rollback restores
// files, refreshes the lock via the executor (no raw pixi install), and
// reinstalls a previously-installed environment.
func TestExecuteJob_RollbackLocksAndAutoInstalls(t *testing.T) {
	db, svc, jobSvc, exec := setupWorkerTest(t)

	ws, _ := newTestWorkspace(t, db, exec, "rollback-ws", models.JobTypeCreate, nil)
	if err := os.MkdirAll(filepath.Join(exec.GetWorkspacePath(ws), ".pixi", "envs"), 0o755); err != nil {
		t.Fatal(err)
	}

	version := models.WorkspaceVersion{
		WorkspaceID:     ws.ID,
		ManifestContent: "[project]\nname = \"old\"\n",
		LockFileContent: "version: 6\n",
		PackageMetadata: "[]",
		CreatedBy:       ws.OwnerID,
		Description:     "old version",
	}
	if err := db.Create(&version).Error; err != nil {
		t.Fatalf("create version: %v", err)
	}

	job := &models.Job{
		WorkspaceID: ws.ID,
		Type:        models.JobTypeRollback,
		Status:      models.JobStatusPending,
		Metadata:    map[string]interface{}{"version_id": version.ID.String()},
	}
	if err := db.Create(job).Error; err != nil {
		t.Fatalf("create job: %v", err)
	}

	w := New(queue.NewMemoryQueue(10), exec, svc, jobSvc, slog.Default(), nil)
	if err := w.executeJob(context.Background(), job, &bytes.Buffer{}); err != nil {
		t.Fatalf("executeJob: %v", err)
	}

	restored, err := os.ReadFile(filepath.Join(exec.GetWorkspacePath(ws), "pixi.toml"))
	if err != nil {
		t.Fatalf("read restored pixi.toml: %v", err)
	}
	if string(restored) != version.ManifestContent {
		t.Errorf("pixi.toml not restored: got %q", restored)
	}
	if exec.solveCalls != 1 {
		t.Errorf("expected rollback to refresh lock via executor.SolveEnvironment, got %d solve calls", exec.solveCalls)
	}
	if exec.installCalls != 1 {
		t.Errorf("expected auto-install after rollback of installed workspace, got %d install calls", exec.installCalls)
	}
}

func setupWorkerTest(t *testing.T) (*gorm.DB, *service.WorkspaceService, *service.JobService, *fakeExecutor) {
	t.Helper()
	return setupWorkerTestMode(t, true)
}

func setupWorkerTestMode(t *testing.T, isLocal bool) (*gorm.DB, *service.WorkspaceService, *service.JobService, *fakeExecutor) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.User{},
		&models.Role{},
		&models.Workspace{},
		&models.Job{},
		&models.Permission{},
		&models.WorkspaceVersion{},
		&models.WorkspaceTag{},
		&models.AuditLog{},
		&models.Package{},
		&models.OCIRegistry{},
		&models.Publication{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := rbac.InitEnforcer(db, slog.Default()); err != nil {
		t.Fatalf("init rbac: %v", err)
	}

	fe := &fakeExecutor{rootDir: t.TempDir()}

	q := queue.NewMemoryQueue(10)
	t.Cleanup(func() { q.Close() })
	svc := service.New(db, q, fe, isLocal, nil, rbac.NewDefaultProvider())
	jobSvc := service.NewJobService(db, isLocal)
	return db, svc, jobSvc, fe
}
