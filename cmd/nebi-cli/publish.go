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
	Use:   "publish [project]",
	Short: "Publish a project to an OCI registry",
	Long: `Publish a project to an OCI registry.

If no project name is given, the current directory's tracked project is used.
The repository name defaults to the project name.
The tag auto-increments (v1, v2, v3, ...) based on existing publications.
If --registry is not specified, the server's default registry is used.

Examples:
  nebi publish                                       # publish current directory project
  nebi publish myproject
  nebi publish myproject --tag v1.0.0
  nebi publish myproject --repo custom-name --registry ghcr`,
	Args:              cobra.MaximumNArgs(1),
	RunE:              runProjectPublish,
	ValidArgsFunction: completeServerProjectNames,
}

func init() {
	publishCmd.Flags().StringVar(&publishRegistry, "registry", "", "Registry name or ID (uses server default if not set)")
	publishCmd.Flags().StringVar(&publishTag, "tag", "", "OCI tag (auto-increments v1, v2, ... if not set)")
	publishCmd.Flags().StringVar(&publishRepo, "repo", "", "OCI repository name (defaults to project name)")
	publishCmd.Flags().BoolVar(&publishLocal, "local", false, "Publish directly to registry without a server")
	publishCmd.Flags().IntVar(&publishConcurrency, "concurrency", 8, "Parallel blob push workers (only with --local)")
}

func runProjectPublish(cmd *cobra.Command, args []string) error {
	if isLocalMode(cmd) {
		return runPublishLocal(args)
	}
	return runPublishServer(args)
}

func runPublishServer(args []string) error {
	var projectName string
	if len(args) == 1 {
		projectName = args[0]
	} else {
		origin, err := lookupOrigin()
		if err != nil {
			return err
		}
		if origin == nil {
			return fmt.Errorf("no project specified and no origin set in current directory;\nusage: nebi publish [project]")
		}
		projectName = origin.OriginName
		fmt.Fprintf(os.Stderr, "Using project %q from origin\n", projectName)
	}

	client, err := getAuthenticatedClient()
	if err != nil {
		return err
	}

	ctx := context.Background()

	project, err := findProjectByName(client, ctx, projectName)
	if err != nil {
		return err
	}

	var registryID string
	if publishRegistry != "" {
		registryID, err = resolveRegistryID(client, ctx, publishRegistry)
		if err != nil {
			return err
		}
	}

	defaults, err := client.GetPublishDefaults(ctx, project.ID, registryID)
	if err != nil {
		return fmt.Errorf("getting publish defaults: %w", err)
	}

	if registryID == "" {
		registryID = defaults.RegistryID
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

	fmt.Fprintf(os.Stderr, "Publishing %s to %s:%s...\n", projectName, repo, tag)
	resp, err := client.PublishProject(ctx, project.ID, req)
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

	// Resolve project from args or current directory
	var project *store.LocalProject
	if len(args) == 1 {
		project, err = s.FindProjectByName(args[0])
		if err != nil {
			return err
		}
		if project == nil {
			return fmt.Errorf("project %q not found in local store; run 'nebi init' in the project directory first", args[0])
		}
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}
		project, err = s.FindProjectByPath(cwd)
		if err != nil {
			return err
		}
		if project == nil {
			return fmt.Errorf("current directory is not a tracked project; run 'nebi init' first")
		}
		fmt.Fprintf(os.Stderr, "Using project %q\n", project.Name)
	}

	// Read pixi files from disk
	pixiTomlPath := filepath.Join(project.Path, "pixi.toml")
	pixiLockPath := filepath.Join(project.Path, "pixi.lock")

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
	// pixi.lock are untouched. Preview walks the project with the
	// same rules Publish will use, so both always agree on the asset
	// set.
	assetRefs, err := oci.PreviewAssetRefs(project.Path)
	if err != nil {
		return fmt.Errorf("preview bundle for tag hash: %w", err)
	}
	tag := contenthash.HashBundle(string(pixiToml), string(pixiLock), assetRefs)
	if publishTag != "" {
		tag = publishTag
	}

	repo := fmt.Sprintf("%s-%s", project.Name, project.ID.String()[:8])
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
	fmt.Fprintf(os.Stderr, "Publishing %s to %s/%s/%s:%s...\n", project.Name, host, ns, repo, tag)
	res, err := oci.Publish(ctx, project.Path, regEndpoint, repo, tag,
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
		ProjectID:  project.ID,
		RegistryID: reg.ID,
		Repository: fullRepo,
		Tag:        tag,
		Digest:     digest,
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
