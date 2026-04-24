package service

import (
	"context"
	"fmt"
	"os"

	"github.com/google/uuid"
	nebicrypto "github.com/nebari-dev/nebi/internal/crypto"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/oci"
	"gorm.io/gorm"
)

// ImportFromRegistryRequest selects the OCI bundle to import.
type ImportFromRegistryRequest struct {
	Repository string
	Tag        string
	Name       string
}

// ImportFromRegistry pulls an OCI bundle from a registered registry and
// creates a new workspace populated from it.
//
// In local mode the full bundle (pixi.toml + pixi.lock + asset layers)
// is extracted synchronously to a staging directory; the create job
// metadata carries the path so the worker seeds the workspace directory
// from it before pixi install runs. Network errors surface synchronously.
//
// In team mode only pixi.toml is fetched (existing behavior); asset
// layers and the published pixi.lock are dropped pending a team-mode
// bundle design.
func (s *WorkspaceService) ImportFromRegistry(ctx context.Context, registryID string, req ImportFromRegistryRequest, userID uuid.UUID) (*models.Workspace, error) {
	var reg models.OCIRegistry
	if err := s.db.Where("id = ?", registryID).First(&reg).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}

	var password string
	if reg.Password != "" {
		var err error
		password, err = nebicrypto.DecryptField(reg.Password, s.encKey)
		if err != nil {
			return nil, fmt.Errorf("decrypt registry credentials: %w", err)
		}
	}

	host, _, plainHTTP := oci.ParseRegistryURLFull(reg.URL)
	repoPath := req.Repository
	if reg.Namespace != "" {
		repoPath = reg.Namespace + "/" + req.Repository
	}
	repoRef := fmt.Sprintf("%s/%s", host, repoPath)

	if s.isLocal {
		stagingDir, err := os.MkdirTemp(s.executor.StagingRoot(), "import-")
		if err != nil {
			return nil, fmt.Errorf("create staging dir: %w", err)
		}

		if _, err := oci.ExtractBundle(ctx, repoRef, req.Tag, stagingDir, oci.PullOptions{
			Username:  reg.Username,
			Password:  password,
			PlainHTTP: plainHTTP,
		}); err != nil {
			_ = os.RemoveAll(stagingDir)
			return nil, fmt.Errorf("extract bundle: %w", err)
		}

		ws, err := s.Create(ctx, CreateRequest{
			Name:             req.Name,
			PackageManager:   "pixi",
			ImportStagingDir: stagingDir,
		}, userID)
		if err != nil {
			_ = os.RemoveAll(stagingDir)
			return nil, err
		}
		return ws, nil
	}

	// Team mode: pixi-only fallback (matches current ImportEnvironment
	// handler behaviour).
	result, err := oci.PullBundle(ctx, repoRef, req.Tag, oci.PullOptions{
		Username:  reg.Username,
		Password:  password,
		PlainHTTP: plainHTTP,
	})
	if err != nil {
		return nil, fmt.Errorf("pull environment: %w", err)
	}
	return s.Create(ctx, CreateRequest{
		Name:           req.Name,
		PackageManager: "pixi",
		PixiToml:       result.PixiToml,
	}, userID)
}
