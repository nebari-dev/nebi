package diff

import (
	"strings"
	"testing"
)

func TestCompareLock_EmptyFiles(t *testing.T) {
	summary, err := CompareLock([]byte{}, []byte{})
	if err != nil {
		t.Fatalf("CompareLock() error = %v", err)
	}
	if summary.PackagesAdded != 0 || summary.PackagesRemoved != 0 || summary.PackagesUpdated != 0 {
		t.Error("Empty files should have no changes")
	}
}

func TestCompareLock_SameContent(t *testing.T) {
	content := []byte(`
version: 1
packages:
  conda:
    - name: numpy
      version: "1.24.0"
    - name: python
      version: "3.11.0"
`)
	summary, err := CompareLock(content, content)
	if err != nil {
		t.Fatalf("CompareLock() error = %v", err)
	}
	if summary.PackagesAdded != 0 || summary.PackagesRemoved != 0 || summary.PackagesUpdated != 0 {
		t.Error("Same content should have no changes")
	}
}

func TestCompareLock_PackageAdded(t *testing.T) {
	oldContent := []byte(`
version: 1
packages:
  conda:
    - name: numpy
      version: "1.24.0"
`)
	newContent := []byte(`
version: 1
packages:
  conda:
    - name: numpy
      version: "1.24.0"
    - name: scipy
      version: "1.11.0"
`)
	summary, err := CompareLock(oldContent, newContent)
	if err != nil {
		t.Fatalf("CompareLock() error = %v", err)
	}
	if summary.PackagesAdded != 1 {
		t.Errorf("PackagesAdded = %d, want 1", summary.PackagesAdded)
	}
	if len(summary.Added) != 1 || !strings.Contains(summary.Added[0], "scipy") {
		t.Errorf("Added = %v, want scipy", summary.Added)
	}
}

func TestCompareLock_PackageRemoved(t *testing.T) {
	oldContent := []byte(`
version: 1
packages:
  conda:
    - name: numpy
      version: "1.24.0"
    - name: scipy
      version: "1.11.0"
`)
	newContent := []byte(`
version: 1
packages:
  conda:
    - name: numpy
      version: "1.24.0"
`)
	summary, err := CompareLock(oldContent, newContent)
	if err != nil {
		t.Fatalf("CompareLock() error = %v", err)
	}
	if summary.PackagesRemoved != 1 {
		t.Errorf("PackagesRemoved = %d, want 1", summary.PackagesRemoved)
	}
	if len(summary.Removed) != 1 || !strings.Contains(summary.Removed[0], "scipy") {
		t.Errorf("Removed = %v, want scipy", summary.Removed)
	}
}

func TestCompareLock_PackageUpdated(t *testing.T) {
	oldContent := []byte(`
version: 1
packages:
  conda:
    - name: numpy
      version: "1.24.0"
`)
	newContent := []byte(`
version: 1
packages:
  conda:
    - name: numpy
      version: "2.0.0"
`)
	summary, err := CompareLock(oldContent, newContent)
	if err != nil {
		t.Fatalf("CompareLock() error = %v", err)
	}
	if summary.PackagesUpdated != 1 {
		t.Errorf("PackagesUpdated = %d, want 1", summary.PackagesUpdated)
	}
	if len(summary.Updated) != 1 {
		t.Fatalf("Updated length = %d, want 1", len(summary.Updated))
	}
	if summary.Updated[0].Name != "numpy" {
		t.Errorf("Updated[0].Name = %q, want %q", summary.Updated[0].Name, "numpy")
	}
	if summary.Updated[0].OldVersion != "1.24.0" {
		t.Errorf("OldVersion = %q, want %q", summary.Updated[0].OldVersion, "1.24.0")
	}
	if summary.Updated[0].NewVersion != "2.0.0" {
		t.Errorf("NewVersion = %q, want %q", summary.Updated[0].NewVersion, "2.0.0")
	}
}

func TestCompareLock_MultipleChanges(t *testing.T) {
	oldContent := []byte(`
version: 1
packages:
  conda:
    - name: numpy
      version: "1.24.0"
    - name: scipy
      version: "1.11.0"
    - name: old-pkg
      version: "1.0.0"
`)
	newContent := []byte(`
version: 1
packages:
  conda:
    - name: numpy
      version: "2.0.0"
    - name: scipy
      version: "1.11.0"
    - name: torch
      version: "2.1.0"
`)
	summary, err := CompareLock(oldContent, newContent)
	if err != nil {
		t.Fatalf("CompareLock() error = %v", err)
	}
	if summary.PackagesAdded != 1 {
		t.Errorf("PackagesAdded = %d, want 1", summary.PackagesAdded)
	}
	if summary.PackagesRemoved != 1 {
		t.Errorf("PackagesRemoved = %d, want 1", summary.PackagesRemoved)
	}
	if summary.PackagesUpdated != 1 {
		t.Errorf("PackagesUpdated = %d, want 1", summary.PackagesUpdated)
	}
}

func TestCompareLock_PypiPackages(t *testing.T) {
	oldContent := []byte(`
version: 1
packages:
  pypi:
    - name: flask
      version: "2.3.0"
`)
	newContent := []byte(`
version: 1
packages:
  pypi:
    - name: flask
      version: "3.0.0"
    - name: requests
      version: "2.31.0"
`)
	summary, err := CompareLock(oldContent, newContent)
	if err != nil {
		t.Fatalf("CompareLock() error = %v", err)
	}
	if summary.PackagesAdded != 1 {
		t.Errorf("PackagesAdded = %d, want 1", summary.PackagesAdded)
	}
	if summary.PackagesUpdated != 1 {
		t.Errorf("PackagesUpdated = %d, want 1", summary.PackagesUpdated)
	}
}

