package store

import (
	"fmt"

	"github.com/google/uuid"
)

// CreatePublication creates a new local publication record.
func (s *Store) CreatePublication(pub *LocalPublication) error {
	return s.db.Create(pub).Error
}

// ListPublications returns all local publications, most recent first.
func (s *Store) ListPublications() ([]LocalPublication, error) {
	var pubs []LocalPublication
	if err := s.db.Order("created_at DESC").Find(&pubs).Error; err != nil {
		return nil, fmt.Errorf("listing publications: %w", err)
	}
	return pubs, nil
}

// ListPublicationsByWorkspace returns publications for a specific workspace.
func (s *Store) ListPublicationsByWorkspace(workspaceID uuid.UUID) ([]LocalPublication, error) {
	var pubs []LocalPublication
	if err := s.db.Where("workspace_id = ?", workspaceID).Order("created_at DESC").Find(&pubs).Error; err != nil {
		return nil, fmt.Errorf("listing publications: %w", err)
	}
	return pubs, nil
}
