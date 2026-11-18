package local

import (
	"context"
	"fmt"
	"regexp"

	"github.com/google/uuid"
)

// projectNamePattern is the OCI distribution spec's rule for a repository
// path component: lowercase letters and digits, in runs separated by one
// '.', one '_', two '_' or any number of '-'. So a name starts and ends
// with a letter or digit, and separators never meet ("a..b", "a._b" and
// "a___b" are refused).
//
// The name is also the default directory name, [workspace].name, the
// server project name on first push and part of the default OCI repository
// (<name>-<id8>), so it has to be a valid repository component.
var projectNamePattern = regexp.MustCompile(`^[a-z0-9]+(?:(?:[._]|__|-+)[a-z0-9]+)*$`)

// maxProjectNameLen caps names at 63 characters on top of the OCI rule,
// which sets no per-component limit.
const maxProjectNameLen = 63

// checkNameFormat applies the name rule without looking at the database.
func checkNameFormat(name string) error {
	if len(name) > maxProjectNameLen || !projectNamePattern.MatchString(name) {
		return fmt.Errorf("%w: %q: use up to 63 lowercase letters and digits, separated by '.', '_', '__' or '-', starting and ending with a letter or digit", ErrInvalidName, name)
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
