package artifact

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestRenderManifest(t *testing.T) {
	got, err := RenderManifest("my-analysis", "Q3 sales analysis", Spec{
		Platforms: []string{"osx-arm64", "linux-64"},
		Channels:  []string{"conda-forge"},
		Packages: []Package{
			{Name: "numpy", Operator: ">=", Constraint: "1.26"},
			{Name: "pandas", Operator: "==", Constraint: "2.2"},
			{Name: "python"},
			{Name: "scipy", Constraint: "1.13.*"},
			{Name: "ruamel.yaml"},
		},
	})
	if err != nil {
		t.Fatalf("RenderManifest: %v", err)
	}
	want := `[workspace]
name = "my-analysis"
description = "Q3 sales analysis"
channels = ["conda-forge"]
platforms = ["osx-arm64", "linux-64"]

[dependencies]
numpy = ">=1.26"
pandas = "==2.2"
python = "*"
scipy = "1.13.*"
"ruamel.yaml" = "*"
`
	if got != want {
		t.Fatalf("rendered manifest mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestRenderManifestOmitsEmptyDescription(t *testing.T) {
	got, err := RenderManifest("demo", "", Spec{Platforms: []string{"linux-64"}, Channels: []string{"conda-forge"}})
	if err != nil {
		t.Fatalf("RenderManifest: %v", err)
	}
	if strings.Contains(got, "description") {
		t.Fatalf("empty description should be left out:\n%s", got)
	}
}

func TestRenderManifestEscapesStrings(t *testing.T) {
	desc := "line one\nsays \"hi\" \\ tab\there \x01"
	out, err := RenderManifest("demo", desc, Spec{Platforms: []string{"linux-64"}, Channels: []string{"conda-forge"}})
	if err != nil {
		t.Fatalf("RenderManifest: %v", err)
	}
	m, err := ParseManifest(out)
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if m.Description != desc {
		t.Fatalf("description round trip: got %q, want %q", m.Description, desc)
	}
}

func TestRenderManifestRejectsBadSpecs(t *testing.T) {
	cf := []string{"conda-forge"}
	linux := []string{"linux-64"}
	cases := map[string]Spec{
		"no platform":           {Channels: cf},
		"empty platform":        {Platforms: []string{" "}, Channels: cf},
		"duplicate platform":    {Platforms: []string{"linux-64", "linux-64"}, Channels: cf},
		"no channel":            {Platforms: linux},
		"empty channel":         {Platforms: linux, Channels: []string{""}},
		"duplicate channel":     {Platforms: linux, Channels: []string{"conda-forge", "conda-forge"}},
		"bad package name":      {Platforms: linux, Channels: cf, Packages: []Package{{Name: "bad name"}}},
		"empty package name":    {Platforms: linux, Channels: cf, Packages: []Package{{Name: ""}}},
		"duplicate package":     {Platforms: linux, Channels: cf, Packages: []Package{{Name: "numpy"}, {Name: "NumPy"}}},
		"unknown operator":      {Platforms: linux, Channels: cf, Packages: []Package{{Name: "numpy", Operator: "=>", Constraint: "1"}}},
		"operator without vers": {Platforms: linux, Channels: cf, Packages: []Package{{Name: "numpy", Operator: ">="}}},
		"operator twice":        {Platforms: linux, Channels: cf, Packages: []Package{{Name: "numpy", Operator: ">=", Constraint: ">=1"}}},
	}
	for name, spec := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := RenderManifest("demo", "", spec)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("want ErrInvalid, got %v", err)
			}
		})
	}
	if _, err := RenderManifest(" ", "", Spec{Platforms: []string{"linux-64"}, Channels: []string{"conda-forge"}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty name: want ErrInvalid, got %v", err)
	}
}

