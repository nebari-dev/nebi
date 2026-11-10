package artifact

import (
	"errors"
	"os"
	"reflect"
	"slices"
	"testing"
)

const lockV6 = `version: 6
environments:
  default:
    channels:
    - url: https://conda.anaconda.org/conda-forge/
    indexes:
    - https://pypi.org/simple
    packages:
      linux-64:
      - conda: https://conda.anaconda.org/conda-forge/linux-64/python-3.12.7-hc5c86c4_0_cpython.conda
      - conda: https://conda.anaconda.org/conda-forge/noarch/tzdata-2024b-hc8b5060_0.conda
      - conda: https://prefix.dev/my-channel/linux-64/ruamel.yaml-0.18.6-py312h66e93f0_1.conda
      - conda: https://repo.example.com/private/linux-64/internal-tool-1.0-h0_0.tar.bz2
      - pypi: https://files.pythonhosted.org/packages/aa/bb/rich-13.9.4-py3-none-any.whl
      - conda: https://conda.anaconda.org/conda-forge/linux-64/missing-1.0-h0_0.conda
      - other: ignored
      osx-arm64:
      - conda: https://conda.anaconda.org/conda-forge/osx-arm64/python-3.12.7-h739c21a_0_cpython.conda
      - pypi: https://files.pythonhosted.org/packages/aa/bb/rich-13.9.4-py3-none-any.whl
  test:
    channels:
    - url: https://conda.anaconda.org/conda-forge/
    packages:
      linux-64:
      - conda: https://conda.anaconda.org/conda-forge/linux-64/python-3.12.7-hc5c86c4_0_cpython.conda
packages:
- conda: https://conda.anaconda.org/conda-forge/linux-64/python-3.12.7-hc5c86c4_0_cpython.conda
  sha256: abc
  size: 31574779
- conda: https://conda.anaconda.org/conda-forge/osx-arm64/python-3.12.7-h739c21a_0_cpython.conda
  size: 12975439
- conda: https://conda.anaconda.org/conda-forge/noarch/tzdata-2024b-hc8b5060_0.conda
  subdir: noarch
  size: 122354
- conda: https://prefix.dev/my-channel/linux-64/ruamel.yaml-0.18.6-py312h66e93f0_1.conda
  size: 1
- conda: https://repo.example.com/private/linux-64/internal-tool-1.0-h0_0.tar.bz2
  name: internal-tool
  version: '1.0'
  size: not-a-number
- pypi: https://files.pythonhosted.org/packages/aa/bb/rich-13.9.4-py3-none-any.whl
  name: rich
  version: 13.9.4
- conda: https://conda.anaconda.org/conda-forge/linux-64/python-3.12.7-hc5c86c4_0_cpython.conda
  size: 999
- something: else
`

const lockV5 = `version: 5
environments:
  default:
    channels:
    - url: https://conda.anaconda.org/conda-forge/
    packages:
      linux-64:
      - conda: https://conda.anaconda.org/conda-forge/linux-64/numpy-2.1.0-py312h1103770_0.conda
      - pypi: https://files.pythonhosted.org/packages/rich-13.0.0-py3-none-any.whl
packages:
- kind: conda
  name: numpy
  version: 2.1.0
  build: py312h1103770_0
  subdir: linux-64
  url: https://conda.anaconda.org/conda-forge/linux-64/numpy-2.1.0-py312h1103770_0.conda
  size: 8456789
- kind: pypi
  name: rich
  version: 13.0.0
  url: https://files.pythonhosted.org/packages/rich-13.0.0-py3-none-any.whl
`

