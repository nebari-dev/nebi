package worker

import (
	"bytes"
	"context"
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

func (noopPackageManager) Name() string                                                   { return testPackageManager }
func (noopPackageManager) Init(context.Context, pkgmgr.InitOptions) error                 { return nil }
func (noopPackageManager) Install(context.Context, pkgmgr.InstallOptions) error           { return nil }
func (noopPackageManager) Remove(context.Context, pkgmgr.RemoveOptions) error             { return nil }
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
	rootDir string
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

func setupWorkerTest(t *testing.T) (*gorm.DB, *service.WorkspaceService, *service.JobService, *fakeExecutor) {
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
	svc := service.New(db, q, fe, true, nil, rbac.NewDefaultProvider())
	jobSvc := service.NewJobService(db)
	return db, svc, jobSvc, fe
}
