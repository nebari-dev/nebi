//go:build unix

package jobs

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/limits"
	"github.com/nebari-dev/nebi/internal/process"
)

// TestCancelStopsProcessAndChildren runs a job body the way PixiRunner
// runs pixi: through process.CommandContext with the job's context. The
// process starts a child of its own, as pixi does. Cancelling the job must
// stop both, and the job must end as cancelled.
func TestCancelStopsProcessAndChildren(t *testing.T) {
	m := newRunner(t, Options{})
	pidFile := filepath.Join(t.TempDir(), "pids")
	// The shell prints a line, starts a long sleep as its child, records
	// both PIDs, and waits for the child. The PID file appears last, so by
	// then the line is in the job's log.
	script := `echo started; sleep 300 & echo "$$ $!" > "$0.tmp" && mv "$0.tmp" "$0"; wait`
	j := mustSubmit(t, m, uuid.New(), TypeInstall, func(ctx context.Context, log io.Writer) error {
		cmd := process.CommandContext(ctx, "/bin/sh", []string{"-c", script, pidFile}, limits.ProcessLimits{})
		cmd.Stdout, cmd.Stderr = log, log
		return cmd.Run()
	})

	pids := readPIDs(t, pidFile)
	if err := m.Cancel(context.Background(), j.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	got := mustWait(t, m, j.ID)
	if got.Status != StatusCancelled {
		t.Fatalf("want cancelled, got %+v", got)
	}
	for _, pid := range pids {
		waitGone(t, pid)
	}
	if out, _ := m.Logs(j.ID); !strings.Contains(string(out), "started") {
		t.Fatalf("the process's output must reach the job's log, got %q", out)
	}
}

// waitGone waits until pid no longer runs. A zombie waiting to be reaped
// by init counts as gone.
func waitGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		out, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
		if state := strings.TrimSpace(string(out)); err != nil || state == "" || strings.HasPrefix(state, "Z") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("process %d still runs after the job was cancelled", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
