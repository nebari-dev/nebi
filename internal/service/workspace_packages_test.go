package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
)

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
