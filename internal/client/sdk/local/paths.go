package local

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ProjectsDirEnv overrides the default projects directory.
const ProjectsDirEnv = "NEBI_PROJECTS_DIR"

// defaultProjectsDirName is the projects directory under the home directory.
const defaultProjectsDirName = "nebi-projects"

// DefaultProjectPath returns <projects_dir>/<name>, the directory a project
// gets when it is created without a path. projects_dir is
// Options.ProjectsDir, else $NEBI_PROJECTS_DIR, else ~/nebi-projects.
func (s *Service) DefaultProjectPath(name string) (string, error) {
	if err := checkNameFormat(name); err != nil {
		return "", err
	}
	dir, err := s.resolvedProjectsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

// resolvedProjectsDir returns the absolute, symlink-resolved projects
// directory.
func (s *Service) resolvedProjectsDir() (string, error) {
	dir := s.projectsDir
	if dir == "" {
		dir = os.Getenv(ProjectsDirEnv)
	}
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("finding the home directory for the default projects directory: %w", err)
		}
		dir = filepath.Join(home, defaultProjectsDirName)
	}
	return normalizePath(dir)
}

// normalizePath expands a leading ~, makes p absolute and resolves symlinks
// in the part of the path that exists.
func normalizePath(p string) (string, error) {
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expanding ~: %w", err)
		}
		p = filepath.Join(home, p[1:])
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("resolving %s: %w", p, err)
	}
	return resolveSymlinks(abs)
}

// resolveSymlinks resolves symlinks in the longest existing prefix of the
// absolute path p and appends the part that does not exist yet, so a
// directory that is about to be created gets the same path it will have
// once it exists.
func resolveSymlinks(p string) (string, error) {
	p = filepath.Clean(p)
	var missing []string
	cur := p
	for {
		resolved, err := filepath.EvalSymlinks(cur)
		if err == nil {
			return filepath.Join(append([]string{resolved}, missing...)...), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("resolving %s: %w", p, err)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return p, nil
		}
		missing = append([]string{filepath.Base(cur)}, missing...)
		cur = parent
	}
}

// isWithin reports whether p is strictly inside dir. Both must be clean
// absolute paths.
func isWithin(dir, p string) bool {
	rel, err := filepath.Rel(dir, p)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
