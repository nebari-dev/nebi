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
	if summary.Updated[0].OldVersion != "1.24.0" || summary.Updated[0].NewVersion != "2.0.0" {
		t.Errorf("Updated = %+v", summary.Updated[0])
	}
}

func TestCompareLock_V6Format_CondaUpdated(t *testing.T) {
	oldContent := []byte(`
version: 6
packages:
- conda: https://conda.anaconda.org/conda-forge/linux-64/numpy-1.24.0-py311h1234_0.conda
  sha256: abc123
`)
	newContent := []byte(`
version: 6
packages:
- conda: https://conda.anaconda.org/conda-forge/linux-64/numpy-2.4.1-py314h2b28147_0.conda
  sha256: def456
`)
	summary, err := CompareLock(oldContent, newContent)
	if err != nil {
		t.Fatalf("CompareLock() error = %v", err)
	}
	if summary.PackagesUpdated != 1 {
		t.Errorf("PackagesUpdated = %d, want 1", summary.PackagesUpdated)
	}
	if summary.Updated[0].Name != "numpy" {
		t.Errorf("Name = %q, want numpy", summary.Updated[0].Name)
	}
}

func TestCompareLock_InvalidYAML(t *testing.T) {
	summary, err := CompareLock([]byte("not: valid: yaml: {{{"), []byte("different: content"))
	if err != nil {
		t.Fatalf("CompareLock() should not error on invalid YAML, got %v", err)
	}
	if summary == nil {
		t.Fatal("summary should not be nil")
	}
}

func TestParseCondaFilename(t *testing.T) {
	tests := []struct {
		url, wantName, wantVersion string
	}{
		{"https://conda.anaconda.org/conda-forge/linux-64/numpy-2.4.1-py314h2b28147_0.conda", "numpy", "2.4.1"},
		{"https://conda.anaconda.org/conda-forge/linux-64/libgcc-ng-14.2.0-h69a702a_2.conda", "libgcc-ng", "14.2.0"},
		{"https://conda.anaconda.org/conda-forge/linux-64/python-3.12.0-hab00c5b_0_cpython.tar.bz2", "python", "3.12.0"},
		{"https://conda.anaconda.org/conda-forge/linux-64/ca-certificates-2024.12.14-hbcca054_0.conda", "ca-certificates", "2024.12.14"},
	}

	for _, tt := range tests {
		name, version := parseCondaFilename(tt.url)
		if name != tt.wantName || version != tt.wantVersion {
			t.Errorf("parseCondaFilename(%q) = (%q, %q), want (%q, %q)", tt.url, name, version, tt.wantName, tt.wantVersion)
		}
	}
}

func TestFormatLockDiffText_WithChanges(t *testing.T) {
	summary := &LockSummary{
		PackagesAdded:   1,
		PackagesRemoved: 1,
		PackagesUpdated: 1,
		Added:           []string{"scipy 1.11.0"},
		Removed:         []string{"old-pkg 1.0.0"},
		Updated:         []PackageUpdate{{Name: "numpy", OldVersion: "1.24.0", NewVersion: "2.0.0"}},
	}

	result := FormatLockDiffText(summary)
	if !strings.Contains(result, "+scipy 1.11.0") {
		t.Errorf("Should contain added package, got %q", result)
	}
	if !strings.Contains(result, "-old-pkg 1.0.0") {
		t.Errorf("Should contain removed package, got %q", result)
	}
	if !strings.Contains(result, "-numpy 1.24.0") || !strings.Contains(result, "+numpy 2.0.0") {
		t.Errorf("Should contain updated package, got %q", result)
	}
}

func TestFormatLockDiffText_Nil(t *testing.T) {
	if result := FormatLockDiffText(nil); result != "" {
		t.Errorf("Should be empty for nil, got %q", result)
	}
}
