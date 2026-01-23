package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aktech/darb/internal/drift"
	"github.com/aktech/darb/internal/localindex"
	"github.com/aktech/darb/internal/nebifile"
	"github.com/spf13/cobra"
)

var (
	pullOutput  string
	pullGlobal  bool
	pullForce   bool
	pullYes     bool
	pullName    string
	pullInstall bool
)

var pullCmd = &cobra.Command{
	Use:   "pull <workspace>[:<tag>]",
	Short: "Pull workspace from server",
	Long: `Pull a workspace's pixi.toml and pixi.lock from the server.

Supports Docker-style references:
  - workspace:tag    - Pull specific tag
  - workspace        - Pull latest version
  - workspace@digest - Pull by digest (immutable)

Modes:
  - Directory pull (default): Writes to current directory or -o path
  - Global pull (--global): Writes to ~/.local/share/nebi/workspaces/<uuid>/<tag>/
    with duplicate prevention (use --force to overwrite)

Examples:
  # Pull latest version to current directory
  nebi pull myworkspace

  # Pull specific tag
  nebi pull myworkspace:v1.0.0

  # Pull to specific directory
  nebi pull myworkspace:v1.0.0 -o ./my-project

  # Pull globally (single copy, shell-accessible)
  nebi pull --global myworkspace:v1.0.0

  # Pull globally with an alias
  nebi pull --global myworkspace:v1.0.0 --name ds-stable

  # Force re-pull of global workspace
  nebi pull --global myworkspace:v1.0.0 --force

  # Pull and install immediately
  nebi pull myworkspace:v1.0.0 --install
  nebi pull -gi myworkspace:v1.0.0`,
	Args: cobra.ExactArgs(1),
	Run:  runPull,
}

func init() {
	pullCmd.Flags().StringVarP(&pullOutput, "output", "o", ".", "Output directory (for directory pulls)")
	pullCmd.Flags().BoolVarP(&pullGlobal, "global", "g", false, "Pull to global storage (~/.local/share/nebi/workspaces/)")
	pullCmd.Flags().BoolVar(&pullForce, "force", false, "Force re-pull (overwrite existing)")
	pullCmd.Flags().BoolVar(&pullYes, "yes", false, "Non-interactive mode (skip confirmations)")
	pullCmd.Flags().StringVar(&pullName, "name", "", "Assign an alias to this global workspace (requires --global)")
	pullCmd.Flags().BoolVarP(&pullInstall, "install", "i", false, "Run pixi install after pulling (uses --frozen)")
}

