package oci

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// PublishFiles publishes a stored version (pixi.toml and pixi.lock
// contents, no project directory) to reg/repo:tag as a two-layer bundle,
// the same artifact PublishPixiOnly produces. The files are staged in a
// private temporary directory that is removed afterwards. Used by the
// client SDK, which publishes versions from the local database rather than
// whatever happens to be on disk.
func PublishFiles(
	ctx context.Context,
	pixiToml, pixiLock []byte,
	reg Registry,
	repo, tag string,
	opts ...PublishOption,
) (PublishResult, error) {
	if len(pixiToml) == 0 {
		return PublishResult{}, fmt.Errorf("pixi.toml is empty")
	}
	dir, err := os.MkdirTemp("", "nebi-publish-*")
	if err != nil {
		return PublishResult{}, fmt.Errorf("create staging directory: %w", err)
	}
	defer os.RemoveAll(dir)
	if err := os.WriteFile(filepath.Join(dir, "pixi.toml"), pixiToml, 0o644); err != nil {
		return PublishResult{}, fmt.Errorf("stage pixi.toml: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pixi.lock"), pixiLock, 0o644); err != nil {
		return PublishResult{}, fmt.Errorf("stage pixi.lock: %w", err)
	}
	return PublishPixiOnly(ctx, dir, reg, repo, tag, opts...)
}