func TestParseLockV6(t *testing.T) {
	l, err := ParseLock([]byte(lockV6))
	if err != nil {
		t.Fatal(err)
	}
	if l.Version != 6 {
		t.Errorf("Version = %d", l.Version)
	}
	env := l.Environment("")
	if env == nil || env.Name != "default" {
		t.Fatalf("default env = %+v", env)
	}
	if !slices.Equal(env.Channels, []string{"https://conda.anaconda.org/conda-forge/"}) {
		t.Errorf("channels = %v", env.Channels)
	}
	if !slices.Equal(env.Platforms(), []string{"linux-64", "osx-arm64"}) {
		t.Errorf("platforms = %v", env.Platforms())
	}
	got := l.Resolved("", "linux-64")
	want := []ResolvedPackage{
		{Name: "internal-tool", Version: "1.0", Build: "h0_0", Source: SourceConda, Platform: "linux-64",
			Channel: "https://repo.example.com/private", ChannelURL: "https://repo.example.com/private",
			URL: "https://repo.example.com/private/linux-64/internal-tool-1.0-h0_0.tar.bz2"},
		{Name: "missing", Version: "1.0", Build: "h0_0", Source: SourceConda, Platform: "linux-64",
			Channel: "conda-forge", ChannelURL: "https://conda.anaconda.org/conda-forge",
			URL: "https://conda.anaconda.org/conda-forge/linux-64/missing-1.0-h0_0.conda"},
		{Name: "python", Version: "3.12.7", Build: "hc5c86c4_0_cpython", Source: SourceConda, Platform: "linux-64",
			Channel: "conda-forge", ChannelURL: "https://conda.anaconda.org/conda-forge",
			URL: "https://conda.anaconda.org/conda-forge/linux-64/python-3.12.7-hc5c86c4_0_cpython.conda", Size: 31574779},
		{Name: "rich", Version: "13.9.4", Source: SourcePyPI, Platform: "linux-64", Channel: "pypi",
			URL: "https://files.pythonhosted.org/packages/aa/bb/rich-13.9.4-py3-none-any.whl"},
		{Name: "ruamel.yaml", Version: "0.18.6", Build: "py312h66e93f0_1", Source: SourceConda, Platform: "linux-64",
			Channel: "my-channel", ChannelURL: "https://prefix.dev/my-channel",
			URL: "https://prefix.dev/my-channel/linux-64/ruamel.yaml-0.18.6-py312h66e93f0_1.conda", Size: 1},
		{Name: "tzdata", Version: "2024b", Build: "hc8b5060_0", Source: SourceConda, Platform: "linux-64",
			Channel: "conda-forge", ChannelURL: "https://conda.anaconda.org/conda-forge",
			URL: "https://conda.anaconda.org/conda-forge/noarch/tzdata-2024b-hc8b5060_0.conda", Size: 122354},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Resolved =\n%#v\nwant\n%#v", got, want)
	}
	if n := TotalSize(got); n != 31574779+122354+1 {
		t.Errorf("TotalSize = %d", n)
	}
	osx := l.Resolved("default", "osx-arm64")
	if len(osx) != 2 || osx[0].Size != 12975439 || osx[0].Platform != "osx-arm64" {
		t.Errorf("osx = %+v", osx)
	}
	if len(l.Resolved("test", "linux-64")) != 1 {
		t.Error("test env")
	}
	if l.Resolved("nope", "linux-64") != nil || l.Resolved("", "win-64") != nil {
		t.Error("missing env/platform should be nil")
	}
}

