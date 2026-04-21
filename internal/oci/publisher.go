package oci

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"golang.org/x/sync/errgroup"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/file"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
)

const (
	// MediaTypePixiConfig is the media type for pixi config (empty JSON)
	MediaTypePixiConfig = "application/vnd.pixi.config.v1+toml"
	// MediaTypePixiToml is the media type for pixi.toml manifest
	MediaTypePixiToml = "application/vnd.pixi.toml.v1+toml"
	// MediaTypePixiLock is the media type for pixi.lock lockfile
	MediaTypePixiLock = "application/vnd.pixi.lock.v1+yaml"
	// MediaTypeNebiAsset is the media type for arbitrary bundled workspace
	// files. Path is carried in the layer's AnnotationTitle.
	MediaTypeNebiAsset = "application/vnd.nebi.asset.v1"
)

// defaultConcurrency is the fallback parallelism for blob transfers when
// PublishOptions.Concurrency (or ImportOptions.Concurrency) is ≤ 0.
const defaultConcurrency = 8

// PublishOptions contains options for publishing a workspace
type PublishOptions struct {
	Repository   string   // Full repository path (e.g., "ghcr.io/myorg/myenv")
	Tag          string   // Primary tag for the manifest (e.g., "sha-a1b2c3d4e5f6")
	ExtraTags    []string // Additional tags to apply (e.g., ["latest"])
	Username     string   // Registry username
	Password     string   // Registry password/token
	RegistryHost string   // Registry hostname (e.g., "ghcr.io")

	// Assets are additional files (relative path + absolute source path) to
	// bundle as layers with MediaTypeNebiAsset. pixi.toml and pixi.lock must
	// NOT appear here — they are always read from envPath and emitted as
	// typed layers 0 and 1. When nil, the published artifact is a classic
	// 2-layer bundle, byte-compatible with pre-bundle readers.
	Assets []AssetFile

	// Concurrency bounds parallel blob pushes. ≤ 0 means default (8).
	Concurrency int

	// PlainHTTP talks to the registry over HTTP instead of HTTPS. Only
	// intended for local registries (dev, tests). Never set this for
	// public registries.
	PlainHTTP bool
}

