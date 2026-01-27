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
	"github.com/aktech/darb/internal/localindex"
	"github.com/aktech/darb/internal/nebifile"
	"github.com/spf13/cobra"
)

var pushDryRun bool

var pushCmd = &cobra.Command{
	Use:   "push <repo>:<tag>",
	Short: "Push repo to server",
	Long: `Push a new version to the Nebi server with a tag.

Looks for pixi.toml and pixi.lock in the current directory.
If the repo doesn't exist on the server, it will be created automatically.

Use 'nebi publish' to distribute a pushed version to an OCI registry.

Examples:
  # Push with tag
  nebi push myrepo:v1.0.0

  # Preview what would be pushed
  nebi push myrepo:v1.1 --dry-run`,
	Args: cobra.ExactArgs(1),
	Run:  runPush,
}

func init() {
	pushCmd.Flags().BoolVar(&pushDryRun, "dry-run", false, "Preview what would be pushed without actually pushing")
}

func runPush(cmd *cobra.Command, args []string) {
	// Parse repo:tag format
	repoName, tag, err := parseRepoRef(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintln(os.Stderr, "Usage: nebi push <repo>:<tag>")
		osExit(1)
	}

	if tag == "" {
		fmt.Fprintf(os.Stderr, "Error: tag is required\n")
		fmt.Fprintln(os.Stderr, "Usage: nebi push <repo>:<tag>")
		osExit(1)
	}

	// Check for local pixi.toml
	pixiTomlContent, err := os.ReadFile("pixi.toml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: pixi.toml not found in current directory\n")
		fmt.Fprintln(os.Stderr, "Run 'pixi init' to create a pixi project first")
		osExit(1)
	}

	// Check for local pixi.lock (optional but recommended)
	pixiLockContent, _ := os.ReadFile("pixi.lock")
	if len(pixiLockContent) == 0 {
		fmt.Fprintln(os.Stderr, "Warning: pixi.lock not found. Run 'pixi install' to generate it.")
	}

	// Handle --dry-run: show diff against origin if .nebi exists
	if pushDryRun {
		runPushDryRun(repoName, tag, pixiTomlContent, pixiLockContent)
		return
	}

	// Check for drift awareness: warn if pushing modified repo
	absDir, _ := filepath.Abs(".")
	showPushDriftWarning(absDir, repoName, tag, pixiTomlContent)

	client := mustGetClient()
	ctx := mustGetAuthContext()

	// Try to find repo by name, create if not found
	env, err := findRepoByName(client, ctx, repoName)
	if err != nil {
		// Repo doesn't exist, create it
		fmt.Printf("Creating repo %q...\n", repoName)
		pixiTomlStr := string(pixiTomlContent)
		pkgMgr := "pixi"
		createReq := cliclient.CreateEnvironmentRequest{
			Name:           repoName,
			PackageManager: &pkgMgr,
			PixiToml:       &pixiTomlStr,
		}

		newEnv, createErr := client.CreateEnvironment(ctx, createReq)
		if createErr != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to create repo %q: %v\n", repoName, createErr)
			osExit(1)
		}
		fmt.Printf("Created repo %q\n", repoName)

		// Wait for environment to be ready
		env, err = waitForEnvReady(client, ctx, newEnv.ID, 60*time.Second)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			osExit(1)
		}
	}

	// Push version to server
	req := cliclient.PushRequest{
		Tag:      tag,
		PixiToml: string(pixiTomlContent),
		PixiLock: string(pixiLockContent),
	}

	fmt.Printf("Pushing %s:%s...\n", repoName, tag)
	resp, err := client.PushVersion(ctx, env.ID, req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to push %s:%s: %v\n", repoName, tag, err)
		osExit(1)
	}

	fmt.Printf("Pushed %s:%s (version %d)\n", repoName, tag, resp.VersionNumber)
	fmt.Printf("\nTo distribute via OCI registry:\n  nebi publish %s:%s -r <registry>\n", repoName, tag)

	// Update .nebi metadata and local index to reflect the pushed state.
	// After a successful push, the local files ARE the origin — status becomes "clean".
	cfg, cfgErr := loadConfig()
	if cfgErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to load config for metadata update: %v\n", cfgErr)
		return
	}

	pixiTomlDigest := nebifile.ComputeDigest(pixiTomlContent)
	var pixiLockDigest string
	if len(pixiLockContent) > 0 {
		pixiLockDigest = nebifile.ComputeDigest(pixiLockContent)
	}

	layers := map[string]nebifile.Layer{
		"pixi.toml": {
			Digest:    pixiTomlDigest,
			Size:      int64(len(pixiTomlContent)),
			MediaType: nebifile.MediaTypePixiToml,
		},
	}
	if pixiLockDigest != "" {
		layers["pixi.lock"] = nebifile.Layer{
			Digest:    pixiLockDigest,
			Size:      int64(len(pixiLockContent)),
			MediaType: nebifile.MediaTypePixiLock,
		}
	}

	nf := nebifile.New(nebifile.Origin{
		SpecName:    repoName,
		VersionName: tag,
		VersionID:   fmt.Sprintf("%d", resp.VersionNumber),
		ServerURL:   cfg.ServerURL,
		PulledAt:    time.Now(),
	})

	if err := nebifile.Write(absDir, nf); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to update .nebi metadata: %v\n", err)
	}

	// Update local index
	idxLayers := map[string]string{
		"pixi.toml": pixiTomlDigest,
	}
	if pixiLockDigest != "" {
		idxLayers["pixi.lock"] = pixiLockDigest
	}

	idxStore := localindex.NewStore()
	entry := localindex.Entry{
		SpecName:    repoName,
		VersionName: tag,
		VersionID:   fmt.Sprintf("%d", resp.VersionNumber),
		ServerURL:   cfg.ServerURL,
		Path:        absDir,
		PulledAt:    time.Now(),
		Layers:      idxLayers,
	}
	if err := idxStore.AddEntry(entry); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to update local index: %v\n", err)
	}
}

