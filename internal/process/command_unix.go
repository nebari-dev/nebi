//go:build unix

package process

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
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
	if runtime.GOOS == "linux" && (limitCfg.CPUSeconds > 0 || limitCfg.MemoryBytes > 0 || limitCfg.Processes > 0) {
		return linuxCgroupLimitScript(limitCfg)
	}

	var b strings.Builder
	if limitCfg.CPUSeconds > 0 {
		appendRequiredULimit(&b, "-t", fmt.Sprint(limitCfg.CPUSeconds), "CPU")
	}
	if limitCfg.FileBytes > 0 {
		appendRequiredULimit(&b, "-f", fmt.Sprint(fileLimitBlocks(limitCfg.FileBytes)), "file size")
	}
	if limitCfg.MemoryBytes > 0 {
		fmt.Fprintf(&b, "echo 'nebi: memory limits are not supported on %s' >&2\nexit 125\n", runtime.GOOS)
		return b.String()
	}
	if limitCfg.Processes > 0 {
		fmt.Fprintf(&b, "echo 'nebi: process limits are not supported on %s' >&2\nexit 125\n", runtime.GOOS)
		return b.String()
	}
	b.WriteString("exec \"$@\"\n")
	return b.String()
}

func linuxCgroupLimitScript(limitCfg limits.ProcessLimits) string {
	var b strings.Builder
	b.WriteString("CGROUP_ROOT=${NEBI_CGROUP_ROOT:-/sys/fs/cgroup}\n")
	b.WriteString("CGROUP=\"$CGROUP_ROOT/nebi-job-$$\"\n")
	b.WriteString("cleanup() {\n")
	b.WriteString("  if [ -n \"${monitor_pid:-}\" ]; then kill \"$monitor_pid\" >/dev/null 2>&1 || true; fi\n")
	b.WriteString("  if [ -n \"${CGROUP:-}\" ]; then rmdir \"$CGROUP\" >/dev/null 2>&1 || true; fi\n")
	b.WriteString("}\n")
	b.WriteString("fail_limit() { echo \"nebi: $1\" >&2; cleanup; exit 125; }\n")
	if limitCfg.FileBytes > 0 {
		appendRequiredULimit(&b, "-f", fmt.Sprint(fileLimitBlocks(limitCfg.FileBytes)), "file size")
	}
	b.WriteString("[ -f \"$CGROUP_ROOT/cgroup.controllers\" ] || fail_limit 'cgroup v2 is required for isolated resource limits'\n")
	b.WriteString("mkdir \"$CGROUP\" || fail_limit 'failed to create job cgroup'\n")
	if limitCfg.MemoryBytes > 0 {
		fmt.Fprintf(&b, "echo %d > \"$CGROUP/memory.max\" || fail_limit 'failed to apply memory limit'\n", limitCfg.MemoryBytes)
	}
	if limitCfg.Processes > 0 {
		fmt.Fprintf(&b, "echo %d > \"$CGROUP/pids.max\" || fail_limit 'failed to apply process limit'\n", limitCfg.Processes)
	}
	b.WriteString("echo $$ > \"$CGROUP/cgroup.procs\" || fail_limit 'failed to enter job cgroup'\n")
	b.WriteString("trap 'if [ -w \"$CGROUP/cgroup.kill\" ]; then echo 1 > \"$CGROUP/cgroup.kill\" >/dev/null 2>&1 || true; fi; cleanup' EXIT INT TERM\n")
	if limitCfg.CPUSeconds > 0 {
		fmt.Fprintf(&b, "CPU_LIMIT_USEC=%d\n", int64(limitCfg.CPUSeconds)*1000000)
		b.WriteString("monitor_cpu() {\n")
		b.WriteString("  while kill -0 \"$child_pid\" >/dev/null 2>&1; do\n")
		b.WriteString("    usage=$(awk '$1 == \"usage_usec\" { print $2 }' \"$CGROUP/cpu.stat\" 2>/dev/null || echo 0)\n")
		b.WriteString("    if [ \"${usage:-0}\" -gt \"$CPU_LIMIT_USEC\" ]; then\n")
		b.WriteString("      echo 'nebi: CPU budget exceeded' >&2\n")
		b.WriteString("      if [ -w \"$CGROUP/cgroup.kill\" ]; then echo 1 > \"$CGROUP/cgroup.kill\" >/dev/null 2>&1 || true; else kill -TERM \"$child_pid\" >/dev/null 2>&1 || true; fi\n")
		b.WriteString("      return\n")
		b.WriteString("    fi\n")
		b.WriteString("    sleep 1\n")
		b.WriteString("  done\n")
		b.WriteString("}\n")
	}
	b.WriteString("\"$@\" &\n")
	b.WriteString("child_pid=$!\n")
	if limitCfg.CPUSeconds > 0 {
		b.WriteString("monitor_cpu &\n")
		b.WriteString("monitor_pid=$!\n")
	}
	b.WriteString("wait \"$child_pid\"\n")
	b.WriteString("status=$?\n")
	b.WriteString("cleanup\n")
	b.WriteString("exit \"$status\"\n")
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
