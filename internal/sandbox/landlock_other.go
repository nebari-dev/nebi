//go:build !linux

package sandbox

import "errors"

// ErrUnsupported reports that this platform cannot confine builds.
var ErrUnsupported = errors.New("sandbox: filesystem confinement requires Linux with Landlock")

// Confine is a no-op stub on non-Linux platforms.
func Confine(Restrictions) error { return ErrUnsupported }
