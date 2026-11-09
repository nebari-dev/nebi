package local

// Owner: U1 (projects). Thin wrappers over internal/artifact for the
// form <-> pixi.toml round trip used by Create project and New version.

import (
	"context"

	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
)

// ParseManifest reads pixi.toml into the GUI form and reports the tables
// the form cannot show.
func (s *Service) ParseManifest(ctx context.Context, pixiToml string) (*shared.ManifestParseResult, error) {
	return nil, shared.NotImplemented("local.ParseManifest")
}

// RenderManifest renders the GUI form to pixi.toml, applying it onto
// req.BasePixiToml when set.
func (s *Service) RenderManifest(ctx context.Context, req shared.ManifestRenderRequest) (*shared.ManifestRenderResult, error) {
	return nil, shared.NotImplemented("local.RenderManifest")
}
