//go:build unix

package jobs

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/limits"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/process"
)

// testSleepJob runs a real child process (like pixi) until cancelled.
const testSleepJob models.JobType = "test_sleep"

var (
	sleepPIDsMu sync.Mutex
	sleepPIDs   = map[uuid.UUID]int{}
)

func init() {
	register(testSleepJob, func(ctx context.Context, rc RunContext) error {
		fmt.Fprintln(rc.Log, "Running: sleep 60")
		cmd := process.CommandContext(ctx, "sleep", []string{"60"}, limits.ProcessLimits{})
		if err := cmd.Start(); err != nil {
			return err
		}
		sleepPIDsMu.Lock()
		sleepPIDs[rc.Job.ID] = cmd.Process.Pid
		sleepPIDsMu.Unlock()
		return cmd.Wait()
	})
}

// TestCancelRunningKillsProcess: Cancel job on a running job cancels its
// context through the worker, the child process is killed, and the job
// is stored as cancelled.
func TestCancelRunningKillsProcess(t *testing.T) {
	h := newHarness(t, true)
	h.startWorker(t)
	ctx := context.Background()
	p := h.project(t, "p")

	job, err := h.svc.Submit(ctx, shared.JobRequest{ProjectID: p.ID, Type: testSleepJob})
	if err != nil {
		t.Fatal(err)
	}
	h.waitStatus(t, job.ID, models.JobStatusRunning)
	var pid int
	deadline := time.Now().Add(5 * time.Second)
	for pid == 0 && time.Now().Before(deadline) {
		sleepPIDsMu.Lock()
		pid = sleepPIDs[job.ID]
		sleepPIDsMu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("child process never started")
	}

	start := time.Now()
	sum, err := h.svc.Cancel(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Status != models.JobStatusCancelled {
		t.Fatalf("cancel running returned %q, want cancelled", sum.Status)
	}
	if time.Since(start) > 10*time.Second {
		t.Fatalf("cancel took %s", time.Since(start))
	}
	if err := syscall.Kill(pid, 0); err == nil {
		t.Fatalf("child process %d still alive after cancel", pid)
	}
	d := h.waitStatus(t, job.ID, models.JobStatusCancelled)
	if !strings.Contains(d.Logs, "Running: sleep 60") {
		t.Fatalf("log not kept: %q", d.Logs)
	}
}
