package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/localstore"
	"github.com/spf13/cobra"
)

var pullServer string
var pullOutput string
var pullGlobal string
var pullForce bool

var pullCmd = &cobra.Command{
	Use:   "pull <workspace>[:<tag>]",
	Short: "Pull workspace spec files from a server",
	Long: `Pull pixi.toml and pixi.lock from a nebi server.

If no tag is specified, the latest version is pulled.

Use --global <name> to pull into a global workspace managed by nebi,
instead of writing to the current directory.

Examples:
  nebi pull myworkspace:v1.0 -s work
  nebi pull myworkspace -s work -o ./my-project
  nebi pull myworkspace:v2.0 --global data-science -s work`,
	Args: cobra.ExactArgs(1),
	RunE: runPull,
}

func init() {
	pullCmd.Flags().StringVarP(&pullServer, "server", "s", "", "Server name or URL (uses default if not set)")
	pullCmd.Flags().StringVarP(&pullOutput, "output", "o", ".", "Output directory")
	pullCmd.Flags().StringVar(&pullGlobal, "global", "", "Save as a global workspace with this name")
	pullCmd.Flags().BoolVar(&pullForce, "force", false, "Overwrite existing global workspace")
}

func runPull(cmd *cobra.Command, args []string) error {
	envName, tag := parseEnvRef(args[0])

	server, err := resolveServerFlag(pullServer)
	if err != nil {
		return err
	}

	client, err := getAuthenticatedClient(server)
	if err != nil {
		return err
	}

	ctx := context.Background()

	// Find environment by name
	env, err := findEnvByName(client, ctx, envName)
	if err != nil {
		return err
	}

	// Resolve version number
	var versionNumber int32

	if tag != "" {
		// Look up tag
		tags, err := client.GetEnvironmentTags(ctx, env.ID)
		if err != nil {
			return fmt.Errorf("failed to get tags: %w", err)
		}
		found := false
		for _, t := range tags {
			if t.Tag == tag {
				versionNumber = int32(t.VersionNumber)
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("tag %q not found for environment %q", tag, envName)
		}
	} else {
		// Use latest version (highest version number)
		versions, err := client.GetEnvironmentVersions(ctx, env.ID)
		if err != nil {
			return fmt.Errorf("failed to get versions: %w", err)
		}
		if len(versions) == 0 {
			return fmt.Errorf("environment %q has no versions", envName)
		}
		latest := versions[0]
		for _, v := range versions {
			if v.VersionNumber > latest.VersionNumber {
				latest = v
			}
		}
		versionNumber = latest.VersionNumber
	}

	// Download spec files
	pixiToml, err := client.GetVersionPixiToml(ctx, env.ID, versionNumber)
	if err != nil {
		return fmt.Errorf("failed to get pixi.toml: %w", err)
	}

	pixiLock, err := client.GetVersionPixiLock(ctx, env.ID, versionNumber)
	if err != nil {
		return fmt.Errorf("failed to get pixi.lock: %w", err)
	}

	// Determine output directory
	outputDir := pullOutput
	if pullGlobal != "" {
		if err := validateWorkspaceName(pullGlobal); err != nil {
			return err
		}
		outputDir, err = setupGlobalWorkspace(pullGlobal, pullForce)
		if err != nil {
			return err
		}
	}

	// Create output directory if needed
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Write files
	if err := os.WriteFile(filepath.Join(outputDir, "pixi.toml"), []byte(pixiToml), 0644); err != nil {
		return fmt.Errorf("failed to write pixi.toml: %w", err)
	}

	if pixiLock != "" {
		if err := os.WriteFile(filepath.Join(outputDir, "pixi.lock"), []byte(pixiLock), 0644); err != nil {
			return fmt.Errorf("failed to write pixi.lock: %w", err)
		}
	}

	absOutput, _ := filepath.Abs(outputDir)

	refStr := envName
	if tag != "" {
		refStr = envName + ":" + tag
	}

	if pullGlobal != "" {
		fmt.Fprintf(os.Stderr, "Pulled %s (version %d) -> global workspace %q (%s)\n", refStr, versionNumber, pullGlobal, absOutput)
	} else {
		fmt.Fprintf(os.Stderr, "Pulled %s (version %d) -> %s\n", refStr, versionNumber, absOutput)
	}
	return nil
}

// setupGlobalWorkspace creates a global workspace directory and registers it in the index.
func setupGlobalWorkspace(name string, force bool) (string, error) {
	store, err := localstore.NewStore()
	if err != nil {
		return "", err
	}

	idx, err := store.LoadIndex()
	if err != nil {
		return "", err
	}

	// Check if a global workspace with this name already exists
	existing := findGlobalWorkspaceByName(idx, name)
	if existing != nil {
		if !force {
			return "", fmt.Errorf("global workspace %q already exists; use --force to overwrite", name)
		}
		// Reuse existing directory
		return existing.Path, nil
	}

	// Create new global workspace
	id := uuid.New().String()
	envDir := store.GlobalEnvDir(id)

	idx.Workspaces[envDir] = &localstore.Workspace{
		ID:     id,
		Name:   name,
		Path:   envDir,
		Global: true,
	}

	if err := store.SaveIndex(idx); err != nil {
		return "", fmt.Errorf("saving index: %w", err)
	}

	return envDir, nil
}
