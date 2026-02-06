package oci

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
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
)

// PublishOptions contains options for publishing a workspace
type PublishOptions struct {
	Repository   string // Full repository path (e.g., "ghcr.io/myorg/myenv")
	Tag          string // Tag for the manifest (e.g., "v1.0.0")
	Username     string // Registry username
	Password     string // Registry password/token
	RegistryHost string // Registry hostname (e.g., "ghcr.io")
}

// PublishWorkspace publishes pixi.toml and pixi.lock to an OCI registry
func PublishWorkspace(ctx context.Context, envPath string, opts PublishOptions) (string, error) {
	// Validate that pixi files exist
	pixiTomlPath := filepath.Join(envPath, "pixi.toml")
	pixiLockPath := filepath.Join(envPath, "pixi.lock")

	if _, err := os.Stat(pixiTomlPath); os.IsNotExist(err) {
		return "", fmt.Errorf("pixi.toml not found in %s", envPath)
	}
	if _, err := os.Stat(pixiLockPath); os.IsNotExist(err) {
		return "", fmt.Errorf("pixi.lock not found in %s", envPath)
	}

	// Create a file store from the environment directory
	fs, err := file.New(envPath)
	if err != nil {
		return "", fmt.Errorf("failed to create file store: %w", err)
	}
	defer fs.Close()

	// Add pixi.toml as a layer
	tomlDesc, err := fs.Add(ctx, "pixi.toml", MediaTypePixiToml, "")
	if err != nil {
		return "", fmt.Errorf("failed to add pixi.toml: %w", err)
	}
	tomlDesc.Annotations = map[string]string{
		ocispec.AnnotationTitle: "pixi.toml",
	}

	// Add pixi.lock as a layer
	lockDesc, err := fs.Add(ctx, "pixi.lock", MediaTypePixiLock, "")
	if err != nil {
		return "", fmt.Errorf("failed to add pixi.lock: %w", err)
	}
	lockDesc.Annotations = map[string]string{
		ocispec.AnnotationTitle: "pixi.lock",
	}

	// Create config descriptor (empty JSON)
	configData := []byte("{}")
	configDesc := ocispec.Descriptor{
		MediaType: MediaTypePixiConfig,
		Digest:    digest.FromBytes(configData),
		Size:      int64(len(configData)),
	}

	// Push config to store
	if err := fs.Push(ctx, configDesc, bytes.NewReader(configData)); err != nil {
		return "", fmt.Errorf("failed to push config: %w", err)
	}

	// Pack the manifest in the file store
	manifestDesc, err := oras.Pack(ctx, fs, "", []ocispec.Descriptor{tomlDesc, lockDesc}, oras.PackOptions{
		ConfigDescriptor:  &configDesc,
		PackImageManifest: true, // Use OCI Image Manifest format
		ManifestAnnotations: map[string]string{
			ocispec.AnnotationDescription: fmt.Sprintf("%s:%s", opts.Repository, opts.Tag),
		},
	})
	if err != nil {
		return "", fmt.Errorf("failed to pack manifest: %w", err)
	}

	// Parse repository reference
	repo, err := remote.NewRepository(opts.Repository)
	if err != nil {
		return "", fmt.Errorf("failed to create repository: %w", err)
	}

	// Configure authentication with credential function
	// This properly handles OAuth2 token exchange for registries like Quay.io
	repo.Client = &auth.Client{
		Credential: func(ctx context.Context, hostname string) (auth.Credential, error) {
			return auth.Credential{
				Username: opts.Username,
				Password: opts.Password,
			}, nil
		},
	}

	// Push blobs and manifest directly to avoid existence check issues with new repos
	// Push config blob
	configReader, err := fs.Fetch(ctx, configDesc)
	if err != nil {
		return "", fmt.Errorf("failed to fetch config: %w", err)
	}
	if err := repo.Push(ctx, configDesc, configReader); err != nil {
		return "", fmt.Errorf("failed to push config to registry: %w", err)
	}

	// Push pixi.toml blob
	tomlReader, err := fs.Fetch(ctx, tomlDesc)
	if err != nil {
		return "", fmt.Errorf("failed to fetch pixi.toml: %w", err)
	}
	if err := repo.Push(ctx, tomlDesc, tomlReader); err != nil {
		return "", fmt.Errorf("failed to push pixi.toml to registry: %w", err)
	}

	// Push pixi.lock blob
	lockReader, err := fs.Fetch(ctx, lockDesc)
	if err != nil {
		return "", fmt.Errorf("failed to fetch pixi.lock: %w", err)
	}
	if err := repo.Push(ctx, lockDesc, lockReader); err != nil {
		return "", fmt.Errorf("failed to push pixi.lock to registry: %w", err)
	}

	// Push manifest
	manifestReader, err := fs.Fetch(ctx, manifestDesc)
	if err != nil {
		return "", fmt.Errorf("failed to fetch manifest: %w", err)
	}
	if err := repo.Push(ctx, manifestDesc, manifestReader); err != nil {
		return "", fmt.Errorf("failed to push manifest to registry: %w", err)
	}

	// Tag the manifest in the remote repository
	if err := repo.Tag(ctx, manifestDesc, opts.Tag); err != nil {
		return "", fmt.Errorf("failed to tag manifest: %w", err)
	}

	// Return the manifest digest
	return manifestDesc.Digest.String(), nil
}
