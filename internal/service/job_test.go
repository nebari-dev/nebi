package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
)

func jobTestSetup(t *testing.T) (*JobService, *WorkspaceService, *gorm.DB) {
	t.Helper()
	wsSvc, db := testSetup(t, false)
	return NewJobService(db), wsSvc, db
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
