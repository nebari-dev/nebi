package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nebari-dev/nebi/internal/store"
	"github.com/spf13/cobra"
)

var pullOutput string
var pullForce bool

var pullCmd = &cobra.Command{
	Use:   "pull [<workspace>[:<tag>]]",
	Short: "Pull workspace spec files from a nebi server",
	Long: `Pull pixi.toml and pixi.lock from a nebi server.

If no argument is given, the workspace and tag from the last push/pull
origin are used.

If no tag is specified, the latest version is pulled.

The local workspace name is derived from the [workspace] name field
in the pulled pixi.toml, not from the server workspace name.

Use --force to skip the overwrite confirmation prompt.

Examples:
  nebi pull myworkspace:v1.0
  nebi pull                                # re-pull from origin
  nebi pull myworkspace -o ./my-project`,
	Args: cobra.RangeArgs(0, 1),
	RunE: runPull,
}

func init() {
	pullCmd.Flags().StringVarP(&pullOutput, "output", "o", ".", "Output directory")
	pullCmd.Flags().BoolVar(&pullForce, "force", false, "Overwrite existing files without prompting")
}

func runPull(cmd *cobra.Command, args []string) error {
	var wsName, tag string
	if len(args) == 1 {
		wsName, tag = parseWsRef(args[0])
	} else {
		origin, err := lookupOrigin()
		if err != nil {
			return err
		}
		if origin == nil {
			return fmt.Errorf("no origin set; specify a workspace: nebi pull <workspace>[:<tag>]")
		}
		wsName = origin.OriginName
		tag = origin.OriginTag
		fmt.Fprintf(os.Stderr, "Using origin %s:%s\n", wsName, tag)
	}

	client, err := getAuthenticatedClient()
	if err != nil {
		return err
	}

	ctx := context.Background()

	ws, err := findWsByName(client, ctx, wsName)
	if err != nil {
		return err
	}

	var versionNumber int32

	if tag != "" {
		tags, err := client.GetWorkspaceTags(ctx, ws.ID)
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
			return fmt.Errorf("tag %q not found for workspace %q", tag, wsName)
		}
	} else {
		versions, err := client.GetWorkspaceVersions(ctx, ws.ID)
		if err != nil {
			return fmt.Errorf("failed to get versions: %w", err)
		}
		if len(versions) == 0 {
			return fmt.Errorf("workspace %q has no versions", wsName)
		}
		latest := versions[0]
		for _, v := range versions {
			if v.VersionNumber > latest.VersionNumber {
				latest = v
			}
		}
		versionNumber = latest.VersionNumber

		tags, err := client.GetWorkspaceTags(ctx, ws.ID)
		if err == nil {
			var bestTag string
			var bestTime string
			for _, t := range tags {
				if int32(t.VersionNumber) == versionNumber {
					if bestTag == "" || t.CreatedAt > bestTime {
						bestTag = t.Tag
						bestTime = t.CreatedAt
					}
				}
			}
			tag = bestTag
		}
	}

	pixiToml, err := client.GetVersionPixiToml(ctx, ws.ID, versionNumber)
	if err != nil {
		return fmt.Errorf("failed to get pixi.toml: %w", err)
	}

	pixiLock, err := client.GetVersionPixiLock(ctx, ws.ID, versionNumber)
	if err != nil {
		return fmt.Errorf("failed to get pixi.lock: %w", err)
	}

	// Check for upstream changes
	if len(args) == 0 {
		origin, _ := lookupOrigin()
		if origin != nil {
			serverTomlHash, _ := store.TomlContentHash(pixiToml)
			if origin.OriginTomlHash != "" && origin.OriginTomlHash != serverTomlHash {
				fmt.Fprintf(os.Stderr, "Note: %s:%s has changed on server since last sync\n", wsName, tag)
			}
		}
	}

	outputDir := pullOutput
	if !pullForce {
		absDir, _ := filepath.Abs(outputDir)
		existing := filepath.Join(absDir, "pixi.toml")
		if _, statErr := os.Stat(existing); statErr == nil {
			if !confirmOverwrite(absDir) {
				fmt.Fprintln(os.Stderr, "Aborted.")
				return nil
			}
		}
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	if err := os.WriteFile(filepath.Join(outputDir, "pixi.toml"), []byte(pixiToml), 0644); err != nil {
		return fmt.Errorf("failed to write pixi.toml: %w", err)
	}

	if pixiLock != "" {
		if err := os.WriteFile(filepath.Join(outputDir, "pixi.lock"), []byte(pixiLock), 0644); err != nil {
			return fmt.Errorf("failed to write pixi.lock: %w", err)
		}
	}

	absOutput, _ := filepath.Abs(outputDir)

	// Auto-track the workspace (name will be read from pulled pixi.toml)
	if err := ensureInit(outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to auto-track workspace: %v\n", err)
	}

	refStr := wsName
	if tag != "" {
		refStr = wsName + ":" + tag
	}

	fmt.Fprintf(os.Stderr, "Pulled %s (version %d) -> %s\n", refStr, versionNumber, absOutput)

	if saveErr := saveOrigin(wsName, tag, "pull", pixiToml, pixiLock); saveErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save origin: %v\n", saveErr)
	}

	return nil
}

func confirmOverwrite(dir string) bool {
	fmt.Fprintf(os.Stderr, "pixi.toml already exists in %s. Overwrite? [y/N] ", dir)
	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes"
}
