package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/audit"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/pkgmgr"
	"gorm.io/gorm"
)

// submitJob validates the workspace is ready, creates a Job record, enqueues it,
// and writes an audit log. This is the common pattern for all async operations.
func (s *WorkspaceService) submitJob(ctx context.Context, wsID string, userID uuid.UUID,
	jobType models.JobType, metadata map[string]interface{}, auditAction string) (*models.Job, error) {

	var ws models.Workspace
	if err := s.db.Where("id = ?", wsID).First(&ws).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if ws.Status != models.WsStatusReady {
		return nil, &ValidationError{Message: "Workspace is not ready"}
	}

	job := &models.Job{
		Type:        jobType,
		WorkspaceID: ws.ID,
		Status:      models.JobStatusPending,
		Metadata:    metadata,
	}
	if err := s.db.Create(job).Error; err != nil {
		return nil, fmt.Errorf("create job: %w", err)
	}
	if err := s.queue.Enqueue(ctx, job); err != nil {
		return nil, fmt.Errorf("enqueue job: %w", err)
	}

	audit.LogAction(s.db, userID, auditAction, fmt.Sprintf("ws:%s", ws.ID.String()), metadata)

	return job, nil
}

// InstallPackages creates and enqueues an install-packages job.
func (s *WorkspaceService) InstallPackages(ctx context.Context, wsID string, packages []string, userID uuid.UUID) (*models.Job, error) {
	metadata := map[string]interface{}{
		"packages": packages,
		"user_id":  userID.String(),
	}
	return s.submitJob(ctx, wsID, userID, models.JobTypeInstall, metadata, audit.ActionInstallPackage)
}

// RemovePackage creates and enqueues a remove-package job.
func (s *WorkspaceService) RemovePackage(ctx context.Context, wsID string, packageName string, userID uuid.UUID) (*models.Job, error) {
	metadata := map[string]interface{}{
		"packages": []string{packageName},
		"user_id":  userID.String(),
	}
	return s.submitJob(ctx, wsID, userID, models.JobTypeRemove, metadata, audit.ActionRemovePackage)
}

// ListPackages returns packages for a workspace, auto-syncing from disk for local workspaces with no DB records.
func (s *WorkspaceService) ListPackages(wsID string) ([]models.Package, error) {
	var ws models.Workspace
	if err := s.db.Where("id = ?", wsID).First(&ws).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}

	var packages []models.Package
	if err := s.db.Where("workspace_id = ?", ws.ID).Find(&packages).Error; err != nil {
		return nil, fmt.Errorf("fetch packages: %w", err)
	}

	// Auto-sync: if local workspace has 0 packages in DB, populate from disk
	if len(packages) == 0 && ws.Source == "local" && ws.Status == models.WsStatusReady {
		if synced := s.syncPackagesFromDisk(&ws); synced != nil {
			packages = synced
		}
	}

	return packages, nil
}

// syncPackagesFromDisk runs the package manager list and populates the DB for a local workspace.
func (s *WorkspaceService) syncPackagesFromDisk(ws *models.Workspace) []models.Package {
	wsPath := s.executor.GetWorkspacePath(ws)

	pmType := ws.PackageManager
	if pmType == "" {
		pmType = "pixi"
	}

	pm, err := pkgmgr.New(pmType)
	if err != nil {
		slog.Warn("syncPackagesFromDisk: failed to create package manager", "error", err)
		return nil
	}

	listed, err := pm.List(context.Background(), pkgmgr.ListOptions{EnvPath: wsPath})
	if err != nil {
		slog.Warn("syncPackagesFromDisk: failed to list packages", "error", err, "path", wsPath)
		return nil
	}

	var result []models.Package
	for _, p := range listed {
		pkg := models.Package{
			WorkspaceID: ws.ID,
			Name:        p.Name,
			Version:     p.Version,
		}
		if err := s.db.Create(&pkg).Error; err != nil {
			slog.Warn("syncPackagesFromDisk: failed to save package", "error", err, "name", p.Name)
			continue
		}
		result = append(result, pkg)
	}

	return result
}

// SyncPackagesFromWorkspace lists packages from the workspace on disk and saves them to the DB.
// Called by the worker after install/remove/create/rollback operations.
func (s *WorkspaceService) SyncPackagesFromWorkspace(ctx context.Context, ws *models.Workspace) error {
	wsPath := s.executor.GetWorkspacePath(ws)

	pm, err := pkgmgr.New(ws.PackageManager)
	if err != nil {
		return fmt.Errorf("failed to create package manager: %w", err)
	}

	pkgs, err := pm.List(ctx, pkgmgr.ListOptions{EnvPath: wsPath})
	if err != nil {
		return fmt.Errorf("failed to list packages: %w", err)
	}

	// Clear existing packages
	s.db.Where("workspace_id = ?", ws.ID).Delete(&models.Package{})

	// Insert new packages
	for _, pkg := range pkgs {
		dbPkg := models.Package{
			WorkspaceID: ws.ID,
			Name:        pkg.Name,
			Version:     pkg.Version,
		}
		if err := s.db.Create(&dbPkg).Error; err != nil {
			slog.Error("Failed to save package", "package", pkg.Name, "error", err)
		}
	}

	return nil
}

// SaveInstalledPackages records newly installed packages in the DB.
func (s *WorkspaceService) SaveInstalledPackages(wsID uuid.UUID, packages []string) {
	for _, pkgName := range packages {
		pkg := models.Package{
			WorkspaceID: wsID,
			Name:        pkgName,
		}
		s.db.Create(&pkg)
	}
}

// DeletePackagesByName removes specific packages from the DB.
func (s *WorkspaceService) DeletePackagesByName(wsID uuid.UUID, packages []string) {
	for _, pkgName := range packages {
		s.db.Where("workspace_id = ? AND name = ?", wsID, pkgName).Delete(&models.Package{})
	}
}

// DeleteAllPackages removes all packages for a workspace from the DB.
func (s *WorkspaceService) DeleteAllPackages(wsID uuid.UUID) {
	s.db.Where("workspace_id = ?", wsID).Delete(&models.Package{})
}
