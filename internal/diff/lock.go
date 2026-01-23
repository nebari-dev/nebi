package diff

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// CompareLock compares two pixi.lock file contents and produces a LockSummary.
// It parses the YAML structure and identifies added, removed, and updated packages.
func CompareLock(oldContent, newContent []byte) (*LockSummary, error) {
	oldPkgs, err := parseLockPackages(oldContent)
	if err != nil {
		// Fall back to simple byte comparison if parsing fails
		return simpleLockSummary(oldContent, newContent), nil
	}

	newPkgs, err := parseLockPackages(newContent)
	if err != nil {
		return simpleLockSummary(oldContent, newContent), nil
	}

	return diffPackages(oldPkgs, newPkgs), nil
}

// parseLockPackages extracts a deduplicated map of packages from lock file content.
// It handles pixi.lock v6 format (flat list with conda:/pypi: URL keys) as well
// as simpler formats with explicit name/version fields.
func parseLockPackages(content []byte) (map[string]string, error) {
	if len(content) == 0 {
		return make(map[string]string), nil
	}

	// Parse as generic YAML to inspect the structure
	var raw map[string]interface{}
	if err := yaml.Unmarshal(content, &raw); err != nil {
		return make(map[string]string), fmt.Errorf("failed to parse lock YAML: %w", err)
	}

	// Try v6 format: packages is a flat list with conda:/pypi: URL entries
	packages := parseV6Packages(content)
	if len(packages) > 0 {
		return packages, nil
	}

	// Fallback: try older format with packages.conda[] / packages.pypi[] sub-keys
	packages = parseLegacyPackages(content)
	if len(packages) > 0 {
		return packages, nil
	}

	// Fallback: flat list with explicit name/version fields
	packages = parseFlatNameVersionPackages(content)
	return packages, nil
}

// parseV6Packages parses pixi.lock v6 format where packages is a flat list
// and each entry has either a "conda: <url>" or "pypi: <url>" key.
//
// Conda entries encode name/version in the URL filename (e.g., numpy-2.4.1-py314h...conda).
// PyPI entries have explicit "name" and "version" fields.
func parseV6Packages(content []byte) map[string]string {
	// Parse just the packages list as a sequence of maps
	type v6Lock struct {
		Packages []map[string]interface{} `yaml:"packages"`
	}

	var lf v6Lock
	if err := yaml.Unmarshal(content, &lf); err != nil {
		return nil
	}

	packages := make(map[string]string)
	for _, entry := range lf.Packages {
		name, version := extractV6Package(entry)
		if name != "" {
			// Deduplicate: first occurrence wins (packages can appear for multiple platforms)
			if _, exists := packages[name]; !exists {
				packages[name] = version
			}
		}
	}
	return packages
}

// extractV6Package extracts name and version from a v6 package entry.
func extractV6Package(entry map[string]interface{}) (name, version string) {
	// Check for PyPI entry (has explicit name and version fields)
	if _, hasPypi := entry["pypi"]; hasPypi {
		if n, ok := entry["name"].(string); ok {
			name = n
		}
		if v, ok := entry["version"].(string); ok {
			version = v
		}
		return name, version
	}

	// Check for conda entry (name/version encoded in URL filename)
	if condaURL, hasConda := entry["conda"]; hasConda {
		urlStr, ok := condaURL.(string)
		if !ok {
			return "", ""
		}
		return parseCondaFilename(urlStr)
	}

	return "", ""
}

// parseCondaFilename extracts package name and version from a conda URL.
// Conda filenames follow the pattern: name-version-build.conda or name-version-build.tar.bz2
// Examples:
//   - https://conda.anaconda.org/conda-forge/linux-64/numpy-2.4.1-py314h2b28147_0.conda
//   - https://conda.anaconda.org/conda-forge/noarch/pip-25.0.1-pyh8b19718_0.conda
//   - https://conda.anaconda.org/conda-forge/linux-64/libgcc-14.2.0-h767d61c_2.conda
func parseCondaFilename(url string) (name, version string) {
	// Get the filename from the URL path
	filename := path.Base(url)

	// Strip extension (.conda or .tar.bz2)
	if strings.HasSuffix(filename, ".tar.bz2") {
		filename = strings.TrimSuffix(filename, ".tar.bz2")
	} else if strings.HasSuffix(filename, ".conda") {
		filename = strings.TrimSuffix(filename, ".conda")
	} else {
		return "", ""
	}

	// Split on "-" to find name-version-build
	// The challenge: package names can contain hyphens (e.g., "libgcc-ng")
	// Strategy: split from the right. Build is the last segment, version is second-to-last,
	// everything else is the name. Version segments start with a digit.
	parts := strings.Split(filename, "-")
	if len(parts) < 3 {
		return "", ""
	}

	// Find the version: scan from right, skip the build string (last part),
	// then find the first part that looks like a version (starts with digit)
	buildIdx := len(parts) - 1
	versionIdx := -1
	for i := buildIdx - 1; i >= 1; i-- {
		if len(parts[i]) > 0 && parts[i][0] >= '0' && parts[i][0] <= '9' {
			versionIdx = i
			break
		}
	}

	if versionIdx < 1 {
		return "", ""
	}

	name = strings.Join(parts[:versionIdx], "-")
	version = parts[versionIdx]
	return name, version
}