func runPull(cmd *cobra.Command, args []string) {
	// Parse workspace:tag or workspace@digest format
	workspaceName, tagOrDigest, err := parseWorkspaceRef(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Determine if it's a digest reference
	isDigest := strings.HasPrefix(tagOrDigest, "@")
	tag := ""
	digest := ""
	if isDigest {
		digest = tagOrDigest[1:] // Remove @ prefix
	} else {
		tag = tagOrDigest
	}

	// Validate --name requires --global
	if pullName != "" && !pullGlobal {
		fmt.Fprintf(os.Stderr, "Error: --name requires --global flag\n")
		os.Exit(1)
	}

	client := mustGetClient()
	ctx := mustGetAuthContext()

	// Load CLI config for server URL
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Find workspace by name
	env, err := findWorkspaceByName(client, ctx, workspaceName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var versionNumber int32
	var manifestDigest string

	if tag != "" || digest != "" {
		// Find the publication matching the tag or digest
		pubs, err := client.GetEnvironmentPublications(ctx, env.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to get publications: %v\n", err)
			os.Exit(1)
		}

		found := false
		for _, pub := range pubs {
			if (tag != "" && pub.Tag == tag) || (digest != "" && pub.Digest == digest) {
				versionNumber = int32(pub.VersionNumber)
				manifestDigest = pub.Digest
				found = true
				break
			}
		}

		if !found && tag != "" {
			fmt.Fprintf(os.Stderr, "Error: Tag %q not found for workspace %q\n", tag, workspaceName)
			os.Exit(1)
		}
		if !found && digest != "" {
			fmt.Fprintf(os.Stderr, "Error: Digest %q not found for workspace %q\n", digest, workspaceName)
			os.Exit(1)
		}
	} else {
		// No tag/digest specified, get the latest version
		versions, err := client.GetEnvironmentVersions(ctx, env.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to get versions: %v\n", err)
			os.Exit(1)
		}

		if len(versions) == 0 {
			fmt.Fprintf(os.Stderr, "Error: Workspace %q has no versions\n", workspaceName)
			os.Exit(1)
		}

		// Use the latest version (highest version number)
		latestVersion := versions[0]
		for _, v := range versions {
			if v.VersionNumber > latestVersion.VersionNumber {
				latestVersion = v
			}
		}
		versionNumber = latestVersion.VersionNumber
		tag = "latest"
	}

	// Initialize local index
	idxStore := localindex.NewStore()

	// Determine output directory
	var outputDir string
	if pullGlobal {
		outputDir, err = handleGlobalPull(idxStore, env.ID, workspaceName, tag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	} else {
		outputDir, err = handleDirectoryPull(idxStore, workspaceName, tag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}

	// Check if already up to date (skip fetch if content matches)
	if !pullForce {
		absCheck, err := filepath.Abs(outputDir)
		if err == nil {
			if skip := checkAlreadyUpToDate(absCheck, workspaceName, tag, manifestDigest); skip {
				return
			}
		}
	}

	// Get pixi.toml
	pixiToml, err := client.GetVersionPixiToml(ctx, env.ID, versionNumber)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to get pixi.toml: %v\n", err)
		os.Exit(1)
	}

	// Get pixi.lock
	pixiLock, err := client.GetVersionPixiLock(ctx, env.ID, versionNumber)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to get pixi.lock: %v\n", err)
		os.Exit(1)
	}

	// Create output directory if needed
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to create output directory: %v\n", err)
		os.Exit(1)
	}

	// Write pixi.toml
	pixiTomlBytes := []byte(pixiToml)
	pixiTomlPath := filepath.Join(outputDir, "pixi.toml")
	if err := os.WriteFile(pixiTomlPath, pixiTomlBytes, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to write pixi.toml: %v\n", err)
		os.Exit(1)
	}

	// Write pixi.lock
	pixiLockBytes := []byte(pixiLock)
	pixiLockPath := filepath.Join(outputDir, "pixi.lock")
	if err := os.WriteFile(pixiLockPath, pixiLockBytes, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to write pixi.lock: %v\n", err)
		os.Exit(1)
	}

	// Compute layer digests
	pixiTomlDigest := nebifile.ComputeDigest(pixiTomlBytes)
	pixiLockDigest := nebifile.ComputeDigest(pixiLockBytes)

	// Write .nebi metadata file
	nf := nebifile.NewFromPull(
		workspaceName, tag, "", cfg.ServerURL,
		versionNumber, manifestDigest,
		pixiTomlDigest, int64(len(pixiTomlBytes)),
		pixiLockDigest, int64(len(pixiLockBytes)),
	)
	if err := nebifile.Write(outputDir, nf); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to write .nebi metadata: %v\n", err)
		os.Exit(1)
	}

	// Resolve absolute path for index
	absOutputDir, err := filepath.Abs(outputDir)
	if err != nil {
		absOutputDir = outputDir
	}

	// Update local index
	entry := localindex.WorkspaceEntry{
		Workspace:       workspaceName,
		Tag:             tag,
		ServerURL:       cfg.ServerURL,
		ServerVersionID: versionNumber,
		Path:            absOutputDir,
		IsGlobal:        pullGlobal,
		PulledAt:        time.Now(),
		ManifestDigest:  manifestDigest,
		Layers: map[string]string{
			"pixi.toml": pixiTomlDigest,
			"pixi.lock": pixiLockDigest,
		},
	}
	if err := idxStore.AddEntry(entry); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to update local index: %v\n", err)
	}

	// Handle alias (--name flag)
	if pullName != "" {
		alias := localindex.Alias{UUID: env.ID, Tag: tag}
		if err := idxStore.SetAlias(pullName, alias); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to set alias: %v\n", err)
		} else {
			fmt.Printf("Alias: %s → %s:%s\n", pullName, workspaceName, tag)
		}
	}

	// Print summary
	refStr := workspaceName
	if tag != "" && tag != "latest" {
		refStr = workspaceName + ":" + tag
	} else if digest != "" {
		refStr = workspaceName + "@" + digest
	}
	fmt.Printf("Pulled %s (version %d) → %s\n", refStr, versionNumber, absOutputDir)

	// Run pixi install if requested
	if pullInstall {
		fmt.Println()
		if err := runPixiInstall(absOutputDir); err != nil {
			fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
			fmt.Fprintln(os.Stderr, "The workspace files were pulled successfully. You can retry with:")
			fmt.Fprintf(os.Stderr, "  cd %s && pixi install --frozen\n", absOutputDir)
			os.Exit(1)
		}
	} else {
		fmt.Println("\nTo install the environment, run:")
		fmt.Printf("  cd %s && pixi install\n", absOutputDir)
	}
}

