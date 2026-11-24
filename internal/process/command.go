package process

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/nebari-dev/nebi/internal/limits"
)

// cancelWaitDelay bounds Wait once a command is cancelled: after it, Wait
// closes the command's output pipes and returns even if a process the cancel
// did not reach still holds them.
const cancelWaitDelay = 5 * time.Second

// Cmd is a command built by CommandContext. It is an exec.Cmd whose
// cancellation, through the context, stops the process and every process it
// started, not just the process itself. Use it like an exec.Cmd: set its
// fields, then Start and Wait, or Run.
//
// Its Start, Wait, Run, Output and CombinedOutput must be used rather than
// the embedded exec.Cmd's. On Windows, Start puts the process in a job
// object before it runs, which is what lets cancel reach its children, and
// Wait closes the job once the command has ended. A process started through
// the embedded exec.Cmd still runs, but cancelling it stops only the process
// itself. Waiting through the embedded exec.Cmd leaves the job open, so
// whatever the command left running is stopped when this process exits.
type Cmd struct {
	*exec.Cmd
	tree processTree
}

// Start starts the command, as exec.Cmd.Start does.
func (c *Cmd) Start() error {
	return c.tree.start(c.Cmd)
}

// Wait waits for the command to exit, as exec.Cmd.Wait does.
func (c *Cmd) Wait() error {
	err := c.Cmd.Wait()
	c.tree.release()
	return err
}

// Run starts the command and waits for it, as exec.Cmd.Run does.
func (c *Cmd) Run() error {
	if err := c.Start(); err != nil {
		return err
	}
	return c.Wait()
}

// Output runs the command and returns its stdout, as exec.Cmd.Output does.
// If Stderr is unset, a failure's *exec.ExitError carries stderr, cut to its
// first and last 32 KiB as exec.Cmd.Output cuts it.
func (c *Cmd) Output() ([]byte, error) {
	if c.Stdout != nil {
		return nil, errors.New("exec: Stdout already set")
	}
	var stdout bytes.Buffer
	c.Stdout = &stdout
	var stderr *stderrSaver
	if c.Stderr == nil {
		stderr = &stderrSaver{n: stderrSaveLimit}
		c.Stderr = stderr
	}
	err := c.Run()
	var exitErr *exec.ExitError
	if stderr != nil && errors.As(err, &exitErr) {
		exitErr.Stderr = stderr.Bytes()
	}
	return stdout.Bytes(), err
}

// CombinedOutput runs the command and returns its stdout and stderr
// together, as exec.Cmd.CombinedOutput does.
func (c *Cmd) CombinedOutput() ([]byte, error) {
	if c.Stdout != nil {
		return nil, errors.New("exec: Stdout already set")
	}
	if c.Stderr != nil {
		return nil, errors.New("exec: Stderr already set")
	}
	var b bytes.Buffer
	c.Stdout, c.Stderr = &b, &b
	err := c.Run()
	return b.Bytes(), err
}

// stderrSaveLimit is how much of the start and of the end of stderr Output
// keeps, the same as exec.Cmd.Output.
const stderrSaveLimit = 32 << 10

// stderrSaver keeps the first and the last n bytes written to it, so a
// command that writes a lot to stderr cannot make Output hold all of it.
type stderrSaver struct {
	n       int
	prefix  []byte
	suffix  []byte
	skipped int64
}

func (w *stderrSaver) Write(p []byte) (int, error) {
	written := len(p)
	if room := w.n - len(w.prefix); room > 0 {
		k := min(room, len(p))
		w.prefix = append(w.prefix, p[:k]...)
		p = p[k:]
	}
	w.suffix = append(w.suffix, p...)
	if extra := len(w.suffix) - w.n; extra > 0 {
		w.skipped += int64(extra)
		w.suffix = append(w.suffix[:0], w.suffix[extra:]...)
	}
	return written, nil
}

// Bytes returns what was kept, with a line saying how much was left out
// between the start and the end, if anything was.
func (w *stderrSaver) Bytes() []byte {
	out := append([]byte(nil), w.prefix...)
	if w.skipped > 0 {
		out = fmt.Appendf(out, "\n... omitting %d bytes ...\n", w.skipped)
	}
	return append(out, w.suffix...)
}

var ErrResourceLimitsUnsupported = errors.New("process resource limits are not supported on this platform")

var killedLineRE = regexp.MustCompile(`(?mi)^killed:?[[:space:]]*$`)

type ResourceLimitError struct {
	Err error
}

func (e *ResourceLimitError) Error() string {
	if e.Err == nil {
		return "process resource limit exceeded"
	}
	return fmt.Sprintf("process resource limit exceeded: %v", e.Err)
}

func (e *ResourceLimitError) Unwrap() error {
	return e.Err
}

func NewResourceLimitError(err error) error {
	if err == nil {
		return nil
	}
	if IsResourceLimitError(err) {
		return err
	}
	return &ResourceLimitError{Err: err}
}

func IsResourceLimitError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrResourceLimitsUnsupported) {
		return true
	}
	var limitErr *ResourceLimitError
	return errors.As(err, &limitErr)
}

func IsResourceLimitSetupExit(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr) && exitErr.ExitCode() == 125
}

// IsResourceLimitExit recognizes explicit resource-limit failures and the
// signal-style exits commonly produced when active OS budgets terminate a job
// before the wrapper can write a nebi marker.
func IsResourceLimitExit(ctx context.Context, err error, limitCfg limits.ProcessLimits, output string) bool {
	if IsResourceLimitError(err) || HasResourceLimitOutput(output) {
		return true
	}
	if ctx.Err() != nil {
		return false
	}
	if !hasLimits(limitCfg) {
		return false
	}
	if HasLikelyResourceLimitOutput(limitCfg, output) {
		return true
	}
	if IsResourceLimitSetupExit(err) {
		return true
	}

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	switch exitErr.ExitCode() {
	case -1:
		return false
	case 143:
		return limitCfg.CPUSeconds > 0
	case 152:
		return limitCfg.CPUSeconds > 0
	case 153:
		return limitCfg.FileBytes > 0
	default:
		return false
	}
}

func HasLikelyResourceLimitOutput(limitCfg limits.ProcessLimits, output string) bool {
	if output == "" || !hasLimits(limitCfg) {
		return false
	}
	lowerOutput := strings.ToLower(output)
	if limitCfg.CPUSeconds > 0 {
		for _, marker := range []string{"cpu time limit exceeded"} {
			if strings.Contains(lowerOutput, marker) {
				return true
			}
		}
		if killedLineRE.MatchString(output) {
			return true
		}
	}
	if limitCfg.FileBytes > 0 {
		for _, marker := range []string{"file size limit exceeded", "file too large"} {
			if strings.Contains(lowerOutput, marker) {
				return true
			}
		}
	}
	return false
}

func HasResourceLimitOutput(output string) bool {
	for _, marker := range []string{
		"nebi: CPU budget exceeded",
		"nebi: failed to apply CPU limit",
		"nebi: failed to apply file size limit",
	} {
		if strings.Contains(output, marker) {
			return true
		}
	}
	return false
}

func hasLimits(limits limits.ProcessLimits) bool {
	return limits.CPUSeconds > 0 || limits.FileBytes > 0
}

func fileLimitBlocks(bytes int64) int64 {
	if bytes <= 0 {
		return 0
	}
	return (bytes + 511) / 512
}
