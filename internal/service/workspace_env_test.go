package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/nebari-dev/nebi/internal/models"
)

// --- InstallWorkspaceEnv tests ---

func TestInstallWorkspaceEnv_CreatesJob(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "env-install", userID)

	job, err := svc.InstallWorkspaceEnv(context.Background(), ws.ID.String(), userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if job.Type != models.JobTypeEnvInstall {
		t.Errorf("expected job type %q, got %q", models.JobTypeEnvInstall, job.Type)
	}
	if job.Status != models.JobStatusPending {
		t.Errorf("expected job status %q, got %q", models.JobStatusPending, job.Status)
	}
	if job.WorkspaceID != ws.ID {
		t.Errorf("expected workspace ID %s, got %s", ws.ID, job.WorkspaceID)
	}
}

func TestInstallWorkspaceEnv_RejectedInTeamMode(t *testing.T) {
	svc, db := testSetup(t, false)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "team-install", userID)

	_, err := svc.InstallWorkspaceEnv(context.Background(), ws.ID.String(), userID)
	var ve *ValidationError
	if !isValidationError(err, &ve) {
		t.Fatalf("expected ValidationError in team mode, got %T: %v", err, err)
	}
}

func TestInstallWorkspaceEnv_RejectsWhileEnvJobActive(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "double-install", userID)

	if _, err := svc.InstallWorkspaceEnv(context.Background(), ws.ID.String(), userID); err != nil {
		t.Fatalf("first install: %v", err)
	}

	_, err := svc.InstallWorkspaceEnv(context.Background(), ws.ID.String(), userID)
	if err == nil {
		t.Fatal("expected second install to be rejected while first job is pending")
	}
}

// --- install_status derivation tests ---

func TestGet_InstallStatus_NotInstalled(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "st-notinstalled", userID)

	resp, err := svc.Get(ws.ID.String())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if resp.InstallStatus != models.InstallStatusNotInstalled {
		t.Errorf("expected install_status %q, got %q", models.InstallStatusNotInstalled, resp.InstallStatus)
	}
}

func TestGet_InstallStatus_InstalledFromDisk(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "st-installed", userID)
	makeEnvInstalled(t, svc, ws)

	resp, err := svc.Get(ws.ID.String())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if resp.InstallStatus != models.InstallStatusInstalled {
		t.Errorf("expected install_status %q, got %q", models.InstallStatusInstalled, resp.InstallStatus)
	}
}

func TestGet_InstallStatus_InstallingWhileJobActive(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "st-installing", userID)

	if _, err := svc.InstallWorkspaceEnv(context.Background(), ws.ID.String(), userID); err != nil {
		t.Fatalf("install: %v", err)
	}

	resp, err := svc.Get(ws.ID.String())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if resp.InstallStatus != models.InstallStatusInstalling {
		t.Errorf("expected install_status %q, got %q", models.InstallStatusInstalling, resp.InstallStatus)
	}
}

func TestGet_InstallStatus_UninstallingWhileJobActive(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "st-uninstalling", userID)
	makeEnvInstalled(t, svc, ws)

	if _, err := svc.UninstallWorkspaceEnv(context.Background(), ws.ID.String(), userID); err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	resp, err := svc.Get(ws.ID.String())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if resp.InstallStatus != models.InstallStatusUninstalling {
		t.Errorf("expected install_status %q, got %q", models.InstallStatusUninstalling, resp.InstallStatus)
	}
}

func TestGet_InstallStatus_FailedWhenLastInstallJobFailed(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "st-failed", userID)

	job, err := svc.InstallWorkspaceEnv(context.Background(), ws.ID.String(), userID)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if err := db.Model(job).Update("status", models.JobStatusFailed).Error; err != nil {
		t.Fatalf("fail job: %v", err)
	}

	resp, err := svc.Get(ws.ID.String())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if resp.InstallStatus != models.InstallStatusFailed {
		t.Errorf("expected install_status %q, got %q", models.InstallStatusFailed, resp.InstallStatus)
	}
}

func TestGet_InstallStatus_OmittedInTeamMode(t *testing.T) {
	svc, db := testSetup(t, false)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "st-team", userID)

	resp, err := svc.Get(ws.ID.String())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if resp.InstallStatus != "" {
		t.Errorf("expected empty install_status in team mode, got %q", resp.InstallStatus)
	}
}

func TestList_InstallStatusPopulatedInLocalMode(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	installed := createReadyWorkspace(t, svc, db, "list-installed", userID)
	makeEnvInstalled(t, svc, installed)
	createReadyWorkspace(t, svc, db, "list-plain", userID)

	list, err := svc.List(userID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	got := map[string]models.InstallStatus{}
	for _, r := range list {
		got[r.Name] = r.InstallStatus
	}
	if got["list-installed"] != models.InstallStatusInstalled {
		t.Errorf("expected list-installed=installed, got %q", got["list-installed"])
	}
	if got["list-plain"] != models.InstallStatusNotInstalled {
		t.Errorf("expected list-plain=not_installed, got %q", got["list-plain"])
	}
}

// makeEnvInstalled creates the on-disk .pixi/envs marker for a workspace.
func makeEnvInstalled(t *testing.T, svc *WorkspaceService, ws *models.Workspace) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(svc.GetWorkspacePath(ws), ".pixi", "envs"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// --- UninstallWorkspaceEnv tests ---

func TestUninstallWorkspaceEnv_CreatesJob(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "env-uninstall", userID)

	job, err := svc.UninstallWorkspaceEnv(context.Background(), ws.ID.String(), userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if job.Type != models.JobTypeEnvUninstall {
		t.Errorf("expected job type %q, got %q", models.JobTypeEnvUninstall, job.Type)
	}
}

func TestUninstallWorkspaceEnv_RejectsWhileEnvJobActive(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "double-uninstall", userID)

	if _, err := svc.InstallWorkspaceEnv(context.Background(), ws.ID.String(), userID); err != nil {
		t.Fatalf("install: %v", err)
	}

	_, err := svc.UninstallWorkspaceEnv(context.Background(), ws.ID.String(), userID)
	if err == nil {
		t.Fatal("expected uninstall to be rejected while install job is pending")
	}
}
