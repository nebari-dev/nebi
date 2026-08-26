package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/audit"
	"github.com/nebari-dev/nebi/internal/limits"
	resourcemetrics "github.com/nebari-dev/nebi/internal/metrics"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/pkgmgr"
)

type staticListPackageManager struct {
	packages []pkgmgr.Package
}

func (m staticListPackageManager) Name() string { return "static-list" }
func (m staticListPackageManager) Init(context.Context, pkgmgr.InitOptions) error {
	return nil
}
func (m staticListPackageManager) Install(context.Context, pkgmgr.InstallOptions) error {
	return nil
}
func (m staticListPackageManager) Remove(context.Context, pkgmgr.RemoveOptions) error {
	return nil
}
func (m staticListPackageManager) List(context.Context, pkgmgr.ListOptions) ([]pkgmgr.Package, error) {
	return m.packages, nil
}
func (m staticListPackageManager) Update(context.Context, pkgmgr.UpdateOptions) error {
	return nil
}
func (m staticListPackageManager) GetManifest(context.Context, string) (*pkgmgr.Manifest, error) {
	return &pkgmgr.Manifest{}, nil
}

// --- InstallPackages tests ---

func TestInstallPackages_CreatesJob(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "install-test", userID)

	job, err := svc.InstallPackages(context.Background(), ws.ID.String(), []string{"numpy", "pandas"}, userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if job.Type != models.JobTypeInstall {
		t.Errorf("expected job type %q, got %q", models.JobTypeInstall, job.Type)
	}
	if job.Status != models.JobStatusPending {
		t.Errorf("expected job status %q, got %q", models.JobStatusPending, job.Status)
	}
	if job.WorkspaceID != ws.ID {
		t.Errorf("expected workspace ID %s, got %s", ws.ID, job.WorkspaceID)
	}

	// Verify packages stored in metadata
	pkgs, ok := job.Metadata["packages"].([]string)
	if !ok {
		t.Fatalf("expected packages in metadata, got %T", job.Metadata["packages"])
	}
	if len(pkgs) != 2 || pkgs[0] != "numpy" || pkgs[1] != "pandas" {
		t.Errorf("expected [numpy pandas], got %v", pkgs)
	}

	// Verify audit log written
	var auditCount int64
	db.Model(&models.AuditLog{}).Where("user_id = ? AND action = ?", userID, "install_package").Count(&auditCount)
	if auditCount != 1 {
		t.Errorf("expected 1 audit log, got %d", auditCount)
	}
}

