package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/queue"
	"gorm.io/gorm"
)

func TestRecoverInterruptedJobs(t *testing.T) {
	for _, local := range []bool{false, true} {
		name := "team"
		if local {
			name = "local"
		}
		t.Run(name, func(t *testing.T) {
			_, db := testSetup(t, local)
			svc := NewJobService(db, local)
			statuses := []models.JobStatus{models.JobStatusPending, models.JobStatusRunning,
				models.JobStatusCompleted, models.JobStatusFailed, models.JobStatusCancelled}
			var jobs []models.Job
			for _, status := range statuses {
				job := models.Job{Type: models.JobTypeUpdate, Status: status, Logs: "saved output", Error: "original error"}
				if err := db.Create(&job).Error; err != nil {
					t.Fatal(err)
				}
				jobs = append(jobs, job)
			}
			var workspaces []models.Workspace
			for _, status := range []models.WorkspaceStatus{models.WsStatusPending, models.WsStatusCreating,
				models.WsStatusDeleting, models.WsStatusReady, models.WsStatusFailed} {
				ws := models.Workspace{Name: string(status), Status: status}
				if err := db.Create(&ws).Error; err != nil {
					t.Fatal(err)
				}
				workspaces = append(workspaces, ws)
			}
			if err := svc.RecoverInterruptedJobs(context.Background()); err != nil {
				t.Fatal(err)
			}
			for _, original := range jobs {
				var stored models.Job
				if err := db.First(&stored, "id = ?", original.ID).Error; err != nil {
					t.Fatal(err)
				}
				want := original.Status
				if want == models.JobStatusPending || want == models.JobStatusRunning {
					want = models.JobStatusFailed
					if stored.CompletedAt == nil || !strings.Contains(stored.Error, "interrupted") {
						t.Fatalf("missing interruption details: %+v", stored)
					}
				} else if stored.Error != original.Error || stored.CompletedAt != nil {
					t.Fatalf("terminal job was modified: %+v", stored)
				}
				if stored.Status != want || stored.Logs != original.Logs {
					t.Fatalf("unexpected recovered job: %+v", stored)
				}
			}
			for _, original := range workspaces {
				var stored models.Workspace
				if err := db.First(&stored, "id = ?", original.ID).Error; err != nil {
					t.Fatal(err)
				}
				want := original.Status
				if want.IsTransitional() {
					want = models.WsStatusFailed
				}
				if stored.Status != want {
					t.Fatalf("workspace %s: got %s, want %s", original.ID, stored.Status, want)
				}
			}
		})
	}
}

