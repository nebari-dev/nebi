//go:build !unix

package process

import (
	"context"
	"os/exec"

	"github.com/nebari-dev/nebi/internal/limits"
)

func CommandContext(ctx context.Context, name string, args []string, limitCfg limits.ProcessLimits) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	if hasLimits(limitCfg) {
		cmd.Err = ErrResourceLimitsUnsupported
	}
	return cmd
}
