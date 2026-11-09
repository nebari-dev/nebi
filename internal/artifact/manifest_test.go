package artifact

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	toml "github.com/pelletier/go-toml/v2"
)

const pixiInitToml = `[workspace]
authors = ["Sam Garcia <sam@example.com>"]
channels = ["conda-forge"]
name = "demo"
platforms = ["osx-arm64"]
version = "0.1.0"

[tasks]
start = "python main.py"

[dependencies]
python = "3.12.*"
numpy = ">=2.0"
`

func TestParseManifest(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		wantForm  ManifestForm
		wantTable string
		wantUnrep []string
		wantPixiV string
	}{
		{
			name: "prototype layout",
			in: `[workspace]
name = "analysis"
channels = ["conda-forge"]
platforms = ["linux-64", "osx-arm64"]

[dependencies]
python = "3.11.*"
numpy = ">=2.0"
ipykernel = "*"
`,
			wantForm: ManifestForm{
				Name: "analysis", Channels: []string{"conda-forge"}, Platforms: []string{"linux-64", "osx-arm64"},
				Requested: []RequestedPackage{
					{Name: "python", Constraint: "3.11.*", Source: SourceConda},
					{Name: "numpy", Constraint: ">=2.0", Source: SourceConda},
					{Name: "ipykernel", Constraint: "*", Source: SourceConda},
				},
			},
			wantTable: "workspace",
		},
		{
			name:      "pixi init output reports extra keys and tables",
			in:        pixiInitToml,
			wantTable: "workspace",
			wantPixiV: "0.1.0",
			wantForm: ManifestForm{
				Name: "demo", Channels: []string{"conda-forge"}, Platforms: []string{"osx-arm64"},
				Requested: []RequestedPackage{
					{Name: "python", Constraint: "3.12.*", Source: SourceConda},
					{Name: "numpy", Constraint: ">=2.0", Source: SourceConda},
				},
			},
			wantUnrep: []string{"workspace.authors", "workspace.version", "[tasks]"},
		},
		{
			name: "legacy project table, pypi, table deps, features",
			in: `# top comment
[project]
name = "legacy"
description = "old style"
channels = ["conda-forge", { channel = "pytorch", priority = 1 }]
platforms = ["linux-64"]

[dependencies]
pytorch = { version = ">=2.4", channel = "pytorch" }
"ruamel.yaml" = ""
scipy = { version = "1.14.*" }

[pypi-dependencies]
rich = ">=13.9"
mypkg = { path = ".", editable = true }

[feature.test.dependencies]
pytest = "*"

[environments]
test = ["test"]

[[tool.thing]]
x = 1
`,
			wantTable: "project",
			wantForm: ManifestForm{
				Name: "legacy", Description: "old style",
				Channels: []string{"conda-forge", "pytorch"}, Platforms: []string{"linux-64"},
				Requested: []RequestedPackage{
					{Name: "pytorch", Constraint: ">=2.4", Source: SourceConda},
					{Name: "ruamel.yaml", Constraint: "*", Source: SourceConda},
					{Name: "scipy", Constraint: "1.14.*", Source: SourceConda},
					{Name: "rich", Constraint: ">=13.9", Source: SourcePyPI},
					{Name: "mypkg", Constraint: "*", Source: SourcePyPI},
				},
			},
			wantUnrep: []string{"project.channels", "dependencies.pytorch", "pypi-dependencies.mypkg",
				"[feature.test.dependencies]", "[environments]", "[[tool.thing]]"},
		},
		{
			name:      "no workspace",
			in:        "[dependencies]\npython = \"*\"\n",
			wantTable: "",
			wantForm: ManifestForm{Channels: []string{}, Platforms: []string{},
				Requested: []RequestedPackage{{Name: "python", Constraint: "*", Source: SourceConda}}},
		},
		{
			name:      "empty",
			in:        "",
			wantForm:  ManifestForm{Channels: []string{}, Platforms: []string{}, Requested: []RequestedPackage{}},
			wantTable: "",
		},
		{
			name: "inline dependency table keeps sorted order",
			in: `dependencies = { b = "1", a = "2" }
[workspace]
name = "x"
platforms = ["linux-64"]
`,
			wantTable: "workspace",
			wantForm: ManifestForm{Name: "x", Channels: []string{}, Platforms: []string{"linux-64"},
				Requested: []RequestedPackage{{Name: "a", Constraint: "2", Source: SourceConda}, {Name: "b", Constraint: "1", Source: SourceConda}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := ParseManifest([]byte(tt.in))
			if err != nil {
				t.Fatalf("ParseManifest: %v", err)
			}
			if m.WorkspaceTable != tt.wantTable || m.HasWorkspace != (tt.wantTable != "") {
				t.Errorf("table = %q/%v, want %q", m.WorkspaceTable, m.HasWorkspace, tt.wantTable)
			}
			if m.PixiVersion != tt.wantPixiV {
				t.Errorf("PixiVersion = %q, want %q", m.PixiVersion, tt.wantPixiV)
			}
			if !reflect.DeepEqual(m.Form, tt.wantForm) {
				t.Errorf("Form =\n%#v\nwant\n%#v", m.Form, tt.wantForm)
			}
			if !slices.Equal(m.Unrepresentable, tt.wantUnrep) {
				t.Errorf("Unrepresentable = %q, want %q", m.Unrepresentable, tt.wantUnrep)
			}
		})
	}
}

