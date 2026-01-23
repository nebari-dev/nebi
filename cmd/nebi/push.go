package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aktech/darb/internal/cliclient"
	"github.com/aktech/darb/internal/diff"
	"github.com/aktech/darb/internal/drift"
	"github.com/aktech/darb/internal/nebifile"
	"github.com/spf13/cobra"
)

var (
	pushRegistry string
	pushDryRun   bool
)

var pushCmd = &cobra.Command{
	Use:   "push <workspace>:<tag>",
	Short: "Push workspace to registry",
	Long: `Push a workspace to an OCI registry with a tag.

Looks for pixi.toml and pixi.lock in the current directory.
If the workspace doesn't exist on the server, it will be created automatically.

Examples:
  # Push with tag
  nebi push myworkspace:v1.0.0 -r ds-team

  # Push using default registry
  nebi push myworkspace:v1.0.0

  # Preview what would be pushed
  nebi push myworkspace:v1.1 --dry-run`,
	Args: cobra.ExactArgs(1),
	Run:  runPush,
}

func init() {
	pushCmd.Flags().StringVarP(&pushRegistry, "registry", "r", "", "Named registry (optional if default set)")
	pushCmd.Flags().BoolVar(&pushDryRun, "dry-run", false, "Preview what would be pushed without actually pushing")
}

func runPush(cmd *cobra.Command, args []string) {
	// Parse workspace:tag format
	workspaceName, tag, err := parseWorkspaceRef(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintln(os.Stderr, "Usage: nebi push <workspace>:<tag>")
		os.Exit(1)
	}

	if tag == "" {
		fmt.Fprintf(os.Stderr, "Error: tag is required\n")
		fmt.Fprintln(os.Stderr, "Usage: nebi push <workspace>:<tag>")
		os.Exit(1)
	}

	// Check for local pixi.toml
	pixiTomlContent, err := os.ReadFile("pixi.toml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: pixi.toml not found in current directory\n")
		fmt.Fprintln(os.Stderr, "Run 'pixi init' to create a pixi project first")
		os.Exit(1)
	}

	// Check for local pixi.lock (optional but recommended)
	pixiLockContent, _ := os.ReadFile("pixi.lock")
	if len(pixiLockContent) == 0 {
		fmt.Fprintln(os.Stderr, "Warning: pixi.lock not found. Run 'pixi install' to generate it.")
	}

	// Handle --dry-run: show diff against origin if .nebi exists
	if pushDryRun {
		runPushDryRun(workspaceName, tag, pixiTomlContent, pixiLockContent)
		return
	}

	// Check for drift awareness: warn if pushing modified workspace
	absDir, _ := filepath.Abs(".")
	showPushDriftWarning(absDir, workspaceName, tag, pixiTomlContent)

	client := mustGetClient()
	ctx := mustGetAuthContext()

	// Try to find workspace by name, create if not found
	env, err := findWorkspaceByName(client, ctx, workspaceName)
	if err != nil {
		// Workspace doesn't exist, create it
		fmt.Printf("Creating workspace %q...\n", workspaceName)
		pixiTomlStr := string(pixiTomlContent)
		pkgMgr := "pixi"
		createReq := cliclient.CreateEnvironmentRequest{
			Name:           workspaceName,
			PackageManager: &pkgMgr,
			PixiToml:       &pixiTomlStr,
		}

		newEnv, createErr := client.CreateEnvironment(ctx, createReq)
		if createErr != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to create workspace %q: %v\n", workspaceName, createErr)
			os.Exit(1)
		}
		fmt.Printf("Created workspace %q\n", workspaceName)

		// Wait for environment to be ready
		env, err = waitForEnvReady(client, ctx, newEnv.ID, 60*time.Second)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}

	// Find registry
	var registry *cliclient.Registry
	if pushRegistry != "" {
		registry, err = findRegistryByName(client, ctx, pushRegistry)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	} else {
		// Find default registry
		registry, err = findDefaultRegistry(client, ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			fmt.Fprintln(os.Stderr, "Hint: Set a default registry with 'nebi registry set-default <name>' or specify one with -r")
			os.Exit(1)
		}
	}

	// Use workspace name as repository
	repository := workspaceName

	req := cliclient.PublishRequest{
		RegistryID: registry.ID,
		Repository: repository,
		Tag:        tag,
	}

	fmt.Printf("Pushing %s:%s to %s...\n", repository, tag, registry.Name)
	resp, err := client.PublishEnvironment(ctx, env.ID, req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to push %s:%s: %v\n", repository, tag, err)
		os.Exit(1)
	}

	fmt.Printf("Pushed %s:%s\n", repository, tag)
	if resp.Digest != "" {
		fmt.Printf("  Digest: %s\n", resp.Digest)
	}
	fmt.Printf("\nSuccessfully pushed to %s\n", registry.Name)
}

