package oci

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// AssetFile is a single file to include in a bundle. RelPath is the
// forward-slash path relative to the bundle root. AbsPath is the resolved
// path on disk.
type AssetFile struct {
	RelPath string
	AbsPath string
	Size    int64
}

// hardcodedDropDirs are directories always excluded from a bundle — they
// either contain VCS metadata or local build/environment state that must
// not leak into shared artifacts.
var hardcodedDropDirs = map[string]struct{}{
	".git":  {},
	".pixi": {},
}

// forceIncludeFiles always land in the bundle regardless of include /
// exclude filters. These are the bundle's core.
var forceIncludeFiles = map[string]struct{}{
	"pixi.toml": {},
	"pixi.lock": {},
}

// gitignorePattern is one parsed .gitignore rule.
type gitignorePattern struct {
	raw      string
	negate   bool // leading !
	dirOnly  bool // trailing /
	anchored bool // leading /, or contains / (non-trailing) → match from root
	pattern  string
}

// parseGitignore reads a .gitignore file, returning the list of patterns
// in the order they appear. Comments and blank lines are skipped.
// Semantics supported: negation (!), anchored (/prefix or slash in middle),
// dir-only (trailing /), and standard glob including ** via doublestar.
func parseGitignore(path string) ([]gitignorePattern, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var rules []gitignorePattern
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		// Preserve trailing spaces that aren't escaped — gitignore spec
		// requires escaping, but few users do. We trim both sides.
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		p := gitignorePattern{raw: line}
		if strings.HasPrefix(line, "!") {
			p.negate = true
			line = line[1:]
		}
		if strings.HasSuffix(line, "/") {
			p.dirOnly = true
			line = strings.TrimSuffix(line, "/")
		}
		// Pattern is anchored if it begins with / or contains a / that
		// is not the last char.
		if strings.HasPrefix(line, "/") {
			p.anchored = true
			line = strings.TrimPrefix(line, "/")
		} else if strings.Contains(line, "/") {
			p.anchored = true
		}
		p.pattern = line
		rules = append(rules, p)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return rules, nil
}

// matches reports whether p (a forward-slash relative path) is ignored by
// the given rules. Later rules override earlier ones (standard gitignore
// semantics). isDir signals whether the path is a directory.
func gitignoreMatches(rules []gitignorePattern, p string, isDir bool) bool {
	ignored := false
	for _, r := range rules {
		if r.dirOnly && !isDir {
			continue
		}
		if matchRule(r, p) {
			ignored = !r.negate
		}
	}
	return ignored
}

func matchRule(r gitignorePattern, p string) bool {
	if r.anchored {
		// Match pattern against the whole path or any ancestor directory
		// prefix — a rule like "build/" should ignore "build/foo" too.
		if ok, _ := doublestar.Match(r.pattern, p); ok {
			return true
		}
		// Also try matching against any ancestor dir to cover the case
		// where the pattern describes a dir and we're looking at a file
		// inside it.
		parts := strings.Split(p, "/")
		for i := 1; i < len(parts); i++ {
			prefix := strings.Join(parts[:i], "/")
			if ok, _ := doublestar.Match(r.pattern, prefix); ok {
				return true
			}
		}
		return false
	}
	// Unanchored: match against base name or any suffix of the path.
	if ok, _ := doublestar.Match(r.pattern, filepath.Base(p)); ok {
		return true
	}
	parts := strings.Split(p, "/")
	for i := range parts {
		suffix := strings.Join(parts[i:], "/")
		if ok, _ := doublestar.Match(r.pattern, suffix); ok {
			return true
		}
		// Also check each intermediate dir segment on its own.
		if ok, _ := doublestar.Match(r.pattern, parts[i]); ok {
			return true
		}
	}
	return false
}

// matchAnyGlob reports whether p matches any glob in patterns.
// Empty patterns slice → false. Uses doublestar syntax.
func matchAnyGlob(patterns []string, p string) bool {
	for _, g := range patterns {
		if ok, _ := doublestar.Match(g, p); ok {
			return true
		}
		// Also match against base name for naked patterns like "*.log".
		if ok, _ := doublestar.Match(g, filepath.Base(p)); ok {
			return true
		}
	}
	return false
}

// WalkBundle returns the sorted list of files to include in a bundle,
// applying (in order): hardcoded drops, include filter, .gitignore,
// exclude filter, force-includes.
//
// root must be the workspace directory. cfg controls include/exclude.
// Errors surface for unreadable directory entries but not for missing
// .gitignore (absent file is fine).
func WalkBundle(root string, cfg BundleConfig) ([]AssetFile, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve root: %w", err)
	}
	gi, err := parseGitignore(filepath.Join(absRoot, ".gitignore"))
	if err != nil {
		return nil, fmt.Errorf("read .gitignore: %w", err)
	}

	var out []AssetFile
	seen := make(map[string]struct{})

	err = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == absRoot {
			return nil
		}
		relFS, err := filepath.Rel(absRoot, path)
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(relFS)

		// Hardcoded drops at any depth (top-level only per spec wording,
		// but treating .git/.pixi as always-drop at any depth is safer).
		if d.IsDir() {
			if _, drop := hardcodedDropDirs[d.Name()]; drop {
				return filepath.SkipDir
			}
			// gitignore dir-only patterns may elide a whole subtree.
			if gitignoreMatches(gi, rel, true) {
				return filepath.SkipDir
			}
			return nil
		}

		// Regular files only.
		if !d.Type().IsRegular() {
			// Symlinks, devices, fifos — silently skip.
			return nil
		}

		_, forceInc := forceIncludeFiles[rel]
		if !forceInc {
			// include filter: when set, only matching files are candidates.
			if len(cfg.Include) > 0 && !matchAnyGlob(cfg.Include, rel) {
				return nil
			}
			// gitignore drop.
			if gitignoreMatches(gi, rel, false) {
				return nil
			}
			// explicit exclude.
			if matchAnyGlob(cfg.Exclude, rel) {
				return nil
			}
		}

		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", rel, err)
		}
		if _, dup := seen[rel]; dup {
			return nil
		}
		seen[rel] = struct{}{}
		out = append(out, AssetFile{
			RelPath: rel,
			AbsPath: path,
			Size:    info.Size(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Ensure force-included files are present even if filters elided them.
	for name := range forceIncludeFiles {
		if _, ok := seen[name]; ok {
			continue
		}
		abs := filepath.Join(absRoot, name)
		info, err := os.Stat(abs)
		if err != nil {
			continue // caller asserts existence separately
		}
		if !info.Mode().IsRegular() {
			continue
		}
		out = append(out, AssetFile{RelPath: name, AbsPath: abs, Size: info.Size()})
		seen[name] = struct{}{}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].RelPath < out[j].RelPath })
	return out, nil
}
