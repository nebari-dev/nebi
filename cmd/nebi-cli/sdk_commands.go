package main

// Foundation for the SDK-backed commands (issue #649, CLI parity rule):
// every function in internal/client/sdk and internal/client/jobs has a CLI
// command that calls the SDK directly against the local database, with
// jobs running inline and their log streamed to the terminal.
//
// Each unit owns one unit_<unit>.go file with a register<Unit>Commands
// function called from registerUnitCommands. Units attach subcommands to
// the parent commands below or to existing parents (projectCmd,
// projectVersionCmd, registryCmd) inside their own register function.
//
// This file is frozen.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/spf13/cobra"
)

// serverCmd groups team-server commands (connection, server projects,
// pull, sharing).
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Connect to a Nebi team server and work with its projects",
}

// jobCmd groups job commands.
var jobCmd = &cobra.Command{
	Use:   "job",
	Short: "List, inspect and cancel background jobs on this machine",
}

// registerUnitCommands calls every unit's register function.
func registerUnitCommands(root *cobra.Command) {
	registerProjectsCommands(root)
	registerDetailCommands(root)
	registerNewVersionCommands(root)
	registerCompareCommands(root)
	registerRemoteCommands(root)
	registerSharingCommands(root)
	registerPublishCommands(root)
	registerJobsCommands(root)
}

// withStack opens the local client stack, runs fn with a context cancelled
// on Ctrl-C (which cancels an inline job), and closes the stack.
func withStack(cmd *cobra.Command, fn func(ctx context.Context, s *client.Stack) error) error {
	stack, closeFn, err := client.OpenCLI(cmd.OutOrStdout())
	if err != nil {
		return err
	}
	defer closeFn()
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return explainSDKError(fn(ctx, stack))
}

// resolveProjectID resolves a project name or UUID argument.
func resolveProjectID(ctx context.Context, s *client.Stack, ref string) (uuid.UUID, error) {
	p, err := s.SDK.Local.ResolveProject(ctx, ref)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return uuid.Nil, fmt.Errorf("project %q not found", ref)
		}
		return uuid.Nil, err
	}
	return p.ID, nil
}

// parseVersionArg parses a version number argument.
func parseVersionArg(arg string) (int, error) {
	n, err := strconv.Atoi(arg)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("invalid version %q: expected a number such as 3", arg)
	}
	return n, nil
}

// explainSDKError turns SDK errors into CLI-friendly messages.
func explainSDKError(err error) error {
	if err == nil {
		return nil
	}
	var verr *shared.ValidationError
	var cerr *shared.ConflictError
	var rerr *shared.RemoteError
	switch {
	case errors.As(err, &verr):
		if verr.Field != "" {
			return fmt.Errorf("%s: %s", verr.Field, verr.Message)
		}
		return errors.New(verr.Message)
	case errors.As(err, &cerr):
		return errors.New(cerr.Message)
	case errors.As(err, &rerr):
		if rerr.Status == 0 {
			return fmt.Errorf("server unreachable: %s", rerr.Message)
		}
		return fmt.Errorf("server error (%d): %s", rerr.Status, rerr.Message)
	case errors.Is(err, shared.ErrNotConnected):
		return errors.New("no server connected; run 'nebi server connect <url>' first")
	}
	return err
}