func TestRecoverInterruptedJobsAllowsRetry(t *testing.T) {
	wsSvc, db := testSetup(t, true)
	userID := createTestUser(t, db, "restart")
	ws := createReadyWorkspace(t, wsSvc, db, "restart", userID)
	if err := db.Model(&models.Job{}).Where("workspace_id = ?", ws.ID).
		Update("status", models.JobStatusCompleted).Error; err != nil {
		t.Fatal(err)
	}
	job, err := wsSvc.InstallWorkspaceEnv(context.Background(), ws.ID.String(), userID)
	if err != nil {
		t.Fatal(err)
	}
	// Restart discards the queue, but keeps the database and workspace files.
	wsSvc.queue.Close()
	wsSvc.queue = queue.NewMemoryQueue(100)
	defer wsSvc.queue.Close()
	svc := NewJobService(db, true)
	if err := svc.RecoverInterruptedJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	var recovered models.Job
	if err := db.First(&recovered, "id = ?", job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got := wsSvc.installStatusFor(ws); got != models.InstallStatusFailed {
		t.Fatalf("install status after restart: %s", got)
	}
	if count, err := activeJobCount(db); err != nil || count != 0 {
		t.Fatalf("abandoned jobs still consume quota: count=%d err=%v", count, err)
	}
	// Repeated recovery leaves the failure timestamp and logs untouched.
	if err := svc.RecoverInterruptedJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	var again models.Job
	if err := db.First(&again, "id = ?", job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if recovered.CompletedAt == nil || again.CompletedAt == nil || !recovered.CompletedAt.Equal(*again.CompletedAt) {
		t.Fatal("recovery changed an already recovered job")
	}
	if _, err := wsSvc.InstallWorkspaceEnv(context.Background(), ws.ID.String(), userID); err != nil {
		t.Fatalf("retry after restart: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := wsSvc.queue.Dequeue(ctx); err != nil {
		t.Fatalf("retry was not queued: %v", err)
	}
}

func jobTestSetup(t *testing.T) (*JobService, *WorkspaceService, *gorm.DB) {
	t.Helper()
	wsSvc, db := testSetup(t, false)
	return NewJobService(db, false), wsSvc, db
}

// --- ListJobs ---

func TestJobListJobs_Empty(t *testing.T) {
	svc, _, db := jobTestSetup(t)
	userID := createTestUser(t, db, "alice")

	jobs, err := svc.ListJobs(userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobs) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(jobs))
	}
}

func TestJobListJobs_ReturnsOwnedOnly(t *testing.T) {
	svc, wsSvc, db := jobTestSetup(t)
	alice := createTestUser(t, db, "alice")
	bob := createTestUser(t, db, "bob")

	// Create workspaces for both users
	wsAlice := createReadyWorkspace(t, wsSvc, db, "alice-ws", alice)
	wsBob := createReadyWorkspace(t, wsSvc, db, "bob-ws", bob)

	// Create jobs via service (install packages)
	wsSvc.InstallPackages(context.Background(), wsAlice.ID.String(), []string{"numpy"}, alice)
	wsSvc.InstallPackages(context.Background(), wsBob.ID.String(), []string{"pandas"}, bob)

	// Alice should see her jobs (create + install = 2), not Bob's
	aliceJobs, err := svc.ListJobs(alice)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	bobJobs, _ := svc.ListJobs(bob)

	if len(aliceJobs) != 2 {
		t.Errorf("expected 2 jobs for alice (create + install), got %d", len(aliceJobs))
	}
	if len(bobJobs) != 2 {
		t.Errorf("expected 2 jobs for bob (create + install), got %d", len(bobJobs))
	}
	// All of Alice's jobs should be for her workspace
	for _, j := range aliceJobs {
		if j.WorkspaceID != wsAlice.ID {
			t.Errorf("expected alice's workspace ID %s, got %s", wsAlice.ID, j.WorkspaceID)
		}
	}
}

// --- GetJob ---

func TestJobGetJob(t *testing.T) {
	svc, wsSvc, db := jobTestSetup(t)
	alice := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, wsSvc, db, "test-ws", alice)

	created, _ := wsSvc.InstallPackages(context.Background(), ws.ID.String(), []string{"numpy"}, alice)

	job, err := svc.GetJob(created.ID.String(), alice)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if job.ID != created.ID {
		t.Errorf("expected job ID %s, got %s", created.ID, job.ID)
	}
	if job.Type != models.JobTypeInstall {
		t.Errorf("expected job type %q, got %q", models.JobTypeInstall, job.Type)
	}
}

func TestJobGetJob_NotFound(t *testing.T) {
	svc, _, db := jobTestSetup(t)
	alice := createTestUser(t, db, "alice")

	_, err := svc.GetJob(uuid.New().String(), alice)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestJobGetJob_WrongOwner(t *testing.T) {
	svc, wsSvc, db := jobTestSetup(t)
	alice := createTestUser(t, db, "alice")
	bob := createTestUser(t, db, "bob")
	ws := createReadyWorkspace(t, wsSvc, db, "alice-ws", alice)

	created, _ := wsSvc.InstallPackages(context.Background(), ws.ID.String(), []string{"numpy"}, alice)

	// Bob should not be able to see Alice's job
	_, err := svc.GetJob(created.ID.String(), bob)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for wrong owner, got %v", err)
	}
}