// runPushDryRun shows what would be pushed without actually pushing.
func runPushDryRun(workspaceName, tag string, pixiTomlContent, pixiLockContent []byte) {
	absDir, _ := filepath.Abs(".")

	fmt.Printf("Would push %s:%s\n\n", workspaceName, tag)

	// Check if we have a .nebi file to diff against
	if nebifile.Exists(absDir) {
		nf, err := nebifile.Read(absDir)
		if err == nil {
			// Fetch origin content for comparison
			client := mustGetClient()
			ctx := mustGetAuthContext()

			env, err := findWorkspaceByName(client, ctx, nf.Origin.Workspace)
			if err == nil {
				vc, err := drift.FetchVersionContent(ctx, client, env.ID, nf.Origin.ServerVersionID)
				if err == nil {
					// Show TOML diff
					tomlDiff, err := diff.CompareToml([]byte(vc.PixiToml), pixiTomlContent)
					if err == nil && tomlDiff.HasChanges() {
						sourceLabel := fmt.Sprintf("origin (%s:%s)", nf.Origin.Workspace, nf.Origin.Tag)
						targetLabel := "local (to be pushed)"
						fmt.Print(diff.FormatUnifiedDiff(tomlDiff, sourceLabel, targetLabel))
					}

					// Show lock file change indicator
					if !bytesEqual([]byte(vc.PixiLock), pixiLockContent) {
						fmt.Println("\n@@ pixi.lock (changed) @@")
					}

					if tomlDiff != nil && !tomlDiff.HasChanges() && bytesEqual([]byte(vc.PixiLock), pixiLockContent) {
						fmt.Println("No changes from origin")
					}
				}
			}
		}
	} else {
		fmt.Println("(No .nebi metadata found - cannot show diff against origin)")
		fmt.Printf("\nFiles to push:\n")
		fmt.Printf("  pixi.toml: %d bytes\n", len(pixiTomlContent))
		if len(pixiLockContent) > 0 {
			fmt.Printf("  pixi.lock: %d bytes\n", len(pixiLockContent))
		}
	}

	fmt.Println("\nRun without --dry-run to push.")
}

// parseWorkspaceRef parses a reference in the format workspace:tag or workspace@digest.
// Returns (workspace, tag, error) for tag references.
// Returns (workspace, "", error) for digest references (digest is in tag field with @ prefix).
func parseWorkspaceRef(ref string) (workspace string, tag string, err error) {
	// Check for digest reference first (workspace@sha256:...)
	if idx := strings.Index(ref, "@"); idx != -1 {
		return ref[:idx], ref[idx:], nil // Return @sha256:... as the "tag"
	}

	// Check for tag reference (workspace:tag)
	if idx := strings.LastIndex(ref, ":"); idx != -1 {
		return ref[:idx], ref[idx+1:], nil
	}

	// No tag or digest specified
	return ref, "", nil
}

// showPushDriftWarning checks if the local workspace has been modified and shows
// an informational note about what's being pushed relative to the origin.
func showPushDriftWarning(dir, workspaceName, tag string, pixiTomlContent []byte) {
	if !nebifile.Exists(dir) {
		return // No origin info available
	}

	nf, err := nebifile.Read(dir)
	if err != nil {
		return
	}

	// Check local drift
	ws, err := drift.Check(dir)
	if err != nil {
		return
	}

	if ws.IsModified() {
		fmt.Printf("Note: This workspace was originally pulled from %s:%s\n",
			nf.Origin.Workspace, nf.Origin.Tag)
		for _, f := range ws.Files {
			switch f.Status {
			case drift.StatusModified:
				fmt.Printf("  - %s modified locally\n", f.Filename)
			case drift.StatusMissing:
				fmt.Printf("  - %s deleted locally\n", f.Filename)
			}
		}
		fmt.Println()
	}

	// Warn if pushing to the same tag as origin (potential overwrite)
	if tag == nf.Origin.Tag && workspaceName == nf.Origin.Workspace && ws.IsModified() {
		fmt.Fprintf(os.Stderr, "Warning: Pushing modified content back to the same tag %q\n", tag)
		fmt.Fprintf(os.Stderr, "  This will overwrite the existing content in the registry.\n")
		fmt.Fprintf(os.Stderr, "  Consider using a new tag (e.g., %s-1) to preserve the original.\n\n",
			tag)
	}
}

// waitForEnvReady polls until the environment is ready or timeout.
func waitForEnvReady(client *cliclient.Client, ctx context.Context, envID string, timeout time.Duration) (*cliclient.Environment, error) {
	deadline := time.Now().Add(timeout)
	fmt.Print("Waiting for environment to be ready")

	for time.Now().Before(deadline) {
		env, err := client.GetEnvironment(ctx, envID)
		if err != nil {
			return nil, fmt.Errorf("failed to get environment status: %v", err)
		}

		switch env.Status {
		case "ready":
			fmt.Println(" done")
			return env, nil
		case "failed", "error":
			fmt.Println(" failed")
			return nil, fmt.Errorf("environment setup failed")
		default:
			fmt.Print(".")
			time.Sleep(500 * time.Millisecond)
		}
	}

	fmt.Println(" timeout")
	return nil, fmt.Errorf("timeout waiting for environment to be ready")
}
