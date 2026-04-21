package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/nebari-dev/nebi/internal/contenthash"
	"github.com/nebari-dev/nebi/internal/oci"
	"github.com/nebari-dev/nebi/internal/store"
	"github.com/spf13/cobra"
)

var (
	publishRegistry    string
	publishTag         string
	publishRepo        string
	publishLocal       bool
	publishConcurrency int
)

var publishCmd = &cobra.Command{
	Use:   "publish [workspace]",
	Short: "Publish a workspace to an OCI registry",
	Long: `Publish a workspace to an OCI registry.

If no workspace name is given, the current directory's tracked workspace is used.
The repository name defaults to the workspace name.
The tag auto-increments (v1, v2, v3, ...) based on existing publications.
If --registry is not specified, the server's default registry is used.

Examples:
  nebi publish                                       # publish current directory workspace
  nebi publish myworkspace
  nebi publish myworkspace --tag v1.0.0
  nebi publish myworkspace --repo custom-name --registry ghcr`,
	Args:              cobra.MaximumNArgs(1),
	RunE:              runWorkspacePublish,
	ValidArgsFunction: completeServerWorkspaceNames,
}

func init() {
	publishCmd.Flags().StringVar(&publishRegistry, "registry", "", "Registry name or ID (uses server default if not set)")
	publishCmd.Flags().StringVar(&publishTag, "tag", "", "OCI tag (auto-increments v1, v2, ... if not set)")
	publishCmd.Flags().StringVar(&publishRepo, "repo", "", "OCI repository name (defaults to workspace name)")
	publishCmd.Flags().BoolVar(&publishLocal, "local", false, "Publish directly to registry without a server")
	publishCmd.Flags().IntVar(&publishConcurrency, "concurrency", 8, "Parallel blob push workers (only with --local)")
}

func runWorkspacePublish(cmd *cobra.Command, args []string) error {
	if isLocalMode(cmd) {
		return runPublishLocal(args)
	}
	return runPublishServer(args)
}

