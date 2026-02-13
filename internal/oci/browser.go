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

// PullResult contains the content pulled from a registry tag
type PullResult struct {
	PixiToml string `json:"pixi_toml"`
	PixiLock string `json:"pixi_lock"`
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

// ListRepositories queries the /v2/_catalog endpoint for a registry
func ListRepositories(ctx context.Context, opts BrowseOptions) ([]RepositoryInfo, error) {
	reg, err := remote.NewRegistry(opts.RegistryHost)
	if err != nil {
		return nil, fmt.Errorf("failed to create registry client: %w", err)
	}

	reg.Client = &auth.Client{
		Credential: func(ctx context.Context, hostname string) (auth.Credential, error) {
			return auth.Credential{
				Username: opts.Username,
				Password: opts.Password,
			}, nil
		},
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

	repo.Client = &auth.Client{
		Credential: func(ctx context.Context, hostname string) (auth.Credential, error) {
			return auth.Credential{
				Username: opts.Username,
				Password: opts.Password,
			}, nil
		},
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

// PullEnvironment fetches pixi.toml and pixi.lock content from a registry tag
func PullEnvironment(ctx context.Context, repoRef, tag string, opts BrowseOptions) (*PullResult, error) {
	repo, err := remote.NewRepository(repoRef)
	if err != nil {
		return nil, fmt.Errorf("failed to create repository client: %w", err)
	}

	repo.Client = &auth.Client{
		Credential: func(ctx context.Context, hostname string) (auth.Credential, error) {
			return auth.Credential{
				Username: opts.Username,
				Password: opts.Password,
			}, nil
		},
	}

	// Resolve the tag to a manifest descriptor
	desc, err := repo.Resolve(ctx, tag)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve tag %s: %w", tag, err)
	}

	// Fetch the manifest
	manifestReader, err := repo.Fetch(ctx, desc)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch manifest: %w", err)
	}
	defer manifestReader.Close()

	manifestData, err := io.ReadAll(manifestReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	// Parse the manifest to find layers
	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	result := &PullResult{}

	// Fetch each layer that matches our media types
	for _, layer := range manifest.Layers {
		switch layer.MediaType {
		case MediaTypePixiToml:
			content, err := fetchLayer(ctx, repo, layer)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch pixi.toml layer: %w", err)
			}
			result.PixiToml = content
		case MediaTypePixiLock:
			content, err := fetchLayer(ctx, repo, layer)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch pixi.lock layer: %w", err)
			}
			result.PixiLock = content
		}
	}

	if result.PixiToml == "" {
		return nil, fmt.Errorf("pixi.toml not found in manifest layers")
	}

	// Update the workspace name in pixi.toml to avoid conflicts
	// The caller can override the name if needed
	return result, nil
}

// IsNebiRepository checks if a repository contains a Nebi OCI image by inspecting
// the manifest config media type of the first available tag.
func IsNebiRepository(ctx context.Context, repoRef string, opts BrowseOptions) bool {
	repo, err := remote.NewRepository(repoRef)
	if err != nil {
		return false
	}

	repo.Client = &auth.Client{
		Credential: func(ctx context.Context, hostname string) (auth.Credential, error) {
			return auth.Credential{
				Username: opts.Username,
				Password: opts.Password,
			}, nil
		},
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

// fetchLayer fetches content from a single layer descriptor
func fetchLayer(ctx context.Context, repo *remote.Repository, desc ocispec.Descriptor) (string, error) {
	reader, err := repo.Fetch(ctx, desc)
	if err != nil {
		return "", err
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(data)), nil
}
