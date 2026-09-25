package store

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ListProjects returns all projects.
func (s *Store) ListProjects() ([]LocalProject, error) {
	var projects []LocalProject
	if err := s.db.Find(&projects).Error; err != nil {
		return nil, fmt.Errorf("listing projects: %w", err)
	}
	return projects, nil
}

// GetProject returns a project by ID.
func (s *Store) GetProject(id uuid.UUID) (*LocalProject, error) {
	var project LocalProject
	if err := s.db.Where("id = ?", id).First(&project).Error; err != nil {
		return nil, fmt.Errorf("getting project: %w", err)
	}
	return &project, nil
}

// FindProjectByPath returns the project at the given path, or nil if not found.
func (s *Store) FindProjectByPath(path string) (*LocalProject, error) {
	normalizedPath, err := normalizeProjectPath(path)
	if err != nil {
		return nil, fmt.Errorf("normalizing project path: %w", err)
	}

	var project LocalProject
	result := s.db.Where("path IN ?", uniquePaths(path, normalizedPath)).First(&project)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return s.findProjectByNormalizedPath(normalizedPath)
		}
		return nil, fmt.Errorf("finding project by path: %w", result.Error)
	}
	return &project, nil
}

func (s *Store) findProjectByNormalizedPath(path string) (*LocalProject, error) {
	projects, err := s.ListProjects()
	if err != nil {
		return nil, err
	}
	for i := range projects {
		projectPath, err := normalizeProjectPath(projects[i].Path)
		if err != nil {
			continue
		}
		if projectPath == path {
			return &projects[i], nil
		}
	}
	return nil, nil
}

// FindProjectByName returns the first project with the given name, or nil if not found.
func (s *Store) FindProjectByName(name string) (*LocalProject, error) {
	var project LocalProject
	result := s.db.Where("name = ?", name).First(&project)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("finding project by name: %w", result.Error)
	}
	return &project, nil
}

// FindProjectsByName returns all projects with the given name.
// Multiple projects can share the same name since path is the unique identifier.
func (s *Store) FindProjectsByName(name string) ([]LocalProject, error) {
	var projects []LocalProject
	if err := s.db.Where("name = ?", name).Find(&projects).Error; err != nil {
		return nil, fmt.Errorf("finding projects by name: %w", err)
	}
	return projects, nil
}

// CreateProject creates a new project record.
func (s *Store) CreateProject(project *LocalProject) error {
	if project.ID == uuid.Nil {
		project.ID = uuid.New()
	}
	if project.Status == "" {
		project.Status = "ready"
	}
	if project.Source == "" {
		project.Source = "local"
	}
	if err := normalizeLocalProjectPath(project); err != nil {
		return err
	}
	return s.db.Create(project).Error
}

// SaveProject updates an existing project record.
func (s *Store) SaveProject(project *LocalProject) error {
	if err := normalizeLocalProjectPath(project); err != nil {
		return err
	}
	return s.db.Save(project).Error
}

// DeleteProject removes a project by ID (hard delete).
func (s *Store) DeleteProject(id uuid.UUID) error {
	return s.db.Unscoped().Where("id = ?", id).Delete(&LocalProject{}).Error
}

func normalizeLocalProjectPath(project *LocalProject) error {
	normalizedPath, err := normalizeProjectPath(project.Path)
	if err != nil {
		return fmt.Errorf("normalizing project path: %w", err)
	}
	project.Path = normalizedPath
	return nil
}

func normalizeProjectPath(path string) (string, error) {
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
