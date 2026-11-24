//go:build unix

package process

import (
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// processGone reports whether pid no longer runs. A zombie waiting to be
// reaped by init counts as gone.
func processGone(pid int) bool {
	if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
		return true
	}
	out, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	state := strings.TrimSpace(string(out))
	return err != nil || state == "" || strings.HasPrefix(state, "Z")
}

// detach starts child in a process group of its own, out of reach of a
// cancel that stops the command's group.
func detach(child *exec.Cmd) {
	child.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// TestCancelWaitReturnsWhenOutputHeldOutsideTree cancels a command whose
// process started a child in another process group. Cancel cannot reach
// that child, which keeps the output pipe open; Wait must still return
// within WaitDelay. (A Windows process can leave the command's job the same
// way, by breaking away, but see detach there for why no test does it.)
func TestCancelWaitReturnsWhenOutputHeldOutsideTree(t *testing.T) {
	dir := t.TempDir()
	cmd, out, cancel := startHelper(t, "detacher", dir)
	pids := waitForPIDs(t, dir, 2)

	cancel()
	elapsed, err := waitBounded(t, cmd)
	if err == nil {
		t.Fatal("a cancelled command must fail")
	}
	if limit := cmd.WaitDelay + 5*time.Second; elapsed > limit {
		t.Fatalf("Wait took %v, more than %v", elapsed, limit)
	}
	for pid, mode := range pids {
		if mode == "detacher" {
			waitProcessGone(t, pid, mode)
		}
	}
	if !strings.Contains(out.String(), "sleep started") {
		t.Fatalf("the detached child must hold the output pipe, got %q", out.String())
	}
}
