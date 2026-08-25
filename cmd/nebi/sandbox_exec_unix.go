//go:build !windows

package main

import (
	"fmt"
	"os"
	"syscall"
)

// execProcess replaces the current process image with argv. On success it
// does not return, which is what makes the Landlock ruleset applied above
// stick to the build command: the restrictions are inherited across execve.
func execProcess(argv []string) error {
	err := syscall.Exec(argv[0], argv, os.Environ())
	return fmt.Errorf("exec %s: %w", argv[0], err)
}
