package boot

import (
	"fmt"
	"net"
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

// A port Windows reserves fails with WSAEACCES and gets the same --port
// hint as a taken one.
func TestIsAddrInUseOnWindows(t *testing.T) {
	for _, errno := range []windows.Errno{windows.WSAEADDRINUSE, windows.WSAEACCES} {
		err := fmt.Errorf("listening: %w", &net.OpError{Op: "listen", Net: "tcp",
			Err: &os.SyscallError{Syscall: "bind", Err: errno}})
		if !isAddrInUse(err) {
			t.Errorf("isAddrInUse(%v) = false", err)
		}
	}
	if isAddrInUse(&os.SyscallError{Syscall: "bind", Err: windows.WSAEINVAL}) {
		t.Error("isAddrInUse(WSAEINVAL) = true")
	}
}
