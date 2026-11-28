package jobs

import (
	"os"
	"strings"
)

// readMachineID returns the systemd machine ID, or the D-Bus one on systems
// without systemd.
func readMachineID() string {
	for _, path := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
		if data, err := os.ReadFile(path); err == nil {
			if id := strings.TrimSpace(string(data)); id != "" {
				return id
			}
		}
	}
	return ""
}
