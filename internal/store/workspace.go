package store

import (
	"fmt"
	"path/filepath"

	"github.com/google/uuid"
)

// ListWorkspaces returns all workspaces.
func (s *Store) ListWorkspaces() ([]LocalWorkspace, error) {
	var wss []LocalWorkspace
	if err := s.db.Find(&wss).Error; err != nil {
		return nil, fmt.Errorf("listing workspaces: %w", err)
	}
	return wss, nil
}

// GetWorkspace returns a workspace by ID.
func (s *Store) GetWorkspace(id uuid.UUID) (*LocalWorkspace, error) {
	var ws LocalWorkspace
	if err := s.db.Where("id = ?", id).First(&ws).Error; err != nil {
		return nil, fmt.Errorf("getting workspace: %w", err)
	}
	return &ws, nil
}

// FindWorkspaceByPath returns the workspace at the given path, or nil if not found.
func (s *Store) FindWorkspaceByPath(path string) (*LocalWorkspace, error) {
	ws, err := s.findWorkspaceByExactPath(path)
	if err != nil || ws != nil {
		return ws, err
	}

	canonicalPath, ok := canonicalizeExistingPath(path)
	if !ok {
		return nil, nil
	}
	if canonicalPath != path {
		ws, err := s.findWorkspaceByExactPath(canonicalPath)
		if err != nil || ws != nil {
			return ws, err
		}
	}

	var workspaces []LocalWorkspace
	if err := s.db.Find(&workspaces).Error; err != nil {
		return nil, fmt.Errorf("finding workspace by equivalent path: %w", err)
	}
	for i := range workspaces {
		storedPath, ok := canonicalizeExistingPath(workspaces[i].Path)
		if ok && storedPath == canonicalPath {
			return &workspaces[i], nil
		}
	}

	return nil, nil
}

func (s *Store) findWorkspaceByExactPath(path string) (*LocalWorkspace, error) {
	var ws LocalWorkspace
	result := s.db.Where("path = ?", path).First(&ws)
	if result.Error != nil {
		if result.RowsAffected == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("finding workspace by path: %w", result.Error)
	}
	return &ws, nil
}

func canonicalizeExistingPath(path string) (string, bool) {
	canonicalPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", false
	}
	return canonicalPath, true
}

// FindWorkspaceByName returns the first workspace with the given name, or nil if not found.
func (s *Store) FindWorkspaceByName(name string) (*LocalWorkspace, error) {
	var ws LocalWorkspace
	result := s.db.Where("name = ?", name).First(&ws)
	if result.Error != nil {
		if result.RowsAffected == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("finding workspace by name: %w", result.Error)
	}
	return &ws, nil
}

// FindWorkspacesByName returns all workspaces with the given name.
// Multiple workspaces can share the same name since path is the unique identifier.
func (s *Store) FindWorkspacesByName(name string) ([]LocalWorkspace, error) {
	var wss []LocalWorkspace
	if err := s.db.Where("name = ?", name).Find(&wss).Error; err != nil {
		return nil, fmt.Errorf("finding workspaces by name: %w", err)
	}
	return wss, nil
}

// CreateWorkspace creates a new workspace record.
func (s *Store) CreateWorkspace(ws *LocalWorkspace) error {
	if ws.ID == uuid.Nil {
		ws.ID = uuid.New()
	}
	if ws.Status == "" {
		ws.Status = "ready"
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
func (s *Store) SaveWorkspace(ws *LocalWorkspace) error {
	return s.db.Save(ws).Error
}

// DeleteWorkspace removes a workspace by ID (hard delete).
func (s *Store) DeleteWorkspace(id uuid.UUID) error {
	return s.db.Unscoped().Where("id = ?", id).Delete(&LocalWorkspace{}).Error
}
