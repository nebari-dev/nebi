//go:build e2e

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nebari-dev/nebi/internal/auth"
	"github.com/nebari-dev/nebi/internal/client/sdk/local"
	"github.com/nebari-dev/nebi/internal/models"
)

// seedLocalProject stores a ready local project with one version per
// pixi.toml in the CLI's local database (NEBI_DATA_DIR).
func seedLocalProject(t *testing.T, name string, tomls ...string) {
	t.Helper()
	dataDir := os.Getenv("NEBI_DATA_DIR")
	gdb, err := local.OpenDB(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := gdb.DB()
	defer sqlDB.Close()
	if err := local.Migrate(gdb); err != nil {
		t.Fatal(err)
	}
	la, err := auth.NewLocalAuthenticator(gdb)
	if err != nil {
		t.Fatal(err)
	}
	user := la.User().ID
	p := models.Project{Name: name, OwnerID: user, Status: models.ProjectStatusReady, Source: "local", Path: filepath.Join(dataDir, "projects", name)}
	if err := gdb.Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	for i, toml := range tomls {
		v := models.ProjectVersion{ProjectID: p.ID, VersionNumber: i + 1, ManifestContent: toml, LockFileContent: "version: 6\n", PackageMetadata: "[]", CreatedBy: user, Description: "seeded"}
		if err := gdb.Create(&v).Error; err != nil {
			t.Fatal(err)
		}
		if err := local.ApplyTags(gdb, p.ID, v.VersionNumber, nil, user); err != nil {
			t.Fatal(err)
		}
	}
}

func TestE2E_ServerConnectPushPull(t *testing.T) {
	setupLocalStore(t)
	dir := t.TempDir()

	res := runCLI(t, dir, "server", "connect", e2eEnv.serverURL, "--token", e2eEnv.token)
	if res.ExitCode != 0 || !strings.Contains(res.Stdout, "Connected to") || !strings.Contains(res.Stdout, "as admin") {
		t.Fatalf("connect: %+v", res)
	}
	res = runCLI(t, dir, "server", "status", "--json=false")
	if res.ExitCode != 0 || !strings.Contains(res.Stdout, "Signed in as  admin") {
		t.Fatalf("status: %+v", res)
	}

	toml1 := "[workspace]\nname = \"u5-sync\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"
	toml2 := toml1 + "\n[dependencies]\npython = \"3.12.*\"\n"
	seedLocalProject(t, "u5-sync", toml1, toml2)

	res = runCLI(t, dir, "project", "push", "u5-sync")
	if res.ExitCode != 0 || !strings.Contains(res.Stdout, "Versions 1 and 2 pushed") {
		t.Fatalf("push: %+v", res)
	}
	res = runCLI(t, dir, "project", "push", "u5-sync")
	if res.ExitCode != 0 || !strings.Contains(res.Stdout, "Nothing to push") {
		t.Fatalf("second push: %+v", res)
	}

	res = runCLI(t, dir, "server", "projects", "--search", "u5-sync", "--json")
	if res.ExitCode != 0 {
		t.Fatalf("projects: %+v", res)
	}
	var list struct {
		Projects []struct {
			ID             string  `json:"id"`
			Name           string  `json:"name"`
			LatestVersion  *int    `json:"latest_version"`
			LocalProjectID *string `json:"local_project_id"`
		} `json:"projects"`
	}
	if err := json.Unmarshal([]byte(res.Stdout), &list); err != nil || len(list.Projects) != 1 {
		t.Fatalf("projects json: %v %s", err, res.Stdout)
	}
	sp := list.Projects[0]
	if sp.LatestVersion == nil || *sp.LatestVersion != 2 || sp.LocalProjectID == nil {
		t.Fatalf("server project: %+v", sp)
	}

	// A second machine pulls it.
	setupLocalStore(t)
	res = runCLI(t, dir, "server", "connect", e2eEnv.serverURL, "--token", e2eEnv.token)
	if res.ExitCode != 0 {
		t.Fatalf("connect machine b: %+v", res)
	}
	res = runCLI(t, dir, "server", "pull", sp.ID, "--name", "u5-pulled")
	if res.ExitCode != 0 || !strings.Contains(res.Stdout, "u5-pulled pulled. Version 2 is on this machine.") {
		t.Fatalf("pull: %+v", res)
	}
	res = runCLI(t, dir, "server", "pull", sp.ID, "--name", "u5-again")
	if res.ExitCode == 0 || !strings.Contains(res.Stderr, "already on this machine as u5-pulled") {
		t.Fatalf("second pull should be refused: %+v", res)
	}
	res = runCLI(t, dir, "server", "show", sp.ID)
	if res.ExitCode != 0 || !strings.Contains(res.Stdout, "u5-sync") || !strings.Contains(res.Stdout, "v2") {
		t.Fatalf("show: %+v", res)
	}
	res = runCLI(t, dir, "server", "disconnect")
	if res.ExitCode != 0 || !strings.Contains(res.Stdout, "Projects you pulled stay on this machine") {
		t.Fatalf("disconnect: %+v", res)
	}
	res = runCLI(t, dir, "server", "status", "--json=false")
	if res.ExitCode != 0 || !strings.Contains(res.Stdout, "Not connected") {
		t.Fatalf("status after disconnect: %+v", res)
	}
}

func TestE2E_LoginStoresSDKConnection(t *testing.T) {
	setupLocalStore(t)
	dir := t.TempDir()
	res := runCLI(t, dir, "login", e2eEnv.serverURL, "--token", e2eEnv.token)
	if res.ExitCode != 0 {
		t.Fatalf("login: %+v", res)
	}
	res = runCLI(t, dir, "server", "status", "--json")
	if res.ExitCode != 0 || !strings.Contains(res.Stdout, `"auth": "token"`) || !strings.Contains(res.Stdout, `"connected": true`) {
		t.Fatalf("status after login: %+v", res)
	}
	res = runCLI(t, dir, "logout")
	if res.ExitCode != 0 || !strings.Contains(res.Stderr, "Logged out from") {
		t.Fatalf("logout: %+v", res)
	}
}
