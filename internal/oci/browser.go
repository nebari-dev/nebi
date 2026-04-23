package oci

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/file"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
)

// emptyBlobDigest is the canonical sha256 of a zero-byte payload. Some
// registries (Quay, observed) refuse to serve this blob on GET, so
// ExtractBundle pre-pushes the empty content to the local store and
// tells oras.Copy to skip the fetch (see the PreCopy hook).
var emptyBlobDigest = digest.FromBytes(nil)

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

// PullOptions controls an OCI bundle pull.
type PullOptions struct {
	Username    string
	Password    string
	Concurrency int // ≤ 0 uses default (8)
	// PlainHTTP talks to the registry over HTTP. Test/local registries
	// only.
	PlainHTTP bool
}

// AssetBlob names a single asset layer in a bundle. It is a listing
// entry; the blob bytes are never carried in-memory — ExtractBundle
// streams each asset straight to disk.
type AssetBlob struct {
	Path string
}

// PullResult contains the metadata pulled from a registry tag. PixiToml
// and PixiLock hold the core layer contents (small text files, always
// buffered). Assets lists every asset layer's path; the blob bytes are
// not populated because asset payloads can be arbitrarily large and
// only make sense as on-disk files (see ExtractBundle).
type PullResult struct {
	PixiToml string      `json:"pixi_toml"`
	PixiLock string      `json:"pixi_lock"`
	Assets   []AssetBlob `json:"assets,omitempty"`
	// Digest is the sha256 of the manifest this result was built from.
	// Callers that peeked via PullBundle can hand it back to a
	// digest-pinned extract so a mutable tag cannot swap content out
	// from under them between calls.
	Digest string `json:"digest,omitempty"`
}

// ParseRegistryURL splits a registry URL into its host and optional namespace.
// For example, "quay.io/nebari" returns host="quay.io", namespace="nebari".
// A plain host like "quay.io" returns namespace="".
func ParseRegistryURL(rawURL string) (host, namespace string) {
	host, namespace, _ = ParseRegistryURLFull(rawURL)
	return
}

// ParseRegistryURLFull is like ParseRegistryURL but also reports whether
// the URL explicitly used the http:// scheme. Only an explicit http://
// is treated as opt-in plaintext; a scheme-less URL defaults to HTTPS.
// Intended so local/test registries can signal "plain HTTP" by writing
// their URL with http://.
func ParseRegistryURLFull(rawURL string) (host, namespace string, plainHTTP bool) {
	stripped, plainHTTP := StripScheme(rawURL)
	stripped = strings.TrimSuffix(stripped, "/")
	parts := strings.SplitN(stripped, "/", 2)
	host = parts[0]
	if len(parts) > 1 {
		namespace = parts[1]
	}
	return
}

// StripScheme removes an http:// or https:// prefix from an OCI reference
// and reports whether the original was plain HTTP. Scheme-less input is
// returned as-is with plainHTTP=false (defaults to HTTPS). Shared by the
// registry URL parser and the CLI import command.
func StripScheme(ref string) (stripped string, plainHTTP bool) {
	if strings.HasPrefix(ref, "http://") {
		return strings.TrimPrefix(ref, "http://"), true
	}
	return strings.TrimPrefix(ref, "https://"), false
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
	manifestDesc ocispec.Descriptor // the manifest itself — used to pin Copy by digest
	pixiToml     ocispec.Descriptor
	pixiLock     ocispec.Descriptor
	assets       []ocispec.Descriptor // path stored in AnnotationTitle
}

