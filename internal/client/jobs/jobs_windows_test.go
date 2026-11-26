//go:build windows

package jobs

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/limits"
	"github.com/nebari-dev/nebi/internal/process"
)

// The Windows cancel test has no shell to start a process tree with, so it
// runs this test binary again with cancelHelperEnv set;
// TestCancelHelperProcess then plays a process of the tree instead of
// running as a test.
const (
	cancelHelperEnv     = "NEBI_TEST_CANCEL_HELPER"
	cancelHelperPIDsEnv = "NEBI_TEST_CANCEL_PIDS"
)

// TestCancelHelperProcess is a process of TestCancelStopsProcessAndChildren's
// tree. It is not a test: without cancelHelperEnv it returns at once.
//
// Modes:
//   - parent: print "started", start a sleep child that shares its stdout
//     and stderr, record both PIDs in the file named by
//     cancelHelperPIDsEnv, and wait for the child.
//   - sleep: block until killed.
func TestCancelHelperProcess(t *testing.T) {
	mode := os.Getenv(cancelHelperEnv)
	if mode == "" {
		return
	}
	fail := func(err error) {
		fmt.Fprintf(os.Stderr, "%s: %v\n", mode, err)
		os.Exit(2)
	}
	switch mode {
	case "sleep":
		time.Sleep(10 * time.Minute) // until killed
		os.Exit(3)
	case "parent":
	default:
		fail(fmt.Errorf("unknown helper mode %q", mode))
	}

	fmt.Fprintln(os.Stdout, "started")
	child := cancelHelperCommand("sleep")
	child.Stdout, child.Stderr = os.Stdout, os.Stderr
	if err := child.Start(); err != nil {
		fail(err)
	}
	// The PID file appears last and complete, so by then the line is in
	// the job's log.
	path := os.Getenv(cancelHelperPIDsEnv)
	pids := fmt.Sprintf("%d %d", os.Getpid(), child.Process.Pid)
	if err := os.WriteFile(path+".tmp", []byte(pids), 0o600); err != nil {
		fail(err)
	}
	if err := os.Rename(path+".tmp", path); err != nil {
		fail(err)
	}
	_ = child.Wait()
	os.Exit(0)
}

func cancelHelperCommand(mode string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=^TestCancelHelperProcess$")
	cmd.Env = append(os.Environ(), cancelHelperEnv+"="+mode)
	return cmd
}

// TestCancelStopsProcessAndChildren runs a job body the way PixiRunner
// runs pixi: through process.CommandContext with the job's context. The
// process starts a child of its own, as pixi does. Cancelling the job must
// stop both, and the job must end as cancelled.
func TestCancelStopsProcessAndChildren(t *testing.T) {
	m := newRunner(t, Options{})
	pidFile := filepath.Join(t.TempDir(), "pids")
	j := mustSubmit(t, m, uuid.New(), TypeInstall, func(ctx context.Context, log io.Writer) error {
		helper := cancelHelperCommand("parent")
		cmd := process.CommandContext(ctx, helper.Path, helper.Args[1:], limits.ProcessLimits{})
		cmd.Env = append(helper.Env, cancelHelperPIDsEnv+"="+pidFile)
		cmd.Stdout, cmd.Stderr = log, log
		return cmd.Run()
	})

	pids := readPIDs(t, pidFile)
	t.Cleanup(func() { killRunning(pids) })
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

// waitGone waits until pid no longer runs.
func waitGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for processRunning(pid) == nil {
		if time.Now().After(deadline) {
			t.Fatalf("process %d still runs after the job was cancelled", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// killRunning kills those of pids still running, so a failed test does not
// leave processes behind.
func killRunning(pids []int) {
	for _, pid := range pids {
		if processRunning(pid) != nil {
			continue
		}
		if p, err := os.FindProcess(pid); err == nil {
			_ = p.Kill()
			_ = p.Release()
		}
	}
}
