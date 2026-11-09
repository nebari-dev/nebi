//go:build e2e

package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/auth"
	"github.com/nebari-dev/nebi/internal/client/sdk/local"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/models"
)

// seedJobsDB creates a local database in a fresh NEBI_DATA_DIR with one
// project and three jobs, and returns the job ids (failed, running, queued).
func seedJobsDB(t *testing.T) (failed, running, queued uuid.UUID) {
	t.Helper()
	dataDir := t.TempDir()
	prev, had := os.LookupEnv("NEBI_DATA_DIR")
	os.Setenv("NEBI_DATA_DIR", dataDir)
	t.Cleanup(func() {
		if had {
			os.Setenv("NEBI_DATA_DIR", prev)
		} else {
			os.Unsetenv("NEBI_DATA_DIR")
		}
	})
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
	p := &models.Project{Name: "jobs-cli", OwnerID: la.User().ID, Status: models.ProjectStatusReady}
	if err := gdb.Create(p).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	started := now.Add(-2 * time.Minute)
	ended := now.Add(-time.Minute)
	v := 3
	jobs := []*models.Job{
		{ProjectID: p.ID, Type: models.JobTypeInstallVersion, Status: models.JobStatusFailed, VersionNumber: &v,
			StartedAt: &started, CompletedAt: &ended, Logs: "Running: pixi install\nError: boom\n", Error: "boom"},
		{ProjectID: p.ID, Type: models.JobTypeCreateVersion, Status: models.JobStatusRunning, StartedAt: &ended, Logs: "solving\n"},
		{ProjectID: p.ID, Type: models.JobTypeUninstall, Status: models.JobStatusPending},
	}
	for _, j := range jobs {
		if err := gdb.Create(j).Error; err != nil {
			t.Fatal(err)
		}
	}
	return jobs[0].ID, jobs[1].ID, jobs[2].ID
}

func TestJobCommands(t *testing.T) {
	failed, running, queued := seedJobsDB(t)
	dir := t.TempDir()

	res := runCLI(t, dir, "jobs", "list", "--json", "--status=", "--type=", "--project=", "--limit=0", "--sort=-started_at")
	if res.ExitCode != 0 {
		t.Fatalf("jobs list: %s %s", res.Stdout, res.Stderr)
	}
	var list []shared.JobSummary
	if err := json.Unmarshal([]byte(res.Stdout), &list); err != nil {
		t.Fatalf("list json: %v (%s)", err, res.Stdout)
	}
	if len(list) != 3 || list[0].ID != queued {
		t.Fatalf("list = %+v", list)
	}

	res = runCLI(t, dir, "job", "list", "--json=false", "--status=failed,queued", "--type=", "--project=jobs-cli", "--limit=0", "--sort=started_at")
	if res.ExitCode != 0 {
		t.Fatalf("job list table: %s %s", res.Stdout, res.Stderr)
	}
	for _, want := range []string{"Install version 3", "Failed", "Queued", "1m 00s", failed.String()[:8]} {
		if !strings.Contains(res.Stdout, want) {
			t.Errorf("table missing %q:\n%s", want, res.Stdout)
		}
	}
	if strings.Contains(res.Stdout, "Create version") {
		t.Errorf("status filter ignored:\n%s", res.Stdout)
	}

	res = runCLI(t, dir, "job", "show", "--json=false", failed.String()[:8])
	if res.ExitCode != 0 || !strings.Contains(res.Stdout, "Error: boom") || !strings.Contains(res.Stdout, "Status:    Failed") {
		t.Fatalf("job show: %d\n%s\n%s", res.ExitCode, res.Stdout, res.Stderr)
	}

	res = runCLI(t, dir, "job", "logs", "--follow=false", failed.String())
	if res.ExitCode != 0 || res.Stdout != "Running: pixi install\nError: boom\n" {
		t.Fatalf("job logs: %q %s", res.Stdout, res.Stderr)
	}

	// Cancelling a queued job; the running one belongs to no live process
	// here, so it is marked cancelled for whichever process runs it.
	for _, id := range []uuid.UUID{queued, running} {
		res = runCLI(t, dir, "job", "cancel", "--json=false", id.String())
		if res.ExitCode != 0 || !strings.Contains(res.Stdout, "Cancelled") {
			t.Fatalf("job cancel %s: %s %s", id, res.Stdout, res.Stderr)
		}
	}
	res = runCLI(t, dir, "job", "cancel", "--json=false", failed.String())
	if res.ExitCode == 0 {
		t.Fatalf("cancelling a finished job must fail: %s", res.Stdout)
	}

	res = runCLI(t, dir, "job", "show", "--json=false", "zz")
	if res.ExitCode == 0 {
		t.Fatal("invalid job id must fail")
	}
}

func TestFormatJobDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{
		0:                                     "0s",
		42 * time.Second:                      "42s",
		125 * time.Second:                     "2m 05s",
		time.Hour + 3*time.Minute:             "1h 03m",
		-5 * time.Second:                      "0s",
		59*time.Second + 600*time.Millisecond: "1m 00s",
	} {
		if got := formatJobDuration(d); got != want {
			t.Errorf("formatJobDuration(%s) = %q, want %q", d, got, want)
		}
	}
	for in, want := range map[string]models.JobStatus{"queued": "pending", "Succeeded": "completed", "failed": "failed", "canceled": "cancelled"} {
		if got := parseJobStatus(in); got != want {
			t.Errorf("parseJobStatus(%q) = %q, want %q", in, got, want)
		}
	}
}
