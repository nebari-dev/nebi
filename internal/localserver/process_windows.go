//go:build windows

package localserver

import "syscall"

// getSysProcAttr returns platform-specific process attributes for spawning a detached server.
func getSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: 0x00000008, // DETACHED_PROCESS
	}
}
