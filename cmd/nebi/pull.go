package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aktech/darb/internal/cliclient"
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
	pullInstall bool
)

var pullCmd = &cobra.Command{
	Use:   "pull <env>[:<version>]",
	Short: "Pull environment from server",
	Long: `Pull an environment's pixi.toml and pixi.lock from the server.

Supports Docker-style references:
  - env:version  - Pull specific version
  - env          - Pull default version (or latest if no default set)
  - env@digest   - Pull by digest (immutable)

Modes:
  - Directory pull (default): Writes to current directory or -o path
  - Global pull (--global): Writes to ~/.local/share/nebi/envs/<uuid>/<version>/
    with duplicate prevention (use --force to overwrite)

Examples:
  # Pull default version to current directory
  nebi pull myenv

  # Pull specific version
  nebi pull myenv:v1.0.0

  # Pull to specific directory
  nebi pull myenv:v1.0.0 -o ./my-project

  # Pull globally (single copy, shell-accessible)
  nebi pull --global myenv:v1.0.0

  # Force re-pull of global environment
  nebi pull --global myenv:v1.0.0 --force

  # Pull and install immediately
  nebi pull myenv:v1.0.0 --install
  nebi pull -gi myenv:v1.0.0`,
	Args: cobra.ExactArgs(1),
	Run:  runPull,
}

func init() {
	pullCmd.Flags().StringVarP(&pullOutput, "output", "o", ".", "Output directory (for directory pulls)")
	pullCmd.Flags().BoolVarP(&pullGlobal, "global", "g", false, "Pull to global storage (~/.local/share/nebi/envs/)")
	pullCmd.Flags().BoolVar(&pullForce, "force", false, "Force re-pull (overwrite existing)")
	pullCmd.Flags().BoolVar(&pullYes, "yes", false, "Non-interactive mode (skip confirmations)")
	pullCmd.Flags().BoolVarP(&pullInstall, "install", "i", false, "Run pixi install after pulling (uses --frozen)")
}

