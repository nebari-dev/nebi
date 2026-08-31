package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nebari-dev/nebi/internal/oci"
	"github.com/nebari-dev/nebi/internal/store"
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

A single-segment reference, or one prefixed with '<registry-name>:', is
resolved against the registries configured with 'nebi registry add
--local'. The prefix picks a registry; without one the default is used.

Anything else is taken as naming its own host, including a reference
whose first segment carries a port or is followed by a slash. Use the
'<registry-name>:' prefix to reach a nested repository through a
configured registry.

Restores pixi.toml, pixi.lock, and any bundled asset files to the output
directory. Works entirely locally — no server connection needed.

The local workspace name is derived from the [workspace] name field
in the imported pixi.toml.

Examples:
  nebi import quay.io/nebari/my-env:v1
  nebi import ghcr.io/myorg/data-science:latest -o ./my-project
  nebi import myreg:my-env:v1
  nebi import myreg:myorg/my-env:v1
  nebi import my-env:v1`,
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

	repoRef, plainHTTP := oci.StripScheme(repoRef)

	// A scheme-qualified reference always carries its own host, so it is
	// never a candidate for alias resolution.
	if !hasScheme(args[0]) {
		alias, repo, hasHost := splitImportRef(repoRef)
		if !hasHost {
			resolved, regPlainHTTP, err := resolveImportRef(alias, repo)
			if err != nil {
				return err
			}
			repoRef, plainHTTP = resolved, regPlainHTTP
		}
	}

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

	// Auto-track the workspace (name will be read from imported pixi.toml)
	if err := ensureInit(outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to auto-track workspace: %v\n", err)
	}

	ref := repoRef + ":" + tag
	fmt.Fprintf(os.Stderr, "Imported %s -> %s (%d asset file(s))\n", ref, absOutput, len(result.Assets))

	return nil
}

// hasScheme reports whether a reference was written with an explicit
// http:// or https:// prefix.
func hasScheme(ref string) bool {
	return strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://")
}

// looksLikeHost reports whether a reference's first segment names a
// registry host rather than a registry alias. A dot covers every public
// registry and any FQDN, and localhost is the one hostless name in
// common use. Callers strip any ":port" before asking.
func looksLikeHost(segment string) bool {
	return strings.Contains(segment, ".") || segment == "localhost"
}

// isAllDigits reports whether s is a non-empty run of ASCII digits. This
// is the only thing separating "<host>:<port>/<repo>" from
// "<alias>:<repo>", which are otherwise the same shape. Empty must be
// false so that "myreg:" stays an alias naming no repository.
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// splitImportRef separates an optional registry alias from the repository
// path of a tag-stripped import reference. A reference that names a host
// is returned untouched with hasHost=true and never reaches the local
// store.
//
// The grammar is genuinely ambiguous in two places, so the rule is
// deliberately narrow:
//
//   - "<alias>:<repo>" and "<host>:<port>/<repo>" are the same shape.
//     An all-digit run after the colon is a port, anything else an alias.
//   - "<host>/<repo>" and a namespace-relative "<org>/<repo>" are also
//     the same shape, and nothing can tell them apart. A slash therefore
//     means the first segment is a host, which is what it means without
//     this feature and what 'nebi publish' prints. To reach a nested
//     repository through a configured registry, name the registry:
//     "myreg:org/my-env:v1".
func splitImportRef(ref string) (alias, repo string, hasHost bool) {
	first, _, hasSlash := strings.Cut(ref, "/")

	if name, port, found := strings.Cut(first, ":"); found {
		if looksLikeHost(name) || isAllDigits(port) {
			return "", ref, true
		}
		return name, ref[len(name)+1:], false
	}

	if hasSlash || looksLikeHost(first) {
		return "", ref, true
	}
	return "", ref, false
}

// resolveImportRef expands an alias-or-default reference into a fully
// qualified host/namespace/repository, mirroring how 'nebi publish
// --local' builds its push target so the two sides address the same
// bundle by the same name.
func resolveImportRef(alias, repo string) (string, bool, error) {
	if repo == "" {
		if alias == "" {
			return "", false, fmt.Errorf("empty reference; use registry/repository:tag (e.g., quay.io/nebari/my-env:v1)")
		}
		return "", false, fmt.Errorf("registry %q named with no repository; use %s:<repository>:<tag>", alias, alias)
	}

	s, err := store.New()
	if err != nil {
		return "", false, err
	}
	defer s.Close()

	// A bare name that matches a configured registry is almost certainly
	// an alias whose tag was swallowed by the reference parser, so say
	// that rather than pulling a repository of the same name from the
	// default registry.
	if alias == "" {
		if _, nameErr := s.GetRegistryByName(repo); nameErr == nil {
			return "", false, fmt.Errorf("%q names a configured registry, not a repository; use %s:<repository>:<tag>", repo, repo)
		}
	}

	reg, err := resolveLocalRegistry(s, alias)
	if err != nil {
		if alias == "" {
			return "", false, fmt.Errorf("%q does not name a registry host and no default registry is configured; use a full reference such as quay.io/org/my-env:v1, or set a default with 'nebi registry add --local --default'", repo)
		}
		return "", false, err
	}

	host, ns, plainHTTP := registryTarget(reg)

	parts := []string{host}
	if ns != "" {
		parts = append(parts, ns)
	}
	parts = append(parts, repo)
	return strings.Join(parts, "/"), plainHTTP, nil
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
