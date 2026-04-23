package oci

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// windowsReserved are base names rejected on Windows filesystems regardless
// of extension. Matching is case-insensitive.
var windowsReserved = map[string]struct{}{
	"CON": {}, "PRN": {}, "AUX": {}, "NUL": {},
	"COM1": {}, "COM2": {}, "COM3": {}, "COM4": {}, "COM5": {},
	"COM6": {}, "COM7": {}, "COM8": {}, "COM9": {},
	"LPT1": {}, "LPT2": {}, "LPT3": {}, "LPT4": {}, "LPT5": {},
	"LPT6": {}, "LPT7": {}, "LPT8": {}, "LPT9": {},
}

var controlCharRE = regexp.MustCompile(`[\x00-\x1f\x7f]`)

// validateAssetPath checks a bundle asset's relative path for safety.
// Rules: no absolute paths, no .. segments, no null/control chars, not a
// reserved core name (pixi.toml / pixi.lock at root), no Windows-hostile
// names, no trailing dot or space on any segment.
//
// Returns a descriptive error on rejection; nil on success. The returned
// error is meant to be wrapped by the caller with the path prefix.
func validateAssetPath(p string) error {
	if p == "" {
		return fmt.Errorf("empty path")
	}
	if controlCharRE.MatchString(p) {
		return fmt.Errorf("contains null or control character")
	}
	// Reject Windows drive letters (C:\foo) and UNC.
	if len(p) >= 2 && p[1] == ':' {
		return fmt.Errorf("absolute path not allowed")
	}
	// Absolute paths (POSIX + Windows backslash form).
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`) {
		return fmt.Errorf("absolute path not allowed")
	}
	// Normalize separators for segment inspection — reject backslash since
	// bundle paths are POSIX.
	if strings.Contains(p, `\`) {
		return fmt.Errorf("backslash separator not allowed")
	}

	cleaned := path.Clean(p)
	if cleaned != p {
		// Non-canonical: either has trailing slash, duplicate slashes, or
		// redundant segments. Allow trailing slash (treat as dir) — but
		// assets should be files. Reject here.
		return fmt.Errorf("non-canonical path")
	}

	// Split and inspect each segment.
	for _, seg := range strings.Split(cleaned, "/") {
		if seg == "" {
			return fmt.Errorf("empty segment")
		}
		if seg == ".." {
			return fmt.Errorf("parent segment not allowed")
		}
		if seg == "." {
			return fmt.Errorf("current segment not allowed")
		}
		if strings.HasSuffix(seg, ".") || strings.HasSuffix(seg, " ") {
			return fmt.Errorf("segment %q ends with dot or space", seg)
		}
		// Windows reserved name check — strip extension.
		base := seg
		if i := strings.Index(seg, "."); i >= 0 {
			base = seg[:i]
		}
		if _, reserved := windowsReserved[strings.ToUpper(base)]; reserved {
			return fmt.Errorf("segment %q is a reserved Windows name", seg)
		}
	}

	// Reject reserved root paths case-insensitively. Case-insensitive
	// filesystems (Windows, default macOS) collapse `Pixi.toml` onto the
	// core `pixi.toml` written at extract; the exact-match check misses
	// those. Subdirectory variants are still allowed.
	switch normalizeForCollision(cleaned) {
	case "pixi.toml", "pixi.lock":
		return fmt.Errorf("path %q collides with core layer", cleaned)
	}
	return nil
}

// normalizeForCollision returns a lowercased NFC form of p for collision
// detection within a bundle. Case-insensitive filesystems (macOS, Windows)
// treat differently-cased paths as the same file, so we must reject them
// before publish.
func normalizeForCollision(p string) string {
	return strings.ToLower(norm.NFC.String(p))
}

// validateAssetPaths runs validateAssetPath on each path and also checks
// for case-insensitive collisions across the set. Returns the first error
// encountered.
func validateAssetPaths(paths []string) error {
	seen := make(map[string]string, len(paths))
	for _, p := range paths {
		if err := validateAssetPath(p); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		key := normalizeForCollision(p)
		if prev, dup := seen[key]; dup {
			return fmt.Errorf("%s: case-insensitive collision with %s", p, prev)
		}
		seen[key] = p
	}
	return nil
}
