package store

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
)

// ListWorkspaces returns all workspaces.
func (s *Store) ListWorkspaces() ([]models.Workspace, error) {
	var wss []models.Workspace
	if err := s.db.Find(&wss).Error; err != nil {
		return nil, fmt.Errorf("listing workspaces: %w", err)
	}
	return wss, nil
}

// GetWorkspace returns a workspace by ID.
func (s *Store) GetWorkspace(id uuid.UUID) (*models.Workspace, error) {
	var ws models.Workspace
	if err := s.db.Where("id = ?", id).First(&ws).Error; err != nil {
		return nil, fmt.Errorf("getting workspace: %w", err)
	}
	return &ws, nil
}

// FindWorkspaceByPath returns the workspace at the given path, or nil if not found.
func (s *Store) FindWorkspaceByPath(path string) (*models.Workspace, error) {
	var ws models.Workspace
	result := s.db.Where("path = ?", path).First(&ws)
	if result.Error != nil {
		if result.RowsAffected == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("finding workspace by path: %w", result.Error)
	}
	return &ws, nil
}

// FindWorkspaceByName returns the first workspace with the given name, or nil if not found.
func (s *Store) FindWorkspaceByName(name string) (*models.Workspace, error) {
	var ws models.Workspace
	result := s.db.Where("name = ?", name).First(&ws)
	if result.Error != nil {
		if result.RowsAffected == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("finding workspace by name: %w", result.Error)
	}
	return &ws, nil
}

// FindGlobalWorkspaceByName returns the first global workspace with the given name.
func (s *Store) FindGlobalWorkspaceByName(name string) (*models.Workspace, error) {
	var ws models.Workspace
	result := s.db.Where("name = ? AND is_global = ?", name, true).First(&ws)
	if result.Error != nil {
		if result.RowsAffected == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("finding global workspace: %w", result.Error)
	}
	return &ws, nil
}

// CreateWorkspace creates a new workspace record.
func (s *Store) CreateWorkspace(ws *models.Workspace) error {
	if ws.ID == uuid.Nil {
		ws.ID = uuid.New()
	}
	if ws.Status == "" {
		ws.Status = models.WsStatusReady
	}
	if ws.Source == "" {
		ws.Source = "local"
	}
	if ws.PackageManager == "" {
		ws.PackageManager = "pixi"
	}
	return s.db.Create(ws).Error
}

// SaveWorkspace updates an existing workspace record.
func (s *Store) SaveWorkspace(ws *models.Workspace) error {
	return s.db.Save(ws).Error
}

// DeleteWorkspace removes a workspace by ID (hard delete).
func (s *Store) DeleteWorkspace(id uuid.UUID) error {
	return s.db.Unscoped().Where("id = ?", id).Delete(&models.Workspace{}).Error
}