func runPublishServer(args []string) error {
	var wsName string
	if len(args) == 1 {
		wsName = args[0]
	} else {
		origin, err := lookupOrigin()
		if err != nil {
			return err
		}
		if origin == nil {
			return fmt.Errorf("no workspace specified and no origin set in current directory;\nusage: nebi publish [workspace]")
		}
		wsName = origin.OriginName
		fmt.Fprintf(os.Stderr, "Using workspace %q from origin\n", wsName)
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

	defaults, err := client.GetPublishDefaults(ctx, ws.ID)
	if err != nil {
		return fmt.Errorf("getting publish defaults: %w", err)
	}

	registryID := defaults.RegistryID
	if publishRegistry != "" {
		var err error
		registryID, err = resolveRegistryID(client, ctx, publishRegistry)
		if err != nil {
			return err
		}
	}

	repo := defaults.Repository
	if publishRepo != "" {
		repo = publishRepo
	}

	tag := defaults.Tag
	if publishTag != "" {
		tag = publishTag
	}

	req := cliclient.PublishRequest{
		RegistryID: registryID,
		Repository: repo,
		Tag:        tag,
	}

	fmt.Fprintf(os.Stderr, "Publishing %s to %s:%s...\n", wsName, repo, tag)
	resp, err := client.PublishWorkspace(ctx, ws.ID, req)
	if err != nil {
		return fmt.Errorf("failed to publish: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Published %s:%s (digest: %s)\n", resp.Repository, resp.Tag, resp.Digest)
	return nil
}

func runPublishLocal(args []string) error {
	s, err := store.New()
	if err != nil {
		return err
	}
	defer s.Close()

	// Resolve workspace from args or current directory
	var ws *store.LocalWorkspace
	if len(args) == 1 {
		ws, err = s.FindWorkspaceByName(args[0])
		if err != nil {
			return err
		}
		if ws == nil {
			return fmt.Errorf("workspace %q not found in local store; run 'nebi init' in the workspace directory first", args[0])
		}
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}
		ws, err = s.FindWorkspaceByPath(cwd)
		if err != nil {
			return err
		}
		if ws == nil {
			return fmt.Errorf("current directory is not a tracked workspace; run 'nebi init' first")
		}
		fmt.Fprintf(os.Stderr, "Using workspace %q\n", ws.Name)
	}

	// Read pixi files from disk
	pixiTomlPath := filepath.Join(ws.Path, "pixi.toml")
	pixiLockPath := filepath.Join(ws.Path, "pixi.lock")

	pixiToml, err := os.ReadFile(pixiTomlPath)
	if err != nil {
		return fmt.Errorf("reading pixi.toml: %w", err)
	}
	pixiLock, err := os.ReadFile(pixiLockPath)
	if err != nil {
		return fmt.Errorf("reading pixi.lock: %w", err)
	}

	// Resolve registry
	var reg *store.LocalRegistry
	if publishRegistry != "" {
		reg, err = s.GetRegistryByName(publishRegistry)
		if err != nil {
			return fmt.Errorf("registry %q not found in local store", publishRegistry)
		}
	} else {
		reg, err = s.GetDefaultRegistry()
		if err != nil {
			return err
		}
	}

	// Get credentials from keyring
	cs := store.NewCredentialStore(s.DataDir())
	password, err := cs.GetPassword(reg.Name)
	if err != nil && reg.Username != "" {
		return fmt.Errorf("no credentials found for registry %q; re-add with 'nebi registry add --local'", reg.Name)
	}

	// Compute defaults
	tag := contenthash.Hash(string(pixiToml), string(pixiLock))
	if publishTag != "" {
		tag = publishTag
	}

	repo := fmt.Sprintf("%s-%s", ws.Name, ws.ID.String()[:8])
	if publishRepo != "" {
		repo = publishRepo
	}

	// Build full repository path
	host, namespace, plainHTTP := oci.ParseRegistryURLFull(reg.URL)
	fullRepo := host
	if reg.Namespace != "" {
		fullRepo = host + "/" + reg.Namespace
	} else if namespace != "" {
		fullRepo = host + "/" + namespace
	}
	fullRepo = fullRepo + "/" + repo

	ctx := context.Background()

	// Collect bundle assets: workspace files filtered by [tool.nebi.bundle]
	// include/exclude plus .gitignore. pixi.toml and pixi.lock are force-
	// included by the walker; we strip them from the asset list because the
	// publisher emits them as typed layers 0 and 1.
	cfg, err := oci.LoadBundleConfig(pixiTomlPath)
	if err != nil {
		return fmt.Errorf("parsing bundle config: %w", err)
	}
	bundleFiles, err := oci.WalkBundle(ws.Path, cfg)
	if err != nil {
		return fmt.Errorf("walking workspace: %w", err)
	}
	assets := make([]oci.AssetFile, 0, len(bundleFiles))
	for _, f := range bundleFiles {
		if f.RelPath == "pixi.toml" || f.RelPath == "pixi.lock" {
			continue
		}
		assets = append(assets, f)
	}

	opts := oci.PublishOptions{
		Repository:   fullRepo,
		Tag:          tag,
		ExtraTags:    []string{"latest"},
		Username:     reg.Username,
		Password:     password,
		RegistryHost: host,
		Assets:       assets,
		Concurrency:  publishConcurrency,
		PlainHTTP:    plainHTTP,
	}

	fmt.Fprintf(os.Stderr, "Publishing %s to %s:%s (%d asset file(s))...\n",
		ws.Name, fullRepo, tag, len(assets))
	digest, err := oci.PublishWorkspace(ctx, ws.Path, opts)
	if err != nil {
		return fmt.Errorf("failed to publish: %w", err)
	}

	// Record publication
	pub := &store.LocalPublication{
		WorkspaceID: ws.ID,
		RegistryID:  reg.ID,
		Repository:  fullRepo,
		Tag:         tag,
		Digest:      digest,
	}
	if err := s.CreatePublication(pub); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to record publication: %v\n", err)
	}

	fmt.Fprintf(os.Stderr, "Published %s:%s (digest: %s)\n", fullRepo, tag, digest)
	return nil
}

// resolveRegistryID resolves a registry name/ID or finds the default registry.
func resolveRegistryID(client *cliclient.Client, ctx context.Context, registry string) (string, error) {
	registries, err := client.ListRegistries(ctx)
	if err != nil {
		return "", fmt.Errorf("listing registries: %w", err)
	}

	for _, r := range registries {
		if r.Name == registry || r.ID == registry {
			return r.ID, nil
		}
	}
	return "", fmt.Errorf("registry %q not found on server", registry)
}
