//go:build windows

package jobs

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// errWouldBlock reports that another handle holds the lock.
var errWouldBlock = windows.ERROR_LOCK_VIOLATION

// lockFile takes an exclusive LockFileEx lock on the first byte of f without
// waiting. The lock belongs to the handle, so two handles in one process
// conflict too, and Windows releases it when the handle is closed or the
// process ends.
func lockFile(f *os.File) error {
	ol := new(windows.Overlapped)
	err := windows.LockFileEx(windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, ol)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_IO_PENDING) {
		return errWouldBlock
	}
	return err
}

func unlockFile(f *os.File) error {
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, new(windows.Overlapped))
}

// Windows refuses to rename over, remove or open a file another process has
// open without FILE_SHARE_DELETE, which Go does not pass. A reader and the
// owner can collide for the moment ReadOwner holds owner.json open, so these
// retry sharing errors briefly.
const shareRetryFor = 2 * time.Second

func retryShared(op func() error) error {
	deadline := time.Now().Add(shareRetryFor)
	for {
		err := op()
		if err == nil || !isSharingError(err) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func isSharingError(err error) bool {
	return errors.Is(err, windows.ERROR_SHARING_VIOLATION) ||
		errors.Is(err, windows.ERROR_ACCESS_DENIED) ||
		errors.Is(err, windows.ERROR_LOCK_VIOLATION)
}

func replaceFile(from, to string) error {
	return retryShared(func() error { return os.Rename(from, to) })
}

func removeFile(path string) error {
	return retryShared(func() error { return os.Remove(path) })
}

func readFile(path string) ([]byte, error) {
	var data []byte
	err := retryShared(func() error {
		var err error
		data, err = os.ReadFile(path)
		return err
	})
	return data, err
}
