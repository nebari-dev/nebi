package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nebari-dev/nebi/internal/oci"
	"github.com/spf13/cobra"
)

var importOutput string
var importGlobal string
var importForce bool

var importCmd = &cobra.Command{
	Use:   "import <oci-reference>",
	Short: "Import a workspace from a public OCI registry",
	Long: `Import pixi.toml and pixi.lock directly from a public OCI registry.

The OCI reference should be in the format: registry/repository:tag
(e.g., quay.io/nebari/my-env:v1)

This works entirely locally — no server connection needed.

Examples:
  nebi import quay.io/nebari/my-env:v1
  nebi import ghcr.io/myorg/data-science:latest -o ./my-project
  nebi import quay.io/nebari/my-env:v1 --global data-science`,
	Args: cobra.ExactArgs(1),
	RunE: runImport,
}

func init() {
	importCmd.Flags().StringVarP(&importOutput, "output", "o", ".", "Output directory")
	importCmd.Flags().StringVar(&importGlobal, "global", "", "Save as a global workspace with this name")
	importCmd.Flags().BoolVar(&importForce, "force", false, "Overwrite existing files without prompting")
}

func runImport(cmd *cobra.Command, args []string) error {
	repoRef, tag := parseWsRef(args[0])
	if tag == "" {
		return fmt.Errorf("tag is required; use format registry/repository:tag (e.g., quay.io/nebari/my-env:v1)")
	}

	host, _ := oci.ParseRegistryURL(repoRef)

	ctx := context.Background()
	result, err := oci.PullEnvironment(ctx, repoRef, tag, oci.BrowseOptions{
		RegistryHost: host,
	})
	if err != nil {
		return fmt.Errorf("failed to pull from registry: %w", err)
	}

	outputDir := importOutput
	if importGlobal != "" {
		if err := validateWorkspaceName(importGlobal); err != nil {
			return err
		}
		outputDir, err = setupGlobalWorkspace(importGlobal, importForce)
		if err != nil {
			return err
		}
	} else {
		if !importForce {
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

	if err := os.WriteFile(filepath.Join(outputDir, "pixi.toml"), []byte(result.PixiToml), 0644); err != nil {
		return fmt.Errorf("failed to write pixi.toml: %w", err)
	}

	if result.PixiLock != "" {
		if err := os.WriteFile(filepath.Join(outputDir, "pixi.lock"), []byte(result.PixiLock), 0644); err != nil {
			return fmt.Errorf("failed to write pixi.lock: %w", err)
		}
	}

	absOutput, _ := filepath.Abs(outputDir)

	if importGlobal == "" {
		if err := ensureInit(outputDir); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to auto-track workspace: %v\n", err)
		}
	}

	ref := repoRef + ":" + tag
	if importGlobal != "" {
		fmt.Fprintf(os.Stderr, "Imported %s -> global workspace %q (%s)\n", ref, importGlobal, absOutput)
	} else {
		fmt.Fprintf(os.Stderr, "Imported %s -> %s\n", ref, absOutput)
	}

	return nil
}
