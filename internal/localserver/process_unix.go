//go:build !windows

package localserver

import "syscall"

// getSysProcAttr returns platform-specific process attributes for spawning a detached server.
func getSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setsid: true, // Create new session so process survives parent exit.
	}
}
