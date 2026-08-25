package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	resourcemetrics "github.com/nebari-dev/nebi/internal/metrics"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/pkgmgr"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var activeJobStatuses = []models.JobStatus{
	models.JobStatusPending,
	models.JobStatusRunning,
}

func (s *WorkspaceService) validateManifestContent(packageManager string, name string, content string) error {
	if content == "" {
		return nil
	}
	if s.limits.ManifestBytes > 0 && len(content) > s.limits.ManifestBytes {
		return &ValidationError{Message: fmt.Sprintf("%s exceeds %d bytes", name, s.limits.ManifestBytes)}
	}
	if s.limits.PackageStringBytes > 0 {
		packages, err := pkgmgr.ManifestPackageNames(packageManager, content)
		if err != nil {
			return &ValidationError{Message: fmt.Sprintf("invalid %s: %v", name, err)}
		}
		for i, pkg := range packages {
			if strings.TrimSpace(pkg) == "" {
				return &ValidationError{Message: fmt.Sprintf("%s package entry at index %d must not be empty", name, i)}
			}
			if s.limits.PackageStringBytes > 0 && len(pkg) > s.limits.PackageStringBytes {
				return &ValidationError{Message: fmt.Sprintf("%s package entry at index %d exceeds %d bytes", name, i, s.limits.PackageStringBytes)}
			}
		}
	}
	return nil
}

func (s *WorkspaceService) validateLockContent(name string, content string) error {
	if content == "" {
		return nil
	}
	if s.limits.LockBytes > 0 && len(content) > s.limits.LockBytes {
		return &ValidationError{Message: fmt.Sprintf("%s exceeds %d bytes", name, s.limits.LockBytes)}
	}
	return nil
}

func (s *WorkspaceService) validatePushRequest(packageManager string, req PushRequest) error {
	if err := s.validateManifestContent(packageManager, "pixi.toml", req.PixiToml); err != nil {
		return err
	}
	return s.validateLockContent("pixi.lock", req.PixiLock)
}

// ValidateVersionContent checks stored manifest/lock content before a worker
// uses it as fresh build input, for example during rollback of legacy versions.
func (s *WorkspaceService) ValidateVersionContent(packageManager string, manifestContent, lockContent string) error {
	if err := s.validateManifestContent(packageManager, "pixi.toml", manifestContent); err != nil {
		return err
	}
	return s.validateLockContent("pixi.lock", lockContent)
}

func (s *WorkspaceService) validateWorkspaceManifestForJob(_ *gorm.DB, ws *models.Workspace) error {
	envPath := s.executor.GetWorkspacePath(ws)
	return s.validateWorkspaceFileForJob(filepath.Join(envPath, "pixi.toml"), "pixi.toml", s.limits.ManifestBytes, func(name string, content string) error {
		return s.validateManifestContent(ws.PackageManager, name, content)
	})
}

func (s *WorkspaceService) validateWorkspaceManifestAndLockForJob(_ *gorm.DB, ws *models.Workspace) error {
	envPath := s.executor.GetWorkspacePath(ws)
	if err := s.validateWorkspaceFileForJob(filepath.Join(envPath, "pixi.toml"), "pixi.toml", s.limits.ManifestBytes, func(name string, content string) error {
		return s.validateManifestContent(ws.PackageManager, name, content)
	}); err != nil {
		return err
	}
	return s.validateWorkspaceFileForJob(filepath.Join(envPath, "pixi.lock"), "pixi.lock", s.limits.LockBytes, s.validateLockContent)
}

func (s *WorkspaceService) validateWorkspaceFileForJob(path string, name string, maxBytes int, validate func(string, string) error) error {
	content, err := s.readLimitedTextFile(path, name, maxBytes)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s for job admission: %w", name, err)
	}
	return validate(name, content)
}

func (s *WorkspaceService) validatePackages(packages []string) error {
	if len(packages) == 0 {
		return &ValidationError{Message: "packages must not be empty"}
	}
	for i, pkg := range packages {
		if strings.TrimSpace(pkg) == "" {
			return &ValidationError{Message: "package names must not be empty"}
		}
		if s.limits.PackageStringBytes > 0 && len(pkg) > s.limits.PackageStringBytes {
			return &ValidationError{Message: fmt.Sprintf("package at index %d exceeds %d bytes", i, s.limits.PackageStringBytes)}
		}
	}
	return nil
}

func (s *WorkspaceService) listOptions(envPath string) pkgmgr.ListOptions {
	return pkgmgr.ListOptions{
		EnvPath:        envPath,
		ResourceLimits: s.limits.ProcessLimits(),
		MaxOutputBytes: s.limits.MetadataBytes,
	}
}

func (s *WorkspaceService) mapPackageManagerListError(err error) error {
	var outputLimitErr *pkgmgr.OutputLimitError
	if errors.As(err, &outputLimitErr) {
		return &ValidationError{Message: outputLimitErr.Error()}
	}
	return err
}

func (s *WorkspaceService) validateListedPackages(name string, packages []pkgmgr.Package) error {
	for i, pkg := range packages {
		fields := []struct {
			name  string
			value string
		}{
			{name: "name", value: pkg.Name},
			{name: "version", value: pkg.Version},
			{name: "channel", value: pkg.Channel},
		}
		if strings.TrimSpace(pkg.Name) == "" {
			return &ValidationError{Message: fmt.Sprintf("%s entry at index %d must have a package name", name, i)}
		}
		for _, field := range fields {
			if s.limits.PackageStringBytes > 0 && len(field.value) > s.limits.PackageStringBytes {
				return &ValidationError{Message: fmt.Sprintf("%s entry at index %d field %s exceeds %d bytes", name, i, field.name, s.limits.PackageStringBytes)}
			}
		}
	}
	return nil
}

