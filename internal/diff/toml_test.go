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
	if removed[0].OldValue != "*" {
		t.Errorf("Removed value = %q, want %q", removed[0].OldValue, "*")
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
	if modified[0].Key != "numpy" {
		t.Errorf("Modified key = %q, want %q", modified[0].Key, "numpy")
	}
	if modified[0].OldValue != ">=2.0" {
		t.Errorf("OldValue = %q, want %q", modified[0].OldValue, ">=2.0")
	}
	if modified[0].NewValue != ">=2.4" {
		t.Errorf("NewValue = %q, want %q", modified[0].NewValue, ">=2.4")
	}
}

func TestCompareToml_MultipleChanges(t *testing.T) {
	oldContent := []byte(`[workspace]
name = "data-science"
version = "0.1.0"

[dependencies]
python = ">=3.11"
numpy = ">=2.0"
tensorflow = ">=2.14"
`)
	newContent := []byte(`[workspace]
name = "data-science"
version = "0.2.0"

[dependencies]
python = ">=3.12"
numpy = ">=2.4"
torch = ">=2.0"
`)

	diff, err := CompareToml(oldContent, newContent)
	if err != nil {
		t.Fatalf("CompareToml() error = %v", err)
	}

	if !diff.HasChanges() {
		t.Fatal("HasChanges() should be true")
	}

	// Check we have various change types
	if len(diff.Added()) == 0 {
		t.Error("Should have added changes (torch)")
	}
	if len(diff.Removed()) == 0 {
		t.Error("Should have removed changes (tensorflow)")
	}
	if len(diff.Modified()) == 0 {
		t.Error("Should have modified changes (version, python, numpy)")
	}
}

func TestCompareToml_NewSection(t *testing.T) {
	oldContent := []byte(`[dependencies]
python = ">=3.11"
`)
	newContent := []byte(`[dependencies]
python = ">=3.11"

[tasks]
test = "pytest"
`)

	diff, err := CompareToml(oldContent, newContent)
	if err != nil {
		t.Fatalf("CompareToml() error = %v", err)
	}

	if !diff.HasChanges() {
		t.Fatal("HasChanges() should be true for new section")
	}

	added := diff.Added()
	found := false
	for _, a := range added {
		if a.Key == "test" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Should have found added task 'test'")
	}
}

func TestCompareToml_RemovedSection(t *testing.T) {
	oldContent := []byte(`[dependencies]
python = ">=3.11"

[tasks]
test = "pytest"
build = "make"
`)
	newContent := []byte(`[dependencies]
python = ">=3.11"
`)

	diff, err := CompareToml(oldContent, newContent)
	if err != nil {
		t.Fatalf("CompareToml() error = %v", err)
	}

	if !diff.HasChanges() {
		t.Fatal("HasChanges() should be true for removed section")
	}

	removed := diff.Removed()
	if len(removed) < 2 {
		t.Errorf("Should have at least 2 removed items (test, build), got %d", len(removed))
	}
}

func TestCompareToml_InvalidOldContent(t *testing.T) {
	_, err := CompareToml([]byte("not valid toml{{{"), []byte("[ok]\nkey = \"val\""))
	if err == nil {
		t.Fatal("Should return error for invalid old content")
	}
}

