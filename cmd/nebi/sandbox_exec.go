package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/nebari-dev/nebi/internal/sandbox"
)

var (
	sandboxExecMode    string
	sandboxExecRW      []string
	sandboxExecRO      []string
	sandboxExecROFiles []string
	sandboxExecPorts   []int
	sandboxExecCheck   bool
)

// sandboxExecCmd is an internal re-exec shim, not a user-facing command. The
// server invokes it as:
//
//	nebi sandbox-exec --mode=strict --allow-rw=/ws [--allow-ro=/usr ...] \
//	    [--allow-ro-file=/etc/resolv.conf ...] [--allow-port=443 ...] \
//	    -- /path/to/pixi install -v
//
// It applies a Landlock ruleset to itself and then execve's the real command,
// which inherits the confinement.
//
// "nebi sandbox-exec --check" is the probe sandbox.NewRunner uses to prove a
// re-exec target really is this binary. It must stay a fast, side-effect-free
// exit 0.
var sandboxExecCmd = &cobra.Command{
	Use:                   "sandbox-exec [flags] -- COMMAND [ARGS...]",
	Short:                 "Internal: run a command under filesystem and network confinement",
	Hidden:                true,
	DisableFlagsInUseLine: true,
	Args: func(cmd *cobra.Command, args []string) error {
		if sandboxExecCheck {
			return nil // --check takes no command
		}
		return cobra.MinimumNArgs(1)(cmd, args)
	},
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Answered before anything else: the probe must not depend on the
		// mode being valid, on a ruleset applying, or on this host
		// supporting Landlock at all. It answers one question only, "are
		// you the shim", and the answer is yes by virtue of being here.
		if sandboxExecCheck {
			return nil
		}

		mode := sandbox.Mode(sandboxExecMode)
		switch mode {
		case sandbox.ModeStrict, sandbox.ModePermissive:
		default:
			failSetup(fmt.Errorf("sandbox-exec requires --mode=strict or --mode=permissive, got %q", sandboxExecMode))
		}
		if !filepath.IsAbs(args[0]) {
			failSetup(fmt.Errorf("sandbox-exec requires an absolute command path, got %q", args[0]))
		}

		err := sandbox.Confine(sandbox.Restrictions{
			RW:              sandboxExecRW,
			RO:              sandboxExecRO,
			ROFiles:         sandboxExecROFiles,
			RWFiles:         devFiles(),
			TCPConnectPorts: sandboxExecPorts,
		})
		switch {
		case err == nil:
		case errors.Is(err, sandbox.ErrNetworkUnrestricted):
			// Filesystem confinement held; only the TCP restriction is
			// missing. A warning even in strict mode, by design.
			fmt.Fprintf(os.Stderr, "[nebi] WARNING: %v\n", err)
		case mode == sandbox.ModePermissive:
			fmt.Fprintf(os.Stderr, "[nebi] WARNING: build is running UNCONFINED: %v\n", err)
		default:
			failSetup(err)
		}

		// execProcess replaces this process image on success and therefore
		// never returns; any return is a failure to launch the build.
		failSetup(execProcess(args))
		return nil // unreachable: failSetup exits
	},
}

// devFiles returns the device nodes a build legitimately needs. Missing
// nodes are skipped so the ruleset stays valid in minimal images.
func devFiles() []string {
	var out []string
	for _, f := range []string{"/dev/null", "/dev/urandom", "/dev/random", "/dev/zero"} {
		if _, err := os.Stat(f); err == nil {
			out = append(out, f)
		}
	}
	return out
}

// failSetup exits with the reserved code so the parent can distinguish a
// broken sandbox from a failed build.
//
// The hint stops short of blaming the kernel. ErrUnsupported also covers
// failures that are not about kernel support (an unreadable path, too many
// stacked rulesets), so it states the requirement conditionally and lets the
// wrapped error speak to the actual cause.
func failSetup(err error) {
	hint := ""
	if errors.Is(err, sandbox.ErrUnsupported) {
		hint = fmt.Sprintf(" (sandbox could not be established on %s/%s; if this host lacks Landlock support, which needs Linux 5.13+, set NEBI_SANDBOX_MODE=permissive to run builds unconfined or NEBI_SANDBOX_MODE=off to disable the sandbox)", runtime.GOOS, runtime.GOARCH)
	}
	fmt.Fprintf(os.Stderr, "[nebi] sandbox setup failed: %v%s\n", err, hint)
	os.Exit(sandbox.SetupFailureExitCode)
}

func init() {
	sandboxExecCmd.Flags().StringVar(&sandboxExecMode, "mode", "strict", "strict or permissive")
	sandboxExecCmd.Flags().StringArrayVar(&sandboxExecRW, "allow-rw", nil, "directory the command may read and write (repeatable)")
	sandboxExecCmd.Flags().StringArrayVar(&sandboxExecRO, "allow-ro", nil, "directory the command may read (repeatable)")
	sandboxExecCmd.Flags().StringArrayVar(&sandboxExecROFiles, "allow-ro-file", nil, "file the command may read (repeatable)")
	sandboxExecCmd.Flags().IntSliceVar(&sandboxExecPorts, "allow-port", nil, "TCP port the command may connect to (repeatable)")
	sandboxExecCmd.Flags().BoolVar(&sandboxExecCheck, "check", false, "exit 0 immediately, proving this binary implements the shim")
}