func TestParseLockV5(t *testing.T) {
	l, err := ParseLock([]byte(lockV5))
	if err != nil {
		t.Fatal(err)
	}
	got := l.Resolved("", "linux-64")
	want := []ResolvedPackage{
		{Name: "numpy", Version: "2.1.0", Build: "py312h1103770_0", Source: SourceConda, Platform: "linux-64",
			Channel: "conda-forge", ChannelURL: "https://conda.anaconda.org/conda-forge",
			URL: "https://conda.anaconda.org/conda-forge/linux-64/numpy-2.1.0-py312h1103770_0.conda", Size: 8456789},
		{Name: "rich", Version: "13.0.0", Source: SourcePyPI, Platform: "linux-64", Channel: "pypi",
			URL: "https://files.pythonhosted.org/packages/rich-13.0.0-py3-none-any.whl"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Resolved =\n%#v\nwant\n%#v", got, want)
	}
}

func TestParseLockV7Real(t *testing.T) {
	content, err := os.ReadFile("testdata/v7/pixi.lock")
	if err != nil {
		t.Fatal(err)
	}
	l, err := ParseLock(content)
	if err != nil {
		t.Fatal(err)
	}
	if l.Version != 7 {
		t.Errorf("Version = %d", l.Version)
	}
	linux := l.Resolved("", "linux-64")
	if len(linux) < 15 {
		t.Fatalf("linux-64 has %d packages", len(linux))
	}
	byName := map[string]ResolvedPackage{}
	for _, p := range linux {
		byName[p.Name] = p
	}
	py := byName["python"]
	if py.Version != "3.12.16" || py.Channel != "conda-forge" || py.Size <= 0 || py.Source != SourceConda {
		t.Errorf("python = %+v", py)
	}
	if six := byName["six"]; six.Version != "1.17.0" || six.Channel != "pypi" || six.Size != 0 {
		t.Errorf("six = %+v", six)
	}
	if tz := byName["tzdata"]; tz.ChannelURL != "https://conda.anaconda.org/conda-forge" {
		t.Errorf("tzdata = %+v", tz)
	}
	if len(l.Resolved("", "osx-arm64")) == 0 {
		t.Error("osx-arm64 empty")
	}
}

func TestParseLockErrors(t *testing.T) {
	l, err := ParseLock(nil)
	if err != nil || len(l.Environments) != 0 {
		t.Fatalf("empty lock = %+v, %v", l, err)
	}
	if _, err := ParseLock([]byte("version: 3\npackage: []\n")); !errors.Is(err, ErrUnsupportedLock) {
		t.Errorf("v3 err = %v", err)
	}
	if _, err := ParseLock([]byte("packages: []\n")); !errors.Is(err, ErrUnsupportedLock) {
		t.Errorf("no version err = %v", err)
	}
	if _, err := ParseLock([]byte("version: [\n")); err == nil {
		t.Error("bad yaml: want error")
	}
	var nilLock *Lock
	if nilLock.Environment("") != nil || nilLock.Resolved("", "x") != nil {
		t.Error("nil lock")
	}
}

func TestParseCondaFilename(t *testing.T) {
	tests := []struct {
		in, name, version, build string
	}{
		{"https://x/linux-64/python-3.12.7-hc5c86c4_0_cpython.conda", "python", "3.12.7", "hc5c86c4_0_cpython"},
		{"ld_impl_linux-64-2.46.1-default_hbd61a6d_103.conda", "ld_impl_linux-64", "2.46.1", "default_hbd61a6d_103"},
		{"python_abi-3.12-5_cp312.tar.bz2", "python_abi", "3.12", "5_cp312"},
		{"tzdata-2024b-hc8b5060_0.conda", "tzdata", "2024b", "hc8b5060_0"},
		{"foo.whl", "", "", ""},
		{"a-b.conda", "", "", ""},
		{"name-nover-build.conda", "", "", ""},
	}
	for _, tt := range tests {
		n, v, b := ParseCondaFilename(tt.in)
		if n != tt.name || v != tt.version || b != tt.build {
			t.Errorf("ParseCondaFilename(%q) = %q %q %q", tt.in, n, v, b)
		}
	}
}

func TestChannelFromPackageURL(t *testing.T) {
	tests := []struct {
		url, subdir, name, base string
	}{
		{"https://conda.anaconda.org/conda-forge/linux-64/x-1-0.conda", "", "conda-forge", "https://conda.anaconda.org/conda-forge"},
		{"https://conda.anaconda.org/pytorch/noarch/x-1-0.conda", "noarch", "pytorch", "https://conda.anaconda.org/pytorch"},
		{"https://prefix.dev/conda-forge/osx-arm64/x-1-0.conda", "", "conda-forge", "https://prefix.dev/conda-forge"},
		{"https://repo.example.com/a/b/custom-sub/x-1-0.conda", "custom-sub", "https://repo.example.com/a/b", "https://repo.example.com/a/b"},
		{"https://repo.example.com/a/x-1-0.conda", "", "https://repo.example.com/a", "https://repo.example.com/a"},
		{".", "", "", ""},
		{"file:///abs/path/x-1-0.conda", "", "", ""},
	}
	for _, tt := range tests {
		n, b := ChannelFromPackageURL(tt.url, tt.subdir)
		if n != tt.name || b != tt.base {
			t.Errorf("ChannelFromPackageURL(%q) = %q, %q; want %q, %q", tt.url, n, b, tt.name, tt.base)
		}
	}
	if ChannelDisplayName("conda-forge") != "conda-forge" ||
		ChannelDisplayName("https://conda.anaconda.org/conda-forge/") != "conda-forge" ||
		ChannelDisplayName("https://example.com/ch/") != "https://example.com/ch" {
		t.Error("ChannelDisplayName")
	}
}
