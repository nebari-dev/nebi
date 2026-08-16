//go:build unix

package process

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestLimitScriptUsesPortableULimit(t *testing.T) {
	script := limitScript(limits.ProcessLimits{CPUSeconds: 60, FileBytes: 1024})
	for _, want := range []string{"ulimit -t 60", "ulimit -f 2", "exec \"$@\""} {
		if !strings.Contains(script, want) {
			t.Fatalf("limit script missing %q: %s", want, script)
		}
	}
	for _, unwanted := range []string{"cgroup.controllers", "memory.max", "pids.max", "cpu.stat"} {
		if strings.Contains(script, unwanted) {
			t.Fatalf("limit script should not use cgroups; found %q in %s", unwanted, script)
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
	ctx := context.Background()
	cmd := exec.Command("/bin/sh", "-c", "exit 152")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected exit error")
	}
	if !IsResourceLimitExit(ctx, err, limits.ProcessLimits{CPUSeconds: 1}, "") {
		t.Fatal("expected SIGXCPU-style exit to count as CPU budget failure")
	}
	if IsResourceLimitExit(ctx, err, limits.ProcessLimits{FileBytes: 1024}, "") {
		t.Fatal("did not expect SIGXCPU-style exit to count as file budget failure")
	}
	if IsResourceLimitExit(ctx, err, limits.ProcessLimits{}, "") {
		t.Fatal("did not expect signal-style exit without active budgets")
	}
	if !HasLikelyResourceLimitOutput(limits.ProcessLimits{CPUSeconds: 1}, "CPU time limit exceeded") {
		t.Fatal("expected CPU budget output marker")
	}
	if !HasLikelyResourceLimitOutput(limits.ProcessLimits{FileBytes: 1024}, "file size limit exceeded") {
		t.Fatal("expected file budget output marker")
	}
	if !HasLikelyResourceLimitOutput(limits.ProcessLimits{CPUSeconds: 1}, "Killed\n") {
		t.Fatal("expected shell killed line marker")
	}
	if HasLikelyResourceLimitOutput(limits.ProcessLimits{CPUSeconds: 1}, "no processes killed") {
		t.Fatal("did not expect incidental killed text to match resource limit output")
	}
	if HasLikelyResourceLimitOutput(limits.ProcessLimits{}, "Killed\n") {
		t.Fatal("did not expect generic killed output without active budgets")
	}
}

func TestIsResourceLimitExitIgnoresContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := CommandContext(ctx, "/bin/sh", []string{"-c", "while :; do sleep 1; done"}, limits.ProcessLimits{CPUSeconds: 60})
	if err := cmd.Start(); err != nil {
		t.Fatalf("start command: %v", err)
	}
	cancel()
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected canceled command to fail")
		}
		if IsResourceLimitExit(ctx, err, limits.ProcessLimits{CPUSeconds: 60}, "") {
			t.Fatalf("context cancellation should not be classified as a resource limit: %v", err)
		}
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("timed out waiting for canceled command")
	}
}
