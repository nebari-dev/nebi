package local

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// RenameProject changes the project's name. Only the row changes: Path, the
// directory name and [workspace].name in pixi.toml are left alone (changing
// pixi.toml would need a new version). Links to server projects use the
// remote project ID, so they keep working.
func (s *Service) RenameProject(ctx context.Context, id uuid.UUID, name string) (*Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, err := s.loadProject(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.ValidateProjectName(ctx, name, &id); err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&Project{}).Where("id = ?", id).Update("name", name).Error; err != nil {
		return nil, fmt.Errorf("renaming project: %w", uniqueViolation(err, &Project{Name: name}))
	}
	p.Name = name
	return p, nil
}
