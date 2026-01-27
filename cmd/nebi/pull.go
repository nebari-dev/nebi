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
	Use:   "pull <repo>[:<tag>]",
	Short: "Pull repo from server",
	Long: `Pull a repo's pixi.toml and pixi.lock from the server.

Supports Docker-style references:
  - repo:tag    - Pull specific tag
  - repo        - Pull latest version
  - repo@digest - Pull by digest (immutable)

Modes:
  - Directory pull (default): Writes to current directory or -o path
  - Global pull (--global): Writes to ~/.local/share/nebi/repos/<uuid>/<tag>/
    with duplicate prevention (use --force to overwrite)

Examples:
  # Pull latest version to current directory
  nebi pull myrepo

  # Pull specific tag
  nebi pull myrepo:v1.0.0

  # Pull to specific directory
  nebi pull myrepo:v1.0.0 -o ./my-project

  # Pull globally (single copy, shell-accessible)
  nebi pull --global myrepo:v1.0.0

  # Pull globally with an alias
  nebi pull --global myrepo:v1.0.0 --name ds-stable

  # Force re-pull of global repo
  nebi pull --global myrepo:v1.0.0 --force

  # Pull and install immediately
  nebi pull myrepo:v1.0.0 --install
  nebi pull -gi myrepo:v1.0.0`,
	Args: cobra.ExactArgs(1),
	Run:  runPull,
}

func init() {
	pullCmd.Flags().StringVarP(&pullOutput, "output", "o", ".", "Output directory (for directory pulls)")
	pullCmd.Flags().BoolVarP(&pullGlobal, "global", "g", false, "Pull to global storage (~/.local/share/nebi/repos/)")
	pullCmd.Flags().BoolVar(&pullForce, "force", false, "Force re-pull (overwrite existing)")
	pullCmd.Flags().BoolVar(&pullYes, "yes", false, "Non-interactive mode (skip confirmations)")
	pullCmd.Flags().StringVar(&pullName, "name", "", "Assign an alias to this global repo (requires --global)")
	pullCmd.Flags().BoolVarP(&pullInstall, "install", "i", false, "Run pixi install after pulling (uses --frozen)")
}

func runPull(cmd *cobra.Command, args []string) {
	// Parse repo:tag or repo@digest format
	repoName, tagOrDigest, err := parseRepoRef(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		osExit(1)
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
		osExit(1)
	}

	client := mustGetClient()
	ctx := mustGetAuthContext()

	// Load CLI config for server URL
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		osExit(1)
	}

	// Find repo by name
	env, err := findRepoByName(client, ctx, repoName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		osExit(1)
	}

	var versionNumber int32
	var manifestDigest string

	if tag != "" || digest != "" {
		found := false

		// First, try resolving from server-side tags (created by push)
		if tag != "" {
			tags, err := client.GetEnvironmentTags(ctx, env.ID)
			if err == nil {
				for _, t := range tags {
					if t.Tag == tag {
						versionNumber = int32(t.VersionNumber)
						found = true
						break
					}
				}
			}
		}

		// Fallback: check publications (backward compat with pre-push/publish-split data)
		if !found {
			pubs, err := client.GetEnvironmentPublications(ctx, env.ID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: Failed to get publications: %v\n", err)
				osExit(1)
			}

			for _, pub := range pubs {
				if (tag != "" && pub.Tag == tag) || (digest != "" && pub.Digest == digest) {
					versionNumber = int32(pub.VersionNumber)
					manifestDigest = pub.Digest
					found = true
					break
				}
			}
		}

		if !found && tag != "" {
			fmt.Fprintf(os.Stderr, "Error: Tag %q not found for repo %q\n", tag, repoName)
			osExit(1)
		}
		if !found && digest != "" {
			fmt.Fprintf(os.Stderr, "Error: Digest %q not found for repo %q\n", digest, repoName)
			osExit(1)
		}
	} else {
		// No tag/digest specified, get the latest version
		versions, err := client.GetEnvironmentVersions(ctx, env.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to get versions: %v\n", err)
			osExit(1)
		}

		if len(versions) == 0 {
			fmt.Fprintf(os.Stderr, "Error: Repo %q has no versions\n", repoName)
			osExit(1)
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
		outputDir, err = handleGlobalPull(idxStore, env.ID, repoName, tag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			osExit(1)
		}
	} else {
		outputDir, err = handleDirectoryPull(idxStore, repoName, tag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			osExit(1)
		}
	}

	// Check if already up to date (skip fetch if content matches)
	if !pullForce {
		absCheck, err := filepath.Abs(outputDir)
		if err == nil {
			if skip := checkAlreadyUpToDate(absCheck, repoName, tag, manifestDigest); skip {
				return
			}
		}
	}

	// Get pixi.toml
	pixiToml, err := client.GetVersionPixiToml(ctx, env.ID, versionNumber)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to get pixi.toml: %v\n", err)
		osExit(1)
	}

	// Get pixi.lock
	pixiLock, err := client.GetVersionPixiLock(ctx, env.ID, versionNumber)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to get pixi.lock: %v\n", err)
		osExit(1)
	}

	// Create output directory if needed
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to create output directory: %v\n", err)
		osExit(1)
	}

	// Write pixi.toml
	pixiTomlBytes := []byte(pixiToml)
	pixiTomlPath := filepath.Join(outputDir, "pixi.toml")
	if err := os.WriteFile(pixiTomlPath, pixiTomlBytes, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to write pixi.toml: %v\n", err)
		osExit(1)
	}

	// Write pixi.lock
	pixiLockBytes := []byte(pixiLock)
	pixiLockPath := filepath.Join(outputDir, "pixi.lock")
	if err := os.WriteFile(pixiLockPath, pixiLockBytes, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to write pixi.lock: %v\n", err)
		osExit(1)
	}

	// Compute layer digests
	pixiTomlDigest := nebifile.ComputeDigest(pixiTomlBytes)
	pixiLockDigest := nebifile.ComputeDigest(pixiLockBytes)

	// Resolve absolute path for index
	absOutputDir, err := filepath.Abs(outputDir)
	if err != nil {
		absOutputDir = outputDir
	}

	// Write .nebi.toml metadata file
	nf := nebifile.NewFromPull(
		repoName, tag, cfg.ServerURL,
		env.ID, fmt.Sprintf("%d", versionNumber), "", // specID, versionID, serverID
	)
	if err := nebifile.Write(outputDir, nf); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to write .nebi.toml metadata: %v\n", err)
		osExit(1)
	}

	// Update local index
	entry := localindex.Entry{
		SpecName:    repoName,
		SpecID:      env.ID,
		VersionName: tag,
		VersionID:   fmt.Sprintf("%d", versionNumber),
		ServerURL:   cfg.ServerURL,
		Path:        absOutputDir,
		PulledAt:    time.Now(),
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
			fmt.Printf("Alias: %s → %s:%s\n", pullName, repoName, tag)
		}
	}

	// Print summary
	refStr := repoName
	if tag != "" && tag != "latest" {
		refStr = repoName + ":" + tag
	} else if digest != "" {
		refStr = repoName + "@" + digest
	}
	fmt.Printf("Pulled %s (version %d) → %s\n", refStr, versionNumber, absOutputDir)

	// Run pixi install if requested
	if pullInstall {
		fmt.Println()
		if err := runPixiInstall(absOutputDir); err != nil {
			fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
			fmt.Fprintln(os.Stderr, "The repo files were pulled successfully. You can retry with:")
			fmt.Fprintf(os.Stderr, "  cd %s && pixi install --frozen\n", absOutputDir)
			osExit(1)
		}
	} else {
		fmt.Println("\nTo install the environment, run:")
		fmt.Printf("  cd %s && pixi install\n", absOutputDir)
	}
}

