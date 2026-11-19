//go:build unix

package jobs

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// errWouldBlock reports that another open file holds the lock.
var errWouldBlock = unix.EWOULDBLOCK

// lockFile takes an exclusive flock on f without waiting. flock locks belong
// to the open file, not the process, so two opens in one process conflict
// too, and the lock goes away when the last descriptor for it is closed.
func lockFile(f *os.File) error {
	for {
		err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if errors.Is(err, unix.EAGAIN) {
			// EAGAIN and EWOULDBLOCK are the same on every Unix Go
			// supports, but be explicit.
			return errWouldBlock
		}
		return err
	}
}

func unlockFile(f *os.File) error {
	for {
		err := unix.Flock(int(f.Fd()), unix.LOCK_UN)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		return err
	}
}

func replaceFile(from, to string) error { return os.Rename(from, to) }

func removeFile(path string) error { return os.Remove(path) }

func readFile(path string) ([]byte, error) { return os.ReadFile(path) }
