package exec

// Owner: U2 (detail). Install a specific version / last installed / latest,
// uninstall, and the corresponding job bodies.

import (
	"context"
	"io"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/models"
)

// InstallVersion submits a JobTypeInstallVersion job. A nil req.Version
// resolves to the last installed version, else the latest version. The
// resolved number is stored on job.VersionNumber.
func (s *Service) InstallVersion(ctx context.Context, projectID uuid.UUID, req shared.InstallRequest) (*shared.JobSummary, error) {
	return nil, shared.NotImplemented("exec.InstallVersion")
}

// Uninstall submits a JobTypeUninstall job for the installed version.
func (s *Service) Uninstall(ctx context.Context, projectID uuid.UUID) (*shared.JobSummary, error) {
	return nil, shared.NotImplemented("exec.Uninstall")
}

// RunInstallVersion is the install_version job body: write the version's
// pixi.toml/pixi.lock into the project directory, pixi install, then set
// Project.InstalledVersion and LastInstalledVersion and the size on disk.
func (s *Service) RunInstallVersion(ctx context.Context, job *models.Job, log io.Writer) error {
	return shared.NotImplemented("exec.RunInstallVersion")
}

// RunUninstall is the uninstall job body: remove .pixi/envs and clear
// Project.InstalledVersion (LastInstalledVersion stays).
func (s *Service) RunUninstall(ctx context.Context, job *models.Job, log io.Writer) error {
	return shared.NotImplemented("exec.RunUninstall")
}
