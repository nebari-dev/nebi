package jobs

import (
	"context"
	"os/exec"
	"regexp"
	"time"
)

var platformUUID = regexp.MustCompile(`"IOPlatformUUID" = "([0-9A-Fa-f-]+)"`)

// readMachineID returns the hardware UUID, IOPlatformUUID, which ioreg
// reports. It does not change with the host name or across OS updates.
func readMachineID() string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/usr/sbin/ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output()
	if err != nil {
		return ""
	}
	if m := platformUUID.FindSubmatch(out); m != nil {
		return string(m[1])
	}
	return ""
}