func TestParseManifestInvalid(t *testing.T) {
	for _, in := range []string{"[workspace\nname=1", "name = ", "[a]\n[a]\n"} {
		if _, err := ParseManifest([]byte(in)); err == nil {
			t.Errorf("ParseManifest(%q) succeeded, want error", in)
		}
	}
}

func TestRenderManifest(t *testing.T) {
	got := string(RenderManifest(ManifestForm{
		Name:      "demo",
		Channels:  []string{"conda-forge"},
		Platforms: []string{"linux-64"},
		Requested: []RequestedPackage{
			{Name: "python", Constraint: ">=3.11"},
			{Name: "ruamel.yaml", Constraint: ""},
			{Name: "rich", Constraint: ">=13", Source: SourcePyPI},
		},
	}))
	want := `[workspace]
name = "demo"
channels = ["conda-forge"]
platforms = ["linux-64"]

[dependencies]
python = ">=3.11"
"ruamel.yaml" = "*"

[pypi-dependencies]
rich = ">=13"
`
	if got != want {
		t.Fatalf("RenderManifest =\n%s\nwant\n%s", got, want)
	}

	empty := string(RenderManifest(ManifestForm{Name: `we"ird`, Description: "d"}))
	wantEmpty := "[workspace]\nname = \"we\\\"ird\"\ndescription = \"d\"\nchannels = []\nplatforms = []\n\n[dependencies]\n"
	if empty != wantEmpty {
		t.Fatalf("RenderManifest(empty) =\n%q\nwant\n%q", empty, wantEmpty)
	}
}

func TestRenderParseRoundTrip(t *testing.T) {
	form := ManifestForm{
		Name: "rt", Description: "multi\nline \"quoted\"",
		Channels:  []string{"conda-forge", "https://repo.example.com/ch"},
		Platforms: []string{"linux-64", "win-64"},
		Requested: []RequestedPackage{
			{Name: "python", Constraint: "3.12.*", Source: SourceConda},
			{Name: "a.b", Constraint: ">=1,<2", Source: SourceConda},
			{Name: "requests", Constraint: "~=2.31", Source: SourcePyPI},
		},
	}
	m, err := ParseManifest(RenderManifest(form))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(m.Form, form) {
		t.Fatalf("round trip =\n%#v\nwant\n%#v", m.Form, form)
	}
	if len(m.Unrepresentable) != 0 {
		t.Fatalf("Unrepresentable = %v", m.Unrepresentable)
	}
}

