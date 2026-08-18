package worker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/limits"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/pkgmgr"
	"github.com/nebari-dev/nebi/internal/process"
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
const testListResourceLimitPackageManager = "test-list-resource-limit"

func init() {
	pkgmgr.Register(testPackageManager, func(context.Context, string) (pkgmgr.PackageManager, error) {
		return noopPackageManager{}, nil
	})
	pkgmgr.RegisterManifestContentParser(testPackageManager, pkgmgr.ManifestContentParser{
		PackageNames:           emptyManifestPackageNames,
		DefaultDependencyNames: emptyManifestPackageNames,
	})
	pkgmgr.Register(testListResourceLimitPackageManager, func(context.Context, string) (pkgmgr.PackageManager, error) {
		return listResourceLimitPackageManager{}, nil
	})
	pkgmgr.RegisterManifestContentParser(testListResourceLimitPackageManager, pkgmgr.ManifestContentParser{
		PackageNames:           emptyManifestPackageNames,
		DefaultDependencyNames: emptyManifestPackageNames,
	})
}

func emptyManifestPackageNames(string) ([]string, error) {
	return nil, nil
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

type listResourceLimitPackageManager struct {
	noopPackageManager
}

func (listResourceLimitPackageManager) Name() string { return testListResourceLimitPackageManager }
func (listResourceLimitPackageManager) List(context.Context, pkgmgr.ListOptions) ([]pkgmgr.Package, error) {
	return nil, process.NewResourceLimitError(errors.New("pixi list failed: exit status 125"))
}

// fakeExecutor is a minimal Executor stub for worker tests. CreateWorkspace
// creates the workspace directory and writes empty manifest/lock files so
// CreateVersionSnapshot reads them successfully; the rest are no-ops.
type fakeExecutor struct {
	rootDir                           string
	solveErr                          error
	installErr                        error
	blockInstall                      bool
	succeedEnvInstallAfterContextDone bool
	createManifest                    string
	createLock                        string
	installLog                        string
	installLock                       string
	installCalls                      int
	uninstallCalls                    int
	solveCalls                        int
	cleanupCalls                      int
	cleanupJobTypes                   []models.JobType
}

func (e *fakeExecutor) CreateWorkspace(ctx context.Context, ws *models.Workspace, w io.Writer, opts executor.CreateWorkspaceOptions) error {
	p := e.GetWorkspacePath(ws)
	if err := os.MkdirAll(p, 0o755); err != nil {
		return err
	}
	manifest := e.createManifest
	lock := e.createLock
	for name, content := range map[string]string{"pixi.toml": manifest, "pixi.lock": lock} {
		if err := os.WriteFile(filepath.Join(p, name), []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}
func (e *fakeExecutor) InstallPackages(ctx context.Context, ws *models.Workspace, _ []string, w io.Writer) error {
	if e.blockInstall {
		<-ctx.Done()
		return ctx.Err()
	}
	if e.installLock != "" {
		if err := os.WriteFile(filepath.Join(e.GetWorkspacePath(ws), "pixi.lock"), []byte(e.installLock), 0o644); err != nil {
			return err
		}
	}
	if e.installLog != "" {
		if _, err := io.WriteString(w, e.installLog); err != nil {
			return err
		}
	}
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
	if e.succeedEnvInstallAfterContextDone {
		<-ctx.Done()
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
func (e *fakeExecutor) CleanupJobArtifacts(ctx context.Context, ws *models.Workspace, jobType models.JobType, w io.Writer) error {
	e.cleanupCalls++
	e.cleanupJobTypes = append(e.cleanupJobTypes, jobType)
	paths := []string{filepath.Join(e.GetWorkspacePath(ws), ".nebi", "pixi-cache")}
	if jobType == models.JobTypeEnvInstall {
		paths = append(paths, filepath.Join(e.GetWorkspacePath(ws), ".pixi", "envs"))
	}
	for _, path := range paths {
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	return nil
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

	w := New(queue.NewMemoryQueue(10), exec, svc, jobSvc, slog.Default(), nil, limits.Defaults())

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

	w := New(queue.NewMemoryQueue(10), exec, svc, jobSvc, slog.Default(), nil, limits.Defaults())

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

	w := New(queue.NewMemoryQueue(10), exec, svc, jobSvc, slog.Default(), nil, limits.Defaults())

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

func TestProcessJob_FailsJobAfterDeadline(t *testing.T) {
	db, svc, jobSvc, exec := setupWorkerTest(t)
	exec.blockInstall = true

	_, job := newTestWorkspace(t, db, exec, "timeout-install", models.JobTypeInstall,
		map[string]interface{}{"packages": []string{"numpy"}})

	w := New(queue.NewMemoryQueue(10), exec, svc, jobSvc, slog.Default(), nil, limits.Defaults())
	w.jobTimeout = 10 * time.Millisecond
	w.processJob(context.Background(), job)

	var stored models.Job
	if err := db.First(&stored, "id = ?", job.ID).Error; err != nil {
		t.Fatalf("reload job: %v", err)
	}
	if stored.Status != models.JobStatusFailed {
		t.Fatalf("expected failed job after timeout, got %q", stored.Status)
	}
	if !strings.Contains(stored.Error, "wall-clock timeout") {
		t.Fatalf("expected timeout error, got %q", stored.Error)
	}

	adminSvc := service.NewAdminService(db, rbac.NewDefaultProvider(), limits.Defaults())
	metrics, err := adminSvc.GetResourceMetrics()
	if err != nil {
		t.Fatalf("GetResourceMetrics: %v", err)
	}
	if metrics.JobTimeoutsTotal != 1 {
		t.Fatalf("expected durable timeout metric, got %d", metrics.JobTimeoutsTotal)
	}
	if exec.cleanupCalls != 1 {
		t.Fatalf("expected timeout cleanup to run once, got %d", exec.cleanupCalls)
	}
}

func TestProcessJob_CompletesWhenExecutionSucceedsAtDeadlineBoundary(t *testing.T) {
	db, svc, jobSvc, exec := setupWorkerTest(t)
	exec.succeedEnvInstallAfterContextDone = true

	_, job := newTestWorkspace(t, db, exec, "deadline-success", models.JobTypeEnvInstall, nil)

	w := New(queue.NewMemoryQueue(10), exec, svc, jobSvc, slog.Default(), nil, limits.Defaults())
	w.jobTimeout = 10 * time.Millisecond
	w.processJob(context.Background(), job)

	var stored models.Job
	if err := db.First(&stored, "id = ?", job.ID).Error; err != nil {
		t.Fatalf("reload job: %v", err)
	}
	if stored.Status != models.JobStatusCompleted {
		t.Fatalf("expected completed job when execution returned nil, got %q with error %q", stored.Status, stored.Error)
	}
	if exec.cleanupCalls != 0 {
		t.Fatalf("expected no cleanup after successful execution, got %d", exec.cleanupCalls)
	}

	adminSvc := service.NewAdminService(db, rbac.NewDefaultProvider(), limits.Defaults())
	metrics, err := adminSvc.GetResourceMetrics()
	if err != nil {
		t.Fatalf("GetResourceMetrics: %v", err)
	}
	if metrics.JobTimeoutsTotal != 0 {
		t.Fatalf("expected no durable timeout metric after successful execution, got %d", metrics.JobTimeoutsTotal)
	}
}

func TestProcessJob_FailsCreateWhenSnapshotExceedsResourceLimit(t *testing.T) {
	limitCfg := limits.Defaults()
	limitCfg.LockBytes = 8
	db, svc, jobSvc, exec := setupWorkerTestWithLimits(t, limitCfg)
	exec.createLock = strings.Repeat("x", 9)

	_, job := newTestWorkspace(t, db, exec, "snapshot-lock-limit", models.JobTypeCreate, nil)

	w := New(queue.NewMemoryQueue(10), exec, svc, jobSvc, slog.Default(), nil, limitCfg)
	w.processJob(context.Background(), job)

	var stored models.Job
	if err := db.First(&stored, "id = ?", job.ID).Error; err != nil {
		t.Fatalf("reload job: %v", err)
	}
	if stored.Status != models.JobStatusFailed {
		t.Fatalf("expected failed job after snapshot limit, got %q", stored.Status)
	}
	if !strings.Contains(stored.Error, "pixi.lock exceeds 8 bytes") {
		t.Fatalf("expected lock limit error, got %q", stored.Error)
	}
	if exec.cleanupCalls != 1 {
		t.Fatalf("expected cleanup after snapshot limit failure, got %d", exec.cleanupCalls)
	}

	var ws models.Workspace
	if err := db.First(&ws, "id = ?", job.WorkspaceID).Error; err != nil {
		t.Fatalf("reload workspace: %v", err)
	}
	if ws.Status != models.WsStatusFailed {
		t.Fatalf("expected workspace status failed, got %q", ws.Status)
	}
}

func TestProcessJob_DoesNotSavePackagesWhenInstallSnapshotExceedsLimit(t *testing.T) {
	limitCfg := limits.Defaults()
	limitCfg.LockBytes = 8
	db, svc, jobSvc, exec := setupWorkerTestWithLimits(t, limitCfg)
	exec.installLock = strings.Repeat("x", 9)

	ws, job := newTestWorkspace(t, db, exec, "install-snapshot-limit", models.JobTypeInstall,
		map[string]interface{}{"packages": []string{"numpy"}})

	w := New(queue.NewMemoryQueue(10), exec, svc, jobSvc, slog.Default(), nil, limitCfg)
	w.processJob(context.Background(), job)

	var stored models.Job
	if err := db.First(&stored, "id = ?", job.ID).Error; err != nil {
		t.Fatalf("reload job: %v", err)
	}
	if stored.Status != models.JobStatusFailed {
		t.Fatalf("expected failed job after snapshot limit, got %q", stored.Status)
	}

	var packageCount int64
	if err := db.Model(&models.Package{}).Where("workspace_id = ?", ws.ID).Count(&packageCount).Error; err != nil {
		t.Fatalf("count packages: %v", err)
	}
	if packageCount != 0 {
		t.Fatalf("expected no package rows after failed snapshot, got %d", packageCount)
	}
}

func TestProcessJob_FailsCreateAndCleansUpWhenPackageListHitsResourceLimit(t *testing.T) {
	db, svc, jobSvc, exec := setupWorkerTest(t)
	ws, job := newTestWorkspace(t, db, exec, "list-resource-limit", models.JobTypeCreate, nil)
	ws.PackageManager = testListResourceLimitPackageManager
	if err := db.Save(ws).Error; err != nil {
		t.Fatalf("save package manager: %v", err)
	}

	w := New(queue.NewMemoryQueue(10), exec, svc, jobSvc, slog.Default(), nil, limits.Defaults())
	w.processJob(context.Background(), job)

	var stored models.Job
	if err := db.First(&stored, "id = ?", job.ID).Error; err != nil {
		t.Fatalf("reload job: %v", err)
	}
	if stored.Status != models.JobStatusFailed {
		t.Fatalf("expected failed job after resource-limit list failure, got %q", stored.Status)
	}
	if !strings.Contains(stored.Error, "process resource limit exceeded") {
		t.Fatalf("expected resource-limit error, got %q", stored.Error)
	}
	if exec.cleanupCalls != 1 {
		t.Fatalf("expected cleanup after package-list resource failure, got %d", exec.cleanupCalls)
	}

	var versions int64
	if err := db.Model(&models.WorkspaceVersion{}).Where("workspace_id = ?", ws.ID).Count(&versions).Error; err != nil {
		t.Fatalf("count versions: %v", err)
	}
	if versions != 0 {
		t.Fatalf("expected no version snapshot after resource failure, got %d", versions)
	}
}

func TestProcessJob_CapsPersistedLogs(t *testing.T) {
	limitCfg := limits.Defaults()
	limitCfg.JobLogBytes = 96
	db, svc, jobSvc, exec := setupWorkerTestWithLimits(t, limitCfg)
	exec.installLog = strings.Repeat("x", 512)

	_, job := newTestWorkspace(t, db, exec, "log-limit", models.JobTypeInstall,
		map[string]interface{}{"packages": []string{"numpy"}})

	w := New(queue.NewMemoryQueue(10), exec, svc, jobSvc, slog.Default(), nil, limitCfg)
	w.processJob(context.Background(), job)

	var stored models.Job
	if err := db.First(&stored, "id = ?", job.ID).Error; err != nil {
		t.Fatalf("reload job: %v", err)
	}
	if len(stored.Logs) > limitCfg.JobLogBytes {
		t.Fatalf("expected logs capped at %d bytes, got %d", limitCfg.JobLogBytes, len(stored.Logs))
	}
	if !strings.Contains(stored.Logs, "[TRUNCATED]") {
		t.Fatalf("expected truncation notice in logs, got %q", stored.Logs)
	}
}

func TestCappedLogWriterReservesTailForNebiErrors(t *testing.T) {
	var buf bytes.Buffer
	w := newCappedLogWriter(&buf, 160)
	if _, err := w.Write([]byte(strings.Repeat("x", 512))); err != nil {
		t.Fatalf("write noisy log: %v", err)
	}
	if _, err := w.Write([]byte("\n[ERROR] Job failed: important diagnostic\n")); err != nil {
		t.Fatalf("write important log: %v", err)
	}

	got := buf.String()
	if len(got) > 160 {
		t.Fatalf("expected capped output, got %d bytes", len(got))
	}
	if !strings.Contains(got, "[TRUNCATED]") {
		t.Fatalf("expected truncation notice, got %q", got)
	}
	if !strings.Contains(got, "[ERROR] Job failed") {
		t.Fatalf("expected important error in reserved tail, got %q", got)
	}
	if _, err := w.Write([]byte("ordinary child output after truncation")); err != nil {
		t.Fatalf("write ordinary post-truncation log: %v", err)
	}
	if strings.Contains(buf.String(), "ordinary child output") {
		t.Fatalf("ordinary post-truncation output should still be dropped, got %q", buf.String())
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

	w := New(queue.NewMemoryQueue(10), exec, svc, jobSvc, slog.Default(), nil, limits.Defaults())
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

	w := New(queue.NewMemoryQueue(10), exec, svc, jobSvc, slog.Default(), nil, limits.Defaults())
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

	w := New(queue.NewMemoryQueue(10), exec, svc, jobSvc, slog.Default(), nil, limits.Defaults())
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

	w := New(queue.NewMemoryQueue(10), exec, svc, jobSvc, slog.Default(), nil, limits.Defaults())
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

	w := New(queue.NewMemoryQueue(10), exec, svc, jobSvc, slog.Default(), nil, limits.Defaults())
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

			w := New(queue.NewMemoryQueue(10), exec, svc, jobSvc, slog.Default(), nil, limits.Defaults())
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

	w := New(queue.NewMemoryQueue(10), exec, svc, jobSvc, slog.Default(), nil, limits.Defaults())
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

func TestProcessJob_RollbackRejectsOversizedLegacyVersionBeforeWrite(t *testing.T) {
	limitCfg := limits.Defaults()
	limitCfg.LockBytes = 8
	db, svc, jobSvc, exec := setupWorkerTestWithLimits(t, limitCfg)

	ws, _ := newTestWorkspace(t, db, exec, "rollback-limit", models.JobTypeCreate, nil)
	wsPath := exec.GetWorkspacePath(ws)
	originalManifest := "[project]\nname = \"current\"\n"
	originalLock := "version\n"
	if err := os.WriteFile(filepath.Join(wsPath, "pixi.toml"), []byte(originalManifest), 0o644); err != nil {
		t.Fatalf("write current pixi.toml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wsPath, "pixi.lock"), []byte(originalLock), 0o644); err != nil {
		t.Fatalf("write current pixi.lock: %v", err)
	}

	version := models.WorkspaceVersion{
		WorkspaceID:     ws.ID,
		ManifestContent: "[project]\nname = \"legacy\"\n",
		LockFileContent: strings.Repeat("x", 9),
		PackageMetadata: "[]",
		CreatedBy:       ws.OwnerID,
		Description:     "legacy oversized version",
	}
	if err := db.Create(&version).Error; err != nil {
		t.Fatalf("create legacy version: %v", err)
	}

	job := &models.Job{
		WorkspaceID: ws.ID,
		Type:        models.JobTypeRollback,
		Status:      models.JobStatusPending,
		Metadata:    map[string]interface{}{"version_id": version.ID.String()},
	}
	if err := db.Create(job).Error; err != nil {
		t.Fatalf("create rollback job: %v", err)
	}

	w := New(queue.NewMemoryQueue(10), exec, svc, jobSvc, slog.Default(), nil, limitCfg)
	w.processJob(context.Background(), job)

	var stored models.Job
	if err := db.First(&stored, "id = ?", job.ID).Error; err != nil {
		t.Fatalf("reload job: %v", err)
	}
	if stored.Status != models.JobStatusFailed {
		t.Fatalf("expected failed rollback job, got %q", stored.Status)
	}
	if !strings.Contains(stored.Error, "pixi.lock exceeds 8 bytes") {
		t.Fatalf("expected lock limit error, got %q", stored.Error)
	}

	manifest, err := os.ReadFile(filepath.Join(wsPath, "pixi.toml"))
	if err != nil {
		t.Fatalf("read pixi.toml: %v", err)
	}
	lock, err := os.ReadFile(filepath.Join(wsPath, "pixi.lock"))
	if err != nil {
		t.Fatalf("read pixi.lock: %v", err)
	}
	if string(manifest) != originalManifest || string(lock) != originalLock {
		t.Fatalf("expected rollback files unchanged, got manifest=%q lock=%q", string(manifest), string(lock))
	}
	if exec.solveCalls != 0 {
		t.Fatalf("expected no solve after rejected version, got %d", exec.solveCalls)
	}
}

// TestExecuteJob_UpdateReinstallFailureDoesNotFailJob proves a reinstall
// failure after a lockfile-changing job (the manifest, lockfile, and
// version snapshot are already committed by this point) does not mark
// the whole job as failed, and instead surfaces as install_status =
// install_failed on the workspace.
func TestExecuteJob_UpdateReinstallFailureDoesNotFailJob(t *testing.T) {
	db, svc, jobSvc, exec := setupWorkerTest(t)

	ws, job := newTestWorkspace(t, db, exec, "update-reinstall-fail", models.JobTypeUpdate, nil)
	if err := os.MkdirAll(filepath.Join(exec.GetWorkspacePath(ws), ".pixi", "envs"), 0o755); err != nil {
		t.Fatal(err)
	}
	exec.installErr = errors.New("pixi install failed: exit status 1")

	w := New(queue.NewMemoryQueue(10), exec, svc, jobSvc, slog.Default(), nil, limits.Defaults())
	if err := w.executeJob(context.Background(), job, &bytes.Buffer{}); err != nil {
		t.Fatalf("executeJob: expected reinstall failure not to fail the job, got %v", err)
	}

	resp, err := svc.Get(ws.ID.String())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if resp.InstallStatus != models.InstallStatusFailed {
		t.Errorf("expected install_status %q, got %q", models.InstallStatusFailed, resp.InstallStatus)
	}
}

func TestProcessJob_UpdateReinstallResourceFailureFailsAndCleansUp(t *testing.T) {
	db, svc, jobSvc, exec := setupWorkerTest(t)

	ws, job := newTestWorkspace(t, db, exec, "update-reinstall-resource-fail", models.JobTypeUpdate, nil)
	if err := os.MkdirAll(filepath.Join(exec.GetWorkspacePath(ws), ".pixi", "envs"), 0o755); err != nil {
		t.Fatal(err)
	}
	exec.installErr = process.NewResourceLimitError(errors.New("CPU budget exceeded"))

	w := New(queue.NewMemoryQueue(10), exec, svc, jobSvc, slog.Default(), nil, limits.Defaults())
	w.processJob(context.Background(), job)

	var stored models.Job
	if err := db.First(&stored, "id = ?", job.ID).Error; err != nil {
		t.Fatalf("reload job: %v", err)
	}
	if stored.Status != models.JobStatusFailed {
		t.Fatalf("expected failed job after reinstall resource failure, got %q", stored.Status)
	}
	if !strings.Contains(stored.Error, "process resource limit exceeded") {
		t.Fatalf("expected resource-limit error, got %q", stored.Error)
	}
	if exec.cleanupCalls != 1 {
		t.Fatalf("expected cleanup after reinstall resource failure, got %d", exec.cleanupCalls)
	}
	if len(exec.cleanupJobTypes) != 1 || exec.cleanupJobTypes[0] != models.JobTypeEnvInstall {
		t.Fatalf("expected env-install cleanup, got %v", exec.cleanupJobTypes)
	}
	if _, err := os.Stat(filepath.Join(exec.GetWorkspacePath(ws), ".pixi", "envs")); !os.IsNotExist(err) {
		t.Fatalf("expected failed reinstall cleanup to remove .pixi/envs, stat err=%v", err)
	}

	resp, err := svc.Get(ws.ID.String())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if resp.InstallStatus != models.InstallStatusFailed {
		t.Errorf("expected install_status %q, got %q", models.InstallStatusFailed, resp.InstallStatus)
	}
}

func setupWorkerTest(t *testing.T) (*gorm.DB, *service.WorkspaceService, *service.JobService, *fakeExecutor) {
	t.Helper()
	return setupWorkerTestMode(t, true)
}

func setupWorkerTestWithLimits(t *testing.T, limitCfg limits.Limits) (*gorm.DB, *service.WorkspaceService, *service.JobService, *fakeExecutor) {
	t.Helper()
	return setupWorkerTestModeWithLimits(t, true, limitCfg)
}

func setupWorkerTestMode(t *testing.T, isLocal bool) (*gorm.DB, *service.WorkspaceService, *service.JobService, *fakeExecutor) {
	t.Helper()
	return setupWorkerTestModeWithLimits(t, isLocal)
}

func setupWorkerTestModeWithLimits(t *testing.T, isLocal bool, limitOpts ...limits.Limits) (*gorm.DB, *service.WorkspaceService, *service.JobService, *fakeExecutor) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	dsn := dbPath + "?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}
	sqlDB.SetMaxOpenConns(30)
	sqlDB.SetMaxIdleConns(5)
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
		&models.ResourceLock{},
		&models.ResourceMetric{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := rbac.InitEnforcer(db, slog.Default()); err != nil {
		t.Fatalf("init rbac: %v", err)
	}

	fe := &fakeExecutor{rootDir: t.TempDir()}

	q := queue.NewMemoryQueue(10)
	t.Cleanup(func() { q.Close() })
	limitCfg := limits.Defaults()
	if len(limitOpts) > 0 {
		limitCfg = limitOpts[0]
	}
	svc := service.New(db, q, fe, isLocal, nil, rbac.NewDefaultProvider(), limitCfg)
	jobSvc := service.NewJobService(db, isLocal)
	return db, svc, jobSvc, fe
}
