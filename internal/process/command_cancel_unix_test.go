//go:build unix

package process

import (
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
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
