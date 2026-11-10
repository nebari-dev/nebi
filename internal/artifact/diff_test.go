package artifact

import (
	"reflect"
	"slices"
	"testing"
)

func TestDiffRequested(t *testing.T) {
	base := []RequestedPackage{
		{Name: "python", Constraint: "3.11.*"},
		{Name: "numpy", Constraint: ">=2.0"},
		{Name: "ipykernel", Constraint: "*"},
		{Name: "rich", Constraint: ">=13", Source: SourcePyPI},
		{Name: "same-constraint", Constraint: ">=1"},
	}
	compare := []RequestedPackage{
		{Name: "python", Constraint: "3.12.*"},
		{Name: "numpy", Constraint: ">=2.0"},
		{Name: "pandas", Constraint: ">=2.2"},
		{Name: "rich", Constraint: ">=13", Source: SourcePyPI},
		{Name: "same-constraint", Constraint: ">=1"},
	}
	baseRes := []ResolvedPackage{
		{Name: "python", Version: "3.11.10", Source: SourceConda, Channel: "conda-forge"},
		{Name: "numpy", Version: "2.1.0", Source: SourceConda},
		{Name: "same-constraint", Version: "1.0", Source: SourceConda},
		{Name: "rich", Version: "13.9.4", Source: SourcePyPI, Channel: "pypi"},
	}
	compareRes := []ResolvedPackage{
		{Name: "python", Version: "3.12.7", Source: SourceConda, Channel: "conda-forge"},
		{Name: "numpy", Version: "2.1.0", Source: SourceConda},
		{Name: "same-constraint", Version: "1.1", Source: SourceConda},
		{Name: "rich", Version: "13.9.4", Source: SourcePyPI, Channel: "pypi"},
	}
	got := DiffRequested(base, compare, baseRes, compareRes)
	want := []PackageDiffEntry{
		{Name: "ipykernel", Source: SourceConda, Change: ChangeRemoved, Base: &PackageRef{Constraint: "*", Version: "*"}},
		{Name: "numpy", Source: SourceConda, Change: ChangeSame,
			Base: &PackageRef{Constraint: ">=2.0", Version: "2.1.0"}, Compare: &PackageRef{Constraint: ">=2.0", Version: "2.1.0"}},
		{Name: "pandas", Source: SourceConda, Change: ChangeAdded, Compare: &PackageRef{Constraint: ">=2.2", Version: ">=2.2"}},
		{Name: "python", Source: SourceConda, Change: ChangeChanged,
			Base:    &PackageRef{Constraint: "3.11.*", Version: "3.11.10", Channel: "conda-forge"},
			Compare: &PackageRef{Constraint: "3.12.*", Version: "3.12.7", Channel: "conda-forge"}},
		{Name: "rich", Source: SourcePyPI, Change: ChangeSame,
			Base: &PackageRef{Constraint: ">=13", Version: "13.9.4", Channel: "pypi"}, Compare: &PackageRef{Constraint: ">=13", Version: "13.9.4", Channel: "pypi"}},
		{Name: "same-constraint", Source: SourceConda, Change: ChangeChanged,
			Base: &PackageRef{Constraint: ">=1", Version: "1.0"}, Compare: &PackageRef{Constraint: ">=1", Version: "1.1"}},
	}
	if !reflect.DeepEqual(got, want) {
		for i := range got {
			t.Logf("got %d: %+v base=%+v compare=%+v", i, got[i], got[i].Base, got[i].Compare)
		}
		t.Fatal("DiffRequested mismatch")
	}
	if c := CountChanges(got); c != (DiffCounts{Added: 1, Removed: 1, Changed: 2, Same: 2}) || !c.HasChanges() {
		t.Errorf("counts = %+v", c)
	}
	if c := CountChanges(DiffRequested(base, base, nil, nil)); c.HasChanges() || c.Same != len(base) {
		t.Errorf("self diff counts = %+v", c)
	}
}

func TestDiffResolved(t *testing.T) {
	base := []ResolvedPackage{
		{Name: "a", Version: "1", Build: "h0", Source: SourceConda},
		{Name: "b", Version: "1", Build: "h0", Source: SourceConda},
		{Name: "c", Version: "1", Source: SourceConda},
		{Name: "c", Version: "9", Source: SourcePyPI},
		{Name: "a", Version: "dup-ignored", Source: SourceConda},
	}
	compare := []ResolvedPackage{
		{Name: "a", Version: "1", Build: "h1", Source: SourceConda},
		{Name: "b", Version: "1", Build: "h0", Source: SourceConda},
		{Name: "c", Version: "2", Source: SourcePyPI},
		{Name: "d", Version: "1", Source: SourceConda},
	}
	got := DiffResolved(base, compare)
	type row struct {
		name   string
		source PackageSource
		change Change
	}
	var rows []row
	for _, e := range got {
		rows = append(rows, row{e.Name, e.Source, e.Change})
	}
	want := []row{
		{"a", SourceConda, ChangeChanged},
		{"b", SourceConda, ChangeSame},
		{"c", SourceConda, ChangeRemoved},
		{"c", SourcePyPI, ChangeChanged},
		{"d", SourceConda, ChangeAdded},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("rows = %+v, want %+v", rows, want)
	}
	if got[0].Base.Build != "h0" || got[0].Compare.Build != "h1" || got[2].Compare != nil || got[4].Base != nil {
		t.Errorf("refs wrong: %+v", got)
	}
}

