package artifact

import (
	"errors"
	"testing"
)

var appliedSpec = Spec{
	Platforms: []string{"linux-64", "osx-arm64"},
	Channels:  []string{"conda-forge", "bioconda"},
	Packages: []Package{
		{Name: "python", Constraint: "3.12.*"},
		{Name: "numpy", Operator: ">=", Constraint: "2"},
	},
}

func TestApplySpecKeepsTablesTheSpecDoesNotCover(t *testing.T) {
	in := `# my env
[workspace]
name = "demo"
description = "keep me"
channels = ["conda-forge"]
platforms = ["linux-64"]

[dependencies]
python = "3.11.*"
# dropped with the table's entries
scipy = "*"

# about the feature below
[feature.test.dependencies]
pytest = "*"

[pypi-dependencies]
requests = ">=2"

[tasks]
start = "python main.py"
`
	want := `# my env
[workspace]
name = "demo"
description = "keep me"
channels = ["conda-forge", "bioconda"]
platforms = ["linux-64", "osx-arm64"]

[dependencies]
python = "3.12.*"
numpy = ">=2"

# about the feature below
[feature.test.dependencies]
pytest = "*"

[pypi-dependencies]
requests = ">=2"

[tasks]
start = "python main.py"
`
	got, err := ApplySpec(in, appliedSpec)
	if err != nil {
		t.Fatalf("ApplySpec: %v", err)
	}
	if got != want {
		t.Fatalf("mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestApplySpecReplacesMultiLineArrays(t *testing.T) {
	in := `[workspace]
name = "demo"
channels = [
  "conda-forge",  # main channel ]
  "defaults",
]
platforms = [
  "linux-64",
]

[dependencies]
python = { version = "3.11.*", channel = "conda-forge" }
`
	want := `[workspace]
name = "demo"
channels = ["conda-forge", "bioconda"]
platforms = ["linux-64", "osx-arm64"]

[dependencies]
python = "3.12.*"
numpy = ">=2"
`
	got, err := ApplySpec(in, appliedSpec)
	if err != nil {
		t.Fatalf("ApplySpec: %v", err)
	}
	if got != want {
		t.Fatalf("mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestApplySpecAddsMissingDependenciesTable(t *testing.T) {
	in := "[project]\nname = \"demo\"\nplatforms = [\"linux-64\"]\n\n\n"
	got, err := ApplySpec(in, appliedSpec)
	if err != nil {
		t.Fatalf("ApplySpec: %v", err)
	}
	want := "[project]\nname = \"demo\"\nchannels = [\"conda-forge\", \"bioconda\"]\nplatforms = [\"linux-64\", \"osx-arm64\"]\n\n[dependencies]\npython = \"3.12.*\"\nnumpy = \">=2\"\n"
	if got != want {
		t.Fatalf("mismatch\n--- got ---\n%q\n--- want ---\n%q", got, want)
	}
}

func TestApplySpecPreservesCRLF(t *testing.T) {
	in := "[workspace]\r\nname = \"demo\"\r\nchannels = [\"conda-forge\"]\r\nplatforms = [\"linux-64\"]\r\n\r\n[dependencies]\r\npython = \"*\"\r\n"
	got, err := ApplySpec(in, Spec{Platforms: []string{"linux-64"}, Channels: []string{"conda-forge"}})
	if err != nil {
		t.Fatalf("ApplySpec: %v", err)
	}
	want := "[workspace]\r\nname = \"demo\"\r\nchannels = [\"conda-forge\"]\r\nplatforms = [\"linux-64\"]\r\n\r\n[dependencies]\r\n"
	if got != want {
		t.Fatalf("mismatch\n--- got ---\n%q\n--- want ---\n%q", got, want)
	}
}

func TestApplySpecRejects(t *testing.T) {
	base := "[workspace]\nname = \"demo\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	cases := map[string]struct {
		in   string
		spec Spec
	}{
		"invalid toml":            {"[workspace\n", appliedSpec},
		"no platform":             {base, Spec{Channels: []string{"conda-forge"}}},
		"no channel":              {base, Spec{Platforms: []string{"linux-64"}}},
		"bad package":             {base, Spec{Platforms: []string{"linux-64"}, Channels: []string{"c"}, Packages: []Package{{Name: "a b"}}}},
		"dotted dependency keys":  {"dependencies.python = \"*\"\n" + base, appliedSpec},
		"inline dependency table": {"dependencies = { python = \"*\" }\n" + base, appliedSpec},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ApplySpec(c.in, c.spec); !errors.Is(err, ErrInvalid) {
				t.Fatalf("want ErrInvalid, got %v", err)
			}
		})
	}
}

func TestHasPathDependencies(t *testing.T) {
	cases := map[string]bool{
		"[workspace]\nname = \"x\"\n\n[dependencies]\npython = \"*\"\n":                                false,
		"[workspace]\nname = \"x\"\n\n[pypi-dependencies]\nmine = { path = \".\", editable = true }\n": true,
		"[workspace]\nname = \"x\"\n\n[feature.dev.pypi-dependencies]\nlib = { path = \"../lib\" }\n":  true,
		"[workspace]\nname = \"x\"\n\n[target.linux-64.dependencies]\npkg = { path = \"./pkg\" }\n":    true,
		"[workspace]\nname = \"x\"\n\n[dev]\nmine = { path = \".\" }\n":                                true,
		"[workspace]\nname = \"x\"\n\n[pypi-dependencies]\nreq = { version = \">=2\" }\n":              false,
		"[workspace]\nname = \"x\"\n\n[tasks]\nbuild = { cmd = \"make\", cwd = \"path\" }\n":           false,
	}
	for in, want := range cases {
		got, err := HasPathDependencies(in)
		if err != nil {
			t.Fatalf("HasPathDependencies(%q): %v", in, err)
		}
		if got != want {
			t.Errorf("HasPathDependencies(%q) = %v, want %v", in, got, want)
		}
	}
	if _, err := HasPathDependencies("[workspace"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid toml: want ErrInvalid, got %v", err)
	}
}
