package boot

import (
	"errors"

	"golang.org/x/sys/windows"
)

func isAddrInUse(err error) bool {
	return errors.Is(err, windows.WSAEADDRINUSE)
}
