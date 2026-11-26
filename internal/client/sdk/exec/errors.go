package exec

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/nebari-dev/nebi/internal/artifact"
)

// CommandError is returned when pixi exits with an error. Line is pixi's
// own error message, its causes joined with ": ", so the error says why
// pixi failed and not only "exit status 1".
type CommandError struct {
	// Command is the pixi command, such as "pixi install --frozen". It is
	// left out of the message, because the SDK labels each step itself.
	Command string
	// Line is pixi's error message; empty when pixi printed none.
	Line string
	// Err is the error from running the process.
	Err error
}

func (e *CommandError) Error() string {
	if e.Line == "" {
		return e.Err.Error()
	}
	return fmt.Sprintf("%s (%v)", e.Line, e.Err)
}

func (e *CommandError) Unwrap() error { return e.Err }

// StaleLockError is returned by CheckLock when pixi.lock is missing or no
// longer satisfies pixi.toml. Its StaleLock method lets the SDK recognise
// it (local.StaleLockError) without this package importing the SDK.
type StaleLockError struct {
	Dir string
	// Detail is pixi's message, or why the check did not need pixi.
	Detail string
}

// Error returns Detail alone, because the SDK says which files are out of
// step itself.
func (e *StaleLockError) Error() string {
	if e.Detail == "" {
		return fmt.Sprintf("%s does not satisfy %s", artifact.LockFile, artifact.ManifestFile)
	}
	return e.Detail
}

// StaleLock reports that the lock is stale.
func (e *StaleLockError) StaleLock() bool { return true }

// staleLockMessage is pixi's error for `pixi lock --check` when the lock
// file would change. pixi exits 1 for every error, so the message is what
// tells a stale lock from a check that could not run. Confirmed on pixi
// 0.82.1 and 0.84.2.
const staleLockMessage = "lock file not up-to-date with the workspace"

// isStaleLockOutput reports whether stderr, from `pixi lock --check`,
// reports a stale lock.
func isStaleLockOutput(stderr string) bool {
	return strings.Contains(errorLine(stderr), staleLockMessage)
}

// needsInstallMessage is pixi's error for `pixi lock --no-install` when it
// must install an environment to read the metadata of PyPI source
// dependencies. Confirmed on pixi 0.82.1 and 0.84.2.
const needsInstallMessage = "installation of conda environment is required to solve PyPI source dependencies"

// isNeedsInstallOutput reports whether stderr, from
// `pixi lock --no-install`, says that pixi must install to lock.
func isNeedsInstallOutput(stderr string) bool {
	return strings.Contains(errorLine(stderr), needsInstallMessage)
}

// maxErrorLine caps the length of the error message taken from pixi.
const maxErrorLine = 2000

// errorLine returns pixi's error message from its stderr: the last report
// that starts with "Error:", with its wrapped lines joined and its causes
// appended after ": ". pixi prints reports like
//
//	Error:   × failed to solve requirements of environment 'default' for platform 'osx-
//	  │ arm64'
//	  ├─▶   × failed to solve the environment
//	  │
//	  ╰─▶ Cannot solve the request because of: No candidates were found for foo *.
//
// The source snippets and help that can follow are left out. Without an
// "Error:" line it returns the last non-empty line.
func errorLine(stderr string) string {
	lines := strings.Split(strings.ReplaceAll(stderr, "\r\n", "\n"), "\n")
	start := -1
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "Error:") {
			start = i
		}
	}
	if start < 0 {
		for i := len(lines) - 1; i >= 0; i-- {
			if l := strings.TrimSpace(lines[i]); l != "" {
				return truncate(l)
			}
		}
		return ""
	}

	var parts []string
	cur := ""
	// lastCause is set inside the last cause, whose wrapped lines are
	// indented with spaces instead of "│".
	lastCause := false
	appendText := func(text string) {
		text = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), "×"))
		switch {
		case text == "":
		case cur == "" || strings.HasSuffix(cur, "-") || strings.HasSuffix(cur, "/"):
			// pixi wraps long messages, breaking words at hyphens and
			// slashes as well as spaces.
			cur += text
		default:
			cur += " " + text
		}
	}
	appendText(strings.TrimPrefix(strings.TrimSpace(lines[start]), "Error:"))
	for _, l := range lines[start+1:] {
		t := strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(t, "├─▶"), strings.HasPrefix(t, "╰─▶"):
			// A cause starts.
			if cur != "" {
				parts = append(parts, cur)
			}
			cur = ""
			lastCause = strings.HasPrefix(t, "╰─▶")
			appendText(strings.TrimPrefix(strings.TrimPrefix(t, "├─▶"), "╰─▶"))
		case strings.HasPrefix(t, "│"):
			appendText(strings.TrimPrefix(t, "│"))
		case t == "":
			// pixi puts blank lines inside and after a report; skip them.
			continue
		case lastCause && strings.HasPrefix(l, "    "):
			appendText(t)
		default:
			// A source snippet, help, or output after the report.
			if cur != "" {
				parts = append(parts, cur)
			}
			return truncate(strings.Join(parts, ": "))
		}
	}
	if cur != "" {
		parts = append(parts, cur)
	}
	return truncate(strings.Join(parts, ": "))
}

func truncate(s string) string {
	if len(s) <= maxErrorLine {
		return s
	}
	cut := maxErrorLine
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}
