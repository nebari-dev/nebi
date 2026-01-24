package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/aktech/darb/internal/drift"
	"github.com/aktech/darb/internal/nebifile"
	"github.com/spf13/cobra"
)

var (
	statusRemote  bool
	statusJSON    bool
	statusVerbose bool
	statusPath    string
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show workspace drift status",
	Long: `Show the current state of a local workspace by comparing local files
against the origin layer digests stored in the .nebi metadata file.

This command works offline unless --remote is specified.

Status values:
  clean    - Local files match what was pulled
  modified - Local files have changed since pull
  missing  - Tracked files have been deleted

Examples:
  # Quick status check
  nebi status

  # Verbose output with next-step suggestions
  nebi status -v

  # Check if remote tag has been updated
  nebi status --remote

  # Machine-readable JSON output
  nebi status --json

  # Check workspace at a specific path
  nebi status -C /path/to/workspace`,
	Args: cobra.NoArgs,
	Run:  runStatus,
}

func init() {
	statusCmd.Flags().BoolVar(&statusRemote, "remote", false, "Also check remote for tag updates")
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "Output as JSON")
	statusCmd.Flags().BoolVarP(&statusVerbose, "verbose", "v", false, "Verbose output")
	statusCmd.Flags().StringVarP(&statusPath, "path", "C", ".", "Workspace directory path")
}

func runStatus(cmd *cobra.Command, args []string) {
	dir := statusPath

	// Resolve absolute path
	absDir, err := filepath.Abs(dir)
	if err != nil {
		absDir = dir
	}

	// Read .nebi metadata
	nf, err := nebifile.Read(absDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintln(os.Stderr, "Hint: Run 'nebi pull' first to create a workspace with tracking metadata.")
		os.Exit(2)
	}

	// Perform local drift check
	ws := drift.CheckWithNebiFile(absDir, nf)

	// Perform remote check if requested
	var remoteStatus *drift.RemoteStatus
	if statusRemote {
		client := mustGetClient()
		ctx := mustGetAuthContext()
		remoteStatus = drift.CheckRemote(ctx, client, nf)
	}

	// Output
	if statusJSON {
		outputStatusJSON(ws, nf, remoteStatus)
	} else if statusVerbose {
		outputStatusVerbose(ws, nf, remoteStatus)
	} else {
		outputStatusCompact(ws, nf, remoteStatus)
	}

	// Exit code
	if ws.IsModified() {
		os.Exit(1)
	}
}

func outputStatusCompact(ws *drift.RepoStatus, nf *nebifile.NebiFile, remote *drift.RemoteStatus) {
	status := string(ws.Overall)
	ref := nf.Origin.Repo
	if nf.Origin.Tag != "" {
		ref += ":" + nf.Origin.Tag
	}

	registry := nf.Origin.RegistryURL
	if registry == "" {
		registry = "server"
	}

	fmt.Printf("%s (%s)  •  pulled %s  •  %s\n", ref, registry, formatTimeAgo(nf.Origin.PulledAt), status)

	if remote != nil && remote.TagHasMoved {
		fmt.Printf("  ⚠ Tag '%s' has been updated on remote\n", nf.Origin.Tag)
	}
}

func outputStatusVerbose(ws *drift.RepoStatus, nf *nebifile.NebiFile, remote *drift.RemoteStatus) {
	fmt.Printf("Repo:      %s:%s\n", nf.Origin.Repo, nf.Origin.Tag)
	if nf.Origin.RegistryURL != "" {
		fmt.Printf("Registry:  %s\n", nf.Origin.RegistryURL)
	}
	fmt.Printf("Server:    %s\n", nf.Origin.ServerURL)
	fmt.Printf("Pulled:    %s (%s)\n", nf.Origin.PulledAt.Format("2006-01-02 15:04:05"), formatTimeAgo(nf.Origin.PulledAt))
	if nf.Origin.ManifestDigest != "" {
		fmt.Printf("Digest:    %s\n", nf.Origin.ManifestDigest)
	}
	fmt.Println()

	fmt.Printf("Status:    %s\n", ws.Overall)
	for _, fs := range ws.Files {
		fmt.Printf("  %-12s %s\n", fs.Filename+":", string(fs.Status))
	}

	if remote != nil {
		fmt.Println()
		fmt.Println("Remote:")
		if remote.Error != "" {
			fmt.Printf("  Error: %s\n", remote.Error)
		} else if remote.TagHasMoved {
			fmt.Printf("  ⚠ Tag '%s' now points to %s (was %s when pulled)\n",
				nf.Origin.Tag, remote.CurrentTagDigest, remote.OriginDigest)
			fmt.Println("  The tag has been updated since you pulled.")
		} else {
			fmt.Println("  Tag unchanged since pull")
		}
	}

	if ws.IsModified() {
		fmt.Println()
		fmt.Println("Next steps:")
		fmt.Println("  nebi diff              # See what changed")
		fmt.Println("  nebi pull --force      # Discard local changes")
		fmt.Printf("  nebi push %s:<tag>  # Publish as new version\n", nf.Origin.Repo)
	}
}

func outputStatusJSON(ws *drift.RepoStatus, nf *nebifile.NebiFile, remote *drift.RemoteStatus) {
	data, err := formatStatusJSONHelper(ws, nf, remote)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to marshal JSON: %v\n", err)
		os.Exit(2)
	}
	fmt.Println(string(data))
}

func formatStatusJSONHelper(ws *drift.RepoStatus, nf *nebifile.NebiFile, remote *drift.RemoteStatus) ([]byte, error) {
	// Use the diff package's JSON formatter
	return formatStatusJSONInternal(ws, nf, remote)
}
