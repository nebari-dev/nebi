package diff

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// LockSummary represents a summary of lock file changes.
type LockSummary struct {
	PackagesAdded   int             `json:"packages_added"`
	PackagesRemoved int             `json:"packages_removed"`
	PackagesUpdated int             `json:"packages_updated"`
	Added           []string        `json:"added,omitempty"`
	Removed         []string        `json:"removed,omitempty"`
	Updated         []PackageUpdate `json:"updated,omitempty"`
}

// PackageUpdate represents a package version change.
type PackageUpdate struct {
	Name       string `json:"name"`
	OldVersion string `json:"old"`
	NewVersion string `json:"new"`
}

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
func parseLockPackages(content []byte) (map[string]string, error) {
	if len(content) == 0 {
		return make(map[string]string), nil
	}

	var raw map[string]interface{}
	if err := yaml.Unmarshal(content, &raw); err != nil {
		return make(map[string]string), fmt.Errorf("failed to parse lock YAML: %w", err)
	}

	// Try v6 format
	packages := parseV6Packages(content)
	if len(packages) > 0 {
		return packages, nil
	}

	// Fallback: older format with packages.conda[] / packages.pypi[]
	packages = parseLegacyPackages(content)
	if len(packages) > 0 {
		return packages, nil
	}

	// Fallback: flat list with explicit name/version fields
	packages = parseFlatNameVersionPackages(content)
	return packages, nil
}

// parseV6Packages parses pixi.lock v6 format.
func parseV6Packages(content []byte) map[string]string {
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
			if _, exists := packages[name]; !exists {
				packages[name] = version
			}
		}
	}
	return packages
}

func extractV6Package(entry map[string]interface{}) (name, version string) {
	if _, hasPypi := entry["pypi"]; hasPypi {
		if n, ok := entry["name"].(string); ok {
			name = n
		}
		if v, ok := entry["version"].(string); ok {
			version = v
		}
		return name, version
	}

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
func parseCondaFilename(url string) (name, version string) {
	filename := path.Base(url)

	if strings.HasSuffix(filename, ".tar.bz2") {
		filename = strings.TrimSuffix(filename, ".tar.bz2")
	} else if strings.HasSuffix(filename, ".conda") {
		filename = strings.TrimSuffix(filename, ".conda")
	} else {
		return "", ""
	}

	parts := strings.Split(filename, "-")
	if len(parts) < 3 {
		return "", ""
	}

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

func diffPackages(oldPkgs, newPkgs map[string]string) *LockSummary {
	summary := &LockSummary{}

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

	sort.Strings(summary.Added)
	sort.Strings(summary.Removed)
	sort.Slice(summary.Updated, func(i, j int) bool {
		return summary.Updated[i].Name < summary.Updated[j].Name
	})

	return summary
}

func simpleLockSummary(oldContent, newContent []byte) *LockSummary {
	if string(oldContent) == string(newContent) {
		return &LockSummary{}
	}
	return &LockSummary{
		PackagesUpdated: -1,
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

	if summary.PackagesUpdated == -1 {
		return "  pixi.lock: changed (unable to parse package details)\n"
	}

	var sb strings.Builder
	sb.WriteString("@@ pixi.lock @@\n")

	for _, pkg := range summary.Added {
		sb.WriteString("+" + pkg + "\n")
	}
	for _, pkg := range summary.Removed {
		sb.WriteString("-" + pkg + "\n")
	}
	for _, u := range summary.Updated {
		sb.WriteString("-" + u.Name + " " + u.OldVersion + "\n")
		sb.WriteString("+" + u.Name + " " + u.NewVersion + "\n")
	}

	sb.WriteString("\n")

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
	sb.WriteString(strings.Join(parts, ", ") + "\n")

	return sb.String()
}

func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return "1 " + singular
	}
	return fmt.Sprintf("%d %s", n, plural)
}
