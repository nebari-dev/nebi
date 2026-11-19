//go:build windows

package jobs

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// stillActive is the exit code GetExitCodeProcess reports for a process that
// has not exited (STILL_ACTIVE).
const stillActive = 259

// processRunning reports whether process pid has not exited.
func processRunning(pid int) error {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return err
	}
	if code != stillActive {
		return fmt.Errorf("process exited with code %d", code)
	}
	return nil
}
