//go:build e2e

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nebari-dev/nebi/internal/client/sdk/local"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/store"
)

// seedLocalProject writes a project with versions straight into the local
// database the CLI stack opens.
func seedLocalProject(t *testing.T, dataDir, name string, versions int) {
	t.Helper()
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
	var user models.User
	gdb.Order("created_at").First(&user)
	p := models.Project{Name: name, OwnerID: user.ID, Status: models.ProjectStatusReady, Path: t.TempDir()}
	if err := gdb.Omit("Owner").Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	for n := 1; n <= versions; n++ {
		toml := "[workspace]\nname = \"" + name + "\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
		v := models.ProjectVersion{ProjectID: p.ID, VersionNumber: n, ManifestContent: toml, LockFileContent: "version: 6\n", PackageMetadata: "[]", CreatedBy: user.ID}
		if err := gdb.Omit("Project", "Job", "CreatedByUser").Create(&v).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func TestE2E_PublishVersionUnpublishAndPull(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("NEBI_DATA_DIR", dataDir)
	regHost := startE2ERegistry(t)

	res := runCLI(t, dataDir, "registry", "add", "--local", "--name", "lab", "--url", "http://"+regHost, "--namespace", "team", "--default")
	if res.ExitCode != 0 {
		t.Fatalf("registry add: %s", res.Stderr)
	}
	res = runCLI(t, dataDir, "registry", "list", "--local", "--json")
	if res.ExitCode != 0 || !strings.Contains(res.Stdout, `"can_unpublish": true`) || !strings.Contains(res.Stdout, `"name": "lab"`) {
		t.Fatalf("registry list: %s %s", res.Stdout, res.Stderr)
	}

	seedLocalProject(t, dataDir, "Project_1", 2)

	res = runCLI(t, dataDir, "project", "publish-version", "Project_1", "2", "--to", "lab")
	if res.ExitCode != 0 {
		t.Fatalf("publish-version: %s %s", res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, regHost+"/team/project_1:2") || !strings.Contains(res.Stdout, "Version 2 published to lab") {
		t.Fatalf("publish output: %s", res.Stdout)
	}
	res = runCLI(t, dataDir, "project", "publish-version", "Project_1", "1", "--to", "lab=team/project_1:stable")
	if res.ExitCode != 0 {
		t.Fatalf("publish-version with ref: %s %s", res.Stdout, res.Stderr)
	}
	res = runCLI(t, dataDir, "project", "publish-version", "Project_1", "1", "--to", "lab=Bad Ref")
	if res.ExitCode == 0 || !strings.Contains(res.Stderr, "Use repository:tag") {
		t.Fatalf("bad ref accepted: %s %s", res.Stdout, res.Stderr)
	}

	res = runCLI(t, dataDir, "project", "publications", "Project_1", "--json")
	if res.ExitCode != 0 {
		t.Fatalf("publications: %s", res.Stderr)
	}
	var pubs []struct {
		Version       int    `json:"version"`
		PublicationID string `json:"publication_id"`
		Reference     string `json:"reference"`
	}
	if err := json.Unmarshal([]byte(res.Stdout), &pubs); err != nil || len(pubs) != 2 || pubs[0].Version != 2 {
		t.Fatalf("publications = %s (%v)", res.Stdout, err)
	}

	res = runCLI(t, dataDir, "project", "unpublish", "Project_1", pubs[0].PublicationID)
	if res.ExitCode != 0 || !strings.Contains(res.Stderr, "Version 2 unpublished from "+regHost) {
		t.Fatalf("unpublish: %s %s", res.Stdout, res.Stderr)
	}

	res = runCLI(t, dataDir, "registry", "repos", "lab")
	if res.ExitCode != 0 || strings.TrimSpace(res.Stdout) != "team/project_1" {
		t.Fatalf("repos: %q %s", res.Stdout, res.Stderr)
	}
	res = runCLI(t, dataDir, "registry", "tags", "lab", "team/project_1")
	if res.ExitCode != 0 || strings.TrimSpace(res.Stdout) != "stable" {
		t.Fatalf("tags: %q %s", res.Stdout, res.Stderr)
	}

	res = runCLI(t, dataDir, "registry", "pull", "lab", "team/project_1", "stable", "--name", "pulled_copy")
	if res.ExitCode != 0 || !strings.Contains(res.Stderr, "into project pulled_copy") {
		t.Fatalf("registry pull: %s %s", res.Stdout, res.Stderr)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "projects", "pulled_copy", "pixi.toml")); err != nil {
		t.Fatalf("pulled project not on disk: %v", err)
	}

	res = runCLI(t, dataDir, "import", "http://"+regHost+"/team/project_1:stable", "--project", "--name", "imported")
	if res.ExitCode != 0 || !strings.Contains(res.Stderr, "as project imported") {
		t.Fatalf("import --project: %s %s", res.Stdout, res.Stderr)
	}
	// A plain OCI reference (no scheme) matches the plain-HTTP registry.
	res = runCLI(t, dataDir, "import", regHost+"/team/project_1:stable", "--project", "--name", "imported_noscheme")
	if res.ExitCode != 0 || !strings.Contains(res.Stderr, "as project imported_noscheme") {
		t.Fatalf("import --project without scheme: %s %s", res.Stdout, res.Stderr)
	}

	res = runCLI(t, dataDir, "registry", "update", "lab", "--name", "lab2")
	if res.ExitCode != 0 {
		t.Fatalf("registry update: %s", res.Stderr)
	}
	res = runCLI(t, dataDir, "registry", "list", "--local")
	if !strings.Contains(res.Stdout, "lab2") {
		t.Fatalf("rename not listed: %s", res.Stdout)
	}
}

// TestE2E_LegacyLocalRegistryIsImported checks that registries an older CLI
// stored in local_registries show up through the SDK-backed commands.
func TestE2E_LegacyLocalRegistryIsImported(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("NEBI_DATA_DIR", dataDir)
	s, err := store.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateRegistry(&store.LocalRegistry{Name: "old-one", URL: "quay.io", Namespace: "legacy"}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	res := runCLI(t, dataDir, "registry", "list", "--local")
	if res.ExitCode != 0 || !strings.Contains(res.Stdout, "old-one") {
		t.Fatalf("legacy registry missing: %s %s", res.Stdout, res.Stderr)
	}
	res = runCLI(t, dataDir, "registry", "remove", "old-one", "--local", "--force")
	if res.ExitCode != 0 {
		t.Fatalf("remove: %s", res.Stderr)
	}
	res = runCLI(t, dataDir, "registry", "list", "--local")
	if strings.Contains(res.Stdout, "old-one") {
		t.Fatalf("legacy registry came back after removal: %s", res.Stdout)
	}
}