func runPull(cmd *cobra.Command, args []string) {
	// Parse env:version or env@digest format
	envName, versionOrDigest, err := parseEnvRef(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		osExit(1)
	}

	// Determine if it's a digest reference
	isDigest := strings.HasPrefix(versionOrDigest, "@")
	version := ""
	digest := ""
	if isDigest {
		digest = versionOrDigest[1:] // Remove @ prefix
	} else {
		version = versionOrDigest
	}

	client := mustGetClient()
	ctx := mustGetAuthContext()

	// Load CLI config for server URL
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		osExit(1)
	}

	// Get server info (for server_id)
	serverInfo, err := client.GetServerInfo(ctx)
	if err != nil {
		// Non-fatal - server might not support /info endpoint yet
		serverInfo = &cliclient.ServerInfo{}
	}

	// Find environment by name
	env, err := findEnvByName(client, ctx, envName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		osExit(1)
	}

	var versionNumber int32
	var manifestDigest string

	if version != "" || digest != "" {
		found := false

		// First, try resolving from server-side versions (created by push)
		if version != "" {
			tags, err := client.GetEnvironmentTags(ctx, env.ID)
			if err == nil {
				for _, t := range tags {
					if t.Tag == version {
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
				if (version != "" && pub.Tag == version) || (digest != "" && pub.Digest == digest) {
					versionNumber = int32(pub.VersionNumber)
					manifestDigest = pub.Digest
					found = true
					break
				}
			}
		}

		if !found && version != "" {
			fmt.Fprintf(os.Stderr, "Error: Version %q not found for environment %q\n", version, envName)
			osExit(1)
		}
		if !found && digest != "" {
			fmt.Fprintf(os.Stderr, "Error: Digest %q not found for environment %q\n", digest, envName)
			osExit(1)
		}
	} else {
		// No version/digest specified - check for default version first, then fall back to latest
		if env.DefaultVersionID != nil {
			// Use the default version
			versionNumber = int32(*env.DefaultVersionID)
			version = "default"
		} else {
			// No default set, get the latest version
			versions, err := client.GetEnvironmentVersions(ctx, env.ID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: Failed to get versions: %v\n", err)
				osExit(1)
			}

			if len(versions) == 0 {
				fmt.Fprintf(os.Stderr, "Error: Environment %q has no versions\n", envName)
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
			version = "latest"
		}
	}

	// Initialize local index
	idxStore := localindex.NewStore()

	// Determine output directory
	var outputDir string
	if pullGlobal {
		outputDir, err = handleGlobalPull(idxStore, env.ID, envName, version)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			osExit(1)
		}
	} else {
		outputDir, err = handleDirectoryPull(idxStore, envName, version)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			osExit(1)
		}
	}

	// Check if already up to date (skip fetch if content matches)
	if !pullForce {
		absCheck, err := filepath.Abs(outputDir)
		if err == nil {
			if skip := checkAlreadyUpToDate(absCheck, envName, version, manifestDigest); skip {
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
		envName, version, cfg.ServerURL,
		env.ID, fmt.Sprintf("%d", versionNumber), serverInfo.ServerID,
	)
	if err := nebifile.Write(outputDir, nf); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to write .nebi.toml metadata: %v\n", err)
		osExit(1)
	}

	// Update local index
	entry := localindex.Entry{
		SpecName:    envName,
		SpecID:      env.ID,
		VersionName: version,
		VersionID:   fmt.Sprintf("%d", versionNumber),
		ServerURL:   cfg.ServerURL,
		ServerID:    serverInfo.ServerID,
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

	// Print summary
	refStr := envName
	if version != "" && version != "latest" {
		refStr = envName + ":" + version
	} else if digest != "" {
		refStr = envName + "@" + digest
	}
	fmt.Printf("Pulled %s (version %d) -> %s\n", refStr, versionNumber, absOutputDir)

	// Run pixi install if requested
	if pullInstall {
		fmt.Println()
		if err := runPixiInstall(absOutputDir); err != nil {
			fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
			fmt.Fprintln(os.Stderr, "The environment files were pulled successfully. You can retry with:")
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
func handleGlobalPull(store *localindex.Store, envID, env, version string) (string, error) {
	// Compute global path using the environment's server-assigned UUID
	outputDir := store.GlobalEnvPath(envID, version)

	// Check if already exists
	existing, err := store.FindGlobal(env, version)
	if err != nil {
		return "", fmt.Errorf("failed to check existing global environment: %v", err)
	}

	if existing != nil && !pullForce {
		return "", fmt.Errorf("%s:%s already exists globally.\n  Location: %s\n  Pulled: %s\nUse --force to re-pull and overwrite",
			env, version, existing.Path, existing.PulledAt.Format(time.RFC3339))
	}

	return outputDir, nil
}

// checkAlreadyUpToDate checks if the local environment already matches what
// would be pulled. Returns true if the pull should be skipped.
func checkAlreadyUpToDate(dir, env, version, remoteDigest string) bool {
	if !nebifile.Exists(dir) {
		return false
	}

	nf, err := nebifile.Read(dir)
	if err != nil {
		return false
	}

	// Different environment or version - not a match
	if nf.Origin.SpecName != env || nf.Origin.VersionName != version {
		return false
	}

	// If we have a remote digest and it differs, the version has been updated remotely
	if remoteDigest != "" && nf.Origin.VersionID != "" && nf.Origin.VersionID != remoteDigest {
		return false
	}

	// Check if local files match the stored layer digests
	ws := drift.CheckWithNebiFile(dir, nf)
	if ws.Overall == drift.StatusClean {
		refStr := env
		if version != "" && version != "latest" {
			refStr = env + ":" + version
		}
		fmt.Printf("Already up to date (%s)\n", refStr)
		return true
	}

	// Local files are modified - prompt user
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
func handleDirectoryPull(store *localindex.Store, env, version string) (string, error) {
	outputDir := pullOutput

	// Resolve absolute path for comparison
	absDir, err := filepath.Abs(outputDir)
	if err != nil {
		return outputDir, nil
	}

	// Check if this directory already has a different env:version
	existing, err := store.FindByPath(absDir)
	if err != nil {
		return outputDir, nil // Non-fatal, proceed with pull
	}

	if existing != nil && existing.SpecName == env && existing.VersionName == version {
		// Same env:version to same directory - re-pull (overwrite), no prompt needed
		return outputDir, nil
	}

	if existing != nil && (existing.SpecName != env || existing.VersionName != version) && !pullForce {
		// Different env:version to same directory - prompt for confirmation
		if !pullYes {
			fmt.Fprintf(os.Stderr, "Warning: %s already contains %s:%s\n", absDir, existing.SpecName, existing.VersionName)
			fmt.Fprintf(os.Stderr, "Overwrite with %s:%s? [y/N]: ", env, version)

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
