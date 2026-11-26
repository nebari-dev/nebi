package pixi

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// MinimumVersion is the oldest pixi nebi runs. Older releases, such as the
// v0.58.0 that earlier nebi versions auto-installed, trust certificate
// authorities that were compromised (#620, #625).
const MinimumVersion = "0.82.1"

// ErrVersionTooOld is matched by the *VersionTooOldError CheckVersion
// returns for a pixi older than MinimumVersion.
var ErrVersionTooOld = errors.New("pixi is older than the minimum version")

// VersionTooOldError is returned for a pixi binary older than
// MinimumVersion. It matches ErrVersionTooOld with errors.Is.
type VersionTooOldError struct {
	Path    string
	Found   string
	Minimum string
}

func (e *VersionTooOldError) Error() string {
	return fmt.Sprintf("pixi at %s is version %s; nebi needs pixi %s or newer: upgrade it (for example with `pixi self-update`) or configure a newer pixi binary",
		e.Path, e.Found, e.Minimum)
}

// Unwrap makes errors.Is(err, ErrVersionTooOld) true.
func (e *VersionTooOldError) Unwrap() error { return ErrVersionTooOld }

// versionRE matches the version in `pixi --version` output, such as
// "pixi 0.82.1" or "pixi 0.83.0-dev".
var versionRE = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

// ParseVersion returns the major, minor and patch numbers of the version
// in s, the output of `pixi --version` or a bare version. A pre-release
// or build suffix is ignored.
func ParseVersion(s string) ([3]int, error) {
	m := versionRE.FindStringSubmatch(s)
	if m == nil {
		return [3]int{}, fmt.Errorf("no pixi version in %q", strings.TrimSpace(s))
	}
	var v [3]int
	for i := range v {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return [3]int{}, fmt.Errorf("pixi version %q: %w", m[0], err)
		}
		v[i] = n
	}
	return v, nil
}

// compareVersions returns -1, 0 or 1 as a is older than, equal to or newer
// than b.
func compareVersions(a, b [3]int) int {
	for i := range a {
		switch {
		case a[i] < b[i]:
			return -1
		case a[i] > b[i]:
			return 1
		}
	}
	return 0
}

// CheckVersion runs `path --version` and returns the version it reports.
// It returns a *VersionTooOldError for a version older than
// MinimumVersion, and an error when the binary does not run or reports no
// version.
func CheckVersion(ctx context.Context, path string) (string, error) {
	out, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return "", fmt.Errorf("running %s --version: %w: %s", path, err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", fmt.Errorf("running %s --version: %w", path, err)
	}
	found, err := ParseVersion(string(out))
	if err != nil {
		return "", fmt.Errorf("pixi at %s: %w", path, err)
	}
	minimum, err := ParseVersion(MinimumVersion)
	if err != nil {
		return "", err
	}
	version := fmt.Sprintf("%d.%d.%d", found[0], found[1], found[2])
	if compareVersions(found, minimum) < 0 {
		return version, &VersionTooOldError{Path: path, Found: version, Minimum: MinimumVersion}
	}
	return version, nil
}
