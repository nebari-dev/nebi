//go:build !darwin && !linux && !windows

package jobs

// readMachineID returns "" where nebi reads no machine ID; ReadOwner then
// compares host names.
func readMachineID() string { return "" }
