package process

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/nebari-dev/nebi/internal/limits"
)

func outputHelper(mode string) *Cmd {
	helper := helperCommand(mode, "")
	cmd := CommandContext(context.Background(), helper.Path, helper.Args[1:], limits.ProcessLimits{})
	cmd.Env = helper.Env
	return cmd
}

func TestOutputReturnsStdout(t *testing.T) {
	out, err := outputHelper("print").Output()
	if err != nil {
		t.Fatalf("Output: %v", err)
	}
	if got, want := string(out), "print started\n"; got != want {
		t.Fatalf("Output = %q, want %q", got, want)
	}
}

func TestOutputFailureCarriesStderr(t *testing.T) {
	out, err := outputHelper("fail").Output()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("want exit status 1, got %v", err)
	}
	if got := string(exitErr.Stderr); got != "fail stderr\n" {
		t.Fatalf("ExitError.Stderr = %q", got)
	}
	if got := string(out); got != "fail started\n" {
		t.Fatalf("Output = %q", got)
	}
}

// TestOutputLeavesSetStderr checks that Output writes stderr to a Stderr the
// caller set, and then does not also put it in the error.
func TestOutputLeavesSetStderr(t *testing.T) {
	cmd := outputHelper("fail")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	_, err := cmd.Output()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("want an exit error, got %v", err)
	}
	if exitErr.Stderr != nil {
		t.Fatalf("ExitError.Stderr = %q, want nil", exitErr.Stderr)
	}
	if got := stderr.String(); got != "fail stderr\n" {
		t.Fatalf("Stderr got %q", got)
	}
}

func TestCombinedOutput(t *testing.T) {
	out, err := outputHelper("print").CombinedOutput()
	if err != nil {
		t.Fatalf("CombinedOutput: %v", err)
	}
	for _, want := range []string{"print started\n", "print stderr\n"} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("CombinedOutput = %q, missing %q", out, want)
		}
	}
}

func TestOutputRejectsSetStdout(t *testing.T) {
	cmd := outputHelper("print")
	cmd.Stdout = new(bytes.Buffer)
	if _, err := cmd.Output(); err == nil {
		t.Fatal("Output with Stdout set must fail")
	}
	cmd = outputHelper("print")
	cmd.Stderr = new(bytes.Buffer)
	if _, err := cmd.CombinedOutput(); err == nil {
		t.Fatal("CombinedOutput with Stderr set must fail")
	}
}

func TestStderrSaver(t *testing.T) {
	short := &stderrSaver{n: 4}
	_, _ = short.Write([]byte("abc"))
	_, _ = short.Write([]byte("defg"))
	if got := string(short.Bytes()); got != "abcdefg" {
		t.Fatalf("under 2n bytes: got %q", got)
	}

	long := &stderrSaver{n: 4}
	for _, p := range []string{"ab", "cdefgh", "ijklm", "nop"} {
		if n, err := long.Write([]byte(p)); n != len(p) || err != nil {
			t.Fatalf("Write(%q) = %d, %v", p, n, err)
		}
	}
	if got, want := string(long.Bytes()), "abcd\n... omitting 8 bytes ...\nmnop"; got != want {
		t.Fatalf("over 2n bytes: got %q, want %q", got, want)
	}
}