// PublishWorkspace publishes a workspace as an OCI bundle. The bundle
// always contains pixi.toml and pixi.lock as the first two layers with
// typed media types; opts.Assets contribute additional MediaTypeNebiAsset
// layers. Blob pushes are parallelized up to opts.Concurrency.
func PublishWorkspace(ctx context.Context, envPath string, opts PublishOptions) (string, error) {
	pixiTomlPath := filepath.Join(envPath, "pixi.toml")
	pixiLockPath := filepath.Join(envPath, "pixi.lock")

	if _, err := os.Stat(pixiTomlPath); os.IsNotExist(err) {
		return "", fmt.Errorf("pixi.toml not found in %s", envPath)
	}
	if _, err := os.Stat(pixiLockPath); os.IsNotExist(err) {
		return "", fmt.Errorf("pixi.lock not found in %s", envPath)
	}

	// Validate every asset path pre-network. One violation aborts the
	// whole publish. The validator also rejects dupes and case-insensitive
	// collisions within the same bundle.
	assetPaths := make([]string, 0, len(opts.Assets))
	for _, a := range opts.Assets {
		assetPaths = append(assetPaths, a.RelPath)
	}
	if err := ValidateAssetPaths(assetPaths); err != nil {
		return "", fmt.Errorf("unsafe path in bundle: %w", err)
	}

	fs, err := file.New(envPath)
	if err != nil {
		return "", fmt.Errorf("failed to create file store: %w", err)
	}
	defer fs.Close()

	// Core layers.
	tomlDesc, err := fs.Add(ctx, "pixi.toml", MediaTypePixiToml, "")
	if err != nil {
		return "", fmt.Errorf("failed to add pixi.toml: %w", err)
	}
	tomlDesc.Annotations = map[string]string{ocispec.AnnotationTitle: "pixi.toml"}

	lockDesc, err := fs.Add(ctx, "pixi.lock", MediaTypePixiLock, "")
	if err != nil {
		return "", fmt.Errorf("failed to add pixi.lock: %w", err)
	}
	lockDesc.Annotations = map[string]string{ocispec.AnnotationTitle: "pixi.lock"}

	layers := make([]ocispec.Descriptor, 0, 2+len(opts.Assets))
	layers = append(layers, tomlDesc, lockDesc)

	// Asset layers. We pass the on-disk absolute path to the file store
	// via its Add(name, mediaType, path) API, which hashes/sizes without
	// copying. Annotation title holds the bundle-relative path.
	assetDescs := make([]ocispec.Descriptor, 0, len(opts.Assets))
	for _, asset := range opts.Assets {
		// Skip core files if accidentally included by caller — they are
		// already layers 0/1.
		if asset.RelPath == "pixi.toml" || asset.RelPath == "pixi.lock" {
			continue
		}
		desc, err := fs.Add(ctx, asset.RelPath, MediaTypeNebiAsset, asset.AbsPath)
		if err != nil {
			return "", fmt.Errorf("cannot read %s: %w", asset.RelPath, err)
		}
		desc.Annotations = map[string]string{ocispec.AnnotationTitle: asset.RelPath}
		layers = append(layers, desc)
		assetDescs = append(assetDescs, desc)
	}

	// Config descriptor (empty JSON).
	configData := []byte("{}")
	configDesc := ocispec.Descriptor{
		MediaType: MediaTypePixiConfig,
		Digest:    digest.FromBytes(configData),
		Size:      int64(len(configData)),
	}
	if err := fs.Push(ctx, configDesc, bytes.NewReader(configData)); err != nil {
		return "", fmt.Errorf("failed to push config: %w", err)
	}

	manifestDesc, err := oras.Pack(ctx, fs, "", layers, oras.PackOptions{
		ConfigDescriptor:  &configDesc,
		PackImageManifest: true,
		ManifestAnnotations: map[string]string{
			ocispec.AnnotationDescription: fmt.Sprintf("%s:%s", opts.Repository, opts.Tag),
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to pack manifest: %w", err)
	}

	repo, err := remote.NewRepository(opts.Repository)
	if err != nil {
		return "", fmt.Errorf("failed to create repository: %w", err)
	}
	repo.PlainHTTP = opts.PlainHTTP
	repo.Client = &auth.Client{
		Credential: func(ctx context.Context, hostname string) (auth.Credential, error) {
			return auth.Credential{
				Username: opts.Username,
				Password: opts.Password,
			}, nil
		},
	}

	// Parallel blob push. Config, pixi.toml, pixi.lock, and every asset
	// are pushed as independent jobs; first error aborts the rest.
	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = defaultConcurrency
	}
	blobs := make([]blobJob, 0, 3+len(assetDescs))
	blobs = append(blobs,
		blobJob{desc: configDesc, label: "config"},
		blobJob{desc: tomlDesc, label: "pixi.toml"},
		blobJob{desc: lockDesc, label: "pixi.lock"},
	)
	for i, d := range assetDescs {
		blobs = append(blobs, blobJob{desc: d, label: opts.Assets[i].RelPath})
	}

	if err := pushBlobsParallel(ctx, fs, repo, blobs, concurrency); err != nil {
		return "", err
	}

	// Push manifest with tag. Extra tags are applied sequentially after.
	manifestReader, err := fs.Fetch(ctx, manifestDesc)
	if err != nil {
		return "", fmt.Errorf("failed to fetch manifest: %w", err)
	}
	if err := repo.PushReference(ctx, manifestDesc, manifestReader, opts.Tag); err != nil {
		return "", fmt.Errorf("failed to push manifest: %w", err)
	}
	for _, extraTag := range opts.ExtraTags {
		if err := repo.Tag(ctx, manifestDesc, extraTag); err != nil {
			return "", fmt.Errorf("failed to tag manifest as %q: %w", extraTag, err)
		}
	}
	return manifestDesc.Digest.String(), nil
}

// blobJob is one blob to push from the file store to the remote repo.
type blobJob struct {
	desc  ocispec.Descriptor
	label string // path or role for error messages
}

// pushBlobsParallel pushes all blobs concurrently up to `limit` in flight.
// First error cancels the rest via errgroup context; returned error is
// annotated with the blob's label.
func pushBlobsParallel(
	ctx context.Context,
	fs *file.Store,
	repo *remote.Repository,
	jobs []blobJob,
	limit int,
) error {
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(limit)
	for _, j := range jobs {
		j := j
		g.Go(func() error {
			r, err := fs.Fetch(ctx, j.desc)
			if err != nil {
				return fmt.Errorf("fetch local %s: %w", j.label, err)
			}
			defer r.Close()
			if err := repo.Push(ctx, j.desc, r); err != nil {
				return fmt.Errorf("failed to push %s: %w", j.label, err)
			}
			return nil
		})
	}
	return g.Wait()
}
