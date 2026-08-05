//go:build linux

package sandbox

import (
	"errors"
	"fmt"

	"github.com/landlock-lsm/go-landlock/landlock"
)

// ErrUnsupported reports that the running kernel cannot confine builds.
var ErrUnsupported = errors.New("sandbox: kernel does not support Landlock filesystem confinement")

// Confine applies a Landlock ruleset to the calling process. It is
// irreversible and inherited across execve, which is what confines pixi and
// everything it spawns.
//
// Filesystem restriction is mandatory: an error here means the caller must
// decide whether to fail (strict) or continue (permissive). It is attempted
// at ABI v2 (Linux 5.19+) first so that the "refer" right can be granted on
// the workspace, then at ABI v1 (Linux 5.13+). The v2 attempt matters:
// Landlock always forbids reparenting a file across directories unless
// "refer" is granted, and v1 has no way to grant it, so under v1 a build
// cannot rename a file from one workspace subdirectory into another. Package
// managers do exactly that when they extract into a staging directory and
// move the result into their cache.
//
// Network restriction requires ABI v4 (Linux 6.7+) and is best-effort, since
// 6.7 is not yet universal.
func Confine(r Restrictions) error {
	if err := r.Validate(); err != nil {
		return err
	}

	// IgnoreIfMissing keeps the ruleset valid if a path is removed between
	// the parent's existence check and this call.
	rules := func(refer bool) []landlock.Rule {
		rw := landlock.RWDirs(r.RW...)
		if refer {
			rw = rw.WithRefer()
		}
		out := []landlock.Rule{rw.IgnoreIfMissing()}
		if len(r.RO) > 0 {
			out = append(out, landlock.RODirs(r.RO...).IgnoreIfMissing())
		}
		if len(r.ROFiles) > 0 {
			out = append(out, landlock.ROFiles(r.ROFiles...).IgnoreIfMissing())
		}
		if len(r.RWFiles) > 0 {
			out = append(out, landlock.RWFiles(r.RWFiles...).IgnoreIfMissing())
		}
		return out
	}

	err := landlock.V2.RestrictPaths(rules(true)...)
	if err != nil {
		// Fall back to the v1 floor. Renames across workspace
		// subdirectories will be denied, but confinement still holds.
		if v1Err := landlock.V1.RestrictPaths(rules(false)...); v1Err != nil {
			return fmt.Errorf("%w: %v", ErrUnsupported, v1Err)
		}
	}

	if len(r.TCPConnectPorts) > 0 {
		netRules := make([]landlock.Rule, 0, len(r.TCPConnectPorts))
		for _, port := range r.TCPConnectPorts {
			// The uint16 conversion is safe because config validation
			// rejects ports outside 1-65535.
			netRules = append(netRules, landlock.ConnectTCP(uint16(port)))
		}
		// Best-effort: on kernels below 6.7 this downgrades to a no-op
		// rather than failing the build.
		if err := landlock.V4.BestEffort().RestrictNet(netRules...); err != nil {
			return fmt.Errorf("restrict network: %w", err)
		}
	}

	return nil
}
