package oci

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"golang.org/x/sync/errgroup"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
)

// BrowseOptions contains options for browsing an OCI registry
type BrowseOptions struct {
	RegistryHost string
	Username     string
	Password     string
}

// RepositoryInfo contains information about a repository
type RepositoryInfo struct {
	Name     string `json:"name"`
	IsPublic *bool  `json:"is_public,omitempty"`
}

// TagInfo contains information about a tag
type TagInfo struct {
	Name string `json:"name"`
}

// PullMode controls which blobs a bundle pull fetches from the registry.
type PullMode int

const (
	// PullModePixiOnly fetches manifest + pixi.toml + pixi.lock only. Asset
	// layers are listed in PullResult.Assets (path only, no Bytes). This
	// matches what the server, UI, and server-driven publish/import need.
	PullModePixiOnly PullMode = iota
	// PullModeFull fetches manifest + every layer including asset blobs.
	// Used by `nebi import` on the CLI.
	PullModeFull
)

// PullOptions controls an OCI bundle pull.
type PullOptions struct {
	Username    string
	Password    string
	Mode        PullMode // defaults to PullModePixiOnly
	Concurrency int      // ≤ 0 uses default (8)
	// PlainHTTP talks to the registry over HTTP. Test/local registries
	// only.
	PlainHTTP bool
}

// AssetBlob is a single asset layer. When fetched via PullModeFull, Bytes
// holds the blob content; in PullModePixiOnly it is nil.
type AssetBlob struct {
	Path  string
	Bytes []byte
}

// PullResult contains the content pulled from a registry tag
type PullResult struct {
	PixiToml string      `json:"pixi_toml"`
	PixiLock string      `json:"pixi_lock"`
	Assets   []AssetBlob `json:"assets,omitempty"`
}

// ParseRegistryURL splits a registry URL into its host and optional namespace.
// For example, "quay.io/nebari" returns host="quay.io", namespace="nebari".
// A plain host like "quay.io" returns namespace="".
func ParseRegistryURL(rawURL string) (host, namespace string) {
	u := strings.TrimPrefix(strings.TrimPrefix(rawURL, "https://"), "http://")
	u = strings.TrimSuffix(u, "/")
	parts := strings.SplitN(u, "/", 2)
	host = parts[0]
	if len(parts) > 1 {
		namespace = parts[1]
	}
	return
}

// newAuthClient returns an auth.Client for the given credentials, or nil
// for anonymous access. When nil, oras-go uses its default client which
// properly handles anonymous bearer token exchange (needed for Quay.io etc).
func newAuthClient(username, password string) *auth.Client {
	if username == "" && password == "" {
		return nil
	}
	return &auth.Client{
		Credential: func(ctx context.Context, hostname string) (auth.Credential, error) {
			return auth.Credential{
				Username: username,
				Password: password,
			}, nil
		},
	}
}

