package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nebari-dev/nebi/internal/artifact"
	"github.com/nebari-dev/nebi/internal/client"
	"github.com/nebari-dev/nebi/internal/client/sdk/local"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/models"
)

func TestParsePackageArg(t *testing.T) {
	cases := map[string]artifact.RequestedPackage{
		"numpy":              {Name: "numpy", Constraint: "*"},
		"numpy=>=2.0":        {Name: "numpy", Constraint: ">=2.0"},
		"numpy>=2.0":         {Name: "numpy", Constraint: ">=2.0"},
		"numpy==2.1":         {Name: "numpy", Constraint: "==2.1"},
		"python=3.12.*":      {Name: "python", Constraint: "3.12.*"},
		"pypi:requests>=2":   {Name: "requests", Constraint: ">=2", Source: artifact.SourcePyPI},
		"conda:ruamel.yaml":  {Name: "ruamel.yaml", Constraint: "*"},
		" scikit-learn = 1 ": {Name: "scikit-learn", Constraint: "1"},
	}
	for in, want := range cases {
		got, err := parsePackageArg(in)
		if err != nil || got != want {
			t.Errorf("parsePackageArg(%q) = %+v, %v; want %+v", in, got, err, want)
		}
	}
	if _, err := parsePackageArg(">=2"); err == nil {
		t.Error("a constraint without a name must fail")
	}
}

func TestApplyFormEdits(t *testing.T) {
	base := shared.ManifestForm{
		Name:      "demo",
		Channels:  []string{"conda-forge"},
		Platforms: []string{"linux-64"},
		Requested: []artifact.RequestedPackage{{Name: "python", Constraint: "3.12.*"}, {Name: "numpy", Constraint: ">=2.0"}},
	}
	got, err := applyFormEdits(base, newVersionOptions{
		addPackages:    []string{"python=3.13.*", "pandas"},
		removePackages: []string{"NumPy"},
		addPlatforms:   []string{"osx-arm64", "linux-64"},
		addChannels:    []string{"pytorch"},
		removeChannels: []string{"conda-forge"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Requested) != 2 || got.Requested[0].Constraint != "3.13.*" || got.Requested[1].Name != "pandas" {
		t.Fatalf("requested = %+v", got.Requested)
	}
	if strings.Join(got.Platforms, ",") != "linux-64,osx-arm64" || strings.Join(got.Channels, ",") != "pytorch" {
		t.Fatalf("platforms = %v channels = %v", got.Platforms, got.Channels)
	}
	if base.Requested[0].Constraint != "3.12.*" || len(base.Channels) != 1 {
		t.Fatal("applyFormEdits must not modify the base form")
	}
	if _, err := applyFormEdits(base, newVersionOptions{removePackages: []string{"scipy"}}); err == nil {
		t.Fatal("removing a missing package must fail")
	}
}

func TestVersionList(t *testing.T) {
	if got := versionList([]int{6}); got != "version 6" {
		t.Fatal(got)
	}
	if got := versionList([]int{5, 6, 7}); got != "versions 5, 6 and 7" {
		t.Fatal(got)
	}
}

// cliFakeExec solves by writing a fake pixi.lock.
type cliFakeExec struct{ staging string }

func (cliFakeExec) CreateProject(context.Context, *models.Project, io.Writer, executor.CreateProjectOptions) error {
	return nil
}
func (cliFakeExec) InstallPackages(context.Context, *models.Project, []string, io.Writer) error {
	return nil
}
func (cliFakeExec) RemovePackages(context.Context, *models.Project, []string, io.Writer) error {
	return nil
}
func (cliFakeExec) DeleteProject(context.Context, *models.Project, io.Writer) error { return nil }
func (cliFakeExec) SolveEnvironment(_ context.Context, p *models.Project, _ io.Writer) error {
	return os.WriteFile(filepath.Join(p.Path, "pixi.lock"), []byte("version: 6\n"), 0o644)
}
func (cliFakeExec) CleanupJobArtifacts(context.Context, *models.Project, models.JobType, io.Writer) error {
	return nil
}
func (cliFakeExec) InstallEnvironment(context.Context, *models.Project, io.Writer) error {
	return nil
}
func (cliFakeExec) UninstallEnvironment(context.Context, *models.Project, io.Writer) error {
	return nil
}
func (cliFakeExec) IsEnvInstalled(*models.Project) bool     { return false }
func (cliFakeExec) GetProjectPath(p *models.Project) string { return p.Path }
func (e cliFakeExec) StagingRoot() string                   { return e.staging }

func TestRunNewVersion(t *testing.T) {
	dataDir := t.TempDir()
	gdb, err := local.OpenDB(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := gdb.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := local.Migrate(gdb); err != nil {
		t.Fatal(err)
	}
	var jobLog strings.Builder
	stack, err := client.NewStack(client.StackOptions{
		DB: gdb, Executor: cliFakeExec{staging: t.TempDir()}, DataDir: dataDir, InlineLog: &jobLog,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	user := stack.SDK.Deps().LocalUserID
	p := models.Project{Name: "demo", OwnerID: user, Status: models.ProjectStatusReady}
	if err := gdb.Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	toml := "[workspace]\nname = \"demo\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n\n[dependencies]\npython = \"3.12.*\"\nnumpy = \">=2.0\"\n"
	for n := 1; n <= 2; n++ {
		v := models.ProjectVersion{ProjectID: p.ID, ManifestContent: toml, LockFileContent: "version: 6\n", PackageMetadata: "[]", CreatedBy: user}
		if err := gdb.Create(&v).Error; err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()

	var out strings.Builder
	err = runNewVersion(ctx, &out, stack, "demo", "", newVersionOptions{from: 1, addPackages: []string{"pandas=>=2.2"}, removePackages: []string{"numpy"}, dryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Version 1 is older than the latest version, v2. Changes made in version 2 won't carry over.",
		"Summary of changes from version 1:",
		"  + pandas >=2.2",
		"  - numpy >=2.0",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("dry run output missing %q:\n%s", want, out.String())
		}
	}
	var count int64
	gdb.Model(&models.ProjectVersion{}).Where("project_id = ?", p.ID).Count(&count)
	if count != 2 {
		t.Fatalf("dry run created a version: %d", count)
	}

	out.Reset()
	err = runNewVersion(ctx, &out, stack, "demo", "", newVersionOptions{addPackages: []string{"rich"}, tags: []string{"stable"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Description (suggested from your changes): Added rich") ||
		!strings.Contains(out.String(), "Version 3 created\nStarted from version 2 · added rich") {
		t.Fatalf("output:\n%s", out.String())
	}

	out.Reset()
	err = runNewVersion(ctx, &out, stack, "demo", "", newVersionOptions{from: 3, addChannels: []string{"conda-forge"}})
	if err == nil || !strings.Contains(out.String(), "Nothing changed in demo.") {
		t.Fatalf("no-change run: err=%v out=%s", err, out.String())
	}
}
