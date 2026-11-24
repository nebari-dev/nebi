package exec

import (
	"errors"
	osexec "os/exec"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

// The stderr samples below are pixi 0.82.1 and 0.84.2 output, captured
// with stderr not a terminal (paths shortened or replaced).

const solveFailure = ` WARN Using local manifest
Error:   × failed to solve requirements of environment 'default' for platform 'osx-
  │ arm64'
  ├─▶   × failed to solve the environment
  │
  ╰─▶ Cannot solve the request because of: No candidates were found for nebi-
      no-such-package-xyz *.

`

const networkFailure = ` WARN Could not read repodata to find the virtual packages the solve can reference, considering all registered detectors: Request failed after 3 retries
Error:   × failed to solve requirements of environment 'default' for platform 'osx-
  │ arm64'
  ├─▶   × Request failed after 3 retries
  │
  ├─▶ error sending request for url (https://nebi-invalid.invalid/chan/noarch/
  │   repodata_shards.msgpack.zst)
  ├─▶ client error (Connect)
  ╰─▶ failed to lookup address information: nodename nor servname provided, or
      not known
`

const manifestFailure = `Error:   × unexpected eof encountered
   ╭─[/tmp/exp/pixi.toml:8:1]
 7 │ garbage = [
   ╰────

`

const staleLock = `i lock file would be updated (not written to disk)
Environment: default
  + (conda) libzlib   1.3.2 h8088a28_3
  + (conda) zlib      1.3.2 h8088a28_3
Error:   × lock file not up-to-date with the workspace

`

const wrappedPath = `Error:   × could not find pixi.toml or pyproject.toml with tool.pixi at directory /
  │ home/user/nebi-projects/a-project-directory-with-a-long-name-for-
  │ wrapping/exp-0.82.1

`

func TestErrorLine(t *testing.T) {
	for _, c := range []struct {
		name, stderr, want string
	}{
		{"solve failure", solveFailure,
			"failed to solve requirements of environment 'default' for platform 'osx-arm64': failed to solve the environment: Cannot solve the request because of: No candidates were found for nebi-no-such-package-xyz *."},
		{"network failure", networkFailure,
			"failed to solve requirements of environment 'default' for platform 'osx-arm64': Request failed after 3 retries: error sending request for url (https://nebi-invalid.invalid/chan/noarch/repodata_shards.msgpack.zst): client error (Connect): failed to lookup address information: nodename nor servname provided, or not known"},
		{"source snippet left out", manifestFailure, "unexpected eof encountered"},
		{"stale lock", staleLock, "lock file not up-to-date with the workspace"},
		{"wrapped path", wrappedPath,
			"could not find pixi.toml or pyproject.toml with tool.pixi at directory /home/user/nebi-projects/a-project-directory-with-a-long-name-for-wrapping/exp-0.82.1"},
		{"crlf", strings.ReplaceAll(staleLock, "\n", "\r\n"), "lock file not up-to-date with the workspace"},
		{"no Error line", "something\nthread 'main' panicked at src/main.rs\n\n", "thread 'main' panicked at src/main.rs"},
		{"empty", "", ""},
		{"last report wins", "Error:   × first\n\nError:   × second\n", "second"},
	} {
		if got := errorLine(c.stderr); got != c.want {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, c.want)
		}
	}
}

func TestErrorLineIsCapped(t *testing.T) {
	got := errorLine("Error:   × " + strings.Repeat("é", maxErrorLine))
	if len(got) > maxErrorLine+len("…") || !utf8.ValidString(got) || !strings.HasSuffix(got, "…") {
		t.Fatalf("want a valid message of at most %d bytes ending in …, got %d bytes", maxErrorLine, len(got))
	}
}

func TestIsStaleLockOutput(t *testing.T) {
	if !isStaleLockOutput(staleLock) {
		t.Error("pixi's stale-lock report must be recognised")
	}
	for _, s := range []string{solveFailure, networkFailure, manifestFailure, wrappedPath, ""} {
		if isStaleLockOutput(s) {
			t.Errorf("not a stale lock: %q", s)
		}
	}
}

func TestCommandError(t *testing.T) {
	exitErr := &osexec.ExitError{}
	err := error(&CommandError{Command: "pixi lock", Line: "No candidates were found for foo", Err: exitErr})
	if !strings.Contains(err.Error(), "pixi lock: No candidates were found for foo") {
		t.Fatalf("the error must carry pixi's message, got %q", err)
	}
	if !errors.Is(err, exitErr) {
		t.Fatal("the error must wrap the process error")
	}
	bare := &CommandError{Command: "pixi lock", Err: errors.New("exit status 1")}
	if bare.Error() != "pixi lock: exit status 1" {
		t.Fatalf("got %q", bare)
	}
}

func TestStaleLockError(t *testing.T) {
	err := &StaleLockError{Dir: "/p", Detail: "lock file not up-to-date with the workspace"}
	if !err.StaleLock() {
		t.Fatal("StaleLock must be true")
	}
	if want := "pixi.lock in /p does not satisfy pixi.toml: lock file not up-to-date with the workspace"; err.Error() != want {
		t.Fatalf("got %q, want %q", err, want)
	}
}

func TestCommandEnv(t *testing.T) {
	got := commandEnv([]string{"HOME=/h", "PIXI_LOCKED=true", "PIXI_FROZEN=1", "PIXI_COLOR=always", "PIXI_CACHE_DIR=/c", "PATH=/bin"})
	want := []string{"HOME=/h", "PIXI_CACHE_DIR=/c", "PATH=/bin", "PIXI_COLOR=never"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestTailBuffer(t *testing.T) {
	b := &tailBuffer{max: 5}
	for _, s := range []string{"ab", "cdef", "g"} {
		if n, err := b.Write([]byte(s)); n != len(s) || err != nil {
			t.Fatalf("Write(%q) = %d, %v", s, n, err)
		}
	}
	if b.String() != "cdefg" {
		t.Fatalf("got %q, want the last 5 bytes", b.String())
	}
}

// TestTailBufferKeepsTail writes sequences of chunks shorter than, equal
// to and longer than the buffer, and checks after every write that it
// holds the last max bytes written so far.
func TestTailBufferKeepsTail(t *testing.T) {
	for _, chunks := range [][]string{
		{},
		{"", "a", ""},
		{"abc", "de"},
		{"abcde", "f"},
		{"abcdefgh", "ij"},
		{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"},
		{"abc", "defg", "hi", "jklmnopq", "r", "st", "uvwxy", "z"},
		{"abcd", "efghijkl", "mn", "o", "pqrstu"},
	} {
		b := &tailBuffer{max: 5}
		all := ""
		for _, c := range chunks {
			if n, err := b.Write([]byte(c)); n != len(c) || err != nil {
				t.Fatalf("%q: Write(%q) = %d, %v", chunks, c, n, err)
			}
			all += c
			want := all[max(len(all)-5, 0):]
			if got := b.String(); got != want {
				t.Fatalf("%q: after %q got %q, want %q", chunks, c, got, want)
			}
		}
	}
}