// ListRepositories queries the /v2/_catalog endpoint for a registry
func ListRepositories(ctx context.Context, opts BrowseOptions) ([]RepositoryInfo, error) {
	reg, err := remote.NewRegistry(opts.RegistryHost)
	if err != nil {
		return nil, fmt.Errorf("failed to create registry client: %w", err)
	}

	if c := newAuthClient(opts.Username, opts.Password); c != nil {
		reg.Client = c
	}

	var repos []RepositoryInfo
	err = reg.Repositories(ctx, "", func(repoNames []string) error {
		for _, name := range repoNames {
			repos = append(repos, RepositoryInfo{Name: name})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list repositories: %w", err)
	}

	return repos, nil
}

// ListRepositoriesViaQuayAPI lists repositories using Quay.io's REST API.
// This is used as a fallback when the standard /v2/_catalog endpoint is not supported.
// If an API token is provided, it is sent as a Bearer token to also list private repos.
// Always includes public=true so public repos are returned regardless of auth.
func ListRepositoriesViaQuayAPI(ctx context.Context, host, namespace, apiToken string) ([]RepositoryInfo, error) {
	if namespace == "" {
		return nil, fmt.Errorf("namespace is required for Quay.io API")
	}

	apiURL := fmt.Sprintf("https://%s/api/v1/repository?namespace=%s&public=true", host, url.QueryEscape(namespace))

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+apiToken)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch from Quay.io API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Quay.io API returned status %d", resp.StatusCode)
	}

	var result struct {
		Repositories []struct {
			Namespace string `json:"namespace"`
			Name      string `json:"name"`
			IsPublic  bool   `json:"is_public"`
		} `json:"repositories"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode Quay.io API response: %w", err)
	}

	var repos []RepositoryInfo
	for _, r := range result.Repositories {
		isPublic := r.IsPublic
		repos = append(repos, RepositoryInfo{
			Name:     fmt.Sprintf("%s/%s", r.Namespace, r.Name),
			IsPublic: &isPublic,
		})
	}
	return repos, nil
}

// ListTags lists all tags for a given repository reference
func ListTags(ctx context.Context, repoRef string, opts BrowseOptions) ([]TagInfo, error) {
	repo, err := remote.NewRepository(repoRef)
	if err != nil {
		return nil, fmt.Errorf("failed to create repository client: %w", err)
	}

	if c := newAuthClient(opts.Username, opts.Password); c != nil {
		repo.Client = c
	}

	var tags []TagInfo
	err = repo.Tags(ctx, "", func(tagNames []string) error {
		for _, name := range tagNames {
			tags = append(tags, TagInfo{Name: name})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list tags: %w", err)
	}

	return tags, nil
}

// classifiedManifest is the result of splitting layers by role.
type classifiedManifest struct {
	pixiToml ocispec.Descriptor
	pixiLock ocispec.Descriptor
	assets   []ocispec.Descriptor // path stored in AnnotationTitle
}

// classifyBundleManifest inspects a parsed OCI manifest and returns its
// classified layers. Rejects bundles that are missing a core layer,
// contain a duplicate core layer, or carry asset layers with unsafe or
// colliding title annotations. Unknown media types are ignored for
// forward compatibility.
func classifyBundleManifest(m ocispec.Manifest) (classifiedManifest, error) {
	var out classifiedManifest
	var haveToml, haveLock bool
	for _, layer := range m.Layers {
		switch layer.MediaType {
		case MediaTypePixiToml:
			if haveToml {
				return out, fmt.Errorf("invalid bundle: duplicate core layer")
			}
			out.pixiToml = layer
			haveToml = true
		case MediaTypePixiLock:
			if haveLock {
				return out, fmt.Errorf("invalid bundle: duplicate core layer")
			}
			out.pixiLock = layer
			haveLock = true
		case MediaTypeNebiAsset:
			out.assets = append(out.assets, layer)
		default:
			// Unknown media type — ignore for forward compat.
		}
	}
	if !haveToml || !haveLock {
		return out, fmt.Errorf("invalid bundle: missing pixi.{toml,lock}")
	}
	// Validate every asset title before fetch. This also rejects dupes
	// and case-insensitive collisions.
	paths := make([]string, 0, len(out.assets))
	for _, a := range out.assets {
		title := a.Annotations[ocispec.AnnotationTitle]
		paths = append(paths, title)
	}
	if err := ValidateAssetPaths(paths); err != nil {
		return out, fmt.Errorf("unsafe path in bundle: %w", err)
	}
	return out, nil
}

// PullBundle fetches a bundle from the registry in the requested mode.
// Always returns pixi.toml and pixi.lock; in PullModeFull also populates
// Assets with each layer's bytes. Asset blobs are fetched in parallel up
// to opts.Concurrency workers. Manifest config media type is verified to
// reject non-Nebi artifacts up front.
func PullBundle(ctx context.Context, repoRef, tag string, opts PullOptions) (*PullResult, error) {
	repo, err := remote.NewRepository(repoRef)
	if err != nil {
		return nil, fmt.Errorf("failed to create repository client: %w", err)
	}
	repo.PlainHTTP = opts.PlainHTTP
	if c := newAuthClient(opts.Username, opts.Password); c != nil {
		repo.Client = c
	}

	desc, err := repo.Resolve(ctx, tag)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve tag %s: %w", tag, err)
	}

	manifestReader, err := repo.Fetch(ctx, desc)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch manifest: %w", err)
	}
	manifestData, err := io.ReadAll(manifestReader)
	manifestReader.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}
	if manifest.Config.MediaType != MediaTypePixiConfig {
		return nil, fmt.Errorf("not a Nebi artifact")
	}

	cm, err := classifyBundleManifest(manifest)
	if err != nil {
		return nil, err
	}

	tomlBytes, err := fetchLayerBytes(ctx, repo, cm.pixiToml)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pixi.toml layer: %w", err)
	}
	lockBytes, err := fetchLayerBytes(ctx, repo, cm.pixiLock)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pixi.lock layer: %w", err)
	}

	result := &PullResult{
		PixiToml: strings.TrimSpace(string(tomlBytes)),
		PixiLock: strings.TrimSpace(string(lockBytes)),
	}

	// Always list asset paths; only populate Bytes in full mode.
	result.Assets = make([]AssetBlob, len(cm.assets))
	for i, a := range cm.assets {
		result.Assets[i] = AssetBlob{Path: a.Annotations[ocispec.AnnotationTitle]}
	}

	if opts.Mode == PullModeFull && len(cm.assets) > 0 {
		limit := opts.Concurrency
		if limit <= 0 {
			limit = defaultConcurrency
		}
		if err := fetchAssetsParallel(ctx, repo, cm.assets, result.Assets, limit); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// PullEnvironment is a backward-compatible alias that fetches only the
// pixi.toml + pixi.lock layers of a bundle. Asset layers (if any) are
// listed in the returned result's Assets slice but never fetched.
func PullEnvironment(ctx context.Context, repoRef, tag string, opts BrowseOptions) (*PullResult, error) {
	return PullBundle(ctx, repoRef, tag, PullOptions{
		Username: opts.Username,
		Password: opts.Password,
		Mode:     PullModePixiOnly,
	})
}

// fetchAssetsParallel fetches each asset blob concurrently and writes its
// bytes into the matching slot in out. First error cancels the rest.
func fetchAssetsParallel(
	ctx context.Context,
	repo *remote.Repository,
	assets []ocispec.Descriptor,
	out []AssetBlob,
	limit int,
) error {
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(limit)
	for i, a := range assets {
		i, a := i, a
		g.Go(func() error {
			b, err := fetchLayerBytes(ctx, repo, a)
			if err != nil {
				return fmt.Errorf("fetch asset %s: %w", out[i].Path, err)
			}
			out[i].Bytes = b
			return nil
		})
	}
	return g.Wait()
}

// IsNebiRepository checks if a repository contains a Nebi OCI image by inspecting
// the manifest config media type of the first available tag.
func IsNebiRepository(ctx context.Context, repoRef string, opts BrowseOptions) bool {
	repo, err := remote.NewRepository(repoRef)
	if err != nil {
		return false
	}

	if c := newAuthClient(opts.Username, opts.Password); c != nil {
		repo.Client = c
	}

	// Get the first tag only — errStopIteration is expected, so we ignore the error.
	var firstTag string
	_ = repo.Tags(ctx, "", func(tags []string) error {
		if len(tags) > 0 {
			firstTag = tags[0]
		}
		return errStopIteration
	})
	if firstTag == "" {
		return false
	}

	// Resolve the tag to get its manifest
	desc, err := repo.Resolve(ctx, firstTag)
	if err != nil {
		return false
	}

	manifestReader, err := repo.Fetch(ctx, desc)
	if err != nil {
		return false
	}
	defer manifestReader.Close()

	manifestData, err := io.ReadAll(manifestReader)
	if err != nil {
		return false
	}

	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return false
	}

	return manifest.Config.MediaType == MediaTypePixiConfig
}

// errStopIteration is a sentinel error used to stop tag pagination early.
var errStopIteration = fmt.Errorf("stop iteration")

// FilterNebiRepositories filters a list of repositories to only include those
// that contain Nebi OCI images. It checks repositories concurrently with a semaphore.
func FilterNebiRepositories(ctx context.Context, repos []RepositoryInfo, host string, opts BrowseOptions) []RepositoryInfo {
	type indexedResult struct {
		index int
		keep  bool
	}

	results := make([]indexedResult, len(repos))
	sem := make(chan struct{}, 10)
	var wg sync.WaitGroup

	for i, repo := range repos {
		wg.Add(1)
		go func(i int, repo RepositoryInfo) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}

			repoRef := fmt.Sprintf("%s/%s", host, repo.Name)
			results[i] = indexedResult{index: i, keep: IsNebiRepository(ctx, repoRef, opts)}
		}(i, repo)
	}

	wg.Wait()

	var filtered []RepositoryInfo
	for i, r := range results {
		if r.keep {
			filtered = append(filtered, repos[i])
		}
	}
	return filtered
}

// ChangeRepositoryVisibility changes the visibility of a repository on Quay.io.
// It calls the Quay.io API: POST /api/v1/repository/{repo}/changevisibility
func ChangeRepositoryVisibility(ctx context.Context, host, repoPath, apiToken string, isPublic bool) error {
	visibility := "private"
	if isPublic {
		visibility = "public"
	}

	apiURL := fmt.Sprintf("https://%s/api/v1/repository/%s/changevisibility", host, repoPath)
	body := fmt.Sprintf(`{"visibility": "%s"}`, visibility)

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+apiToken)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call visibility API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("visibility API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// fetchLayerBytes returns the full raw bytes for a layer descriptor.
func fetchLayerBytes(ctx context.Context, repo *remote.Repository, desc ocispec.Descriptor) ([]byte, error) {
	reader, err := repo.Fetch(ctx, desc)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}