func TestInstallPackages_RejectsNotReady(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")

	// Create workspace but don't mark ready (stays pending)
	ws, _ := svc.Create(context.Background(), CreateRequest{Name: "pending"}, userID)

	_, err := svc.InstallPackages(context.Background(), ws.ID.String(), []string{"numpy"}, userID)
	if err == nil {
		t.Fatal("expected error for non-ready workspace")
	}
	var ve *ValidationError
	if !isValidationError(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
}

func TestInstallPackages_NotFound(t *testing.T) {
	svc, _ := testSetup(t, true)

	_, err := svc.InstallPackages(context.Background(), uuid.New().String(), []string{"numpy"}, uuid.New())
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestInstallPackages_AllowsLargePackageRequest(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "many-pkgs", userID)

	packages := make([]string, 200)
	for i := range packages {
		packages[i] = fmt.Sprintf("pkg-%d", i)
	}

	if _, err := svc.InstallPackages(context.Background(), ws.ID.String(), packages, userID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var jobs int64
	db.Model(&models.Job{}).Where("workspace_id = ? AND type = ?", ws.ID, models.JobTypeInstall).Count(&jobs)
	if jobs != 1 {
		t.Fatalf("expected 1 install job, got %d", jobs)
	}
}

func TestInstallPackages_RejectsOversizedWorkspaceManifestBeforeJobWrite(t *testing.T) {
	svc, db := testSetup(t, true)
	limitCfg := limits.Defaults()
	limitCfg.ManifestBytes = 8
	svc.limits = limitCfg
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "install-manifest-limit", userID)
	if err := writeWorkspaceFiles(t, svc, ws, strings.Repeat("x", 9), "version: 6\n"); err != nil {
		t.Fatal(err)
	}

	_, err := svc.InstallPackages(context.Background(), ws.ID.String(), []string{"numpy"}, userID)

	var ve *ValidationError
	if !isValidationError(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
	var jobs int64
	db.Model(&models.Job{}).Where("workspace_id = ? AND type = ?", ws.ID, models.JobTypeInstall).Count(&jobs)
	if jobs != 0 {
		t.Fatalf("expected no install job writes, got %d", jobs)
	}
}

func TestInstallPackages_RejectsPerUserQuotaBeforeJobWrite(t *testing.T) {
	svc, db := testSetup(t, true)
	limitCfg := limits.Defaults()
	limitCfg.ActiveJobsPerUser = 1
	svc.limits = limitCfg
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "user-quota", userID)

	_, err := svc.InstallPackages(context.Background(), ws.ID.String(), []string{"numpy"}, userID)

	var ce *ConflictError
	if !isConflictError(err, &ce) {
		t.Fatalf("expected ConflictError, got %T: %v", err, err)
	}

	var jobs int64
	db.Model(&models.Job{}).Where("workspace_id = ? AND type = ?", ws.ID, models.JobTypeInstall).Count(&jobs)
	if jobs != 0 {
		t.Fatalf("expected no install job writes, got %d", jobs)
	}

	snapshot, err := resourcemetrics.Snapshot(db)
	if err != nil {
		t.Fatalf("metrics snapshot: %v", err)
	}
	if snapshot.QuotaRejections.User != 1 {
		t.Fatalf("expected durable user quota rejection metric, got %+v", snapshot.QuotaRejections)
	}
}

func TestInstallPackages_RejectsPerWorkspaceQuotaBeforeJobWrite(t *testing.T) {
	svc, db := testSetup(t, true)
	limitCfg := limits.Defaults()
	limitCfg.ActiveJobsPerUser = 10
	limitCfg.ActiveJobsPerWorkspace = 1
	limitCfg.ActiveJobsGlobal = 10
	svc.limits = limitCfg
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "workspace-quota", userID)
	if err := db.Model(&models.Job{}).Where("workspace_id = ?", ws.ID).Update("status", models.JobStatusCompleted).Error; err != nil {
		t.Fatalf("complete setup job: %v", err)
	}
	if err := db.Create(&models.Job{
		WorkspaceID: ws.ID,
		UserID:      userID,
		Type:        models.JobTypeUpdate,
		Status:      models.JobStatusPending,
	}).Error; err != nil {
		t.Fatalf("create active job: %v", err)
	}

	_, err := svc.InstallPackages(context.Background(), ws.ID.String(), []string{"numpy"}, userID)

	var ce *ConflictError
	if !isConflictError(err, &ce) {
		t.Fatalf("expected ConflictError, got %T: %v", err, err)
	}

	var installJobs int64
	db.Model(&models.Job{}).Where("workspace_id = ? AND type = ?", ws.ID, models.JobTypeInstall).Count(&installJobs)
	if installJobs != 0 {
		t.Fatalf("expected no install job writes, got %d", installJobs)
	}

	snapshot, err := resourcemetrics.Snapshot(db)
	if err != nil {
		t.Fatalf("metrics snapshot: %v", err)
	}
	if snapshot.QuotaRejections.Workspace != 1 {
		t.Fatalf("expected durable workspace quota rejection metric, got %+v", snapshot.QuotaRejections)
	}
}

