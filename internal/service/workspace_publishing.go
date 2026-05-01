package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/audit"
	"github.com/nebari-dev/nebi/internal/contenthash"
	nebicrypto "github.com/nebari-dev/nebi/internal/crypto"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/oci"
	"gorm.io/gorm"
)

// PublishWorkspace publishes a workspace's pixi.toml and pixi.lock to an OCI registry.
func (s *WorkspaceService) PublishWorkspace(ctx context.Context, wsID string, req PublishWorkspaceRequest, userID uuid.UUID) (*PublicationResult, error) {
	var ws models.Workspace
	if err := s.db.Where("id = ?", wsID).First(&ws).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if ws.Status != models.WsStatusReady {
		return nil, &ValidationError{Message: "Workspace must be in ready state to publish"}
	}

	// Get the latest version
	var latestVersion models.WorkspaceVersion
	if err := s.db.Where("workspace_id = ?", wsID).Order("version_number DESC").First(&latestVersion).Error; err != nil {
		return nil, &ValidationError{Message: "Workspace has no versions to publish"}
	}

	ep, err := s.loadRegistryEndpoint(req.RegistryID)
	if err != nil {
		if err == ErrNotFound {
			return nil, &ValidationError{Message: "Registry not found"}
		}
		return nil, err
	}
	registry := *ep.Registry
	fullRepo := ep.NamespaceRelativeRepoRef(req.Repository)

	wsPath := s.executor.GetWorkspacePath(&ws)

	// Collect extra OCI tags
	extraTagSet := map[string]bool{"latest": true}
	var wsTags []models.WorkspaceTag
	s.db.Where("workspace_id = ? AND version_number = ?", ws.ID, latestVersion.VersionNumber).Find(&wsTags)
	for _, t := range wsTags {
		extraTagSet[t.Tag] = true
	}
	delete(extraTagSet, req.Tag)
	var extraTags []string
	for t := range extraTagSet {
		extraTags = append(extraTags, t)
	}

	var digest string
	if s.isLocal {
		regEndpoint := oci.Registry{
			Host:      ep.Host,
			Namespace: ep.Namespace,
			Username:  ep.Username,
			Password:  ep.Password,
			PlainHTTP: ep.PlainHTTP,
		}
		res, err := oci.Publish(ctx, wsPath, regEndpoint, req.Repository, req.Tag,
			oci.WithExtraTags(extraTags...),
		)
		if err != nil {
			return nil, fmt.Errorf("publish failed: %w", err)
		}
		digest = res.Digest
	} else {
		d, err := oci.PublishWorkspace(ctx, wsPath, oci.PublishOptions{
			Repository:   fullRepo,
			Tag:          req.Tag,
			ExtraTags:    extraTags,
			Username:     ep.Username,
			Password:     ep.Password,
			RegistryHost: ep.Host,
		})
		if err != nil {
			return nil, fmt.Errorf("publish failed: %w", err)
		}
		digest = d
	}

	// Create publication record
	publication := models.Publication{
		WorkspaceID:   ws.ID,
		VersionNumber: latestVersion.VersionNumber,
		RegistryID:    registry.ID,
		Repository:    req.Repository,
		Tag:           req.Tag,
		Digest:        digest,
		PublishedBy:   userID,
	}
	if err := s.db.Create(&publication).Error; err != nil {
		return nil, fmt.Errorf("save publication record: %w", err)
	}

	// Load relations for response
	s.db.Preload("Registry").Preload("PublishedByUser").First(&publication, publication.ID)

	audit.Log(s.db, userID, audit.ActionPublishWorkspace, audit.ResourceWorkspace, ws.ID, map[string]interface{}{
		"registry":   registry.Name,
		"repository": req.Repository,
		"tag":        req.Tag,
	})

	return publicationToResult(&publication), nil
}

// ListPublications returns all publications for a workspace.
func (s *WorkspaceService) ListPublications(wsID string) ([]PublicationResult, error) {
	var ws models.Workspace
	if err := s.db.Where("id = ?", wsID).First(&ws).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}

	var publications []models.Publication
	if err := s.db.Where("workspace_id = ?", wsID).
		Preload("Registry").
		Preload("PublishedByUser").
		Order("created_at DESC").
		Find(&publications).Error; err != nil {
		return nil, fmt.Errorf("fetch publications: %w", err)
	}

	results := make([]PublicationResult, len(publications))
	for i, pub := range publications {
		results[i] = *publicationToResult(&pub)
	}
	return results, nil
}

