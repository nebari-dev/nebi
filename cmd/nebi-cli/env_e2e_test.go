//go:build e2e

package main

import (
	"strings"
	"testing"
)

// The e2e server runs in team mode, where environment install is not
// available. These tests pin the CLI wiring: name resolution, the API
// call, and how server-side rejection surfaces to the user.

const envTestPixiToml = "[project]\nname = \"env-test\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n"

func TestE2E_ProjectInstall_TeamModeRejected(t *testing.T) {
	setupLocalStore(t)
	dir := t.TempDir()

	createProjectViaAPI(t, e2eEnv.serverURL, e2eEnv.token, "env-install-team", envTestPixiToml)

	res := runCLI(t, dir, "project", "install", "env-install-team")
	if res.ExitCode == 0 {
		t.Fatalf("expected install to fail in team mode\nstdout: %s\nstderr: %s", res.Stdout, res.Stderr)
	}
	combined := res.Stdout + res.Stderr
	if !strings.Contains(combined, "local mode") {
		t.Errorf("expected local-mode rejection message, got:\n%s", combined)
	}
}

func TestE2E_ProjectUninstall_TeamModeRejected(t *testing.T) {
	setupLocalStore(t)
	dir := t.TempDir()

	createProjectViaAPI(t, e2eEnv.serverURL, e2eEnv.token, "env-uninstall-team", envTestPixiToml)

	res := runCLI(t, dir, "project", "uninstall", "env-uninstall-team")
	if res.ExitCode == 0 {
		t.Fatalf("expected uninstall to fail in team mode\nstdout: %s\nstderr: %s", res.Stdout, res.Stderr)
	}
}

func TestE2E_ProjectList_InstalledFilter(t *testing.T) {
	setupLocalStore(t)
	dir := t.TempDir()

	createProjectViaAPI(t, e2eEnv.serverURL, e2eEnv.token, "env-list-filter", envTestPixiToml)

	// Team mode: no project carries install_status, so the filter
	// yields an empty result rather than an error.
	res := runCLI(t, dir, "project", "list", "--installed")
	if res.ExitCode != 0 {
		t.Fatalf("project list --installed failed\nstdout: %s\nstderr: %s", res.Stdout, res.Stderr)
	}
	if strings.Contains(res.Stdout, "env-list-filter") {
		t.Errorf("not-installed project should be filtered out, got:\n%s", res.Stdout)
	}
}

func TestE2E_ProjectList_RemoteShowsInstallColumn(t *testing.T) {
	setupLocalStore(t)
	dir := t.TempDir()

	createProjectViaAPI(t, e2eEnv.serverURL, e2eEnv.token, "env-list-column", envTestPixiToml)

	res := runCLI(t, dir, "project", "list", "--remote")
	if res.ExitCode != 0 {
		t.Fatalf("project list --remote failed\nstdout: %s\nstderr: %s", res.Stdout, res.Stderr)
	}
	if !strings.Contains(res.Stdout, "INSTALL") {
		t.Errorf("expected INSTALL column header, got:\n%s", res.Stdout)
	}
}