// --- GetJobForStreaming ---

func TestJobGetJobForStreaming(t *testing.T) {
	svc, wsSvc, db := jobTestSetup(t)
	alice := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, wsSvc, db, "test-ws", alice)

	created, _ := wsSvc.InstallPackages(context.Background(), ws.ID.String(), []string{"numpy"}, alice)

	job, err := svc.GetJobForStreaming(created.ID, alice)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if job.ID != created.ID {
		t.Errorf("expected job ID %s, got %s", created.ID, job.ID)
	}
}

func TestJobGetJobForStreaming_WrongOwner(t *testing.T) {
	svc, wsSvc, db := jobTestSetup(t)
	alice := createTestUser(t, db, "alice")
	bob := createTestUser(t, db, "bob")
	ws := createReadyWorkspace(t, wsSvc, db, "alice-ws", alice)

	created, _ := wsSvc.InstallPackages(context.Background(), ws.ID.String(), []string{"numpy"}, alice)

	_, err := svc.GetJobForStreaming(created.ID, bob)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for wrong owner, got %v", err)
	}
}

// --- Local mode: jobs are visible regardless of workspace owner, matching
// how workspace listing behaves in local mode (single-user machine, all
// requests run as the synthetic local-user).

func localJobTestSetup(t *testing.T) (*JobService, *WorkspaceService, *gorm.DB) {
	t.Helper()
	wsSvc, db := testSetup(t, true)
	return NewJobService(db, true), wsSvc, db
}

func TestJobListJobs_LocalModeReturnsAllOwners(t *testing.T) {
	svc, wsSvc, db := localJobTestSetup(t)
	admin := createTestUser(t, db, "admin")
	localUser := createTestUser(t, db, "local-user")

	ws := createReadyWorkspace(t, wsSvc, db, "admin-ws", admin)
	wsSvc.InstallPackages(context.Background(), ws.ID.String(), []string{"numpy"}, admin)

	jobs, err := svc.ListJobs(localUser)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("expected 2 jobs visible in local mode regardless of owner, got %d", len(jobs))
	}
}

func TestJobGetJob_LocalModeIgnoresOwner(t *testing.T) {
	svc, wsSvc, db := localJobTestSetup(t)
	admin := createTestUser(t, db, "admin")
	localUser := createTestUser(t, db, "local-user")

	ws := createReadyWorkspace(t, wsSvc, db, "admin-ws2", admin)
	created, err := wsSvc.InstallPackages(context.Background(), ws.ID.String(), []string{"numpy"}, admin)
	if err != nil {
		t.Fatalf("create job: %v", err)
	}

	if _, err := svc.GetJob(created.ID.String(), localUser); err != nil {
		t.Errorf("expected job visible in local mode, got %v", err)
	}
	if _, err := svc.GetJobForStreaming(created.ID, localUser); err != nil {
		t.Errorf("expected job streamable in local mode, got %v", err)
	}
}

// --- RecordFailedEnvInstall ---

// TestJobRecordFailedEnvInstall_SurfacesAsInstallFailed proves a reinstall
// failure recorded outside the normal env-install job flow (e.g. the
// auto-reinstall the worker runs after an update or rollback) still shows
// up as install_failed through the workspace's derived install_status.
func TestJobRecordFailedEnvInstall_SurfacesAsInstallFailed(t *testing.T) {
	svc, wsSvc, db := localJobTestSetup(t)
	alice := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, wsSvc, db, "reinstall-fail", alice)

	if err := svc.RecordFailedEnvInstall(ws.ID, "pixi install failed: exit status 1"); err != nil {
		t.Fatalf("RecordFailedEnvInstall: %v", err)
	}

	resp, err := wsSvc.Get(ws.ID.String())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if resp.InstallStatus != models.InstallStatusFailed {
		t.Errorf("expected install_status %q, got %q", models.InstallStatusFailed, resp.InstallStatus)
	}
}
