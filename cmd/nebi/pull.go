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
var pullGlobal string
var pullForce bool

var pullCmd = &cobra.Command{
	Use:   "pull [<workspace>[:<tag>]]",
	Short: "Pull workspace spec files from a nebi server",
	Long: `Pull pixi.toml and pixi.lock from a nebi server.

If no argument is given, the workspace and tag from the last push/pull
origin are used.

If no tag is specified, the latest version is pulled.

Use --global <name> to pull into a global workspace managed by nebi,
instead of writing to the current directory.

Use --force to skip the overwrite confirmation prompt.

Examples:
  nebi pull myworkspace:v1.0
  nebi pull                                # re-pull from origin
  nebi pull myworkspace -o ./my-project
  nebi pull myworkspace:v2.0 --global data-science`,
	Args: cobra.RangeArgs(0, 1),
	RunE: runPull,
}

func init() {
	pullCmd.Flags().StringVarP(&pullOutput, "output", "o", ".", "Output directory")
	pullCmd.Flags().StringVar(&pullGlobal, "global", "", "Save as a global workspace with this name")
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
	if pullGlobal != "" {
		if err := validateWorkspaceName(pullGlobal); err != nil {
			return err
		}
		outputDir, err = setupGlobalWorkspace(pullGlobal, pullForce)
		if err != nil {
			return err
		}
	} else {
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

	if pullGlobal == "" {
		if err := ensureInit(outputDir); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to auto-track workspace: %v\n", err)
		}
	}

	refStr := wsName
	if tag != "" {
		refStr = wsName + ":" + tag
	}

	if pullGlobal != "" {
		fmt.Fprintf(os.Stderr, "Pulled %s (version %d) -> global workspace %q (%s)\n", refStr, versionNumber, pullGlobal, absOutput)
	} else {
		fmt.Fprintf(os.Stderr, "Pulled %s (version %d) -> %s\n", refStr, versionNumber, absOutput)
	}

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

func setupGlobalWorkspace(name string, force bool) (string, error) {
	s, err := store.New()
	if err != nil {
		return "", err
	}
	defer s.Close()

	existing, err := s.FindGlobalWorkspaceByName(name)
	if err != nil {
		return "", err
	}
	if existing != nil {
		if !force {
			if !confirmOverwrite(existing.Path) {
				return "", fmt.Errorf("aborted")
			}
		}
		return existing.Path, nil
	}

	wsDir := s.GlobalWorkspaceDir(name)
	ws := &store.LocalWorkspace{
		Name: name,
		Path: wsDir,
	}
	if err := s.CreateWorkspace(ws); err != nil {
		return "", fmt.Errorf("saving workspace: %w", err)
	}

	return wsDir, nil
}