// runPushDryRun shows what would be pushed without actually pushing.
func runPushDryRun(repoName, tag string, pixiTomlContent, pixiLockContent []byte) {
	absDir, _ := filepath.Abs(".")

	fmt.Printf("Would push %s:%s\n\n", repoName, tag)

	// Check if we have a .nebi.toml file to diff against
	if nebifile.Exists(absDir) {
		nf, err := nebifile.Read(absDir)
		if err == nil {
			// Fetch origin content for comparison
			client := mustGetClient()
			ctx := mustGetAuthContext()

			env, err := findRepoByName(client, ctx, nf.Origin.SpecName)
			if err == nil {
				var versionID int32
				fmt.Sscanf(nf.Origin.VersionID, "%d", &versionID)
				vc, err := drift.FetchVersionContent(ctx, client, env.ID, versionID)
				if err == nil {
					// Show TOML diff
					tomlDiff, err := diff.CompareToml([]byte(vc.PixiToml), pixiTomlContent)
					if err == nil && tomlDiff.HasChanges() {
						sourceLabel := fmt.Sprintf("origin (%s:%s)", nf.Origin.SpecName, nf.Origin.VersionName)
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
		fmt.Println("(No .nebi.toml metadata found - cannot show diff against origin)")
		fmt.Printf("\nFiles to push:\n")
		fmt.Printf("  pixi.toml: %d bytes\n", len(pixiTomlContent))
		if len(pixiLockContent) > 0 {
			fmt.Printf("  pixi.lock: %d bytes\n", len(pixiLockContent))
		}
	}

	fmt.Println("\nRun without --dry-run to push.")
}

// parseRepoRef parses a reference in the format repo:tag or repo@digest.
// Returns (repo, tag, error) for tag references.
// Returns (repo, "", error) for digest references (digest is in tag field with @ prefix).
func parseRepoRef(ref string) (repo string, tag string, err error) {
	// Check for digest reference first (repo@sha256:...)
	if idx := strings.Index(ref, "@"); idx != -1 {
		return ref[:idx], ref[idx:], nil // Return @sha256:... as the "tag"
	}

	// Check for tag reference (repo:tag)
	if idx := strings.LastIndex(ref, ":"); idx != -1 {
		return ref[:idx], ref[idx+1:], nil
	}

	// No tag or digest specified
	return ref, "", nil
}

// showPushDriftWarning checks if the local repo has been modified and shows
// an informational note about what's being pushed relative to the origin.
func showPushDriftWarning(dir, repoName, tag string, pixiTomlContent []byte) {
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

	if ws.Overall == drift.StatusModified {
		fmt.Printf("Note: This repo was originally pulled from %s:%s\n",
			nf.Origin.SpecName, nf.Origin.VersionName)
		for _, f := range ws.Files {
			if f.Status == drift.StatusModified {
				fmt.Printf("  - %s modified locally\n", f.Filename)
			}
		}
		fmt.Println()
	}

	// Warn if pushing to the same tag as origin (potential overwrite)
	if tag == nf.Origin.VersionName && repoName == nf.Origin.SpecName && ws.Overall == drift.StatusModified {
		fmt.Fprintf(os.Stderr, "Warning: Pushing modified content back to the same tag %q\n", tag)
		fmt.Fprintf(os.Stderr, "  This will overwrite the existing version on the server.\n")
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
