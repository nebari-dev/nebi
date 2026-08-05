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
// Filesystem confinement is mandatory: ErrUnsupported means the caller must
// decide whether to fail (strict) or continue (permissive). Network
// confinement is best-effort; ErrNetworkUnrestricted means the filesystem is
// confined but the kernel is too old to restrict TCP, which is a warning
// rather than a setup failure.
//
// Everything is applied as a SINGLE ruleset rather than one for the
// filesystem and a second for the network. Landlock denies reparenting a
// file across directories by default in every domain, and the right to do it
// ("refer") can only be granted inside the domain that handles it. A second,
// stacked domain therefore re-denies refer no matter what the first one
// granted. Splitting the two calls silently breaks renames between
// subdirectories of the workspace, which package managers do constantly when
// they stage a download and then move it into their cache. Verified against
// kernel 6.17: FS-then-net gives EXDEV, combined does not.
//
// The ABI ladder degrades one capability at a time:
//
//	v4 (Linux 6.7+)  filesystem + refer + TCP
//	v2 (Linux 5.19+) filesystem + refer, network unrestricted
//	v1 (Linux 5.13+) filesystem only; cross-directory renames fail with EXDEV
func Confine(r Restrictions) error {
	if err := r.Validate(); err != nil {
		return err
	}

	if err := landlock.V4.Restrict(append(fsRules(r, true), netRules(r.TCPConnectPorts)...)...); err == nil {
		return nil
	}
	if err := landlock.V2.RestrictPaths(fsRules(r, true)...); err == nil {
		return ErrNetworkUnrestricted
	}
	if err := landlock.V1.RestrictPaths(fsRules(r, false)...); err != nil {
		return fmt.Errorf("%w: %v", ErrUnsupported, err)
	}
	return ErrNetworkUnrestricted
}

// fsRules builds the filesystem half of the ruleset. refer additionally
// grants the workspace the right to have files moved between its own
// subdirectories, which needs ABI v2.
//
// IgnoreIfMissing keeps the ruleset valid if a path disappears between the
// parent's existence check and this call.
func fsRules(r Restrictions, refer bool) []landlock.Rule {
	rw := landlock.RWDirs(r.RW...)
	if refer {
		rw = rw.WithRefer()
	}
	rules := []landlock.Rule{rw.IgnoreIfMissing()}
	if len(r.RO) > 0 {
		rules = append(rules, landlock.RODirs(r.RO...).IgnoreIfMissing())
	}
	if len(r.ROFiles) > 0 {
		rules = append(rules, landlock.ROFiles(r.ROFiles...).IgnoreIfMissing())
	}
	if len(r.RWFiles) > 0 {
		rules = append(rules, landlock.RWFiles(r.RWFiles...).IgnoreIfMissing())
	}
	return rules
}

// netRules turns allowed TCP ports into Landlock rules. An empty or nil
// slice yields no rules, which under a v4 ruleset denies every TCP bind and
// connect: "allowed_ports: []" must mean fully offline builds, not
// unrestricted ones. Bind is denied either way; builds have no reason to
// listen.
func netRules(ports []int) []landlock.Rule {
	rules := make([]landlock.Rule, 0, len(ports))
	for _, port := range ports {
		// The uint16 conversion is safe because Restrictions.Validate
		// rejects ports outside 1-65535.
		rules = append(rules, landlock.ConnectTCP(uint16(port)))
	}
	return rules
}
