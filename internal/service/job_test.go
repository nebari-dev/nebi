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
			var projects []models.Project
			for _, status := range []models.ProjectStatus{models.ProjectStatusPending, models.ProjectStatusCreating,
				models.ProjectStatusDeleting, models.ProjectStatusReady, models.ProjectStatusFailed} {
				project := models.Project{Name: string(status), Status: status}
				if err := db.Create(&project).Error; err != nil {
					t.Fatal(err)
				}
				projects = append(projects, project)
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
			for _, original := range projects {
				var stored models.Project
				if err := db.First(&stored, "id = ?", original.ID).Error; err != nil {
					t.Fatal(err)
				}
				want := original.Status
				if want.IsTransitional() {
					want = models.ProjectStatusFailed
				}
				if stored.Status != want {
					t.Fatalf("project %s: got %s, want %s", original.ID, stored.Status, want)
				}
			}
		})
	}
}

func TestRecoverInterruptedJobsAllowsRetry(t *testing.T) {
	projectSvc, db := testSetup(t, true)
	userID := createTestUser(t, db, "restart")
	project := createReadyProject(t, projectSvc, db, "restart", userID)
	if err := db.Model(&models.Job{}).Where("project_id = ?", project.ID).
		Update("status", models.JobStatusCompleted).Error; err != nil {
		t.Fatal(err)
	}
	job, err := projectSvc.InstallProjectEnv(context.Background(), project.ID.String(), userID)
	if err != nil {
		t.Fatal(err)
	}
	// Restart discards the queue, but keeps the database and project files.
	projectSvc.queue.Close()
	projectSvc.queue = queue.NewMemoryQueue(100)
	defer projectSvc.queue.Close()
	svc := NewJobService(db, true)
	if err := svc.RecoverInterruptedJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	var recovered models.Job
	if err := db.First(&recovered, "id = ?", job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got := projectSvc.installStatusFor(project); got != models.InstallStatusFailed {
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
	if _, err := projectSvc.InstallProjectEnv(context.Background(), project.ID.String(), userID); err != nil {
		t.Fatalf("retry after restart: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := projectSvc.queue.Dequeue(ctx); err != nil {
		t.Fatalf("retry was not queued: %v", err)
	}
}

func jobTestSetup(t *testing.T) (*JobService, *ProjectService, *gorm.DB) {
	t.Helper()
	projectSvc, db := testSetup(t, false)
	return NewJobService(db, false), projectSvc, db
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
	svc, projectSvc, db := jobTestSetup(t)
	alice := createTestUser(t, db, "alice")
	bob := createTestUser(t, db, "bob")

	// Create projects for both users
	projectAlice := createReadyProject(t, projectSvc, db, "alice-ws", alice)
	projectBob := createReadyProject(t, projectSvc, db, "bob-ws", bob)

	// Create jobs via service (install packages)
	projectSvc.InstallPackages(context.Background(), projectAlice.ID.String(), []string{"numpy"}, alice)
	projectSvc.InstallPackages(context.Background(), projectBob.ID.String(), []string{"pandas"}, bob)

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
	// All of Alice's jobs should be for her project
	for _, j := range aliceJobs {
		if j.ProjectID != projectAlice.ID {
			t.Errorf("expected alice's project ID %s, got %s", projectAlice.ID, j.ProjectID)
		}
	}
}

// --- GetJob ---

func TestJobGetJob(t *testing.T) {
	svc, projectSvc, db := jobTestSetup(t)
	alice := createTestUser(t, db, "alice")
	project := createReadyProject(t, projectSvc, db, "test-ws", alice)

	created, _ := projectSvc.InstallPackages(context.Background(), project.ID.String(), []string{"numpy"}, alice)

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
	svc, projectSvc, db := jobTestSetup(t)
	alice := createTestUser(t, db, "alice")
	bob := createTestUser(t, db, "bob")
	project := createReadyProject(t, projectSvc, db, "alice-ws", alice)

	created, _ := projectSvc.InstallPackages(context.Background(), project.ID.String(), []string{"numpy"}, alice)

	// Bob should not be able to see Alice's job
	_, err := svc.GetJob(created.ID.String(), bob)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for wrong owner, got %v", err)
	}
}

// --- GetJobForStreaming ---

func TestJobGetJobForStreaming(t *testing.T) {
	svc, projectSvc, db := jobTestSetup(t)
	alice := createTestUser(t, db, "alice")
	project := createReadyProject(t, projectSvc, db, "test-ws", alice)

	created, _ := projectSvc.InstallPackages(context.Background(), project.ID.String(), []string{"numpy"}, alice)

	job, err := svc.GetJobForStreaming(created.ID, alice)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if job.ID != created.ID {
		t.Errorf("expected job ID %s, got %s", created.ID, job.ID)
	}
}

func TestJobGetJobForStreaming_WrongOwner(t *testing.T) {
	svc, projectSvc, db := jobTestSetup(t)
	alice := createTestUser(t, db, "alice")
	bob := createTestUser(t, db, "bob")
	project := createReadyProject(t, projectSvc, db, "alice-ws", alice)

	created, _ := projectSvc.InstallPackages(context.Background(), project.ID.String(), []string{"numpy"}, alice)

	_, err := svc.GetJobForStreaming(created.ID, bob)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for wrong owner, got %v", err)
	}
}

// --- Local mode: jobs are visible regardless of project owner, matching
// how project listing behaves in local mode (single-user machine, all
// requests run as the synthetic local-user).

func localJobTestSetup(t *testing.T) (*JobService, *ProjectService, *gorm.DB) {
	t.Helper()
	projectSvc, db := testSetup(t, true)
	return NewJobService(db, true), projectSvc, db
}

func TestJobListJobs_LocalModeReturnsAllOwners(t *testing.T) {
	svc, projectSvc, db := localJobTestSetup(t)
	admin := createTestUser(t, db, "admin")
	localUser := createTestUser(t, db, "local-user")

	project := createReadyProject(t, projectSvc, db, "admin-ws", admin)
	projectSvc.InstallPackages(context.Background(), project.ID.String(), []string{"numpy"}, admin)

	jobs, err := svc.ListJobs(localUser)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("expected 2 jobs visible in local mode regardless of owner, got %d", len(jobs))
	}
}

func TestJobGetJob_LocalModeIgnoresOwner(t *testing.T) {
	svc, projectSvc, db := localJobTestSetup(t)
	admin := createTestUser(t, db, "admin")
	localUser := createTestUser(t, db, "local-user")

	project := createReadyProject(t, projectSvc, db, "admin-project2", admin)
	created, err := projectSvc.InstallPackages(context.Background(), project.ID.String(), []string{"numpy"}, admin)
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
// up as install_failed through the project's derived install_status.
func TestJobRecordFailedEnvInstall_SurfacesAsInstallFailed(t *testing.T) {
	svc, projectSvc, db := localJobTestSetup(t)
	alice := createTestUser(t, db, "alice")
	project := createReadyProject(t, projectSvc, db, "reinstall-fail", alice)

	if err := svc.RecordFailedEnvInstall(project.ID, "pixi install failed: exit status 1"); err != nil {
		t.Fatalf("RecordFailedEnvInstall: %v", err)
	}

	resp, err := projectSvc.Get(project.ID.String(), alice)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if resp.InstallStatus != models.InstallStatusFailed {
		t.Errorf("expected install_status %q, got %q", models.InstallStatusFailed, resp.InstallStatus)
	}
}