func TestInstallPackages_RejectsGlobalQuotaBeforeJobWrite(t *testing.T) {
	svc, db := testSetup(t, true)
	limitCfg := limits.Defaults()
	limitCfg.ActiveJobsPerUser = 10
	limitCfg.ActiveJobsPerWorkspace = 10
	limitCfg.ActiveJobsGlobal = 1
	svc.limits = limitCfg
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "global-quota", userID)
	if err := db.Model(&models.Job{}).Where("workspace_id = ?", ws.ID).Update("status", models.JobStatusCompleted).Error; err != nil {
		t.Fatalf("complete setup job: %v", err)
	}
	if err := db.Create(&models.Job{
		WorkspaceID: ws.ID,
		UserID:      userID,
		Type:        models.JobTypeUpdate,
		Status:      models.JobStatusPending,
	}).Error; err != nil {
		t.Fatalf("create active job: %v", err)
	}

	_, err := svc.InstallPackages(context.Background(), ws.ID.String(), []string{"numpy"}, userID)

	var ce *ConflictError
	if !isConflictError(err, &ce) {
		t.Fatalf("expected ConflictError, got %T: %v", err, err)
	}

	var installJobs int64
	db.Model(&models.Job{}).Where("workspace_id = ? AND type = ?", ws.ID, models.JobTypeInstall).Count(&installJobs)
	if installJobs != 0 {
		t.Fatalf("expected no install job writes, got %d", installJobs)
	}

	snapshot, err := resourcemetrics.Snapshot(db)
	if err != nil {
		t.Fatalf("metrics snapshot: %v", err)
	}
	if snapshot.QuotaRejections.Global != 1 {
		t.Fatalf("expected durable global quota rejection metric, got %+v", snapshot.QuotaRejections)
	}
}

func TestInstallPackages_LegacyOwnerJobCountsAgainstPerUserQuota(t *testing.T) {
	svc, db := testSetup(t, true)
	limitCfg := limits.Defaults()
	limitCfg.ActiveJobsPerUser = 1
	svc.limits = limitCfg
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "legacy-user-quota", userID)

	if err := db.Model(&models.Job{}).Where("workspace_id = ?", ws.ID).Update("status", models.JobStatusCompleted).Error; err != nil {
		t.Fatalf("complete create jobs: %v", err)
	}
	legacyJob := &models.Job{
		WorkspaceID: ws.ID,
		Type:        models.JobTypeUpdate,
		Status:      models.JobStatusPending,
	}
	if err := db.Create(legacyJob).Error; err != nil {
		t.Fatalf("create legacy job: %v", err)
	}

	_, err := svc.InstallPackages(context.Background(), ws.ID.String(), []string{"numpy"}, userID)

	var ce *ConflictError
	if !isConflictError(err, &ce) {
		t.Fatalf("expected ConflictError, got %T: %v", err, err)
	}
}

func TestInstallPackages_ConcurrentPerUserQuotaBeforeJobWrite(t *testing.T) {
	svc, db := testSetup(t, true)
	limitCfg := limits.Defaults()
	limitCfg.ActiveJobsPerUser = 2
	limitCfg.ActiveJobsPerWorkspace = 10
	limitCfg.ActiveJobsGlobal = 10
	svc.limits = limitCfg
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "concurrent-user-quota", userID)
	if err := db.Model(&models.Job{}).Where("workspace_id = ?", ws.ID).Update("status", models.JobStatusCompleted).Error; err != nil {
		t.Fatalf("complete setup job: %v", err)
	}

	const attempts = 8
	start := make(chan struct{})
	errs := make(chan error, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, err := svc.InstallPackages(context.Background(), ws.ID.String(), []string{fmt.Sprintf("pkg-%d", i)}, userID)
			errs <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)

	var admitted, rejected int
	for err := range errs {
		var ce *ConflictError
		switch {
		case err == nil:
			admitted++
		case isConflictError(err, &ce):
			rejected++
		default:
			t.Fatalf("unexpected concurrent admission error: %T: %v", err, err)
		}
	}
	if admitted != limitCfg.ActiveJobsPerUser {
		t.Fatalf("expected %d admitted jobs, got %d", limitCfg.ActiveJobsPerUser, admitted)
	}
	if rejected != attempts-limitCfg.ActiveJobsPerUser {
		t.Fatalf("expected %d rejected jobs, got %d", attempts-limitCfg.ActiveJobsPerUser, rejected)
	}

	var activeInstallJobs int64
	db.Model(&models.Job{}).
		Where("workspace_id = ? AND type = ? AND status IN ?", ws.ID, models.JobTypeInstall, activeJobStatuses).
		Count(&activeInstallJobs)
	if activeInstallJobs != int64(limitCfg.ActiveJobsPerUser) {
		t.Fatalf("expected %d active install jobs, got %d", limitCfg.ActiveJobsPerUser, activeInstallJobs)
	}
}

