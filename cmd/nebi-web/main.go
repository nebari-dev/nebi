package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/nebari-dev/nebi/internal/client/api"
	"github.com/nebari-dev/nebi/internal/client/boot"
	"github.com/nebari-dev/nebi/internal/client/web"
	"github.com/nebari-dev/nebi/internal/frontend"
	"github.com/spf13/cobra"
)

// Version is set via ldflags at build time.
var Version = "dev"

// Commit is the git commit hash, set via ldflags at build time.
var Commit = ""

var (
	port int
	// host is the removed --host flag, kept hidden to explain its removal.
	host string
)

var rootCmd = &cobra.Command{
	Use:   "nebi-web",
	Short: "Run the Nebi local web app",
	Long: `Start the Nebi local web app: the client API and the client UI on
127.0.0.1, for your browser. It prints a sign-in URL to open.

Only one nebi app runs for a data directory at a time. When the desktop app
or another nebi-web already runs for it, nebi-web prints that app's URL and
exits.

Environment:
  NEBI_DATA_DIR      Data directory (default: the OS application data directory)
  NEBI_PROJECTS_DIR  Where new projects go when no path is given (default: ~/nebi-projects)
  NEBI_PIXI_PATH     The pixi binary (default: pixi on PATH, else ~/.pixi/bin,
                     Homebrew's folders on macOS, or ~/.local/bin)

Examples:
  nebi-web                    # Serve on 127.0.0.1:8460
  nebi-web --port 8080        # Serve on another port`,
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          run,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version",
	Long:  `Print the version of the nebi-web executable.`,
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("nebi-web version %s\n", Version)
	},
}

func init() {
	rootCmd.Flags().IntVarP(&port, "port", "p", web.DefaultPort, "Port to listen on, on 127.0.0.1")
	rootCmd.Flags().StringVar(&host, "host", "", "Removed: nebi-web listens on 127.0.0.1 only")
	_ = rootCmd.Flags().MarkHidden("host")
	rootCmd.AddCommand(versionCmd)
}

func run(cmd *cobra.Command, _ []string) error {
	if cmd.Flags().Changed("host") {
		return fmt.Errorf("--host was removed: nebi-web listens on 127.0.0.1 only, behind a session token")
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("--port %d is not a port number", port)
	}
	dataDir, err := boot.DataDir()
	if err != nil {
		return err
	}
	pixiPath, err := boot.FindPixi()
	if err != nil {
		return err
	}
	assets, err := frontend.ClientApp()
	if err != nil {
		return fmt.Errorf("loading the client UI: %w", err)
	}

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// After the first signal, a second one stops the process at once.
	go func() {
		<-ctx.Done()
		stop()
	}()

	return web.Run(ctx, web.Options{
		Port:        port,
		DataDir:     dataDir,
		ProjectsDir: boot.ProjectsDir(),
		PixiPath:    pixiPath,
		Version:     api.VersionInfo{NebiVersion: Version, Commit: Commit},
		Assets:      assets,
		Stdout:      cmd.OutOrStdout(),
		Stderr:      cmd.ErrOrStderr(),
	})
}

func main() {
	if err := rootCmd.ExecuteContext(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