func TestCompareLock_InvalidYAML(t *testing.T) {
	// Invalid YAML should not error, just return simple summary
	oldContent := []byte("not: valid: yaml: {{{")
	newContent := []byte("different: content")

	summary, err := CompareLock(oldContent, newContent)
	if err != nil {
		t.Fatalf("CompareLock() should not error on invalid YAML, got %v", err)
	}
	// Should still detect they differ
	if summary == nil {
		t.Fatal("summary should not be nil")
	}
}

func TestCompareLock_FlatPackageFormat(t *testing.T) {
	oldContent := []byte(`
packages:
  - name: numpy
    version: "1.24.0"
`)
	newContent := []byte(`
packages:
  - name: numpy
    version: "2.0.0"
  - name: scipy
    version: "1.11.0"
`)
	summary, err := CompareLock(oldContent, newContent)
	if err != nil {
		t.Fatalf("CompareLock() error = %v", err)
	}
	if summary.PackagesAdded != 1 {
		t.Errorf("PackagesAdded = %d, want 1", summary.PackagesAdded)
	}
	if summary.PackagesUpdated != 1 {
		t.Errorf("PackagesUpdated = %d, want 1", summary.PackagesUpdated)
	}
}

func TestDiffPackages_Deterministic(t *testing.T) {
	oldPkgs := map[string]string{
		"numpy": "1.0", "scipy": "1.0", "pandas": "1.0",
	}
	newPkgs := map[string]string{
		"scipy": "2.0", "torch": "1.0", "flask": "2.0",
	}

	summary := diffPackages(oldPkgs, newPkgs)

	// Added: flask, torch (sorted)
	if summary.PackagesAdded != 2 {
		t.Errorf("PackagesAdded = %d, want 2", summary.PackagesAdded)
	}
	if summary.Added[0] != "flask 2.0" {
		t.Errorf("Added[0] = %q, want %q", summary.Added[0], "flask 2.0")
	}

	// Removed: numpy, pandas (sorted)
	if summary.PackagesRemoved != 2 {
		t.Errorf("PackagesRemoved = %d, want 2", summary.PackagesRemoved)
	}

	// Updated: scipy
	if summary.PackagesUpdated != 1 {
		t.Errorf("PackagesUpdated = %d, want 1", summary.PackagesUpdated)
	}
}

func TestFormatLockDiffText_WithChanges(t *testing.T) {
	summary := &LockSummary{
		PackagesAdded:   1,
		PackagesRemoved: 1,
		PackagesUpdated: 1,
		Added:           []string{"scipy 1.11.0"},
		Removed:         []string{"old-pkg 1.0.0"},
		Updated: []PackageUpdate{
			{Name: "numpy", OldVersion: "1.24.0", NewVersion: "2.0.0"},
		},
	}

	result := FormatLockDiffText(summary)

	if !strings.Contains(result, "+scipy 1.11.0") {
		t.Errorf("Should contain added package, got %q", result)
	}
	if !strings.Contains(result, "-old-pkg 1.0.0") {
		t.Errorf("Should contain removed package, got %q", result)
	}
	if !strings.Contains(result, "-numpy 1.24.0") {
		t.Errorf("Should contain old version, got %q", result)
	}
	if !strings.Contains(result, "+numpy 2.0.0") {
		t.Errorf("Should contain new version, got %q", result)
	}
}

func TestFormatLockDiffText_NoChanges(t *testing.T) {
	summary := &LockSummary{}
	result := FormatLockDiffText(summary)
	if !strings.Contains(result, "no package changes") {
		t.Errorf("Should indicate no changes, got %q", result)
	}
}

func TestFormatLockDiffText_Nil(t *testing.T) {
	result := FormatLockDiffText(nil)
	if result != "" {
		t.Errorf("Should be empty for nil, got %q", result)
	}
}

func TestFormatLockDiffText_UnparsableChanges(t *testing.T) {
	summary := &LockSummary{PackagesUpdated: -1}
	result := FormatLockDiffText(summary)
	if !strings.Contains(result, "unable to parse") {
		t.Errorf("Should indicate unparsable, got %q", result)
	}
}

func TestParseLockPackages_EmptyContent(t *testing.T) {
	pkgs, err := parseLockPackages([]byte{})
	if err != nil {
		t.Fatalf("parseLockPackages() error = %v", err)
	}
	if len(pkgs) != 0 {
		t.Errorf("Empty content should return empty map, got %d entries", len(pkgs))
	}
}

func TestCompareLock_MixedCondaAndPypi(t *testing.T) {
	oldContent := []byte(`
version: 1
packages:
  conda:
    - name: numpy
      version: "1.24.0"
  pypi:
    - name: flask
      version: "2.3.0"
`)
	newContent := []byte(`
version: 1
packages:
  conda:
    - name: numpy
      version: "2.0.0"
  pypi:
    - name: flask
      version: "3.0.0"
    - name: django
      version: "4.2.0"
`)
	summary, err := CompareLock(oldContent, newContent)
	if err != nil {
		t.Fatalf("CompareLock() error = %v", err)
	}
	if summary.PackagesUpdated != 2 {
		t.Errorf("PackagesUpdated = %d, want 2 (numpy + flask)", summary.PackagesUpdated)
	}
	if summary.PackagesAdded != 1 {
		t.Errorf("PackagesAdded = %d, want 1 (django)", summary.PackagesAdded)
	}
}
