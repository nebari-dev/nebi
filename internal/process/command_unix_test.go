//go:build unix

package process

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nebari-dev/nebi/internal/limits"
)

func TestLimitScriptFailsClosed(t *testing.T) {
	script := limitScript(limits.ProcessLimits{CPUSeconds: 60})
	if strings.Contains(script, "ulimit") && strings.Contains(script, "|| true") {
		t.Fatalf("limit script must not ignore ulimit failures: %s", script)
	}
	for _, want := range []string{"exit 125"} {
		if !strings.Contains(script, want) {
			t.Fatalf("limit script missing %q: %s", want, script)
		}
	}
}

func TestLimitScriptAppliesFileSizeLimit(t *testing.T) {
	script := limitScript(limits.ProcessLimits{FileBytes: 1024})
	for _, want := range []string{"ulimit -f 2", "failed to apply file size limit", "exec \"$@\""} {
		if !strings.Contains(script, want) {
			t.Fatalf("file-size limit script missing %q: %s", want, script)
		}
	}
}

func TestCommandContextLimitsExternalFileWrites(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "external.bin")
	cmd := CommandContext(context.Background(), "/bin/sh", []string{
		"-c",
		"dd if=/dev/zero of=\"$1\" bs=1024 count=8 2>/dev/null",
		"sh",
		outPath,
	}, limits.ProcessLimits{FileBytes: 512})

	if err := cmd.Run(); err == nil {
		t.Fatal("expected external file write to exceed process file-size limit")
	}
	if info, err := os.Stat(outPath); err == nil && info.Size() > 1024 {
		t.Fatalf("expected external file capped near 512 bytes, got %d", info.Size())
	}
}

func TestIsResourceLimitExitRecognizesSignalStatusesOnlyWithMatchingBudgets(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "exit 137")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected exit error")
	}
	if !IsResourceLimitExit(err, limits.ProcessLimits{MemoryBytes: 1024}, "") {
		t.Fatal("expected SIGKILL-style exit to count as memory budget failure")
	}
	if IsResourceLimitExit(err, limits.ProcessLimits{FileBytes: 1024}, "") {
		t.Fatal("did not expect SIGKILL-style exit to count as file budget failure")
	}
	if IsResourceLimitExit(err, limits.ProcessLimits{}, "") {
		t.Fatal("did not expect signal-style exit without active budgets")
	}
	if !HasLikelyResourceLimitOutput(limits.ProcessLimits{Processes: 2}, "fork: Resource temporarily unavailable") {
		t.Fatal("expected process budget output marker")
	}
	if !HasLikelyResourceLimitOutput(limits.ProcessLimits{FileBytes: 1024}, "file size limit exceeded") {
		t.Fatal("expected file budget output marker")
	}
	if HasLikelyResourceLimitOutput(limits.ProcessLimits{}, "killed") {
		t.Fatal("did not expect generic killed output without active budgets")
	}
}

func TestLinuxCgroupLimitScriptUsesAggregateBudgets(t *testing.T) {
	script := linuxCgroupLimitScript(limits.ProcessLimits{
		CPUSeconds:  60,
		MemoryBytes: 1024,
		Processes:   16,
		FileBytes:   2048,
	})
	for _, want := range []string{
		"cgroup.controllers",
		"memory.max",
		"pids.max",
		"cpu.stat",
		"cgroup.kill",
		"ulimit -f 4",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("linux cgroup script missing %q: %s", want, script)
		}
	}
	if strings.Contains(script, "|| true\n\"$@\"") {
		t.Fatalf("linux cgroup setup must not continue after setup failures: %s", script)
	}
}

func TestNonLinuxAggregateLimitsFailClosed(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("non-Linux behavior")
	}
	script := limitScript(limits.ProcessLimits{MemoryBytes: 1024})
	if !strings.Contains(script, "memory limits are not supported") || !strings.Contains(script, "exit 125") {
		t.Fatalf("expected unsupported memory limit to fail closed, got: %s", script)
	}
}
