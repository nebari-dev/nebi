package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nebari-dev/nebi/internal/oci"
	"github.com/spf13/cobra"
)

var (
	importOutput      string
	importForce       bool
	importConcurrency int
)

var importCmd = &cobra.Command{
	Use:   "import <oci-reference>",
	Short: "Import a workspace from a public OCI registry",
	Long: `Import a Nebi workspace bundle from an OCI registry.

The OCI reference should be in the format: registry/repository:tag
(e.g., quay.io/nebari/my-env:v1)

Restores pixi.toml, pixi.lock, and any bundled asset files to the output
directory. Works entirely locally — no server connection needed.

The local workspace name is derived from the [workspace] name field
in the imported pixi.toml.

Examples:
  nebi import quay.io/nebari/my-env:v1
  nebi import ghcr.io/myorg/data-science:latest -o ./my-project`,
	Args: cobra.ExactArgs(1),
	RunE: runImport,
}

func init() {
	importCmd.Flags().StringVarP(&importOutput, "output", "o", ".", "Output directory")
	importCmd.Flags().BoolVar(&importForce, "force", false, "Overwrite existing files without prompting (only when the bundle contains no asset layers)")
	importCmd.Flags().IntVar(&importConcurrency, "concurrency", 8, "Parallel blob fetch workers")
}

func runImport(cmd *cobra.Command, args []string) error {
	repoRef, tag := parseWsRef(args[0])
	if tag == "" {
		return fmt.Errorf("tag is required; use format registry/repository:tag (e.g., quay.io/nebari/my-env:v1)")
	}

	plainHTTP := strings.HasPrefix(repoRef, "http://")
	if plainHTTP {
		repoRef = strings.TrimPrefix(repoRef, "http://")
	} else {
		repoRef = strings.TrimPrefix(repoRef, "https://")
	}

	ctx := context.Background()
	result, err := oci.PullBundle(ctx, repoRef, tag, oci.PullOptions{
		Mode:        oci.PullModeFull,
		Concurrency: importConcurrency,
		PlainHTTP:   plainHTTP,
	})
	if err != nil {
		return fmt.Errorf("failed to pull from registry: %w", err)
	}

	outputDir := importOutput
	absDir, _ := filepath.Abs(outputDir)

	// Non-empty destination handling. For bundles with asset layers we
	// follow the spec strictly: abort if the directory exists and is not
	// empty, no --force bypass. For legacy zero-asset artifacts we keep
	// the previous UX so existing users aren't broken.
	hasAssets := len(result.Assets) > 0
	if hasAssets {
		if nonEmpty, err := dirIsNonEmpty(absDir); err != nil {
			return err
		} else if nonEmpty {
			return fmt.Errorf("destination %s not empty", absDir)
		}
	} else if !importForce {
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

	if err := os.WriteFile(filepath.Join(outputDir, "pixi.toml"), []byte(result.PixiToml), 0644); err != nil {
		return fmt.Errorf("import failed: %w; partial files at %s", err, absDir)
	}

	if result.PixiLock != "" {
		if err := os.WriteFile(filepath.Join(outputDir, "pixi.lock"), []byte(result.PixiLock), 0644); err != nil {
			return fmt.Errorf("import failed: %w; partial files at %s", err, absDir)
		}
	}

	// Restore assets. PullModeFull already populated Bytes in parallel and
	// classifyBundleManifest already validated every title, but we re-run
	// the per-path check here so offline code paths remain defence-in-depth.
	for _, asset := range result.Assets {
		if err := oci.ValidateAssetPath(asset.Path); err != nil {
			return fmt.Errorf("unsafe path in bundle: %s: %w", asset.Path, err)
		}
		target := filepath.Join(outputDir, filepath.FromSlash(asset.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return fmt.Errorf("import failed: %w; partial files at %s", err, absDir)
		}
		if err := os.WriteFile(target, asset.Bytes, 0644); err != nil {
			return fmt.Errorf("import failed: %w; partial files at %s", err, absDir)
		}
	}

	absOutput, _ := filepath.Abs(outputDir)

	// Auto-track the workspace (name will be read from imported pixi.toml)
	if err := ensureInit(outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to auto-track workspace: %v\n", err)
	}

	ref := repoRef + ":" + tag
	fmt.Fprintf(os.Stderr, "Imported %s -> %s (%d asset file(s))\n", ref, absOutput, len(result.Assets))

	return nil
}

// dirIsNonEmpty reports whether path exists and contains at least one
// entry. Missing directory returns (false, nil).
func dirIsNonEmpty(path string) (bool, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("reading %s: %w", path, err)
	}
	return len(entries) > 0, nil
}
