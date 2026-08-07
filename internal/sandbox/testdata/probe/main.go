// Command probe is a deliberately hostile stand-in for a build backend. The
// Linux containment test in the parent package runs it inside the sandbox and
// asks it to do the things a malicious "pixi install" would try: dump the
// server environment, read another tenant's workspace, write outside its own
// workspace, open a nebi config file, and dial the database port. It also
// performs the one thing a legitimate build does constantly, renaming a
// staged file into a sibling directory, so the test can prove confinement did
// not break it.
//
// It is a compiled Go program rather than a /bin/sh script for three reasons:
//
//   - There is no raw rename(2) in a shell. GNU mv falls back to
//     copy-and-unlink when rename returns EXDEV, so it reports success even
//     while Landlock is blocking reparenting, which makes it useless as an
//     oracle.
//   - dash, which is /bin/sh on Debian and Ubuntu, has no /dev/tcp.
//   - Going through syscalls directly lets every operation report its errno,
//     which is what lets the test tell "the kernel denied this" apart from
//     "this failed for some unrelated reason".
//
// Output is one machine-readable line on stdout:
//
//	<op> OK                                (payload lines may follow)
//	<op> ERR errno=<NAME> err=<message>    (exit status 1)
//
// stderr is deliberately left untouched so the test can read the sandbox
// shim's own warnings there without them being mixed into the payload.
package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("probe ERR errno=none err=usage: probe <op> [args...]")
		os.Exit(2)
	}

	op, args := os.Args[1], os.Args[2:]
	payload, err := run(op, args)
	if err != nil {
		fmt.Printf("%s ERR errno=%s err=%v\n", op, errnoName(err), err)
		os.Exit(1)
	}
	fmt.Printf("%s OK\n", op)
	if payload != "" {
		fmt.Print(payload)
	}
}

func run(op string, args []string) (string, error) {
	switch op {
	case "env":
		if err := wantArgs(args, 0); err != nil {
			return "", err
		}
		env := append([]string(nil), os.Environ()...)
		sort.Strings(env)
		return strings.Join(env, "\n") + "\n", nil

	case "read":
		if err := wantArgs(args, 1); err != nil {
			return "", err
		}
		b, err := os.ReadFile(args[0])
		if err != nil {
			return "", err
		}
		return string(b), nil

	case "write":
		if err := wantArgs(args, 2); err != nil {
			return "", err
		}
		return "", os.WriteFile(args[0], []byte(args[1]), 0o600)

	case "rename":
		if err := wantArgs(args, 2); err != nil {
			return "", err
		}
		// os.Rename is renameat2/renameat, never a copy. Landlock without
		// the refer right fails this with EXDEV even within one filesystem.
		return "", os.Rename(args[0], args[1])

	case "connect":
		if err := wantArgs(args, 1); err != nil {
			return "", err
		}
		conn, err := net.DialTimeout("tcp", args[0], 10*time.Second)
		if err != nil {
			return "", err
		}
		return "", conn.Close()

	default:
		return "", fmt.Errorf("unknown op %q", op)
	}
}

func wantArgs(args []string, n int) error {
	if len(args) != n {
		return fmt.Errorf("expected %d argument(s), got %d", n, len(args))
	}
	return nil
}

// errnoName renders the underlying errno so the test can assert on the reason
// an operation failed, not merely that it did.
func errnoName(err error) string {
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return "none"
	}
	switch errno {
	case syscall.EACCES:
		return "EACCES"
	case syscall.EPERM:
		return "EPERM"
	case syscall.EXDEV:
		return "EXDEV"
	case syscall.EROFS:
		return "EROFS"
	case syscall.ENOENT:
		return "ENOENT"
	case syscall.EISDIR:
		return "EISDIR"
	case syscall.ECONNREFUSED:
		return "ECONNREFUSED"
	case syscall.ETIMEDOUT:
		return "ETIMEDOUT"
	case syscall.ENETUNREACH:
		return "ENETUNREACH"
	case syscall.EAFNOSUPPORT:
		return "EAFNOSUPPORT"
	}
	return fmt.Sprintf("errno(%d)", int(errno))
}
