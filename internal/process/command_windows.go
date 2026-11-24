//go:build windows

package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/nebari-dev/nebi/internal/limits"
)

const (
	// terminatedExitCode is the exit code cancelled processes end with, the
	// same one os.Process.Kill uses.
	terminatedExitCode = 1

	// stillActive is the exit code GetExitCodeProcess reports for a process
	// that has not exited (STILL_ACTIVE).
	stillActive = 259
)

// CommandContext builds a command whose cancellation, through ctx, stops the
// process and every process it started. Resource limits are not supported on
// Windows: a command given limits fails to start with
// ErrResourceLimitsUnsupported.
//
// Windows has no process groups, so Cmd.Start puts the process in a job
// object instead, before it runs any code. Every process it starts, and
// every process those start, joins the job too, and cancelling terminates
// the whole job. The job is also set to kill its processes when its last
// handle closes, so the tree is stopped if this process dies while the
// command runs.
//
// A process started with CREATE_BREAKAWAY_FROM_JOB leaves the job, and
// cancelling does not stop it, as a unix process that moves to a process
// group of its own is out of reach of the group's kill. The job allows
// this so that such a start still succeeds; refusing it would make the
// command fail on Windows alone.
func CommandContext(ctx context.Context, name string, args []string, limitCfg limits.ProcessLimits) *Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	if hasLimits(limitCfg) {
		cmd.Err = ErrResourceLimitsUnsupported
	}
	c := &Cmd{Cmd: cmd}
	cmd.Cancel = func() error { return c.tree.cancel(cmd.Process) }
	cmd.WaitDelay = cancelWaitDelay
	return c
}

// processTree is the job object holding a command's processes.
type processTree struct {
	// mu orders cancel after start: os/exec may call Cancel as soon as the
	// process exists, before start has put it in the job.
	mu sync.Mutex
	// job is the job object, or 0 before start, after release, or when the
	// process could not be put in one.
	job windows.Handle
}

// start starts cmd suspended, puts it in a new job object, and then lets it
// run. Suspending it first means no process it starts can be created
// outside the job.
//
// If the job cannot be set up the command still runs, without one, and
// cancelling it stops only the process itself. That is not expected on the
// Windows versions Go supports: since Windows 8 jobs nest, so even a process
// whose parent runs in a job can join a new one.
func (t *processTree) start(cmd *exec.Cmd) error {
	t.mu.Lock()
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_SUSPENDED
	if err := cmd.Start(); err != nil {
		t.mu.Unlock()
		return err
	}
	t.job = newJob(cmd.Process)
	err := resumeProcess(uint32(cmd.Process.Pid))
	t.mu.Unlock()
	if err != nil {
		// A process left suspended never runs, so stop it and reap it:
		// Start reports failure, so the caller will not call Wait. Wait
		// must run without mu held, as it waits for Cancel.
		_ = t.cancel(cmd.Process)
		_ = cmd.Wait()
		t.release()
		return fmt.Errorf("resume %s: %w", cmd.Path, err)
	}
	return nil
}

// newJob puts p in a new job object that kills its processes when its last
// handle closes and lets them break away, and returns the job, or 0 if that
// fails.
func newJob(p *os.Process) windows.Handle {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0
	}
	if err := setJobLimits(job, true); err != nil {
		windows.CloseHandle(job)
		return 0
	}
	herr := p.WithHandle(func(h uintptr) {
		err = windows.AssignProcessToJobObject(job, windows.Handle(h))
	})
	if herr != nil || err != nil {
		windows.CloseHandle(job)
		return 0
	}
	return job
}

// cancel terminates the job, stopping the process and everything it
// started. It returns os.ErrProcessDone when the process had already
// exited, which os/exec treats as a cancel that found nothing to stop.
func (t *processTree) cancel(p *os.Process) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.job == 0 {
		return p.Kill()
	}
	exited := processExited(p)
	if err := windows.TerminateJobObject(t.job, terminatedExitCode); err != nil {
		return err
	}
	if exited {
		return os.ErrProcessDone
	}
	return nil
}

// release closes the job once the command has ended. It first stops the job
// from killing what is left in it: like a process group on unix, a command
// that exits on its own leaves the processes it started running. Cancel has
// already stopped them otherwise.
func (t *processTree) release() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.job == 0 {
		return
	}
	_ = setJobLimits(t.job, false)
	windows.CloseHandle(t.job)
	t.job = 0
}

// jobLimits is what every command's job allows: a process in it started
// with CREATE_BREAKAWAY_FROM_JOB leaves it (see CommandContext).
const jobLimits = windows.JOB_OBJECT_LIMIT_BREAKAWAY_OK

// setJobLimits sets job's limits to jobLimits, and makes it kill its
// processes when its last handle closes if killOnClose is set.
func setJobLimits(job windows.Handle, killOnClose bool) error {
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = jobLimits
	if killOnClose {
		info.BasicLimitInformation.LimitFlags |= windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	}
	_, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
	return err
}

// resumeProcess resumes the threads of the suspended process pid. A process
// created suspended has a single thread, its main one. os.StartProcess
// closes the handle to it, so it is found through a thread snapshot; pid
// stays the process's while os.Process holds its handle.
func resumeProcess(pid uint32) error {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(snap)

	resumed := 0
	e := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	for err = windows.Thread32First(snap, &e); err == nil; err = windows.Thread32Next(snap, &e) {
		if e.OwnerProcessID != pid {
			continue
		}
		h, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, e.ThreadID)
		if err != nil {
			return err
		}
		_, err = windows.ResumeThread(h)
		windows.CloseHandle(h)
		if err != nil {
			return err
		}
		resumed++
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return err
	}
	if resumed == 0 {
		return errors.New("no thread to resume")
	}
	return nil
}

// processExited reports whether p has exited.
func processExited(p *os.Process) bool {
	exited := false
	err := p.WithHandle(func(h uintptr) {
		var code uint32
		exited = windows.GetExitCodeProcess(windows.Handle(h), &code) == nil && code != stillActive
	})
	return exited || errors.Is(err, os.ErrProcessDone)
}
