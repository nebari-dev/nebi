package main

import "fmt"

// execProcess is unimplemented on Windows: there is no execve, and there is
// no Landlock to inherit across one. The shim exists so the CLI keeps
// building for windows/amd64; the server that invokes it only runs on Linux.
func execProcess(argv []string) error {
	return fmt.Errorf("sandbox-exec is not supported on Windows (cannot exec %s)", argv[0])
}
