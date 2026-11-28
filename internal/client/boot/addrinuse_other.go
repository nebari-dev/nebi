//go:build !windows

package boot

import (
	"errors"
	"syscall"
)

// addrInUseReason says why listening failed when isAddrInUse.
const addrInUseReason = "another program is listening"

func isAddrInUse(err error) bool {
	return errors.Is(err, syscall.EADDRINUSE)
}
