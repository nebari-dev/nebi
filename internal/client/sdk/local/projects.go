package local

// Owner: U1 (projects). Projects list, create, delete, name validation,
// size on disk, install status, default location.

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
)

// ListProjects returns every local project for the projects list, newest
// first, then runs the registered ProjectDecorators.
func (s *Service) ListProjects(ctx context.Context) ([]shared.ProjectListItem, error) {
	return nil, shared.NotImplemented("local.ListProjects")
}

// GetProject returns one project as a list item (decorated).
func (s *Service) GetProject(ctx context.Context, id uuid.UUID) (*shared.ProjectListItem, error) {
	return nil, shared.NotImplemented("local.GetProject")
}

// ResolveProject finds a project by UUID or by name (CLI arguments).
// Returns shared.ErrNotFound when nothing matches. Implemented by the
// foundation because every unit's CLI commands use it.
func (s *Service) ResolveProject(ctx context.Context, ref string) (*models.Project, error) {
	var p models.Project
	db := s.deps.DB.WithContext(ctx)
	if id, err := uuid.Parse(ref); err == nil {
		if err := db.Where("id = ?", id).First(&p).Error; err == nil {
			return &p, nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	err := db.Where("name = ?", ref).Order("created_at DESC").First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("project %q: %w", ref, shared.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// CheckName validates a new project name: allowed characters and no
// duplicate among local projects.
func (s *Service) CheckName(ctx context.Context, name string) (shared.NameCheckResult, error) {
	return shared.NameCheckResult{}, shared.NotImplemented("local.CheckName")
}

// CreateProject validates the request (name, at least one platform),
// creates the project row (status pending) and submits a JobTypeCreate job
// with metadata {"pixi_toml", "path", "description", "tags", "install"}.
func (s *Service) CreateProject(ctx context.Context, req shared.CreateProjectRequest) (*shared.CreateProjectResult, error) {
	return nil, shared.NotImplemented("local.CreateProject")
}

// DeleteProject submits a JobTypeDelete job for the project.
func (s *Service) DeleteProject(ctx context.Context, id uuid.UUID) (*shared.JobSummary, error) {
	return nil, shared.NotImplemented("local.DeleteProject")
}

// ProjectSize computes the size on disk of the project's environment.
func (s *Service) ProjectSize(ctx context.Context, id uuid.UUID) (int64, error) {
	return 0, shared.NotImplemented("local.ProjectSize")
}

// RunCreateProject is the create job body (client-submitted create jobs
// only): write pixi.toml at the project path, solve the lock, store version
// 1 (description, tags via ApplyTags), mark the project ready and, when the
// job metadata has install=true, install version 1 through the exec SDK
// job body. Sets job.VersionNumber = 1.
func (s *Service) RunCreateProject(ctx context.Context, job *models.Job, log io.Writer) error {
	return shared.NotImplemented("local.RunCreateProject")
}

// RunDeleteProject is the delete job body (client-submitted): uninstall,
// remove the project directory when Nebi created it, and soft-delete the
// project with its versions and remotes.
func (s *Service) RunDeleteProject(ctx context.Context, job *models.Job, log io.Writer) error {
	return shared.NotImplemented("local.RunDeleteProject")
}
