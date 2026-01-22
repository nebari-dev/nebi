package diff

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// lockFile represents the structure of a pixi.lock (YAML) file.
// We only parse what we need for package comparison.
type lockFile struct {
	Version  int                `yaml:"version"`
	Packages lockFilePackages   `yaml:"packages"`
}

// lockFilePackages handles both the conda and pypi package formats in pixi.lock.
type lockFilePackages struct {
	Conda []lockPackage `yaml:"conda,omitempty"`
	Pypi  []lockPackage `yaml:"pypi,omitempty"`
}

// lockPackage represents a package entry in the lock file.
type lockPackage struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
	Source  string `yaml:"source,omitempty"`
	Channel string `yaml:"channel,omitempty"`
}

// packageKey uniquely identifies a package across sources.
type packageKey struct {
	Name   string
	Source string // "conda" or "pypi"
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

	var lf lockFile
	if err := yaml.Unmarshal(content, &lf); err != nil {
		// Try alternate format: flat list of packages
		return parseFlatLockPackages(content)
	}

	packages := make(map[string]string)

	for _, pkg := range lf.Packages.Conda {
		if pkg.Name != "" {
			packages[pkg.Name] = pkg.Version
		}
	}
	for _, pkg := range lf.Packages.Pypi {
		if pkg.Name != "" {
			// Prefer conda version if both exist; pypi suffix for disambiguation
			key := pkg.Name
			if _, exists := packages[key]; exists {
				key = pkg.Name + " (pypi)"
			}
			packages[key] = pkg.Version
		}
	}

	// If structured parsing yielded nothing, try flat format
	if len(packages) == 0 {
		return parseFlatLockPackages(content)
	}

	return packages, nil
}

// parseFlatLockPackages handles lock files with a flat package list format.
func parseFlatLockPackages(content []byte) (map[string]string, error) {
	// Try parsing as a map with package entries directly
	type flatLock struct {
		Packages []struct {
			Name    string `yaml:"name"`
			Version string `yaml:"version"`
		} `yaml:"packages"`
	}

	var fl flatLock
	if err := yaml.Unmarshal(content, &fl); err != nil {
		return make(map[string]string), nil
	}

	packages := make(map[string]string)
	for _, pkg := range fl.Packages {
		if pkg.Name != "" {
			packages[pkg.Name] = pkg.Version
		}
	}
	return packages, nil
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
