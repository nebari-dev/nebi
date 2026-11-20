package artifact

import (
	"errors"
	"strings"
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
python = { version = "3.11.*" }
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

func TestApplySpecRefusesToDropSettings(t *testing.T) {
	const head = "[workspace]\nname = \"demo\"\nplatforms = [\"linux-64\"]\n"
	refused := map[string]string{
		"dependency channel":   head + "channels = [\"conda-forge\"]\n\n[dependencies]\nPython = { version = \"3.11.*\", channel = \"conda-forge\" }\n",
		"dependency build":     head + "channels = [\"conda-forge\"]\n\n[dependencies]\nnumpy = { version = \"2.*\", build = \"py312*\" }\n",
		"channel priority":     head + "channels = [{ channel = \"bioconda\", priority = 1 }, \"conda-forge\"]\n",
		"multi-line dep table": head + "channels = [\"conda-forge\"]\n\n[dependencies.numpy]\nversion = \"2.*\"\nchannel = \"conda-forge\"\n",
	}
	for name, in := range refused {
		t.Run(name, func(t *testing.T) {
			if _, err := ApplySpec(in, appliedSpec); !errors.Is(err, ErrInvalid) {
				t.Fatalf("want ErrInvalid instead of dropping the setting, got %v", err)
			}
		})
	}

	// Settings of a package or channel the spec removes go with it.
	in := head + "channels = [{ channel = \"pytorch\", priority = 1 }]\n\n[dependencies]\npytorch = { version = \"2.*\", channel = \"pytorch\" }\n"
	got, err := ApplySpec(in, appliedSpec)
	if err != nil {
		t.Fatalf("ApplySpec: %v", err)
	}
	if strings.Contains(got, "pytorch") {
		t.Fatalf("removed package and channel must be gone:\n%s", got)
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

func TestHasLocalPaths(t *testing.T) {
	const ws = "[workspace]\nname = \"x\"\nplatforms = [\"linux-64\"]\n"
	cases := map[string]bool{
		// Path dependencies, anywhere.
		ws + "channels = [\"conda-forge\"]\n\n[dependencies]\npython = \"*\"\n":  false,
		ws + "\n[pypi-dependencies]\nmine = { path = \".\", editable = true }\n": true,
		ws + "\n[feature.dev.pypi-dependencies]\nlib = { path = \"../lib\" }\n":  true,
		ws + "\n[target.linux-64.dependencies]\npkg = { path = \"./pkg\" }\n":    true,
		ws + "\n[dev]\nmine = { path = \".\" }\n":                                true,
		ws + "\n[pypi-dependencies]\nreq = { version = \">=2\" }\n":              false,
		ws + "\n[tasks]\nbuild = { cmd = \"make\", cwd = \"path\" }\n":           false,
		// Channels.
		ws + "channels = [\"./local-channel\"]\n":                                                                  true,
		ws + "channels = [\"conda-forge\", { channel = \"../chan\", priority = 1 }]\n":                             true,
		ws + "channels = [\"file://./chan\"]\n":                                                                    true,
		ws + "channels = [\"file:///opt/chan\", \"/opt/chan\", \"~/chan\"]\n":                                      false,
		ws + "channels = [\"conda-forge/label/dev\", \"https://prefix.dev/x\"]\n":                                  false,
		ws + "channels = [\"conda-forge\"]\n\n[feature.x]\nchannels = [\"./feat\"]\n":                              true,
		ws + "channels = [\"conda-forge\"]\n\n[dependencies]\npkg = { version = \"*\", channel = \"./c\" }\n":      true,
		ws + "channels = [\"conda-forge\"]\n\n[dependencies]\npkg = { version = \"*\", channel = \"bioconda\" }\n": false,
		// PyPI options.
		ws + "\n[pypi-options]\nfind-links = [{ path = \"./wheels\" }]\n":                                    true,
		ws + "\n[pypi-options]\nfind-links = [{ path = \"wheels\" }]\n":                                      true,
		ws + "\n[pypi-options]\nfind-links = [{ path = \"/srv/wheels\" }, { url = \"https://x/simple\" }]\n": false,
		ws + "\n[pypi-options]\nfind-links = [{ url = \"file:./wheels\" }]\n":                                true,
		ws + "\n[pypi-options]\nindex-url = \"file:./index\"\n":                                              true,
		ws + "\n[pypi-options]\nextra-index-urls = [\"https://pypi.org/simple\", \"file:../idx\"]\n":         true,
		ws + "\n[pypi-options]\nindex-url = \"https://pypi.org/simple\"\n":                                   false,
		ws + "\n[feature.gpu.pypi-options]\nfind-links = [{ path = \"./gpu-wheels\" }]\n":                    true,
	}
	for in, want := range cases {
		got, err := HasLocalPaths(in)
		if err != nil {
			t.Fatalf("HasLocalPaths(%q): %v", in, err)
		}
		if got != want {
			t.Errorf("HasLocalPaths(%q) = %v, want %v", in, got, want)
		}
	}
	if _, err := HasLocalPaths("[workspace"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid toml: want ErrInvalid, got %v", err)
	}
}