func (s *WorkspaceService) packageMetadataJSON(packages []pkgmgr.Package) ([]byte, error) {
	if err := s.validateListedPackages("package metadata", packages); err != nil {
		return nil, err
	}
	packageMetadata, err := json.Marshal(packages)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize package metadata: %w", err)
	}
	if s.limits.MetadataBytes > 0 && len(packageMetadata) > s.limits.MetadataBytes {
		return nil, &ValidationError{Message: fmt.Sprintf("package metadata exceeds %d bytes", s.limits.MetadataBytes)}
	}
	return packageMetadata, nil
}

func (s *WorkspaceService) validateJobMetadata(metadata map[string]interface{}) error {
	if len(metadata) == 0 {
		return nil
	}
	genericMetadata := make(map[string]interface{}, len(metadata))
	for key, value := range metadata {
		if key == "pixi_toml" {
			continue
		}
		genericMetadata[key] = value
	}
	if len(genericMetadata) == 0 {
		return nil
	}
	encoded, err := json.Marshal(genericMetadata)
	if err != nil {
		return &ValidationError{Message: fmt.Sprintf("invalid job metadata: %v", err)}
	}
	if s.limits.MetadataBytes > 0 && len(encoded) > s.limits.MetadataBytes {
		return &ValidationError{Message: fmt.Sprintf("job metadata exceeds %d bytes", s.limits.MetadataBytes)}
	}
	return nil
}

func (s *WorkspaceService) readLimitedTextFile(path string, name string, maxBytes int) (string, error) {
	if maxBytes <= 0 {
		content, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return string(content), nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.Size() > int64(maxBytes) {
		return "", &ValidationError{Message: fmt.Sprintf("%s exceeds %d bytes", name, maxBytes)}
	}

	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	content, err := io.ReadAll(io.LimitReader(file, int64(maxBytes)+1))
	if err != nil {
		return "", err
	}
	if len(content) > maxBytes {
		return "", &ValidationError{Message: fmt.Sprintf("%s exceeds %d bytes", name, maxBytes)}
	}
	return string(content), nil
}

func (s *WorkspaceService) lockJobAdmission(tx *gorm.DB) error {
	// This row deliberately serializes admissions server-wide. Manifest reads
	// and quota checks happen while the row is write-locked so job creation,
	// audit logging, and quota decisions observe one consistent active-job set.
	lock := models.ResourceLock{Name: models.ResourceLockJobAdmission}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&lock).Error; err != nil {
		return fmt.Errorf("ensure job admission lock: %w", err)
	}
	result := tx.Model(&models.ResourceLock{}).
		Where("name = ?", models.ResourceLockJobAdmission).
		Update("updated_at", time.Now().UTC())
	if result.Error != nil {
		return fmt.Errorf("lock job admission: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("lock job admission: lock row missing")
	}
	return nil
}

func (s *WorkspaceService) checkActiveJobQuotas(tx *gorm.DB, userID, workspaceID uuid.UUID) error {
	if s.limits.ActiveJobsGlobal > 0 {
		count, err := activeJobCount(tx)
		if err != nil {
			return err
		}
		if count >= int64(s.limits.ActiveJobsGlobal) {
			return newQuotaExceededError("global", fmt.Sprintf("global active job limit %d reached", s.limits.ActiveJobsGlobal))
		}
	}

	if userID != uuid.Nil && s.limits.ActiveJobsPerUser > 0 {
		count, err := activeJobCountForUser(tx, userID)
		if err != nil {
			return err
		}
		if count >= int64(s.limits.ActiveJobsPerUser) {
			return newQuotaExceededError("user", fmt.Sprintf("active job limit %d reached for user", s.limits.ActiveJobsPerUser))
		}
	}

	if workspaceID != uuid.Nil && s.limits.ActiveJobsPerWorkspace > 0 {
		count, err := activeJobCount(tx.Where("workspace_id = ?", workspaceID))
		if err != nil {
			return err
		}
		if count >= int64(s.limits.ActiveJobsPerWorkspace) {
			return newQuotaExceededError("workspace", fmt.Sprintf("active job limit %d reached for workspace", s.limits.ActiveJobsPerWorkspace))
		}
	}

	return nil
}

func activeJobCount(query *gorm.DB) (int64, error) {
	var count int64
	if err := query.Model(&models.Job{}).Where("status IN ?", activeJobStatuses).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count active jobs: %w", err)
	}
	return count, nil
}

func activeJobCountForUser(query *gorm.DB, userID uuid.UUID) (int64, error) {
	var count int64
	err := query.Model(&models.Job{}).
		Joins("LEFT JOIN workspaces ON workspaces.id = jobs.workspace_id").
		Where("jobs.status IN ?", activeJobStatuses).
		Where(`
			jobs.user_id = ?
			OR ((jobs.user_id IS NULL OR jobs.user_id = '' OR jobs.user_id = ?) AND workspaces.owner_id = ?)
		`, userID, uuid.Nil.String(), userID).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("count active jobs by user: %w", err)
	}
	return count, nil
}

type quotaExceededError struct {
	scope   string
	message string
}

func newQuotaExceededError(scope, message string) error {
	return &quotaExceededError{
		scope:   scope,
		message: message,
	}
}

func (e *quotaExceededError) Error() string {
	return e.message
}

func (s *WorkspaceService) finishAdmissionError(err error) error {
	var quotaErr *quotaExceededError
	if errors.As(err, &quotaErr) {
		_ = resourcemetrics.IncQuotaRejected(s.db, quotaErr.scope)
		return &ConflictError{Message: quotaErr.message}
	}
	return err
}