// handleGlobalPull handles the --global pull workflow.
// Returns the output directory path or an error.
func handleGlobalPull(store *localindex.Store, envID, repo, tag string) (string, error) {
	// Compute global path using the repo's server-assigned UUID
	outputDir := store.GlobalRepoPath(envID, tag)

	// Check if already exists
	existing, err := store.FindGlobal(repo, tag)
	if err != nil {
		return "", fmt.Errorf("failed to check existing global repo: %v", err)
	}

	if existing != nil && !pullForce {
		return "", fmt.Errorf("%s:%s already exists globally.\n  Location: %s\n  Pulled: %s\nUse --force to re-pull and overwrite",
			repo, tag, existing.Path, existing.PulledAt.Format(time.RFC3339))
	}

	return outputDir, nil
}

// checkAlreadyUpToDate checks if the local repo already matches what
// would be pulled. Returns true if the pull should be skipped.
func checkAlreadyUpToDate(dir, repo, tag, remoteDigest string) bool {
	if !nebifile.Exists(dir) {
		return false
	}

	nf, err := nebifile.Read(dir)
	if err != nil {
		return false
	}

	// Different repo or tag — not a match
	if nf.Origin.SpecName != repo || nf.Origin.VersionName != tag {
		return false
	}

	// If we have a remote digest and it differs, the tag has been updated remotely
	if remoteDigest != "" && nf.Origin.VersionID != "" && nf.Origin.VersionID != remoteDigest {
		return false
	}

	// Check if local files match the stored layer digests
	ws := drift.CheckWithNebiFile(dir, nf)
	if ws.Overall == drift.StatusClean {
		refStr := repo
		if tag != "" && tag != "latest" {
			refStr = repo + ":" + tag
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
func handleDirectoryPull(store *localindex.Store, repo, tag string) (string, error) {
	outputDir := pullOutput

	// Resolve absolute path for comparison
	absDir, err := filepath.Abs(outputDir)
	if err != nil {
		return outputDir, nil
	}

	// Check if this directory already has a different repo:tag
	existing, err := store.FindByPath(absDir)
	if err != nil {
		return outputDir, nil // Non-fatal, proceed with pull
	}

	if existing != nil && existing.SpecName == repo && existing.VersionName == tag {
		// Same repo:tag to same directory - re-pull (overwrite), no prompt needed
		return outputDir, nil
	}

	if existing != nil && (existing.SpecName != repo || existing.VersionName != tag) && !pullForce {
		// Different repo:tag to same directory - prompt for confirmation
		if !pullYes {
			fmt.Fprintf(os.Stderr, "Warning: %s already contains %s:%s\n", absDir, existing.SpecName, existing.VersionName)
			fmt.Fprintf(os.Stderr, "Overwrite with %s:%s? [y/N]: ", repo, tag)

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