// parseLegacyPackages handles the older format with packages.conda[] / packages.pypi[] sub-keys.
func parseLegacyPackages(content []byte) map[string]string {
	type legacyLock struct {
		Packages struct {
			Conda []struct {
				Name    string `yaml:"name"`
				Version string `yaml:"version"`
			} `yaml:"conda,omitempty"`
			Pypi []struct {
				Name    string `yaml:"name"`
				Version string `yaml:"version"`
			} `yaml:"pypi,omitempty"`
		} `yaml:"packages"`
	}

	var lf legacyLock
	if err := yaml.Unmarshal(content, &lf); err != nil {
		return nil
	}

	packages := make(map[string]string)
	for _, pkg := range lf.Packages.Conda {
		if pkg.Name != "" {
			packages[pkg.Name] = pkg.Version
		}
	}
	for _, pkg := range lf.Packages.Pypi {
		if pkg.Name != "" {
			key := pkg.Name
			if _, exists := packages[key]; exists {
				key = pkg.Name + " (pypi)"
			}
			packages[key] = pkg.Version
		}
	}
	return packages
}

// parseFlatNameVersionPackages handles a flat list with explicit name/version fields.
func parseFlatNameVersionPackages(content []byte) map[string]string {
	type flatLock struct {
		Packages []struct {
			Name    string `yaml:"name"`
			Version string `yaml:"version"`
		} `yaml:"packages"`
	}

	var fl flatLock
	if err := yaml.Unmarshal(content, &fl); err != nil {
		return make(map[string]string)
	}

	packages := make(map[string]string)
	for _, pkg := range fl.Packages {
		if pkg.Name != "" {
			packages[pkg.Name] = pkg.Version
		}
	}
	return packages
}

// diffPackages compares two package maps and produces a LockSummary.
func diffPackages(oldPkgs, newPkgs map[string]string) *LockSummary {
	summary := &LockSummary{}

	// Find removed and updated packages
	for name, oldVer := range oldPkgs {
		newVer, exists := newPkgs[name]
		if !exists {
			summary.PackagesRemoved++
			display := name
			if oldVer != "" {
				display += " " + oldVer
			}
			summary.Removed = append(summary.Removed, display)
		} else if oldVer != newVer {
			summary.PackagesUpdated++
			summary.Updated = append(summary.Updated, PackageUpdate{
				Name:       name,
				OldVersion: oldVer,
				NewVersion: newVer,
			})
		}
	}

	// Find added packages
	for name, newVer := range newPkgs {
		if _, exists := oldPkgs[name]; !exists {
			summary.PackagesAdded++
			display := name
			if newVer != "" {
				display += " " + newVer
			}
			summary.Added = append(summary.Added, display)
		}
	}

	// Sort for deterministic output
	sort.Strings(summary.Added)
	sort.Strings(summary.Removed)
	sort.Slice(summary.Updated, func(i, j int) bool {
		return summary.Updated[i].Name < summary.Updated[j].Name
	})

	return summary
}

// simpleLockSummary returns a basic summary when lock file parsing fails.
func simpleLockSummary(oldContent, newContent []byte) *LockSummary {
	if string(oldContent) == string(newContent) {
		return &LockSummary{}
	}
	// We can't determine specifics, just indicate there are changes
	return &LockSummary{
		PackagesUpdated: -1, // Sentinel: unknown number of changes
	}
}

// FormatLockDiffText formats a LockSummary as detailed text with package lists.
func FormatLockDiffText(summary *LockSummary) string {
	if summary == nil {
		return ""
	}

	total := summary.PackagesAdded + summary.PackagesRemoved + summary.PackagesUpdated
	if total == 0 {
		return "  pixi.lock: no package changes\n"
	}

	// Handle sentinel value from simpleLockSummary
	if summary.PackagesUpdated == -1 {
		return "  pixi.lock: changed (unable to parse package details)\n"
	}

	var sb strings.Builder
	sb.WriteString("@@ pixi.lock @@\n")

	if summary.PackagesAdded > 0 {
		sb.WriteString(formatAddedSection(summary))
	}
	if summary.PackagesRemoved > 0 {
		sb.WriteString(formatRemovedSection(summary))
	}
	if summary.PackagesUpdated > 0 {
		sb.WriteString(formatUpdatedSection(summary))
	}

	sb.WriteString("\n")
	sb.WriteString(formatSummaryLine(summary))

	return sb.String()
}

func formatAddedSection(summary *LockSummary) string {
	var sb strings.Builder
	for _, pkg := range summary.Added {
		sb.WriteString("+" + pkg + "\n")
	}
	return sb.String()
}

func formatRemovedSection(summary *LockSummary) string {
	var sb strings.Builder
	for _, pkg := range summary.Removed {
		sb.WriteString("-" + pkg + "\n")
	}
	return sb.String()
}

func formatUpdatedSection(summary *LockSummary) string {
	var sb strings.Builder
	for _, u := range summary.Updated {
		sb.WriteString("-" + u.Name + " " + u.OldVersion + "\n")
		sb.WriteString("+" + u.Name + " " + u.NewVersion + "\n")
	}
	return sb.String()
}

func formatSummaryLine(summary *LockSummary) string {
	parts := []string{}
	if summary.PackagesAdded > 0 {
		parts = append(parts, pluralize(summary.PackagesAdded, "package", "packages")+" added")
	}
	if summary.PackagesRemoved > 0 {
		parts = append(parts, pluralize(summary.PackagesRemoved, "package", "packages")+" removed")
	}
	if summary.PackagesUpdated > 0 {
		parts = append(parts, pluralize(summary.PackagesUpdated, "package", "packages")+" updated")
	}
	return strings.Join(parts, ", ") + "\n"
}

func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return "1 " + singular
	}
	return fmt.Sprintf("%d %s", n, plural)
}
