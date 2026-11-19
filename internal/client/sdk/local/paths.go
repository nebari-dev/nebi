package local

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// defaultProjectsDirName is the projects directory under the home directory.
const defaultProjectsDirName = "nebi-projects"

// DefaultProjectPath returns <projects_dir>/<name>, the directory a project
// gets when it is created without a path. projects_dir is
// Options.ProjectsDir, else ~/nebi-projects.
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

// sameDir reports whether a and b are the same directory: equal paths, or
// existing paths that are the same file on disk. Comparing files rather
// than strings also matches paths that differ only in case on a
// case-insensitive filesystem (the default on macOS and Windows).
//
// A path that does not exist is a different directory. Any other error
// (for example a directory on the way that cannot be read) is returned,
// so a caller guarding another project's files can refuse rather than
// assume the two are unrelated.
func sameDir(a, b string) (bool, error) {
	if a == b {
		return true, nil
	}
	ai, err := statDir(a)
	if ai == nil || err != nil {
		return false, err
	}
	bi, err := statDir(b)
	if bi == nil || err != nil {
		return false, err
	}
	return os.SameFile(ai, bi), nil
}

// inside reports whether p is strictly inside dir: by path, or because an
// existing ancestor of p is the same file on disk as dir. Like sameDir it
// matches paths that differ only in case, and paths that reach dir through
// a symlink, including p itself being a symlink to a directory inside dir.
// Both must be clean absolute paths.
//
// Like sameDir, it returns an error when a path cannot be checked for a
// reason other than not existing, instead of reporting p as outside dir.
// A path that cannot be checked does not end the check, since another of
// p's ancestors can still match: the first error is returned only when
// nothing matched.
func inside(dir, p string) (bool, error) {
	if isWithin(dir, p) {
		return true, nil
	}
	di, err := statDir(dir)
	if di == nil || err != nil {
		return false, err
	}
	var firstErr error
	starts := []string{filepath.Dir(p)}
	// The walk below starts at p's parent, so it never follows p itself.
	// A stored project path can be a symlink by now (the user moved the
	// directory and left a link behind), so check where p points too.
	resolved, err := filepath.EvalSymlinks(p)
	switch {
	case err != nil && !missing(err):
		firstErr = fmt.Errorf("resolving %s: %w", p, err)
	case err == nil && resolved != p:
		if isWithin(dir, resolved) {
			return true, nil
		}
		starts = append(starts, filepath.Dir(resolved))
	}
	for _, start := range starts {
		for cur := start; ; cur = filepath.Dir(cur) {
			ci, err := statDir(cur)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
			} else if ci != nil && os.SameFile(di, ci) {
				return true, nil
			}
			if filepath.Dir(cur) == cur {
				break
			}
		}
	}
	return false, firstErr
}

// statDir stats p. It returns nil info and no error when p does not exist,
// and the error for any other failure.
func statDir(p string) (fs.FileInfo, error) {
	info, err := os.Stat(p)
	if err == nil {
		return info, nil
	}
	if missing(err) {
		return nil, nil
	}
	return nil, fmt.Errorf("checking %s: %w", p, err)
}

// missing reports whether err means the path does not exist: no such file,
// or a component of the path that is a file rather than a directory.
func missing(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR)
}

// isWithin reports whether p is strictly inside dir, comparing paths only.
// Both must be clean absolute paths.
func isWithin(dir, p string) bool {
	rel, err := filepath.Rel(dir, p)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
