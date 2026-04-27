package oci

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"github.com/nebari-dev/nebi/internal/contenthash"
)

// PreviewAssetRefs walks a workspace directory with the same filters
// Publish applies, and returns each asset (paths relative to workspaceDir,
// forward-slash) paired with its SHA-256 content digest. pixi.toml and
// pixi.lock are stripped — callers fold those in through
// contenthash.HashBundle's first two arguments. The digest is the SHA-256
// of the file bytes, not an OCI blob digest, but both the CLI-local
// publish path and the server publish-defaults derive the hash the same
// way from the same bytes, so the resulting tag is stable across paths.
func PreviewAssetRefs(workspaceDir string) ([]contenthash.AssetRef, error) {
	assets, err := Preview(context.Background(), workspaceDir)
	if err != nil {
		return nil, err
	}
	out := make([]contenthash.AssetRef, 0, len(assets))
	for _, a := range assets {
		if a.RelPath == "pixi.toml" || a.RelPath == "pixi.lock" {
			continue
		}
		sum, err := fileSHA256Preview(a.AbsPath)
		if err != nil {
			return nil, fmt.Errorf("hash %s: %w", a.RelPath, err)
		}
		out = append(out, contenthash.AssetRef{Path: a.RelPath, Digest: "sha256:" + sum})
	}
	return out, nil
}

func fileSHA256Preview(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
