//go:build e2e

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nebari-dev/nebi/internal/artifact"
	"github.com/nebari-dev/nebi/internal/auth"
	"github.com/nebari-dev/nebi/internal/client/sdk/local"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/models"
)

// seedDetailProject stores project "artlock" with versions 1 and 2 in the
// local database of a fresh data dir.
func seedDetailProject(t *testing.T) (toml string) {
	t.Helper()
	dataDir := t.TempDir()
	t.Setenv("NEBI_DATA_DIR", dataDir)
	readFile := func(name string) string {
		b, err := os.ReadFile(filepath.Join("..", "..", "internal", "artifact", "testdata", "v7", name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	toml = readFile("pixi.toml")
	lock := readFile("pixi.lock")

	gdb, err := local.OpenDB(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if sqlDB, err := gdb.DB(); err == nil {
			sqlDB.Close()
		}
	}()
	if err := local.Migrate(gdb); err != nil {
		t.Fatal(err)
	}
	la, err := auth.NewLocalAuthenticator(gdb)
	if err != nil {
		t.Fatal(err)
	}
	user := la.User().ID
	p := models.Project{Name: "artlock", OwnerID: user, Status: models.ProjectStatusReady, Path: filepath.Join(dataDir, "projects", "artlock")}
	if err := gdb.Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	for n, desc := range []string{"First version", "Added six"} {
		v := models.ProjectVersion{ProjectID: p.ID, VersionNumber: n + 1, ManifestContent: toml, LockFileContent: lock, PackageMetadata: "[]", CreatedBy: user, Description: desc}
		if err := gdb.Create(&v).Error; err != nil {
			t.Fatal(err)
		}
		if err := local.ApplyTags(gdb, p.ID, n+1, []string{"stable"}, user); err != nil {
			t.Fatal(err)
		}
	}
	return toml
}

func TestDetailCLICommands(t *testing.T) {
	toml := seedDetailProject(t)
	work := t.TempDir()

	res := runCLI(t, work, "project", "versions", "artlock")
	if res.ExitCode != 0 {
		t.Fatalf("versions: %+v", res)
	}
	for _, want := range []string{"VERSION", "v2", "latest, stable", "Added six", "v1", "First version"} {
		if !strings.Contains(res.Stdout, want) {
			t.Fatalf("versions output lacks %q:\n%s", want, res.Stdout)
		}
	}

	res = runCLI(t, work, "project", "file", "artlock", "1", "pixi.toml")
	if res.ExitCode != 0 || res.Stdout != toml {
		t.Fatalf("file: %+v", res)
	}
	if res := runCLI(t, work, "project", "file", "artlock", "1", "notes.txt"); res.ExitCode == 0 {
		t.Fatal("unknown file name must fail")
	}

	res = runCLI(t, work, "project", "packages", "artlock", "2", "--platform", "linux-64")
	if res.ExitCode != 0 || !strings.Contains(res.Stdout, "Platform: linux-64") || !strings.Contains(res.Stdout, "python") || !strings.Contains(res.Stdout, "conda-forge") {
		t.Fatalf("packages: %+v", res)
	}
	if res := runCLI(t, work, "project", "packages", "artlock", "2", "--platform", "win-64"); res.ExitCode == 0 {
		t.Fatal("platform missing from the lock must fail")
	}

	out := filepath.Join(work, "v2.tar.gz")
	res = runCLI(t, work, "project", "archive", "artlock", "2", "-o", out)
	if res.ExitCode != 0 {
		t.Fatalf("archive: %+v", res)
	}
	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	got, err := artifact.ReadArchive(f, 0)
	f.Close()
	if err != nil || got.Metadata.Version != 2 || string(got.Toml) != toml {
		t.Fatalf("archive content: %v", err)
	}

	if res := runCLI(t, work, "project", "version", "uninstall", "artlock"); res.ExitCode == 0 {
		t.Fatal("uninstall with nothing installed must fail")
	}
	if res := runCLI(t, work, "project", "version", "install", "artlock", "7"); res.ExitCode == 0 {
		t.Fatal("installing a missing version must fail")
	} else if strings.Contains(res.Stdout+res.Stderr, "Usage:") {
		t.Fatalf("a runtime error must not print the usage:\n%s", res.Stderr)
	}
	if res := runCLI(t, work, "project", "file", "artlock", "7", "pixi.toml"); res.ExitCode == 0 {
		t.Fatal("a version that is neither local nor on a server must fail")
	}

	res = runCLI(t, work, "project", "versions", "artlock", "--json")
	if res.ExitCode != 0 {
		t.Fatalf("versions --json: %+v", res)
	}
	var items []shared.VersionItem
	if err := json.Unmarshal([]byte(res.Stdout), &items); err != nil || len(items) != 2 || items[0].Number != 2 {
		t.Fatalf("versions --json = %s (%v)", res.Stdout, err)
	}
}
