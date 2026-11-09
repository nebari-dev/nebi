package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nebari-dev/nebi/internal/artifact"
	"github.com/nebari-dev/nebi/internal/client"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/models"
)

const cliCompareToml1 = `[workspace]
name = "demo"
channels = ["conda-forge"]
platforms = ["linux-64"]

[dependencies]
python = "3.12.*"
scipy = "*"
`

const cliCompareToml2 = `[workspace]
name = "demo"
channels = ["conda-forge", "bioconda"]
platforms = ["linux-64"]

[dependencies]
python = "3.13.*"
pandas = "*"

[pypi-dependencies]
rich = ">=13"
`

func TestCompareVersionLabel(t *testing.T) {
	cases := []struct {
		ref  shared.CompareVersionRef
		want string
	}{
		{shared.CompareVersionRef{Number: 3}, "v3"},
		{shared.CompareVersionRef{Number: 7, Installed: true}, "v7 (installed)"},
		{shared.CompareVersionRef{Number: 6, ServerLatest: true, ServerOnly: true}, "v6 (latest on server)"},
		{shared.CompareVersionRef{Number: 5, Installed: true, ServerLatest: true}, "v5 (installed, latest on server)"},
		{shared.CompareVersionRef{Number: 4, ServerOnly: true}, "v4 (on server)"},
	}
	for _, tc := range cases {
		if got := compareVersionLabel(tc.ref); got != tc.want {
			t.Errorf("compareVersionLabel(%+v) = %q, want %q", tc.ref, got, tc.want)
		}
	}
}

func TestWriteVersionDiff(t *testing.T) {
	d, err := artifact.DiffVersions(
		artifact.VersionContent{Toml: []byte(cliCompareToml1)},
		artifact.VersionContent{Toml: []byte(cliCompareToml2)},
		artifact.DiffOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	writeVersionDiff(&buf, "v1", "v2 (installed)", d, true)
	out := buf.String()
	for _, want := range []string{
		"Compare versions: v1 -> v2 (installed)",
		"Summary of changes: +2 -1 ~1",
		"Channels: +bioconda",
		"Requested packages",
		"rich (pypi)",
		"Resolved packages (no lock): No changes",
		"(none)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output misses %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\u2014") {
		t.Errorf("output must not use em-dashes:\n%s", out)
	}
	assertRow(t, out, "+", "pandas", "-", "*")
	assertRow(t, out, "-", "scipy", "*", "-")
	assertRow(t, out, "~", "python", "3.12.*", "3.13.*")

	same, _ := artifact.DiffVersions(
		artifact.VersionContent{Toml: []byte(cliCompareToml1)},
		artifact.VersionContent{Toml: []byte(cliCompareToml1)},
		artifact.DiffOptions{},
	)
	buf.Reset()
	writeVersionDiff(&buf, "v1", "v1", same, false)
	if !strings.Contains(buf.String(), "Summary of changes: No changes") || strings.Contains(buf.String(), "Resolved packages") {
		t.Errorf("same-version output:\n%s", buf.String())
	}
}

// assertRow finds the table row of name and checks its marker and sides.
func assertRow(t *testing.T, out, marker, name, base, compare string) {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if marker != " " && len(fields) == 4 && fields[1] == name {
			if fields[0] != marker || fields[2] != base || fields[3] != compare {
				t.Errorf("row %q = %q, want %s %s %s %s", name, line, marker, name, base, compare)
			}
			return
		}
	}
	t.Errorf("no row for %q in:\n%s", name, out)
}

// runCLI executes the root command with args and returns its stdout.
func runCompareCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(io.Discard)
	rootCmd.SetArgs(args)
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
	})
	err := rootCmd.Execute()
	return out.String(), err
}

func TestProjectCompareCommand(t *testing.T) {
	t.Setenv("NEBI_DATA_DIR", t.TempDir())
	t.Setenv("NEBI_STORAGE_PROJECTS_DIR", t.TempDir())

	stack, closeFn, err := client.OpenCLI(io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	gdb := stack.SDK.Deps().DB
	user := stack.SDK.Deps().LocalUserID
	installed := 2
	p := models.Project{Name: "cli-compare", OwnerID: user, Status: models.ProjectStatusReady, InstalledVersion: &installed}
	if err := gdb.Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	for n, toml := range map[int]string{1: cliCompareToml1, 2: cliCompareToml2} {
		v := models.ProjectVersion{ProjectID: p.ID, VersionNumber: n, ManifestContent: toml, PackageMetadata: "[]", CreatedBy: user}
		if err := gdb.Create(&v).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := closeFn(); err != nil {
		t.Fatal(err)
	}

	out, err := runCompareCLI(t, "project", "compare", "cli-compare", "1", "2")
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if !strings.Contains(out, "Compare versions: v1 -> v2 (installed)") || !strings.Contains(out, "Summary of changes: +2 -1 ~1") {
		t.Errorf("output:\n%s", out)
	}

	if _, err := runCompareCLI(t, "project", "compare", "cli-compare", "1", "9"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("missing version: %v", err)
	}
	if _, err := runCompareCLI(t, "project", "compare", "no-such-project", "1", "2"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("missing project: %v", err)
	}
	if _, err := runCompareCLI(t, "project", "compare", "cli-compare", "x", "2"); err == nil || !strings.Contains(err.Error(), "invalid version") {
		t.Errorf("bad version arg: %v", err)
	}
}

func TestDiffPackagesFlag(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "a")
	b := filepath.Join(root, "b")
	for dir, toml := range map[string]string{a: cliCompareToml1, b: cliCompareToml2} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "pixi.toml"), []byte(toml), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { diffPackages, diffLock = false, false })

	out, err := runCompareCLI(t, "diff", "--packages", a, b)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Compare versions: "+a+" -> "+b) || !strings.Contains(out, "Summary of changes: +2 -1 ~1") {
		t.Errorf("output:\n%s", out)
	}
	assertRow(t, out, "+", "pandas", "-", "*")
}
