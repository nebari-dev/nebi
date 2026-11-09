package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nebari-dev/nebi/internal/cliclient"
	"github.com/nebari-dev/nebi/internal/client"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/contenthash"
	"github.com/nebari-dev/nebi/internal/models"
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
  nebi publish myproject --repo custom-name --registry ghcr
  nebi publish myproject --local --version 5          # publish stored version 5

Without a server (or with --local) the registries of this machine are
used ('nebi registry list --local'). See also 'nebi project publish-version'
to publish one version to several registries.`,
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
	publishCmd.Flags().Int("version", 0, "Publish this stored version of a local project instead of the project directory (local only)")
}

func runProjectPublish(cmd *cobra.Command, args []string) error {
	if isLocalMode(cmd) {
		return runPublishLocal(cmd, args)
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

// runPublishLocal publishes without a server. Registries and their
// passwords come from the client SDK (the same registries the desktop app
// and nebi-web use). With --version N it publishes that stored version of a
// local project through the SDK; otherwise it bundles the tracked project
// directory (pixi files plus extra files) as before and, when the directory
// matches a stored version, records the publication on that version.
func runPublishLocal(cmd *cobra.Command, args []string) error {
	version, _ := cmd.Flags().GetInt("version")
	if version > 0 {
		defer resetFlagsByName(cmd, "version")
		return runPublishLocalVersion(cmd, args, version)
	}

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
	// The SDK stack opens the same database; release the store first.
	s.Close()

	// Read pixi files from disk
	pixiToml, err := os.ReadFile(filepath.Join(project.Path, "pixi.toml"))
	if err != nil {
		return fmt.Errorf("reading pixi.toml: %w", err)
	}
	pixiLock, err := os.ReadFile(filepath.Join(project.Path, "pixi.lock"))
	if err != nil {
		return fmt.Errorf("reading pixi.lock: %w", err)
	}

	return withStack(cmd, func(ctx context.Context, st *client.Stack) error {
		reg, err := st.SDK.Registry.ResolveRegistry(ctx, publishRegistry)
		if err != nil {
			if publishRegistry != "" {
				return fmt.Errorf("registry %q not found in local store", publishRegistry)
			}
			return err
		}
		password := ""
		if reg.Username != "" {
			password, _ = st.SDK.Local.RegistryPassword(ctx, reg.Name)
			if password == "" {
				return fmt.Errorf("no credentials found for registry %q; re-add with 'nebi registry add --local'", reg.Name)
			}
		}

		// The tag is content-addressed across the full bundle (pixi files
		// plus every asset's path and content SHA), so changing a bundled
		// asset shifts the tag even when pixi.toml and pixi.lock are
		// untouched.
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

		host, _, plainHTTP := oci.ParseRegistryURLFull(reg.URL)
		regEndpoint := oci.Registry{
			Host:      host,
			Namespace: reg.Namespace,
			Username:  reg.Username,
			Password:  password,
			PlainHTTP: plainHTTP,
		}
		fmt.Fprintf(os.Stderr, "Publishing %s to %s/%s/%s:%s...\n", project.Name, host, reg.Namespace, repo, tag)
		res, err := oci.Publish(ctx, project.Path, regEndpoint, repo, tag,
			oci.WithExtraTags("latest"),
			oci.WithConcurrency(publishConcurrency),
		)
		if err != nil {
			return fmt.Errorf("failed to publish: %w", err)
		}

		// Legacy CLI record, kept for older tooling.
		legacy := &store.LocalPublication{
			ProjectID:  project.ID,
			RegistryID: reg.ID,
			Repository: res.Repository,
			Tag:        tag,
			Digest:     res.Digest,
		}
		if err := st.SDK.Deps().DB.WithContext(ctx).Create(legacy).Error; err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to record publication: %v\n", err)
		}
		// The Published badge, when the directory is a stored version.
		var v models.ProjectVersion
		if err := st.SDK.Deps().DB.WithContext(ctx).
			Where("project_id = ? AND manifest_content = ? AND lock_file_content = ?", project.ID, string(pixiToml), string(pixiLock)).
			Order("version_number DESC").First(&v).Error; err == nil {
			repoPath := strings.TrimPrefix(res.Repository, host+"/")
			if _, err := st.SDK.Registry.RecordPublication(ctx, project.ID, v.VersionNumber, reg.ID, repoPath, tag, res.Digest); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to record the publication on version %d: %v\n", v.VersionNumber, err)
			}
		}

		fmt.Fprintf(os.Stderr, "Published %s:%s (digest: %s)\n", res.Repository, tag, res.Digest)
		return nil
	})
}

// runPublishLocalVersion publishes one stored version of a local project
// through the client SDK (the same path as the desktop app's Publish).
func runPublishLocalVersion(cmd *cobra.Command, args []string, version int) error {
	if len(args) != 1 {
		return fmt.Errorf("--version needs a project name: nebi publish <project> --version N")
	}
	return withStack(cmd, func(ctx context.Context, st *client.Stack) error {
		id, err := resolveProjectID(ctx, st, args[0])
		if err != nil {
			return err
		}
		reg, err := st.SDK.Registry.ResolveRegistry(ctx, publishRegistry)
		if err != nil {
			return err
		}
		target := shared.PublishTarget{RegistryID: reg.ID, Tag: fmt.Sprint(version)}
		defaults, err := st.SDK.Registry.PublishDefaults(ctx, id, version)
		if err != nil {
			return err
		}
		for _, d := range defaults {
			if d.RegistryID == reg.ID {
				target.Repository, target.Tag = d.Repository, d.Tag
			}
		}
		if publishRepo != "" {
			target.Repository = publishRepo
			if !strings.Contains(publishRepo, "/") && reg.Namespace != "" {
				target.Repository = reg.Namespace + "/" + publishRepo
			}
		}
		if publishTag != "" {
			target.Tag = publishTag
		}
		res, err := st.SDK.Registry.Publish(ctx, id, version, shared.PublishRequest{Targets: []shared.PublishTarget{target}})
		if err != nil {
			return err
		}
		r := res.Results[0]
		if r.Status != "published" {
			return fmt.Errorf("failed to publish: %s", r.Error)
		}
		fmt.Fprintf(os.Stderr, "Published %s (digest: %s)\n", r.Reference, r.Digest)
		return nil
	})
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