func TestParseManifest(t *testing.T) {
	t.Run("workspace table", func(t *testing.T) {
		m, err := ParseManifest("[workspace]\nname = \"a\"\ndescription = \"d\"\nplatforms = [\"linux-64\", \"osx-arm64\"]\n")
		if err != nil {
			t.Fatal(err)
		}
		want := &Manifest{Name: "a", Description: "d", Platforms: []string{"linux-64", "osx-arm64"}}
		if !reflect.DeepEqual(m, want) {
			t.Fatalf("got %+v, want %+v", m, want)
		}
	})
	t.Run("legacy project table", func(t *testing.T) {
		m, err := ParseManifest("[project]\nname = \"old\"\nplatforms = [\"win-64\"]\n")
		if err != nil {
			t.Fatal(err)
		}
		if m.Name != "old" || !reflect.DeepEqual(m.Platforms, []string{"win-64"}) {
			t.Fatalf("got %+v", m)
		}
	})
	t.Run("no platforms", func(t *testing.T) {
		m, err := ParseManifest("[workspace]\nname = \"a\"\n")
		if err != nil {
			t.Fatal(err)
		}
		if len(m.Platforms) != 0 {
			t.Fatalf("want no platforms, got %v", m.Platforms)
		}
	})
	bad := map[string]string{
		"not toml":             "this is = = not toml",
		"no workspace table":   "[dependencies]\nnumpy = \"*\"\n",
		"platforms not a list": "[workspace]\nplatforms = \"linux-64\"\n",
		"platform not string":  "[workspace]\nplatforms = [1]\n",
		"name not string":      "[workspace]\nname = 3\nplatforms = [\"linux-64\"]\n",
	}
	for name, content := range bad {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseManifest(content); !errors.Is(err, ErrInvalid) {
				t.Fatalf("want ErrInvalid, got %v", err)
			}
		})
	}
}

func TestPackageVersionSpec(t *testing.T) {
	cases := []struct {
		pkg  Package
		want string
	}{
		{Package{Name: "a"}, "*"},
		{Package{Name: "a", Constraint: ">=1,<2"}, ">=1,<2"},
		{Package{Name: "a", Operator: "==", Constraint: "1.0"}, "==1.0"},
		{Package{Name: "a", Operator: " ~= ", Constraint: " 1.4 "}, "~=1.4"},
	}
	for _, c := range cases {
		got, err := c.pkg.VersionSpec()
		if err != nil || got != c.want {
			t.Errorf("%+v: got %q, %v; want %q", c.pkg, got, err, c.want)
		}
	}
}

// TestParseManifestLeavesChannelsToPixi checks that channels are not read
// or checked: any form, including one pixi would reject, parses, and pixi
// lock reports the problem.
func TestParseManifestLeavesChannelsToPixi(t *testing.T) {
	for name, content := range map[string]string{
		"names and tables":      "[workspace]\nchannels = [\"conda-forge\", { channel = \"bioconda\", priority = 1 }]\nplatforms = [\"linux-64\"]\n",
		"empty":                 "[workspace]\nchannels = []\nplatforms = [\"linux-64\"]\n",
		"channels not a list":   "[workspace]\nchannels = \"conda-forge\"\nplatforms = [\"linux-64\"]\n",
		"channel table no name": "[workspace]\nchannels = [{ priority = 1 }]\nplatforms = [\"linux-64\"]\n",
	} {
		m, err := ParseManifest(content)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if !reflect.DeepEqual(m.Platforms, []string{"linux-64"}) {
			t.Errorf("%s: platforms = %v", name, m.Platforms)
		}
	}
}

func TestManifestValidate(t *testing.T) {
	// Platform names are not checked against a list: these are newer than
	// some pixi releases, and pixi lock decides which it supports.
	for name, m := range map[string]*Manifest{
		"common platforms": {Platforms: []string{"linux-64", "osx-arm64", "win-64"}},
		"newer platforms":  {Platforms: []string{"linux-64", "linux-loongarch64", "freebsd-64", "noarch", "linux-ppc", "ios-arm64", "android-arm64"}},
		"no platforms":     {},
	} {
		if err := m.Validate(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	dup := &Manifest{Platforms: []string{"osx-64", "osx-64"}}
	if err := dup.Validate(); !errors.Is(err, ErrInvalid) {
		t.Errorf("duplicate platform: want ErrInvalid, got %v", err)
	}
}

func TestRenderManifestLeavesPlatformNamesToPixi(t *testing.T) {
	out, err := RenderManifest("demo", "", Spec{Platforms: []string{"linux-64", "freebsd-64"}, Channels: []string{"conda-forge"}})
	if err != nil {
		t.Fatalf("RenderManifest: %v", err)
	}
	if !strings.Contains(out, `platforms = ["linux-64", "freebsd-64"]`) {
		t.Fatalf("platforms not rendered as given:\n%s", out)
	}
}
