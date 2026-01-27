//go:build linux

package localserver

import (
	"fmt"
	"os"
	"strings"
)

// getProcessStartTime returns the start time of a process from /proc/<pid>/stat.
// The start time is field 22 (1-indexed) and is measured in clock ticks since boot.
// Returns 0 if the start time cannot be determined.
func getProcessStartTime(pid int) int64 {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0
	}

	// The stat file format has the command name in parens which may contain spaces.
	// Find the last ')' to skip past the command name field.
	content := string(data)
	closeParen := strings.LastIndex(content, ")")
	if closeParen == -1 {
		return 0
	}

	// Fields after the command name (starting from field 3).
	// Field 22 (1-indexed) is starttime, which is field 20 (0-indexed) after the ')'.
	fields := strings.Fields(content[closeParen+2:])
	if len(fields) < 20 {
		return 0
	}

	// Field 22 in the full stat line = index 19 in the post-paren fields (0-indexed).
	var startTime int64
	if _, err := fmt.Sscanf(fields[19], "%d", &startTime); err != nil {
		return 0
	}

	return startTime
}
