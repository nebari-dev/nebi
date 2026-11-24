//go:build windows

package process

import (
	"os/exec"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// processGone reports whether pid no longer runs.
func processGone(pid int) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return true
	}
	defer windows.CloseHandle(h)
	var code uint32
	return windows.GetExitCodeProcess(h, &code) != nil || code != stillActive
}

// detach starts child out of the command's job, which allows breakaway (see
// CommandContext). Whether the start succeeds also depends on any job the
// test itself runs in, so TestCancelWaitReturnsWhenOutputHeldOutsideTree
// skips when it fails, and TestJobLimits checks the job's limits too.
func detach(child *exec.Cmd) {
	if child.SysProcAttr == nil {
		child.SysProcAttr = &syscall.SysProcAttr{}
	}
	child.SysProcAttr.CreationFlags |= windows.CREATE_BREAKAWAY_FROM_JOB
}

// TestJobLimits checks the limits of a running command's job: its
// processes are killed when its last handle closes, and they may break
// away from it.
func TestJobLimits(t *testing.T) {
	dir := t.TempDir()
	cmd, _, cancel := startHelper(t, "sleep", dir)
	pids := waitForPIDs(t, dir, 1)

	cmd.tree.mu.Lock()
	job := cmd.tree.job
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	err := windows.QueryInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), nil)
	cmd.tree.mu.Unlock()
	if job == 0 {
		t.Fatal("the command was not put in a job")
	}
	if err != nil {
		t.Fatalf("QueryInformationJobObject: %v", err)
	}
	want := uint32(windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | windows.JOB_OBJECT_LIMIT_BREAKAWAY_OK)
	if got := info.BasicLimitInformation.LimitFlags; got != want {
		t.Fatalf("job limit flags = %#x, want %#x", got, want)
	}

	cancel()
	assertStoppedPromptly(t, cmd, pids)
}

// TestTreeStoppedWhenOwnerDies starts a command through CommandContext in a
// helper process, the way nebi starts pixi, and then kills that helper. The
// job's last handle closes with it, so the command's process and its child
// must be stopped too rather than left running.
func TestTreeStoppedWhenOwnerDies(t *testing.T) {
	dir := t.TempDir()
	owner := helperCommand("owner", dir)
	if err := owner.Start(); err != nil {
		t.Fatalf("start owner: %v", err)
	}
	t.Cleanup(func() { killRecorded(dir) })
	pids := waitForPIDs(t, dir, 3)

	if err := owner.Process.Kill(); err != nil {
		t.Fatalf("kill owner: %v", err)
	}
	_ = owner.Wait()
	for pid, mode := range pids {
		waitProcessGone(t, pid, mode)
	}
}
