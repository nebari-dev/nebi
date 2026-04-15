package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
)

// --- RollbackToVersion tests ---

func TestRollbackToVersion_CreatesJob(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "rollback-test", userID)

	// Push a version first so there's something to roll back to
	svc.PushVersion(context.Background(), ws.ID.String(), PushRequest{
		PixiToml: "[project]\nname = \"test\"",
		PixiLock: "version: 6",
	}, userID)

	job, err := svc.RollbackToVersion(context.Background(), ws.ID.String(), 1, userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if job.Type != models.JobTypeRollback {
		t.Errorf("expected job type %q, got %q", models.JobTypeRollback, job.Type)
	}
	if job.Status != models.JobStatusPending {
		t.Errorf("expected job status %q, got %q", models.JobStatusPending, job.Status)
	}

	// Verify version_id in metadata
	versionIDStr, ok := job.Metadata["version_id"].(string)
	if !ok || versionIDStr == "" {
		t.Error("expected version_id in job metadata")
	}

	// Verify audit log
	var auditCount int64
	db.Model(&models.AuditLog{}).Where("user_id = ? AND action = ?", userID, "rollback_workspace").Count(&auditCount)
	if auditCount != 1 {
		t.Errorf("expected 1 audit log, got %d", auditCount)
	}
}

func TestRollbackToVersion_RejectsNotReady(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws, _ := svc.Create(context.Background(), CreateRequest{Name: "pending"}, userID)

	_, err := svc.RollbackToVersion(context.Background(), ws.ID.String(), 1, userID)
	var ve *ValidationError
	if !isValidationError(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
}

func TestRollbackToVersion_VersionNotFound(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "rollback-test", userID)

	_, err := svc.RollbackToVersion(context.Background(), ws.ID.String(), 999, userID)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestRollbackToVersion_WorkspaceNotFound(t *testing.T) {
	svc, _ := testSetup(t, true)

	_, err := svc.RollbackToVersion(context.Background(), uuid.New().String(), 1, uuid.New())
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// --- SetWorkspaceStatus tests ---

func TestSetWorkspaceStatus(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws, _ := svc.Create(context.Background(), CreateRequest{Name: "status-test"}, userID)

	if err := svc.SetWorkspaceStatus(ws.ID, models.WsStatusReady); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify in DB
	var updated models.Workspace
	db.First(&updated, ws.ID)
	if updated.Status != models.WsStatusReady {
		t.Errorf("expected status %q, got %q", models.WsStatusReady, updated.Status)
	}
}

// --- SetWorkspacePath tests ---

func TestSetWorkspacePath(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws, _ := svc.Create(context.Background(), CreateRequest{Name: "path-test"}, userID)

	if err := svc.SetWorkspacePath(ws.ID, "/new/path"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updated models.Workspace
	db.First(&updated, ws.ID)
	if updated.Path != "/new/path" {
		t.Errorf("expected path %q, got %q", "/new/path", updated.Path)
	}
}

// --- SoftDeleteWorkspace tests ---

func TestSoftDeleteWorkspace(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws, _ := svc.Create(context.Background(), CreateRequest{Name: "delete-test"}, userID)

	if err := svc.SoftDeleteWorkspace(ws.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should not be found with default (non-unscoped) query
	var count int64
	db.Model(&models.Workspace{}).Where("id = ?", ws.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected workspace to be soft-deleted, still found %d", count)
	}

	// Should still exist in unscoped query
	db.Unscoped().Model(&models.Workspace{}).Where("id = ?", ws.ID).Count(&count)
	if count != 1 {
		t.Errorf("expected workspace in unscoped query, got %d", count)
	}
}

// --- CreateVersionSnapshot tests ---

func TestCreateVersionSnapshot(t *testing.T) {
	// This test requires a working pixi binary because CreateVersionSnapshot
	// calls pkgmgr.List to capture package metadata in the snapshot.
	if _, err := exec.LookPath("pixi"); err != nil {
		t.Skip("pixi not in PATH, skipping snapshot test")
	}

	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "snapshot-test", userID)

	// Create workspace directory with valid pixi files
	wsPath := svc.executor.GetWorkspacePath(ws)
	os.MkdirAll(wsPath, 0755)
	manifest := "[project]\nname = \"test\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	lock := "version: 6\npackages: []\n"
	os.WriteFile(filepath.Join(wsPath, "pixi.toml"), []byte(manifest), 0644)
	os.WriteFile(filepath.Join(wsPath, "pixi.lock"), []byte(lock), 0644)

	// Run pixi install first so pixi list works
	cmd := exec.Command("pixi", "install")
	cmd.Dir = wsPath
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("pixi install failed (env may not support this platform): %s", out)
	}

	jobID := uuid.New()
	err := svc.CreateVersionSnapshot(context.Background(), ws, jobID, userID, "Test snapshot")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify version created in DB
	var versions []models.WorkspaceVersion
	db.Where("workspace_id = ?", ws.ID).Find(&versions)
	if len(versions) != 1 {
		t.Fatalf("expected 1 version, got %d", len(versions))
	}

	v := versions[0]
	if v.ManifestContent != manifest {
		t.Errorf("unexpected manifest content: %q", v.ManifestContent)
	}
	if v.Description != "Test snapshot" {
		t.Errorf("expected description %q, got %q", "Test snapshot", v.Description)
	}
	if v.CreatedBy != userID {
		t.Errorf("expected created_by %s, got %s", userID, v.CreatedBy)
	}
	if v.JobID == nil || *v.JobID != jobID {
		t.Errorf("expected job_id %s, got %v", jobID, v.JobID)
	}
}

func TestCreateVersionSnapshot_MissingPixiToml(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "no-files", userID)

	// Workspace dir exists but no pixi files
	wsPath := svc.executor.GetWorkspacePath(ws)
	os.MkdirAll(wsPath, 0755)

	err := svc.CreateVersionSnapshot(context.Background(), ws, uuid.New(), userID, "Should fail")
	if err == nil {
		t.Fatal("expected error for missing pixi.toml")
	}
}
