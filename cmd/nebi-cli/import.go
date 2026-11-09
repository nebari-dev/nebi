package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nebari-dev/nebi/internal/client"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
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
	Short: "Import a project from a public OCI registry",
	Long: `Import a Nebi project bundle from an OCI registry.

The OCI reference should be in the format: registry/repository:tag
(e.g., quay.io/nebari/my-env:v1)

Restores pixi.toml, pixi.lock, and any bundled asset files to the output
directory. Works entirely locally — no server connection needed.

The local project name is derived from the [workspace] name field
in the imported pixi.toml.

With --project the artifact becomes a new local project instead (the same
as "Pull from registry" in the app): it goes to Nebi's projects folder as
version 1. The reference's host must be one of this machine's registries
('nebi registry list --local'), so its credentials are used.

Examples:
  nebi import quay.io/nebari/my-env:v1
  nebi import ghcr.io/myorg/data-science:latest -o ./my-project
  nebi import quay.io/nebari/my-env:5 --project --name my-env --install`,
	Args: cobra.ExactArgs(1),
	RunE: runImport,
}

func init() {
	importCmd.Flags().Bool("project", false, "Create a local project from the artifact (uses this machine's registries)")
	importCmd.Flags().String("name", "", "Local project name with --project (defaults to the repository name)")
	importCmd.Flags().Bool("install", false, "Install the new project's version 1 with --project")
	importCmd.Flags().StringVarP(&importOutput, "output", "o", ".", "Output directory")
	importCmd.Flags().BoolVar(&importForce, "force", false, "Overwrite existing files without prompting (only when the bundle contains no asset layers)")
	importCmd.Flags().IntVar(&importConcurrency, "concurrency", 8, "Parallel blob fetch workers")
}

func runImport(cmd *cobra.Command, args []string) error {
	repoRef, tag := parseProjectRef(args[0])
	if tag == "" {
		return fmt.Errorf("tag is required; use format registry/repository:tag (e.g., quay.io/nebari/my-env:v1)")
	}

	if asProject, _ := cmd.Flags().GetBool("project"); asProject {
		defer resetFlagsByName(cmd, "project", "name", "install")
		return runImportAsProject(cmd, repoRef, tag)
	}

	repoRef, plainHTTP := oci.StripScheme(repoRef)

	ctx := context.Background()

	// Peek at manifest first so we can enforce the empty-destination
	// policy before any bytes land on disk. This is cheap (one small
	// GET) and avoids partial-extract state on a rejected destination.
	peek, err := oci.PullBundle(ctx, repoRef, tag, oci.PullOptions{
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
	hasAssets := len(peek.Assets) > 0
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

	// Stream every layer straight to disk via oras.Copy + file.Store.
	// Asset blobs never land fully in RAM regardless of size.
	result, err := oci.ExtractBundle(ctx, repoRef, tag, outputDir, oci.PullOptions{
		Concurrency: importConcurrency,
		PlainHTTP:   plainHTTP,
	})
	if err != nil {
		return fmt.Errorf("import failed: %w; partial files at %s", err, absDir)
	}

	absOutput, _ := filepath.Abs(outputDir)

	// Auto-track the project (name will be read from imported pixi.toml)
	if err := ensureInit(outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to auto-track project: %v\n", err)
	}

	ref := repoRef + ":" + tag
	fmt.Fprintf(os.Stderr, "Imported %s -> %s (%d asset file(s))\n", ref, absOutput, len(result.Assets))

	return nil
}

// dirIsNonEmpty reports whether path exists and contains at least one
// entry. Missing directory returns (false, nil). Reads a single dirent
// via File.ReadDir(1) rather than slurping the whole directory, so the
// check stays cheap even against a pathologically populated target.
func dirIsNonEmpty(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()
	if _, err := f.ReadDir(1); err != nil {
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		return false, fmt.Errorf("reading %s: %w", path, err)
	}
	return true, nil
}

// runImportAsProject pulls host/repository:tag into a new local project
// through the client SDK, using the configured registry for host.
func runImportAsProject(cmd *cobra.Command, repoRef, tag string) error {
	stripped, plainHTTP := oci.StripScheme(repoRef)
	host, repository, ok := strings.Cut(stripped, "/")
	if !ok || repository == "" {
		return fmt.Errorf("use registry/repository:tag, e.g. quay.io/nebari/my-env:5")
	}
	name, _ := cmd.Flags().GetString("name")
	install, _ := cmd.Flags().GetBool("install")
	return withStack(cmd, func(ctx context.Context, st *client.Stack) error {
		regs, err := st.SDK.Registry.ListRegistries(ctx)
		if err != nil {
			return err
		}
		var match *shared.RegistryItem
		for i := range regs {
			h, _, plain := oci.ParseRegistryURLFull(regs[i].URL)
			if strings.EqualFold(h, host) && plain == plainHTTP {
				match = &regs[i]
				break
			}
		}
		if match == nil {
			return fmt.Errorf("%s is not one of this machine's registries; add it with 'nebi registry add --local --name <name> --url %s --namespace <namespace>'", host, host)
		}
		res, err := st.SDK.Registry.Pull(ctx, match.ID, shared.RegistryPullRequest{
			Repository: repository,
			Tag:        tag,
			LocalName:  name,
			Install:    install,
		})
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Imported %s:%s as project %s (%s)\n", stripped, tag, res.Job.ProjectName, res.ProjectID)
		return nil
	})
}
