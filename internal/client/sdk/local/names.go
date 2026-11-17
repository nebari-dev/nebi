package local

import (
	"context"
	"fmt"
	"regexp"

	"github.com/google/uuid"
)

// projectNamePattern is the project name rule: lowercase letters, digits,
// '.', '_' and '-', starting with a letter or digit, 1 to 63 characters.
//
// The name is also the default directory name, [workspace].name, the
// server project name on first push and part of the default OCI repository
// (<name>-<id8>), and OCI repository components must be lowercase.
var projectNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,62}$`)

// checkNameFormat applies the name rule without looking at the database.
func checkNameFormat(name string) error {
	if !projectNamePattern.MatchString(name) {
		return fmt.Errorf("%w: %q: use 1-63 lowercase letters, digits, '.', '_' or '-', starting with a letter or digit", ErrInvalidName, name)
	}
	return nil
}

// ValidateProjectName checks name against the project name rule and returns
// ErrNameTaken when another project already uses it, ignoring case. Pass
// the project's own ID as excludeID when renaming, so a project does not
// collide with itself.
func (s *Service) ValidateProjectName(ctx context.Context, name string, excludeID *uuid.UUID) error {
	if err := checkNameFormat(name); err != nil {
		return err
	}
	q := s.db.WithContext(ctx).Model(&Project{}).Where("name = ? COLLATE NOCASE", name)
	if excludeID != nil {
		q = q.Where("id <> ?", *excludeID)
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return fmt.Errorf("checking project name: %w", err)
	}
	if count > 0 {
		return fmt.Errorf("%w: %q", ErrNameTaken, name)
	}
	return nil
}
