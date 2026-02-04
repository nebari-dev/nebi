package diff

import (
	"strings"
	"testing"
)

func TestCompareToml_NoDifferences(t *testing.T) {
	content := []byte(`[workspace]
name = "test"
version = "1.0"

[dependencies]
python = ">=3.11"
numpy = ">=2.0"
`)

	diff, err := CompareToml(content, content)
	if err != nil {
		t.Fatalf("CompareToml() error = %v", err)
	}
	if diff.HasChanges() {
		t.Errorf("HasChanges() = true, want false for identical content")
	}
}

func TestCompareToml_AddedDependency(t *testing.T) {
	oldContent := []byte(`[dependencies]
python = ">=3.11"
numpy = ">=2.0"
`)
	newContent := []byte(`[dependencies]
python = ">=3.11"
numpy = ">=2.0"
scipy = ">=1.17"
`)

	diff, err := CompareToml(oldContent, newContent)
	if err != nil {
		t.Fatalf("CompareToml() error = %v", err)
	}
	if !diff.HasChanges() {
		t.Fatal("HasChanges() = false, want true")
	}

	added := diff.Added()
	if len(added) != 1 {
		t.Fatalf("Added() length = %d, want 1", len(added))
	}
	if added[0].Key != "scipy" {
		t.Errorf("Added key = %q, want %q", added[0].Key, "scipy")
	}
	if added[0].NewValue != ">=1.17" {
		t.Errorf("Added value = %q, want %q", added[0].NewValue, ">=1.17")
	}
}

func TestCompareToml_RemovedDependency(t *testing.T) {
	oldContent := []byte(`[dependencies]
python = ">=3.11"
numpy = ">=2.0"
old-pkg = "*"
`)
	newContent := []byte(`[dependencies]
python = ">=3.11"
numpy = ">=2.0"
`)

	diff, err := CompareToml(oldContent, newContent)
	if err != nil {
		t.Fatalf("CompareToml() error = %v", err)
	}

	removed := diff.Removed()
	if len(removed) != 1 {
		t.Fatalf("Removed() length = %d, want 1", len(removed))
	}
	if removed[0].Key != "old-pkg" {
		t.Errorf("Removed key = %q, want %q", removed[0].Key, "old-pkg")
	}
}

func TestCompareToml_ModifiedVersion(t *testing.T) {
	oldContent := []byte(`[dependencies]
numpy = ">=2.0"
`)
	newContent := []byte(`[dependencies]
numpy = ">=2.4"
`)

	diff, err := CompareToml(oldContent, newContent)
	if err != nil {
		t.Fatalf("CompareToml() error = %v", err)
	}

	modified := diff.Modified()
	if len(modified) != 1 {
		t.Fatalf("Modified() length = %d, want 1", len(modified))
	}
	if modified[0].OldValue != ">=2.0" || modified[0].NewValue != ">=2.4" {
		t.Errorf("Modified = %q → %q, want >=2.0 → >=2.4", modified[0].OldValue, modified[0].NewValue)
	}
}

func TestCompareToml_DeeplyNestedFeature(t *testing.T) {
	oldContent := []byte(`[dependencies]
python = ">=3.11"
`)
	newContent := []byte(`[dependencies]
python = ">=3.11"

[feature.test.dependencies]
pytest = "*"
`)

	diff, err := CompareToml(oldContent, newContent)
	if err != nil {
		t.Fatalf("CompareToml() error = %v", err)
	}

	added := diff.Added()
	found := false
	for _, a := range added {
		if a.Key == "pytest" && a.NewValue == "*" {
			found = true
			if !strings.Contains(a.Section, "test") || !strings.Contains(a.Section, "dependencies") {
				t.Errorf("Section = %q, want it to contain feature.test.dependencies", a.Section)
			}
		}
	}
	if !found {
		t.Errorf("Should find added pytest, got %+v", added)
	}
}

func TestCompareToml_EmptyFiles(t *testing.T) {
	diff, err := CompareToml([]byte(""), []byte(""))
	if err != nil {
		t.Fatalf("CompareToml() error = %v", err)
	}
	if diff.HasChanges() {
		t.Error("Empty files should not have changes")
	}
}

func TestCompareToml_InvalidContent(t *testing.T) {
	_, err := CompareToml([]byte("not valid toml{{{"), []byte("[ok]\nkey = \"val\""))
	if err == nil {
		t.Fatal("Should return error for invalid content")
	}
}

func TestFormatUnifiedDiff_WithChanges(t *testing.T) {
	diff := &TomlDiff{
		Changes: []Change{
			{Section: "dependencies", Key: "scipy", Type: ChangeAdded, NewValue: ">=1.17"},
			{Section: "dependencies", Key: "numpy", Type: ChangeModified, OldValue: ">=2.0", NewValue: ">=2.4"},
			{Section: "dependencies", Key: "old-pkg", Type: ChangeRemoved, OldValue: "*"},
		},
	}

	result := FormatUnifiedDiff(diff, "a", "b")

	if !strings.Contains(result, "--- a") {
		t.Error("Should contain source label")
	}
	if !strings.Contains(result, "+++ b") {
		t.Error("Should contain target label")
	}
	if !strings.Contains(result, "+scipy") {
		t.Error("Should show added package")
	}
	if !strings.Contains(result, "-old-pkg") {
		t.Error("Should show removed package")
	}
}

func TestFormatUnifiedDiff_NoChanges(t *testing.T) {
	diff := &TomlDiff{Changes: []Change{}}
	result := FormatUnifiedDiff(diff, "a", "b")
	if result != "" {
		t.Errorf("Should return empty for no changes, got %q", result)
	}
}

func TestCompareToml_ArrayValues(t *testing.T) {
	oldContent := []byte(`[workspace]
channels = ["conda-forge"]
platforms = ["linux-64"]
`)
	newContent := []byte(`[workspace]
channels = ["conda-forge", "pytorch"]
platforms = ["linux-64", "osx-arm64"]
`)

	diff, err := CompareToml(oldContent, newContent)
	if err != nil {
		t.Fatalf("CompareToml() error = %v", err)
	}
	if !diff.HasChanges() {
		t.Fatal("HasChanges() should be true for array changes")
	}
}

func TestFormatValue_NoMapSyntax(t *testing.T) {
	val := map[string]interface{}{"key": "value"}
	result := formatValue(val)
	if strings.Contains(result, "map[") {
		t.Errorf("formatValue(map) should not produce Go map syntax, got %q", result)
	}
}