func TestDiffVersions(t *testing.T) {
	baseToml := []byte(`[workspace]
name = "demo"
channels = ["conda-forge"]
platforms = ["linux-64", "osx-arm64"]

[dependencies]
python = "3.12.*"
zlib = "*"

[pypi-dependencies]
six = ">=1.16"
`)
	compareToml := []byte(`[workspace]
name = "demo"
channels = ["conda-forge", "bioconda"]
platforms = ["osx-arm64"]

[dependencies]
python = "3.13.*"
`)
	baseLock := []byte(lockV6)

	t.Run("explicit platform", func(t *testing.T) {
		d, err := DiffVersions(VersionContent{Toml: baseToml, Lock: baseLock}, VersionContent{Toml: compareToml}, DiffOptions{Platform: "linux-64"})
		if err != nil {
			t.Fatal(err)
		}
		if d.Platform != "linux-64" {
			t.Errorf("platform = %q", d.Platform)
		}
		if d.RequestedCounts != (DiffCounts{Removed: 2, Changed: 1}) {
			t.Errorf("requested counts = %+v", d.RequestedCounts)
		}
		// python resolved 3.12.7 in base; compare has no lock so version falls back to the constraint.
		py := d.Requested[slices.IndexFunc(d.Requested, func(e PackageDiffEntry) bool { return e.Name == "python" })]
		if py.Base.Version != "3.12.7" || py.Compare.Version != "3.13.*" {
			t.Errorf("python = %+v / %+v", py.Base, py.Compare)
		}
		if d.ResolvedCounts.Removed != 6 || d.ResolvedCounts.Added != 0 {
			t.Errorf("resolved counts = %+v", d.ResolvedCounts)
		}
		if !slices.Equal(d.Channels.Added, []string{"bioconda"}) || len(d.Channels.Removed) != 0 {
			t.Errorf("channels = %+v", d.Channels)
		}
		if !slices.Equal(d.Platforms.Removed, []string{"linux-64"}) || len(d.Platforms.Added) != 0 {
			t.Errorf("platforms = %+v", d.Platforms)
		}
	})

	t.Run("platform chosen from compare manifest", func(t *testing.T) {
		d, err := DiffVersions(VersionContent{Toml: baseToml, Lock: baseLock}, VersionContent{Toml: compareToml}, DiffOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if d.Platform != "osx-arm64" {
			t.Errorf("platform = %q, want osx-arm64", d.Platform)
		}
	})

	t.Run("platform falls back to sorted lock platforms", func(t *testing.T) {
		toml := []byte("[workspace]\nname='x'\nplatforms=['win-64']\n")
		d, err := DiffVersions(VersionContent{Toml: toml, Lock: baseLock}, VersionContent{Toml: toml, Lock: baseLock}, DiffOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if d.Platform != "linux-64" || d.ResolvedCounts.HasChanges() || d.ResolvedCounts.Same != 6 {
			t.Errorf("platform = %q counts = %+v", d.Platform, d.ResolvedCounts)
		}
	})

	t.Run("no locks", func(t *testing.T) {
		d, err := DiffVersions(VersionContent{Toml: baseToml}, VersionContent{Toml: baseToml}, DiffOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if d.Platform != "" || len(d.Resolved) != 0 || d.RequestedCounts.HasChanges() {
			t.Errorf("diff = %+v", d)
		}
	})

	t.Run("errors", func(t *testing.T) {
		good := VersionContent{Toml: baseToml}
		for _, pair := range [][2]VersionContent{
			{{Toml: []byte("[")}, good},
			{good, {Toml: []byte("[")}},
			{{Toml: baseToml, Lock: []byte("version: 2\n")}, good},
			{good, {Toml: baseToml, Lock: []byte("version: [")}},
		} {
			if _, err := DiffVersions(pair[0], pair[1], DiffOptions{}); err == nil {
				t.Error("want error")
			}
		}
	})
}

func TestDiffList(t *testing.T) {
	d := DiffList([]string{"a", "b"}, []string{"b", "c"})
	if !slices.Equal(d.Added, []string{"c"}) || !slices.Equal(d.Removed, []string{"a"}) {
		t.Fatalf("DiffList = %+v", d)
	}
}

func TestSummarizeRequestedChanges(t *testing.T) {
	p := func(n, c string) RequestedPackage { return RequestedPackage{Name: n, Constraint: c} }
	tests := []struct {
		name          string
		base, compare []RequestedPackage
		want          []string
	}{
		{"none", []RequestedPackage{p("a", "1")}, []RequestedPackage{p("a", "1")}, nil},
		{"one added", nil, []RequestedPackage{p("a", "")}, []string{"added a"}},
		{"two added", nil, []RequestedPackage{p("pandas", ""), p("rich", "")}, []string{"added pandas and rich"}},
		{"three added", nil, []RequestedPackage{p("a", ""), p("b", ""), p("c", "")}, []string{"added a, b and c"}},
		{"all kinds", []RequestedPackage{p("python", "3.11.*"), p("numpy", "*")}, []RequestedPackage{p("python", "3.12.*"), p("scikit-learn", ">=1.5")},
			[]string{"added scikit-learn", "removed numpy", "changed python"}},
		{"empty constraint equals star", []RequestedPackage{p("a", "")}, []RequestedPackage{p("a", "*")}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SummarizeRequestedChanges(tt.base, tt.compare); !slices.Equal(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
