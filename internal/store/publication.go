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

// ListPublicationsByProject returns publications for a specific project.
func (s *Store) ListPublicationsByProject(projectID uuid.UUID) ([]LocalPublication, error) {
	var pubs []LocalPublication
	if err := s.db.Where("project_id = ?", projectID).Order("created_at DESC").Find(&pubs).Error; err != nil {
		return nil, fmt.Errorf("listing publications: %w", err)
	}
	return pubs, nil
}
