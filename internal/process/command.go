package process

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/nebari-dev/nebi/internal/limits"
)

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