func TestInstallPackages_RejectsOversizedMetadataBeforeJobWrite(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "metadata-limit", userID)
	if err := db.Model(&models.Job{}).Where("workspace_id = ?", ws.ID).Update("status", models.JobStatusCompleted).Error; err != nil {
		t.Fatalf("complete setup job: %v", err)
	}
	limitCfg := limits.Defaults()
	limitCfg.MetadataBytes = 8
	svc.limits = limitCfg

	_, err := svc.InstallPackages(context.Background(), ws.ID.String(), []string{"numpy"}, userID)

	var ve *ValidationError
	if !isValidationError(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}

	var installJobs int64
	db.Model(&models.Job{}).Where("workspace_id = ? AND type = ?", ws.ID, models.JobTypeInstall).Count(&installJobs)
	if installJobs != 0 {
		t.Fatalf("expected no install job writes, got %d", installJobs)
	}
	var auditCount int64
	db.Model(&models.AuditLog{}).Where("user_id = ? AND action = ?", userID, audit.ActionInstallPackage).Count(&auditCount)
	if auditCount != 0 {
		t.Fatalf("expected no install audit writes, got %d", auditCount)
	}
}

func TestSolveWorkspace_RejectsOversizedWorkspaceManifestBeforeJobWrite(t *testing.T) {
	svc, db := testSetup(t, true)
	limitCfg := limits.Defaults()
	limitCfg.ManifestBytes = 8
	svc.limits = limitCfg
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "solve-manifest-limit", userID)
	if err := writeWorkspaceFiles(t, svc, ws, strings.Repeat("x", 9), "version: 6\n"); err != nil {
		t.Fatal(err)
	}

	_, err := svc.SolveWorkspace(context.Background(), ws.ID.String(), userID)

	var ve *ValidationError
	if !isValidationError(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
	var updateJobs int64
	db.Model(&models.Job{}).Where("workspace_id = ? AND type = ?", ws.ID, models.JobTypeUpdate).Count(&updateJobs)
	if updateJobs != 0 {
		t.Fatalf("expected no solve job writes, got %d", updateJobs)
	}
}

// --- RemovePackage tests ---

func TestRemovePackage_CreatesJob(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "remove-test", userID)

	job, err := svc.RemovePackage(context.Background(), ws.ID.String(), "numpy", userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if job.Type != models.JobTypeRemove {
		t.Errorf("expected job type %q, got %q", models.JobTypeRemove, job.Type)
	}

	pkgs, ok := job.Metadata["packages"].([]string)
	if !ok {
		t.Fatalf("expected packages in metadata, got %T", job.Metadata["packages"])
	}
	if len(pkgs) != 1 || pkgs[0] != "numpy" {
		t.Errorf("expected [numpy], got %v", pkgs)
	}
}

func TestRemovePackage_RejectsNotReady(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws, _ := svc.Create(context.Background(), CreateRequest{Name: "pending"}, userID)

	_, err := svc.RemovePackage(context.Background(), ws.ID.String(), "numpy", userID)
	var ve *ValidationError
	if !isValidationError(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
}

func writeWorkspaceFiles(t *testing.T, svc *WorkspaceService, ws *models.Workspace, manifest string, lock string) error {
	t.Helper()
	wsPath := svc.executor.GetWorkspacePath(ws)
	if err := os.MkdirAll(wsPath, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(wsPath, "pixi.toml"), []byte(manifest), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(wsPath, "pixi.lock"), []byte(lock), 0o644)
}

// --- ListPackages tests ---

func TestListPackages_Empty(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "empty-pkgs", userID)

	pkgs, err := svc.ListPackages(ws.ID.String())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pkgs) != 0 {
		t.Errorf("expected 0 packages, got %d", len(pkgs))
	}
}

func TestListPackages_ReturnsInserted(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "with-pkgs", userID)

	// Simulate worker having saved packages
	db.Create(&models.Package{WorkspaceID: ws.ID, Name: "numpy", Version: "1.24.0"})
	db.Create(&models.Package{WorkspaceID: ws.ID, Name: "pandas", Version: "2.0.0"})

	pkgs, err := svc.ListPackages(ws.ID.String())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(pkgs))
	}

	names := map[string]bool{}
	for _, p := range pkgs {
		names[p.Name] = true
	}
	if !names["numpy"] || !names["pandas"] {
		t.Errorf("expected numpy and pandas, got %v", names)
	}
}

