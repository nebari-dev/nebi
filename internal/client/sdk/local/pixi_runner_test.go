package local

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"testing"

	sdkexec "github.com/nebari-dev/nebi/internal/client/sdk/exec"
)

// The real runner and the fake both implement PixiRunner.
var (
	_ PixiRunner = (*sdkexec.Runner)(nil)
	_ PixiRunner = (*fakePixi)(nil)
)

// TestStaleLockErrorFromRunner checks that the stale-lock errors of the
// real runner and of the fake are recognised through wrapping, and that
// the runner's other errors are not.
func TestStaleLockErrorFromRunner(t *testing.T) {
	for _, c := range []struct {
		name  string
		err   error
		stale bool
	}{
		{"runner stale lock", &sdkexec.StaleLockError{Dir: "p", Detail: "lock file not up-to-date with the workspace"}, true},
		{"wrapped runner stale lock", fmt.Errorf("checking: %w", &sdkexec.StaleLockError{Dir: "p"}), true},
		{"fake stale lock", fmt.Errorf("pixi lock --check: %w", fakeStaleLock{"p"}), true},
		{"runner command error", &sdkexec.CommandError{Command: "pixi lock --check", Line: "failed to fetch", Err: &exec.ExitError{}}, false},
		{"cancelled", fmt.Errorf("pixi lock --check: %w", context.Canceled), false},
		{"plain error", errors.New("boom"), false},
		{"nil", nil, false},
	} {
		if got := isStaleLock(c.err); got != c.stale {
			t.Errorf("%s: isStaleLock = %v, want %v", c.name, got, c.stale)
		}
	}
}
