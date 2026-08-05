package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
The repository name defaults to the registry's configured repository, or to the workspace name.
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
	publishCmd.Flags().StringVar(&publishRepo, "repo", "", "OCI repository name (overrides the registry/workspace default)")
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
	var selectedRegistry *cliclient.Registry
	if publishRegistry != "" {
		var err error
		selectedRegistry, err = resolveRegistry(client, ctx, publishRegistry)
		if err != nil {
			return err
		}
		registryID = selectedRegistry.ID
	}

	workspaceRepo := publishWorkspaceRepository(ws.Name, ws.ID)
	repo := defaults.Repository
	if publishRepo != "" {
		repo = publishRepo
	} else if selectedRegistry != nil {
		repo = publishDefaultRepository(workspaceRepo, selectedRegistry.DefaultRepository)
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

	// Compute defaults. The tag is content-addressed across the full
	// bundle — pixi files + every asset's path and content SHA — so
	// changing a bundled asset shifts the tag even when pixi.toml and
	// pixi.lock are untouched. Preview walks the workspace with the
	// same rules Publish will use, so both always agree on the asset
	// set.
	assetRefs, err := oci.PreviewAssetRefs(ws.Path)
	if err != nil {
		return fmt.Errorf("preview bundle for tag hash: %w", err)
	}
	tag := contenthash.HashBundle(string(pixiToml), string(pixiLock), assetRefs)
	if publishTag != "" {
		tag = publishTag
	}

	repo := publishDefaultRepository(fmt.Sprintf("%s-%s", ws.Name, ws.ID.String()[:8]), reg.DefaultRepository)
	if publishRepo != "" {
		repo = publishRepo
	}

	host, ns, plainHTTP := oci.ParseRegistryURLFull(reg.URL)
	if reg.Namespace != "" {
		ns = reg.Namespace
	}
	regEndpoint := oci.Registry{
		Host:      host,
		Namespace: ns,
		Username:  reg.Username,
		Password:  password,
		PlainHTTP: plainHTTP,
	}

	ctx := context.Background()
	fmt.Fprintf(os.Stderr, "Publishing %s to %s/%s/%s:%s...\n", ws.Name, host, ns, repo, tag)
	res, err := oci.Publish(ctx, ws.Path, regEndpoint, repo, tag,
		oci.WithExtraTags("latest"),
		oci.WithConcurrency(publishConcurrency),
	)
	if err != nil {
		return fmt.Errorf("failed to publish: %w", err)
	}
	digest := res.Digest
	fullRepo := res.Repository

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
	reg, err := resolveRegistry(client, ctx, registry)
	if err != nil {
		return "", err
	}
	return reg.ID, nil
}

func resolveRegistry(client *cliclient.Client, ctx context.Context, registry string) (*cliclient.Registry, error) {
	registries, err := client.ListRegistries(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing registries: %w", err)
	}

	for i := range registries {
		r := &registries[i]
		if r.Name == registry || r.ID == registry {
			return r, nil
		}
	}
	return nil, fmt.Errorf("registry %q not found on server", registry)
}

func publishDefaultRepository(fallback, configured string) string {
	if repo := strings.TrimSpace(configured); repo != "" {
		return repo
	}
	return fallback
}

func publishWorkspaceRepository(name, id string) string {
	if len(id) >= 8 {
		return fmt.Sprintf("%s-%s", name, id[:8])
	}
	return name
}