// UpdatePublication updates a publication's visibility.
func (s *WorkspaceService) UpdatePublication(ctx context.Context, wsID string, pubID string, isPublic bool) (*PublicationResult, error) {
	var ws models.Workspace
	if err := s.db.Where("id = ?", wsID).First(&ws).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}

	var publication models.Publication
	if err := s.db.Where("id = ? AND workspace_id = ?", pubID, wsID).First(&publication).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}

	publication.IsPublic = isPublic
	if err := s.db.Save(&publication).Error; err != nil {
		return nil, fmt.Errorf("update publication: %w", err)
	}

	// Best-effort: change repository visibility on the registry
	var registry models.OCIRegistry
	if err := s.db.Where("id = ?", publication.RegistryID).First(&registry).Error; err == nil && registry.APIToken != "" {
		apiToken, err := nebicrypto.DecryptField(registry.APIToken, s.encKey)
		if err == nil {
			host, _ := oci.ParseRegistryURL(registry.URL)
			repoPath := publication.Repository
			if registry.Namespace != "" {
				repoPath = registry.Namespace + "/" + publication.Repository
			}
			if visErr := oci.ChangeRepositoryVisibility(ctx, host, repoPath, apiToken, isPublic); visErr != nil {
				slog.Warn("Failed to change registry visibility", "error", visErr, "repo", repoPath)
			}
		}
	}

	// Load relations for response
	s.db.Preload("Registry").Preload("PublishedByUser").First(&publication, publication.ID)

	return publicationToResult(&publication), nil
}

// GetPublishDefaults returns default values for the publish dialog.
func (s *WorkspaceService) GetPublishDefaults(wsID string) (*PublishDefaultsResult, error) {
	var ws models.Workspace
	if err := s.db.Where("id = ?", wsID).First(&ws).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}

	var registry models.OCIRegistry
	if err := s.db.Where("is_default = ?", true).First(&registry).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}

	repo := fmt.Sprintf("%s-%s", ws.Name, ws.ID.String()[:8])

	tag := "latest"
	var latestVersion models.WorkspaceVersion
	hasVersion := s.db.Where("workspace_id = ?", wsID).Order("version_number DESC").First(&latestVersion).Error == nil

	if hasVersion {
		if s.isLocal {
			wsPath := s.executor.GetWorkspacePath(&ws)
			pixiToml, tomlErr := os.ReadFile(filepath.Join(wsPath, "pixi.toml"))
			pixiLock, lockErr := os.ReadFile(filepath.Join(wsPath, "pixi.lock"))
			// A workspace that has been pushed to but never installed yet
			// has no pixi.lock on disk — return the "latest" fallback so
			// the publish dialog stays usable instead of 500'ing.
			if (tomlErr == nil || os.IsNotExist(tomlErr)) && (lockErr == nil || os.IsNotExist(lockErr)) {
				refs, err := oci.PreviewAssetRefs(wsPath)
				if err != nil {
					return nil, fmt.Errorf("preview bundle for default tag: %w", err)
				}
				tag = contenthash.HashBundle(string(pixiToml), string(pixiLock), refs)
			} else if tomlErr != nil {
				return nil, fmt.Errorf("read pixi.toml for bundle hash: %w", tomlErr)
			} else {
				return nil, fmt.Errorf("read pixi.lock for bundle hash: %w", lockErr)
			}
		} else if latestVersion.ContentHash != "" {
			tag = latestVersion.ContentHash
		}
	}

	return &PublishDefaultsResult{
		RegistryID:   registry.ID,
		RegistryName: registry.Name,
		Namespace:    registry.Namespace,
		Repository:   repo,
		Tag:          tag,
	}, nil
}

func publicationToResult(pub *models.Publication) *PublicationResult {
	return &PublicationResult{
		ID:                pub.ID,
		VersionNumber:     pub.VersionNumber,
		RegistryName:      pub.Registry.Name,
		RegistryURL:       pub.Registry.URL,
		RegistryNamespace: pub.Registry.Namespace,
		Repository:        pub.Repository,
		Tag:               pub.Tag,
		Digest:            pub.Digest,
		IsPublic:          pub.IsPublic,
		PublishedBy:       pub.PublishedByUser.Username,
		PublishedAt:       pub.CreatedAt.Format("2006-01-02 15:04:05"),
	}
}