// handleGlobalPull handles the --global pull workflow.
// Returns the output directory path or an error.
func handleGlobalPull(store *localindex.Store, envID, workspace, tag string) (string, error) {
	// Compute global path using the workspace's server-assigned UUID
	outputDir := store.GlobalWorkspacePath(envID, tag)

	// Check if already exists
	existing, err := store.FindGlobal(workspace, tag)
	if err != nil {
		return "", fmt.Errorf("failed to check existing global workspace: %v", err)
	}

	if existing != nil && !pullForce {
		return "", fmt.Errorf("%s:%s already exists globally.\n  Location: %s\n  Pulled: %s\nUse --force to re-pull and overwrite",
			workspace, tag, existing.Path, existing.PulledAt.Format(time.RFC3339))
	}

	return outputDir, nil
}

// checkAlreadyUpToDate checks if the local workspace already matches what
// would be pulled. Returns true if the pull should be skipped.
func checkAlreadyUpToDate(dir, workspace, tag, remoteDigest string) bool {
	if !nebifile.Exists(dir) {
		return false
	}

	nf, err := nebifile.Read(dir)
	if err != nil {
		return false
	}

	// Different workspace or tag — not a match
	if nf.Origin.Workspace != workspace || nf.Origin.Tag != tag {
		return false
	}

	// If we have a remote digest and it differs, the tag has been updated remotely
	if remoteDigest != "" && nf.Origin.ManifestDigest != "" && nf.Origin.ManifestDigest != remoteDigest {
		return false
	}

	// Check if local files match the stored layer digests
	ws := drift.CheckWithNebiFile(dir, nf)
	if ws.Overall == drift.StatusClean {
		refStr := workspace
		if tag != "" && tag != "latest" {
			refStr = workspace + ":" + tag
		}
		fmt.Printf("Already up to date (%s)\n", refStr)
		return true
	}

	// Local files are modified — prompt user
	if !pullYes {
		fmt.Fprintf(os.Stderr, "Local files have been modified since last pull.\n")
		fmt.Fprintf(os.Stderr, "Re-pull to discard local changes? [y/N]: ")

		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("Pull skipped (local modifications preserved)")
			return true
		}
	}

	return false
}

// handleDirectoryPull handles the default directory pull workflow.
// Returns the output directory path or an error.
func handleDirectoryPull(store *localindex.Store, workspace, tag string) (string, error) {
	outputDir := pullOutput

	// Resolve absolute path for comparison
	absDir, err := filepath.Abs(outputDir)
	if err != nil {
		return outputDir, nil
	}

	// Check if this directory already has a different workspace:tag
	existing, err := store.FindByPath(absDir)
	if err != nil {
		return outputDir, nil // Non-fatal, proceed with pull
	}

	if existing != nil && existing.Workspace == workspace && existing.Tag == tag {
		// Same workspace:tag to same directory - re-pull (overwrite), no prompt needed
		return outputDir, nil
	}

	if existing != nil && (existing.Workspace != workspace || existing.Tag != tag) && !pullForce {
		// Different workspace:tag to same directory - prompt for confirmation
		if !pullYes {
			fmt.Fprintf(os.Stderr, "Warning: %s already contains %s:%s\n", absDir, existing.Workspace, existing.Tag)
			fmt.Fprintf(os.Stderr, "Overwrite with %s:%s? [y/N]: ", workspace, tag)

			reader := bufio.NewReader(os.Stdin)
			response, _ := reader.ReadString('\n')
			response = strings.TrimSpace(strings.ToLower(response))
			if response != "y" && response != "yes" {
				return "", fmt.Errorf("pull cancelled")
			}
		}
	}

	return outputDir, nil
}