func TestApplyForm(t *testing.T) {
	base := ManifestForm{
		Name: "demo", Channels: []string{"conda-forge"}, Platforms: []string{"osx-arm64"},
		Requested: []RequestedPackage{
			{Name: "python", Constraint: "3.12.*", Source: SourceConda},
			{Name: "numpy", Constraint: ">=2.0", Source: SourceConda},
		},
	}
	with := func(mut func(*ManifestForm)) ManifestForm {
		f := base
		f.Channels = slices.Clone(base.Channels)
		f.Platforms = slices.Clone(base.Platforms)
		f.Requested = slices.Clone(base.Requested)
		mut(&f)
		return f
	}

	tests := []struct {
		name     string
		existing string
		form     ManifestForm
		want     string
	}{
		{
			name:     "unchanged form is a no-op",
			existing: pixiInitToml,
			form:     base,
			want:     pixiInitToml,
		},
		{
			name:     "missing trailing newline is added",
			existing: strings.TrimSuffix(pixiInitToml, "\n"),
			form:     base,
			want:     pixiInitToml,
		},
		{
			name:     "empty existing renders fresh",
			existing: "  \n",
			form:     base,
			want:     string(RenderManifest(base)),
		},
		{
			name:     "edit name and platforms, add and remove deps, keep tasks",
			existing: pixiInitToml,
			form: with(func(f *ManifestForm) {
				f.Name = "demo2"
				f.Platforms = []string{"osx-arm64", "linux-64"}
				f.Requested = []RequestedPackage{
					{Name: "python", Constraint: "3.13.*"},
					{Name: "pandas", Constraint: ">=2.2"},
					{Name: "rich", Constraint: ">=13.9", Source: SourcePyPI},
				}
			}),
			want: `[workspace]
authors = ["Sam Garcia <sam@example.com>"]
channels = ["conda-forge"]
name = "demo2"
platforms = ["osx-arm64", "linux-64"]
version = "0.1.0"

[tasks]
start = "python main.py"

[dependencies]
python = "3.13.*"
pandas = ">=2.2"

[pypi-dependencies]
rich = ">=13.9"
`,
		},
		{
			name: "comments, multi-line arrays and table deps are preserved",
			existing: `# Managed by hand
[project]   # legacy header
name = "demo"
channels = [
  "conda-forge",  # main
]
platforms = [
  "osx-arm64",
]
description = """
Multi-line
[not-a-table]
"""

[dependencies]
# interpreter
python = "3.12.*"
numpy = { version = ">=2.0", channel = "conda-forge" }
scipy = "*"

[feature.gpu.dependencies]
cuda = "*"
`,
			form: ManifestForm{
				Name: "demo", Description: "Multi-line\n[not-a-table]\n",
				Channels: []string{"conda-forge"}, Platforms: []string{"osx-arm64"},
				Requested: []RequestedPackage{
					{Name: "python", Constraint: "3.12.*"},
					{Name: "numpy", Constraint: ">=2.1"},
				},
			},
			want: `# Managed by hand
[project]   # legacy header
name = "demo"
channels = [
  "conda-forge",  # main
]
platforms = [
  "osx-arm64",
]
description = """
Multi-line
[not-a-table]
"""

[dependencies]
# interpreter
python = "3.12.*"
numpy = { channel = "conda-forge", version = ">=2.1" }

[feature.gpu.dependencies]
cuda = "*"
`,
		},
		{
			name: "add workspace when missing and drop description",
			existing: `# deps only
[dependencies]
python = "*"
`,
			form: ManifestForm{
				Name: "fresh", Channels: []string{"conda-forge"}, Platforms: []string{"linux-64"},
				Requested: []RequestedPackage{{Name: "python", Constraint: "*"}},
			},
			want: `# deps only

[workspace]
name = "fresh"
channels = ["conda-forge"]
platforms = ["linux-64"]

[dependencies]
python = "*"
`,
		},
		{
			name: "remove description, add missing keys after last key",
			existing: `[workspace]
name = "x"
description = "old"
# trailing comment

[dependencies]
`,
			form: ManifestForm{Name: "x", Platforms: []string{"linux-64"}},
			want: `[workspace]
name = "x"
channels = []
platforms = ["linux-64"]
# trailing comment

[dependencies]
`,
		},
		{
			name: "new dependencies table goes after workspace",
			existing: `[workspace]
name = "x"
channels = ["conda-forge"]
platforms = ["linux-64"]
[tasks]
a = "b"
`,
			form: ManifestForm{Name: "x", Channels: []string{"conda-forge"}, Platforms: []string{"linux-64"},
				Requested: []RequestedPackage{{Name: "zlib", Constraint: ""}, {Name: "six", Source: SourcePyPI}}},
			want: `[workspace]
name = "x"
channels = ["conda-forge"]
platforms = ["linux-64"]

[dependencies]
zlib = "*"

[pypi-dependencies]
six = "*"

[tasks]
a = "b"
`,
		},
		{
			name: "removing all pypi deps keeps the header",
			existing: `[workspace]
name = "x"
channels = []
platforms = ["linux-64"]

[dependencies]

[pypi-dependencies]
six = "*"
`,
			form: ManifestForm{Name: "x", Platforms: []string{"linux-64"}},
			want: `[workspace]
name = "x"
channels = []
platforms = ["linux-64"]

[dependencies]

[pypi-dependencies]
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ApplyForm([]byte(tt.existing), tt.form)
			if err != nil {
				t.Fatalf("ApplyForm: %v", err)
			}
			if string(got) != tt.want {
				t.Fatalf("ApplyForm =\n%s\nwant\n%s", got, tt.want)
			}
			var check map[string]any
			if err := toml.Unmarshal(got, &check); err != nil {
				t.Fatalf("result is not valid TOML: %v", err)
			}
			m, err := ParseManifest(got)
			if err != nil {
				t.Fatal(err)
			}
			if m.Form.Name != tt.form.Name || !slices.Equal(m.Form.Platforms, nonNil(tt.form.Platforms)) {
				t.Errorf("re-parsed form = %+v, want %+v", m.Form, tt.form)
			}
			for i, r := range tt.form.Requested {
				got := m.Form.Requested[i]
				if got.Name != r.Name || got.Constraint != NormalizeConstraint(r.Constraint) || got.Source != r.sourceOrDefault() {
					t.Errorf("requested[%d] = %+v, want %+v", i, got, r)
				}
			}
			if len(m.Form.Requested) != len(tt.form.Requested) {
				t.Errorf("requested len = %d, want %d", len(m.Form.Requested), len(tt.form.Requested))
			}
		})
	}
}

func TestApplyFormInlineDeps(t *testing.T) {
	existing := "dependencies = { python = \"*\" }\n[workspace]\nname = \"x\"\nplatforms = [\"linux-64\"]\n"
	// Unchanged deps: fine.
	if _, err := ApplyForm([]byte(existing), ManifestForm{Name: "y", Platforms: []string{"linux-64"},
		Requested: []RequestedPackage{{Name: "python", Constraint: "*"}}}); err != nil {
		t.Fatalf("unchanged inline deps: %v", err)
	}
	// Changed deps can't be expressed without a header.
	if _, err := ApplyForm([]byte(existing), ManifestForm{Name: "y", Platforms: []string{"linux-64"},
		Requested: []RequestedPackage{{Name: "numpy", Constraint: "*"}}}); err == nil {
		t.Fatal("changed inline deps: want error")
	}
	if _, err := ApplyForm([]byte("workspace = { name = \"x\" }\n"), ManifestForm{Name: "y"}); err == nil {
		t.Fatal("inline workspace: want error")
	}
	if _, err := ApplyForm([]byte("not toml ["), ManifestForm{}); err == nil {
		t.Fatal("invalid toml: want error")
	}
}

func TestSpecFromManifest(t *testing.T) {
	s, err := SpecFromManifest([]byte(pixiInitToml))
	if err != nil {
		t.Fatal(err)
	}
	want := &Spec{
		Channels:  []string{"conda-forge"},
		Platforms: []string{"osx-arm64"},
		Requested: []RequestedPackage{
			{Name: "python", Constraint: "3.12.*", Source: SourceConda},
			{Name: "numpy", Constraint: ">=2.0", Source: SourceConda},
		},
	}
	if !reflect.DeepEqual(s, want) {
		t.Fatalf("Spec = %+v, want %+v", s, want)
	}
	if _, err := SpecFromManifest([]byte("[")); err == nil {
		t.Fatal("want error")
	}
}

func TestSortRequested(t *testing.T) {
	ps := []RequestedPackage{{Name: "b", Source: SourcePyPI}, {Name: "z"}, {Name: "a", Source: SourcePyPI}, {Name: "c", Source: SourceConda}}
	SortRequested(ps)
	var got []string
	for _, p := range ps {
		got = append(got, p.Name)
	}
	if !slices.Equal(got, []string{"c", "z", "a", "b"}) {
		t.Fatalf("order = %v", got)
	}
}

func TestRequestedPackageOperator(t *testing.T) {
	p := RequestedPackage{Name: "x", Constraint: ">=1.2"}
	if p.Operator() != OpGE || p.Version() != "1.2" {
		t.Fatalf("got %q %q", p.Operator(), p.Version())
	}
}
