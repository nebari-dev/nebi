package store

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/google/uuid"
	"gorm.io/gorm"
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
	normalizedPath, err := normalizeWorkspacePath(path)
	if err != nil {
		return nil, fmt.Errorf("normalizing workspace path: %w", err)
	}

	var ws LocalWorkspace
	result := s.db.Where("path IN ?", uniquePaths(path, normalizedPath)).First(&ws)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return s.findWorkspaceByNormalizedPath(normalizedPath)
		}
		return nil, fmt.Errorf("finding workspace by path: %w", result.Error)
	}
	return &ws, nil
}

func (s *Store) findWorkspaceByNormalizedPath(path string) (*LocalWorkspace, error) {
	workspaces, err := s.ListWorkspaces()
	if err != nil {
		return nil, err
	}
	for i := range workspaces {
		workspacePath, err := normalizeWorkspacePath(workspaces[i].Path)
		if err != nil {
			continue
		}
		if workspacePath == path {
			return &workspaces[i], nil
		}
	}
	return nil, nil
}

// FindWorkspaceByName returns a workspace only when its name is unambiguous.
func (s *Store) FindWorkspaceByName(name string) (*LocalWorkspace, error) {
	workspaces, err := s.FindWorkspacesByName(name)
	if err != nil {
		return nil, err
	}
	if len(workspaces) > 1 {
		return nil, fmt.Errorf("multiple workspaces named %q; use a path or id::<uuid>", name)
	}
	if len(workspaces) == 0 {
		return nil, nil
	}
	return &workspaces[0], nil
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
	if err := normalizeLocalWorkspacePath(ws); err != nil {
		return err
	}
	return s.db.Create(ws).Error
}

// SaveWorkspace updates an existing workspace record.
func (s *Store) SaveWorkspace(ws *LocalWorkspace) error {
	if err := normalizeLocalWorkspacePath(ws); err != nil {
		return err
	}
	return s.db.Save(ws).Error
}

// DeleteWorkspace removes a workspace by ID (hard delete).
func (s *Store) DeleteWorkspace(id uuid.UUID) error {
	return s.db.Unscoped().Where("id = ?", id).Delete(&LocalWorkspace{}).Error
}

func normalizeLocalWorkspacePath(ws *LocalWorkspace) error {
	normalizedPath, err := normalizeWorkspacePath(ws.Path)
	if err != nil {
		return fmt.Errorf("normalizing workspace path: %w", err)
	}
	ws.Path = normalizedPath
	return nil
}

func normalizeWorkspacePath(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if resolvedPath, err := filepath.EvalSymlinks(absPath); err == nil {
		absPath = resolvedPath
	}
	return filepath.Clean(absPath), nil
}

func uniquePaths(paths ...string) []string {
	seen := make(map[string]struct{}, len(paths))
	unique := make([]string, 0, len(paths))
	for _, path := range paths {
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		unique = append(unique, path)
	}
	return unique
}

// RenameWorkspace changes only the label of a settled workspace with a stable
// path. The store is SQLite: acquire its writer lock before reading state so
// another connection cannot admit a job between the checks and the rename.
func (s *Store) RenameWorkspace(id uuid.UUID, name string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&LocalWorkspace{}).Where("id = ?", id).UpdateColumn("name", gorm.Expr("name"))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		var ws LocalWorkspace
		if err := tx.First(&ws, "id = ?", id).Error; err != nil {
			return err
		}
		if !filepath.IsAbs(ws.Path) {
			return fmt.Errorf("workspace has no stable absolute path; resolve its path before renaming")
		}
		if ws.Status != "ready" && ws.Status != "failed" {
			return fmt.Errorf("cannot rename workspace while status is %q", ws.Status)
		}
		// CLI-only stores have no jobs table; desktop/server stores do.
		if tx.Migrator().HasTable("jobs") {
			var count int64
			if err := tx.Table("jobs").Where("workspace_id = ? AND status IN ?", id, []string{"pending", "running"}).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return fmt.Errorf("cannot rename workspace while jobs are pending or running")
			}
		}
		return tx.Model(&LocalWorkspace{}).Where("id = ?", id).Update("name", name).Error
	})
}
