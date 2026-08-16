//go:build unix

package process

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/nebari-dev/nebi/internal/limits"
)

// CommandContext builds a command with OS resource limits. The command runs in
// its own process group so context cancellation kills child processes spawned
// by package managers, not just the immediate shell/process.
func CommandContext(ctx context.Context, name string, args []string, limitCfg limits.ProcessLimits) *exec.Cmd {
	var cmd *exec.Cmd
	if hasLimits(limitCfg) {
		script := limitScript(limitCfg)
		cmdArgs := append([]string{"-c", script, "nebi-limited-command", name}, args...)
		cmd = exec.CommandContext(ctx, "/bin/sh", cmdArgs...)
	} else {
		cmd = exec.CommandContext(ctx, name, args...)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	setCommandCancel(cmd)
	return cmd
}

func limitScript(limitCfg limits.ProcessLimits) string {
	var b strings.Builder
	if limitCfg.CPUSeconds > 0 {
		appendRequiredULimit(&b, "-t", fmt.Sprint(limitCfg.CPUSeconds), "CPU")
	}
	if limitCfg.FileBytes > 0 {
		appendRequiredULimit(&b, "-f", fmt.Sprint(fileLimitBlocks(limitCfg.FileBytes)), "file size")
	}
	b.WriteString("exec \"$@\"\n")
	return b.String()
}

func appendRequiredULimit(b *strings.Builder, flag string, value string, label string) {
	fmt.Fprintf(b, "if ! ulimit %s %s; then\n", flag, value)
	fmt.Fprintf(b, "  echo 'nebi: failed to apply %s limit' >&2\n", label)
	b.WriteString("  exit 125\n")
	b.WriteString("fi\n")
}

func setCommandCancel(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM); err != nil && err != syscall.ESRCH {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			return err
		}
		go func(pid int) {
			time.Sleep(2 * time.Second)
			_ = syscall.Kill(-pid, syscall.SIGKILL)
		}(cmd.Process.Pid)
		return nil
	}
	cmd.WaitDelay = 5 * time.Second
}