func TestCompareToml_InvalidNewContent(t *testing.T) {
	_, err := CompareToml([]byte("[ok]\nkey = \"val\""), []byte("not valid toml{{{"))
	if err == nil {
		t.Fatal("Should return error for invalid new content")
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

func TestFormatUnifiedDiff_NoChanges(t *testing.T) {
	diff := &TomlDiff{Changes: []Change{}}
	result := FormatUnifiedDiff(diff, "source", "target")
	if result != "" {
		t.Errorf("FormatUnifiedDiff should return empty string for no changes, got %q", result)
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

	result := FormatUnifiedDiff(diff, "pulled (ws:v1.0)", "local")

	if !strings.Contains(result, "--- pulled (ws:v1.0)") {
		t.Error("Should contain source label")
	}
	if !strings.Contains(result, "+++ local") {
		t.Error("Should contain target label")
	}
	if !strings.Contains(result, "@@ pixi.toml @@") {
		t.Error("Should contain section header")
	}
	if !strings.Contains(result, "+scipy") {
		t.Error("Should show added package")
	}
	if !strings.Contains(result, "-old-pkg") {
		t.Error("Should show removed package")
	}
}

func TestFormatValue(t *testing.T) {
	tests := []struct {
		input    interface{}
		expected string
	}{
		{"hello", "hello"},
		{int64(42), "42"},
		{float64(3.14), "3.14"},
		{true, "true"},
		{false, "false"},
		{nil, ""},
		{[]interface{}{"a", "b"}, "[a, b]"},
	}

	for _, tt := range tests {
		got := formatValue(tt.input)
		if got != tt.expected {
			t.Errorf("formatValue(%v) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestChangeType_Constants(t *testing.T) {
	if ChangeAdded != "added" {
		t.Errorf("ChangeAdded = %q, unexpected", ChangeAdded)
	}
	if ChangeRemoved != "removed" {
		t.Errorf("ChangeRemoved = %q, unexpected", ChangeRemoved)
	}
	if ChangeModified != "modified" {
		t.Errorf("ChangeModified = %q, unexpected", ChangeModified)
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

	if !diff.HasChanges() {
		t.Fatal("HasChanges() should be true for new nested section")
	}

	added := diff.Added()
	found := false
	for _, a := range added {
		if a.Key == "pytest" && a.NewValue == "*" {
			found = true
			// Section should be the full dotted path
			if !strings.Contains(a.Section, "test") || !strings.Contains(a.Section, "dependencies") {
				t.Errorf("Section = %q, want it to contain the full path (feature.test.dependencies)", a.Section)
			}
			break
		}
	}
	if !found {
		t.Errorf("Should find added pytest with value *, got changes: %+v", added)
	}
}

func TestCompareToml_DeeplyNestedFeatureRemoved(t *testing.T) {
	oldContent := []byte(`[dependencies]
python = ">=3.11"

[feature.cuda.system-requirements]
cuda = "12.0"
`)
	newContent := []byte(`[dependencies]
python = ">=3.11"
`)

	diff, err := CompareToml(oldContent, newContent)
	if err != nil {
		t.Fatalf("CompareToml() error = %v", err)
	}

	removed := diff.Removed()
	found := false
	for _, r := range removed {
		if r.Key == "cuda" && r.OldValue == "12.0" {
			found = true
			if !strings.Contains(r.Section, "cuda") || !strings.Contains(r.Section, "system-requirements") {
				t.Errorf("Section = %q, want it to contain full path", r.Section)
			}
			break
		}
	}
	if !found {
		t.Errorf("Should find removed cuda, got changes: %+v", removed)
	}
}

func TestCompareToml_NestedNoMapString(t *testing.T) {
	// Ensure nested maps don't produce "map[...]" in values
	oldContent := []byte(`[dependencies]
python = ">=3.11"
`)
	newContent := []byte(`[dependencies]
python = ">=3.11"

[feature.test.dependencies]
pytest = "*"
coverage = ">=7.0"
`)

	diff, err := CompareToml(oldContent, newContent)
	if err != nil {
		t.Fatalf("CompareToml() error = %v", err)
	}

	for _, c := range diff.Changes {
		if strings.Contains(c.NewValue, "map[") {
			t.Errorf("Change value contains Go map representation: %+v", c)
		}
		if strings.Contains(c.OldValue, "map[") {
			t.Errorf("Change value contains Go map representation: %+v", c)
		}
	}
}

func TestFormatUnifiedDiff_NestedSections(t *testing.T) {
	diff := &TomlDiff{
		Changes: []Change{
			{Section: "feature.test.dependencies", Key: "pytest", Type: ChangeAdded, NewValue: "*"},
			{Section: "feature.test.dependencies", Key: "coverage", Type: ChangeAdded, NewValue: ">=7.0"},
		},
	}

	result := FormatUnifiedDiff(diff, "pulled (ws:v1.0)", "local")

	if !strings.Contains(result, "[feature.test.dependencies]") {
		t.Errorf("Should render full dotted section path, got:\n%s", result)
	}
	if !strings.Contains(result, "+pytest") {
		t.Error("Should show added pytest")
	}
	if !strings.Contains(result, "+coverage") {
		t.Error("Should show added coverage")
	}
}

func TestFormatValue_MapProducesToml(t *testing.T) {
	// If a map somehow reaches formatValue, it should produce TOML-like output
	val := map[string]interface{}{
		"key": "value",
	}
	result := formatValue(val)
	if strings.Contains(result, "map[") {
		t.Errorf("formatValue(map) should not produce Go map syntax, got %q", result)
	}
	if !strings.Contains(result, "key") || !strings.Contains(result, "value") {
		t.Errorf("formatValue(map) should contain key and value, got %q", result)
	}
}

func TestCompareToml_BooleanValues(t *testing.T) {
	oldContent := []byte(`[settings]
verbose = false
`)
	newContent := []byte(`[settings]
verbose = true
`)

	diff, err := CompareToml(oldContent, newContent)
	if err != nil {
		t.Fatalf("CompareToml() error = %v", err)
	}

	modified := diff.Modified()
	if len(modified) != 1 {
		t.Fatalf("Modified() length = %d, want 1", len(modified))
	}
	if modified[0].OldValue != "false" {
		t.Errorf("OldValue = %q, want %q", modified[0].OldValue, "false")
	}
	if modified[0].NewValue != "true" {
		t.Errorf("NewValue = %q, want %q", modified[0].NewValue, "true")
	}
}
