package boot

import (
	"errors"

	"golang.org/x/sys/windows"
)

// addrInUseReason says why listening failed when isAddrInUse.
const addrInUseReason = "another program is listening, or Windows reserves the port " +
	"(netsh interface ipv4 show excludedportrange protocol=tcp lists the reserved ones)"

// isAddrInUse reports whether listening failed because the port is taken.
// Windows answers WSAEACCES, not WSAEADDRINUSE, for a port it reserves, as
// Hyper-V, WSL2 and Docker do.
func isAddrInUse(err error) bool {
	return errors.Is(err, windows.WSAEADDRINUSE) || errors.Is(err, windows.WSAEACCES)
}
