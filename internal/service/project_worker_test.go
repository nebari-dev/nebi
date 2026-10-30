package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/audit"
	"github.com/nebari-dev/nebi/internal/limits"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/pixi"
)

// --- RollbackToVersion tests ---

func TestRollbackToVersion_CreatesJob(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	project := createReadyProject(t, svc, db, "rollback-test", userID)

	// Push a version first so there's something to roll back to
	svc.PushVersion(context.Background(), project.ID.String(), PushRequest{
		PixiToml: "[project]\nname = \"test\"",
		PixiLock: "version: 6",
	}, userID)

	job, err := svc.RollbackToVersion(context.Background(), project.ID.String(), 1, userID)
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
	db.Model(&models.AuditLog{}).Where("user_id = ? AND action = ?", userID, "rollback_project").Count(&auditCount)
	if auditCount != 1 {
		t.Errorf("expected 1 audit log, got %d", auditCount)
	}
}

func TestRollbackToVersion_RejectsNotReady(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	project, _ := svc.Create(context.Background(), CreateRequest{Name: "pending"}, userID)

	_, err := svc.RollbackToVersion(context.Background(), project.ID.String(), 1, userID)
	var ve *ValidationError
	if !isValidationError(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
}

func TestRollbackToVersion_VersionNotFound(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	project := createReadyProject(t, svc, db, "rollback-test", userID)

	_, err := svc.RollbackToVersion(context.Background(), project.ID.String(), 999, userID)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestRollbackToVersion_RejectsOversizedLegacyVersionBeforeJobWrite(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	project := createReadyProject(t, svc, db, "rollback-limit", userID)
	limitCfg := limits.Defaults()
	limitCfg.LockBytes = 8
	svc.limits = limitCfg

	version := models.ProjectVersion{
		ProjectID:       project.ID,
		ManifestContent: "[project]\nname = \"legacy\"\n",
		LockFileContent: strings.Repeat("x", 9),
		PackageMetadata: "[]",
		CreatedBy:       userID,
		Description:     "legacy oversized version",
	}
	if err := db.Create(&version).Error; err != nil {
		t.Fatalf("create legacy version: %v", err)
	}

	_, err := svc.RollbackToVersion(context.Background(), project.ID.String(), version.VersionNumber, userID)

	var ve *ValidationError
	if !isValidationError(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
	var rollbackJobs int64
	if err := db.Model(&models.Job{}).Where("project_id = ? AND type = ?", project.ID, models.JobTypeRollback).Count(&rollbackJobs).Error; err != nil {
		t.Fatalf("count rollback jobs: %v", err)
	}
	if rollbackJobs != 0 {
		t.Fatalf("expected no rollback job write, got %d", rollbackJobs)
	}
	var rollbackAudits int64
	if err := db.Model(&models.AuditLog{}).Where("user_id = ? AND action = ?", userID, audit.ActionRollbackProject).Count(&rollbackAudits).Error; err != nil {
		t.Fatalf("count rollback audits: %v", err)
	}
	if rollbackAudits != 0 {
		t.Fatalf("expected no rollback audit write, got %d", rollbackAudits)
	}
}

func TestRollbackToVersion_ProjectNotFound(t *testing.T) {
	svc, _ := testSetup(t, true)

	_, err := svc.RollbackToVersion(context.Background(), uuid.New().String(), 1, uuid.New())
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// --- SetProjectStatus tests ---

func TestSetProjectStatus(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	project, _ := svc.Create(context.Background(), CreateRequest{Name: "status-test"}, userID)

	if err := svc.SetProjectStatus(project.ID, models.ProjectStatusReady); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify in DB
	var updated models.Project
	db.First(&updated, project.ID)
	if updated.Status != models.ProjectStatusReady {
		t.Errorf("expected status %q, got %q", models.ProjectStatusReady, updated.Status)
	}
}

// --- SetProjectPath tests ---

func TestSetProjectPath(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	project, _ := svc.Create(context.Background(), CreateRequest{Name: "path-test"}, userID)

	if err := svc.SetProjectPath(project.ID, "/new/path"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var updated models.Project
	db.First(&updated, project.ID)
	if updated.Path != "/new/path" {
		t.Errorf("expected path %q, got %q", "/new/path", updated.Path)
	}
}

// --- SoftDeleteProject tests ---

func TestSoftDeleteProject(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	project, _ := svc.Create(context.Background(), CreateRequest{Name: "delete-test"}, userID)

	if err := svc.SoftDeleteProject(project.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should not be found with default (non-unscoped) query
	var count int64
	db.Model(&models.Project{}).Where("id = ?", project.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected project to be soft-deleted, still found %d", count)
	}

	// Should still exist in unscoped query
	db.Unscoped().Model(&models.Project{}).Where("id = ?", project.ID).Count(&count)
	if count != 1 {
		t.Errorf("expected project in unscoped query, got %d", count)
	}
}

// --- CreateVersionSnapshot tests ---

func TestCreateVersionSnapshot(t *testing.T) {
	// This test requires a working pixi binary because CreateVersionSnapshot
	// runs pixi list to capture package metadata in the snapshot.
	if _, err := exec.LookPath("pixi"); err != nil {
		t.Skip("pixi not in PATH, skipping snapshot test")
	}

	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	project := createReadyProject(t, svc, db, "snapshot-test", userID)

	// Create project directory with valid pixi files
	projectPath := svc.executor.GetProjectPath(project)
	os.MkdirAll(projectPath, 0755)
	manifest := "[project]\nname = \"test\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	lock := "version: 6\npackages: []\n"
	os.WriteFile(filepath.Join(projectPath, "pixi.toml"), []byte(manifest), 0644)
	os.WriteFile(filepath.Join(projectPath, "pixi.lock"), []byte(lock), 0644)

	// Run pixi install first so pixi list works
	cmd := exec.Command("pixi", "install")
	cmd.Dir = projectPath
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("pixi install failed (env may not support this platform): %s", out)
	}

	jobID := uuid.New()
	err := svc.CreateVersionSnapshot(context.Background(), project, jobID, userID, "Test snapshot")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify version created in DB
	var versions []models.ProjectVersion
	db.Where("project_id = ?", project.ID).Find(&versions)
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
	project := createReadyProject(t, svc, db, "no-files", userID)

	// Project dir exists but no pixi files
	projectPath := svc.executor.GetProjectPath(project)
	os.MkdirAll(projectPath, 0755)

	err := svc.CreateVersionSnapshot(context.Background(), project, uuid.New(), userID, "Should fail")
	if err == nil {
		t.Fatal("expected error for missing pixi.toml")
	}
}

func TestCreateVersionSnapshot_RejectsOversizedLockBeforeVersionWrite(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	project := createReadyProject(t, svc, db, "snapshot-lock-limit", userID)
	limitCfg := limits.Defaults()
	limitCfg.LockBytes = 8
	svc.limits = limitCfg

	projectPath := svc.executor.GetProjectPath(project)
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, "pixi.toml"), []byte("[project]\nname = \"snapshot-lock-limit\"\n"), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, "pixi.lock"), []byte(strings.Repeat("x", 9)), 0o644); err != nil {
		t.Fatalf("write lock: %v", err)
	}

	err := svc.CreateVersionSnapshot(context.Background(), project, uuid.New(), userID, "too large")

	var ve *ValidationError
	if !isValidationError(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
	var versions int64
	db.Model(&models.ProjectVersion{}).Where("project_id = ?", project.ID).Count(&versions)
	if versions != 0 {
		t.Fatalf("expected no version writes, got %d", versions)
	}
}

func TestCreateVersionSnapshot_StoresAllResolvedPackages(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	project := createReadyProject(t, svc, db, "snapshot-package-limit", userID)

	stubPixiList(t, []pixi.Package{
		{Name: "numpy", Version: "1.0.0"},
		{Name: "pandas", Version: "2.0.0"},
	})

	projectPath := svc.executor.GetProjectPath(project)
	if err := os.MkdirAll(projectPath, 0o755); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, "pixi.toml"), []byte("[project]\nname = \"snapshot-package-limit\"\n"), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, "pixi.lock"), []byte("version: 6\n"), 0o644); err != nil {
		t.Fatalf("write lock: %v", err)
	}

	err := svc.CreateVersionSnapshot(context.Background(), project, uuid.New(), userID, "resolved packages above cap")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var versions []models.ProjectVersion
	db.Where("project_id = ?", project.ID).Find(&versions)
	if len(versions) != 1 {
		t.Fatalf("expected 1 version, got %d", len(versions))
	}
	if !strings.Contains(versions[0].PackageMetadata, "numpy") || !strings.Contains(versions[0].PackageMetadata, "pandas") {
		t.Fatalf("expected both resolved packages in metadata, got %q", versions[0].PackageMetadata)
	}
}

// Regression test for https://github.com/nebari-dev/nebi/issues/497: rolling
// back to a known-good version must be possible from the "failed" state.
func TestRollbackToVersion_AllowedOnFailedProject(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	project := createReadyProject(t, svc, db, "rollback-after-failure", userID)

	svc.PushVersion(context.Background(), project.ID.String(), PushRequest{
		PixiToml: "[project]\nname = \"test\"",
		PixiLock: "version: 6",
	}, userID)
	db.Model(project).Update("status", models.ProjectStatusFailed)

	job, err := svc.RollbackToVersion(context.Background(), project.ID.String(), 1, userID)
	if err != nil {
		t.Fatalf("rollback on failed project should be allowed, got %T: %v", err, err)
	}
	if job.Type != models.JobTypeRollback {
		t.Errorf("expected job type %q, got %q", models.JobTypeRollback, job.Type)
	}
}