// classifyBundleManifest inspects a parsed OCI manifest and returns its
// classified layers. Rejects bundles that are missing a core layer,
// contain a duplicate core layer, carry asset layers with unsafe or
// colliding title annotations, carry an unknown media-type layer, or
// use the wrong AnnotationTitle on a core layer. Unknown media types
// are rejected rather than tolerated: the extract path delegates to
// oras.Copy which would still download them, so silently classifying
// them away gives a false sense of safety.
func classifyBundleManifest(m ocispec.Manifest) (classifiedManifest, error) {
	var out classifiedManifest
	var haveToml, haveLock bool
	for _, layer := range m.Layers {
		switch layer.MediaType {
		case MediaTypePixiToml:
			if haveToml {
				return out, fmt.Errorf("invalid bundle: duplicate core layer")
			}
			if title := layer.Annotations[ocispec.AnnotationTitle]; title != "pixi.toml" {
				return out, fmt.Errorf("invalid bundle: pixi.toml core layer has title %q, expected \"pixi.toml\"", title)
			}
			out.pixiToml = layer
			haveToml = true
		case MediaTypePixiLock:
			if haveLock {
				return out, fmt.Errorf("invalid bundle: duplicate core layer")
			}
			if title := layer.Annotations[ocispec.AnnotationTitle]; title != "pixi.lock" {
				return out, fmt.Errorf("invalid bundle: pixi.lock core layer has title %q, expected \"pixi.lock\"", title)
			}
			out.pixiLock = layer
			haveLock = true
		case MediaTypeNebiAsset:
			out.assets = append(out.assets, layer)
		default:
			return out, fmt.Errorf("invalid bundle: unknown media type %q", layer.MediaType)
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
	if err := validateAssetPaths(paths); err != nil {
		return out, fmt.Errorf("unsafe path in bundle: %w", err)
	}
	return out, nil
}

// resolveBundleManifest opens a remote repo, resolves the tag, fetches
// the manifest, verifies it's a Nebi artifact, and classifies its
// layers. Shared between PullBundle (metadata-only) and ExtractBundle
// (streaming pull) so both hit exactly one source of truth for auth,
// media-type gating, and path-safety validation.
func resolveBundleManifest(
	ctx context.Context,
	repoRef, tag string,
	opts PullOptions,
) (*remote.Repository, classifiedManifest, error) {
	var cm classifiedManifest
	repo, err := remote.NewRepository(repoRef)
	if err != nil {
		return nil, cm, fmt.Errorf("failed to create repository client: %w", err)
	}
	repo.PlainHTTP = opts.PlainHTTP
	if c := newAuthClient(opts.Username, opts.Password); c != nil {
		repo.Client = c
	}

	desc, err := repo.Resolve(ctx, tag)
	if err != nil {
		return nil, cm, fmt.Errorf("failed to resolve tag %s: %w", tag, err)
	}
	manifestReader, err := repo.Fetch(ctx, desc)
	if err != nil {
		return nil, cm, fmt.Errorf("failed to fetch manifest: %w", err)
	}
	manifestData, err := io.ReadAll(manifestReader)
	manifestReader.Close()
	if err != nil {
		return nil, cm, fmt.Errorf("failed to read manifest: %w", err)
	}
	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, cm, fmt.Errorf("failed to parse manifest: %w", err)
	}
	if manifest.Config.MediaType != MediaTypePixiConfig {
		return nil, cm, fmt.Errorf("not a Nebi artifact")
	}
	cm, err = classifyBundleManifest(manifest)
	if err != nil {
		return nil, cm, err
	}
	cm.manifestDesc = desc
	return repo, cm, nil
}

// assetListing converts classified asset layers into the path-only
// AssetBlob form returned by PullBundle and ExtractBundle.
func assetListing(assets []ocispec.Descriptor) []AssetBlob {
	out := make([]AssetBlob, len(assets))
	for i, a := range assets {
		out[i] = AssetBlob{Path: a.Annotations[ocispec.AnnotationTitle]}
	}
	return out
}

// PullBundle fetches a bundle's metadata from the registry: pixi.toml
// and pixi.lock contents (small text files, always buffered) plus the
// list of asset layer paths. Asset blob bytes are NOT fetched — callers
// that need the bytes on disk should use ExtractBundle instead.
func PullBundle(ctx context.Context, repoRef, tag string, opts PullOptions) (*PullResult, error) {
	repo, cm, err := resolveBundleManifest(ctx, repoRef, tag, opts)
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

	return &PullResult{
		PixiToml: strings.TrimSpace(string(tomlBytes)),
		PixiLock: strings.TrimSpace(string(lockBytes)),
		Assets:   assetListing(cm.assets),
		Digest:   cm.manifestDesc.Digest.String(),
	}, nil
}

// ExtractBundle pulls a bundle from the registry and writes every
// layer (pixi.toml, pixi.lock, each asset) to destDir, streaming each
// blob straight from the network to disk — no asset ever lands fully
// in RAM. Honors the layer's AnnotationTitle as the on-disk relative
// path. destDir is created if missing; caller is responsible for the
// "empty destination" policy (see cmd/nebi/import.go).
//
// Implementation delegates blob transfer to oras.Copy over a file.Store
// rooted at destDir. That gives us streamed I/O, parallel fetches
// configured via opts.Concurrency, and file.Store's
// AllowPathTraversalOnWrite=false default as defense-in-depth over the
// pre-validated titles.
func ExtractBundle(ctx context.Context, repoRef, tag, destDir string, opts PullOptions) (*PullResult, error) {
	repo, cm, err := resolveBundleManifest(ctx, repoRef, tag, opts)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, fmt.Errorf("create dest %s: %w", destDir, err)
	}
	fs, err := file.New(destDir)
	if err != nil {
		return nil, fmt.Errorf("create file store at %s: %w", destDir, err)
	}
	defer fs.Close()

	copyOpts := oras.DefaultCopyOptions
	if opts.Concurrency > 0 {
		copyOpts.Concurrency = opts.Concurrency
	} else {
		copyOpts.Concurrency = defaultConcurrency
	}
	// PreCopy short-circuits zero-byte blobs: we push the empty content
	// into the local file store and return SkipNode so oras.Copy never
	// issues the blob GET. This papers over registries that 404 the
	// canonical empty-blob digest (Quay), and — because we also verify
	// the descriptor's digest matches the canonical empty hash — it
	// rejects malformed manifests that claim Size=0 for non-empty data.
	copyOpts.PreCopy = func(ctx context.Context, desc ocispec.Descriptor) error {
		if desc.Size != 0 {
			return nil
		}
		if desc.Digest != emptyBlobDigest {
			return fmt.Errorf("zero-size layer %q has non-empty digest %s", desc.Annotations[ocispec.AnnotationTitle], desc.Digest)
		}
		if err := fs.Push(ctx, desc, bytes.NewReader(nil)); err != nil {
			return fmt.Errorf("write empty layer %q: %w", desc.Annotations[ocispec.AnnotationTitle], err)
		}
		return oras.SkipNode
	}
	// Copy by resolved manifest digest, not the original tag — pins the
	// extract to exactly the manifest we validated above, so a tag move
	// between classify and copy cannot swap in a different bundle.
	srcRef := cm.manifestDesc.Digest.String()
	if _, err := oras.Copy(ctx, repo, srcRef, fs, srcRef, copyOpts); err != nil {
		return nil, fmt.Errorf("extract bundle: %w", err)
	}

	// pixi.toml / pixi.lock are written by oras.Copy via their
	// AnnotationTitle. Read them back so callers get identical
	// contract to PullBundle (whitespace-trimmed strings).
	tomlBytes, err := os.ReadFile(filepath.Join(destDir, "pixi.toml"))
	if err != nil {
		return nil, fmt.Errorf("read extracted pixi.toml: %w", err)
	}
	lockBytes, err := os.ReadFile(filepath.Join(destDir, "pixi.lock"))
	if err != nil {
		return nil, fmt.Errorf("read extracted pixi.lock: %w", err)
	}
	return &PullResult{
		PixiToml: strings.TrimSpace(string(tomlBytes)),
		PixiLock: strings.TrimSpace(string(lockBytes)),
		Assets:   assetListing(cm.assets),
		Digest:   cm.manifestDesc.Digest.String(),
	}, nil
}

// PullEnvironment is a backward-compatible alias that fetches only the
// pixi.toml + pixi.lock layers of a bundle. Asset layers (if any) are
// listed in the returned result's Assets slice but never fetched.
func PullEnvironment(ctx context.Context, repoRef, tag string, opts BrowseOptions) (*PullResult, error) {
	return PullBundle(ctx, repoRef, tag, PullOptions{
		Username: opts.Username,
		Password: opts.Password,
	})
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