func TestListPackages_NotFound(t *testing.T) {
	svc, _ := testSetup(t, true)

	_, err := svc.ListPackages(uuid.New().String())
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestSyncPackagesFromWorkspace_SavesAllResolvedPackages(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "listed-package-limit", userID)

	pmType := "static-list-" + uuid.NewString()
	pkgmgr.Register(pmType, func(context.Context, string) (pkgmgr.PackageManager, error) {
		return staticListPackageManager{
			packages: []pkgmgr.Package{
				{Name: "numpy", Version: "1.0.0"},
				{Name: "pandas", Version: "2.0.0"},
			},
		}, nil
	})
	ws.PackageManager = pmType
	if err := db.Save(ws).Error; err != nil {
		t.Fatalf("save workspace package manager: %v", err)
	}

	if err := svc.SyncPackagesFromWorkspace(context.Background(), ws); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var packages []models.Package
	if err := db.Where("workspace_id = ?", ws.ID).Find(&packages).Error; err != nil {
		t.Fatalf("list packages: %v", err)
	}
	if len(packages) != 2 {
		t.Fatalf("expected both resolved packages saved, got %#v", packages)
	}
}

// --- SaveInstalledPackages / DeletePackagesByName / DeleteAllPackages tests ---

func TestSaveInstalledPackages(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "save-pkgs", userID)

	svc.SaveInstalledPackages(ws.ID, []string{"scipy", "matplotlib"})

	var count int64
	db.Model(&models.Package{}).Where("workspace_id = ?", ws.ID).Count(&count)
	if count != 2 {
		t.Errorf("expected 2 packages saved, got %d", count)
	}
}

func TestDeletePackagesByName(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "del-pkgs", userID)

	db.Create(&models.Package{WorkspaceID: ws.ID, Name: "numpy"})
	db.Create(&models.Package{WorkspaceID: ws.ID, Name: "pandas"})
	db.Create(&models.Package{WorkspaceID: ws.ID, Name: "scipy"})

	svc.DeletePackagesByName(ws.ID, []string{"numpy", "pandas"})

	var remaining []models.Package
	db.Where("workspace_id = ?", ws.ID).Find(&remaining)
	if len(remaining) != 1 || remaining[0].Name != "scipy" {
		t.Errorf("expected only scipy remaining, got %v", remaining)
	}
}

func TestDeleteAllPackages(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "del-all", userID)

	db.Create(&models.Package{WorkspaceID: ws.ID, Name: "numpy"})
	db.Create(&models.Package{WorkspaceID: ws.ID, Name: "pandas"})

	svc.DeleteAllPackages(ws.ID)

	var count int64
	db.Model(&models.Package{}).Where("workspace_id = ?", ws.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 packages after delete all, got %d", count)
	}
}

// Regression test for https://github.com/nebari-dev/nebi/issues/497: a failed
// solve must not leave the workspace terminally stuck. A workspace in the
// "failed" state must accept a new solve job so a corrected spec can recover it.
func TestSolveWorkspace_AllowedOnFailedWorkspace(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "solve-after-failure", userID)
	if err := writeWorkspaceFiles(t, svc, ws, "[project]\nname = \"solve-after-failure\"\n", "version: 6\n"); err != nil {
		t.Fatal(err)
	}
	db.Model(ws).Update("status", models.WsStatusFailed)

	job, err := svc.SolveWorkspace(context.Background(), ws.ID.String(), userID)
	if err != nil {
		t.Fatalf("solve on failed workspace should be allowed, got %T: %v", err, err)
	}
	if job.Type != models.JobTypeUpdate {
		t.Errorf("expected job type %q, got %q", models.JobTypeUpdate, job.Type)
	}
	if job.Status != models.JobStatusPending {
		t.Errorf("expected job status %q, got %q", models.JobStatusPending, job.Status)
	}
}
