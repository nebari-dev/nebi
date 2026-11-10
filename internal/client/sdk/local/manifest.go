package local

// Owner: U1 (projects). Thin wrappers over internal/artifact for the
// form <-> pixi.toml round trip used by Create project and New version.

import (
	"context"
	"fmt"
	"strings"

	"github.com/nebari-dev/nebi/internal/artifact"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
)

// ParseManifest reads pixi.toml into the GUI form and reports the tables
// the form cannot show.
func (s *Service) ParseManifest(ctx context.Context, pixiToml string) (*shared.ManifestParseResult, error) {
	m, err := artifact.ParseManifest([]byte(pixiToml))
	if err != nil {
		return nil, &shared.ValidationError{
			Field:   artifact.FieldManifest,
			Message: fmt.Sprintf("pixi.toml isn’t valid TOML: %v", err),
		}
	}
	unrepresentable := m.Unrepresentable
	if unrepresentable == nil {
		unrepresentable = []string{}
	}
	return &shared.ManifestParseResult{Form: m.Form, Unrepresentable: unrepresentable}, nil
}

// RenderManifest renders the GUI form to pixi.toml, applying it onto
// req.BasePixiToml when set.
func (s *Service) RenderManifest(ctx context.Context, req shared.ManifestRenderRequest) (*shared.ManifestRenderResult, error) {
	form := req.Form
	if form.Channels == nil {
		form.Channels = []string{}
	}
	if form.Platforms == nil {
		form.Platforms = []string{}
	}
	requested := make([]shared.RequestedPackage, len(form.Requested))
	for i, p := range form.Requested {
		p.Constraint = artifact.NormalizeConstraint(p.Constraint)
		requested[i] = p
	}
	form.Requested = requested
	if strings.TrimSpace(req.BasePixiToml) == "" {
		return &shared.ManifestRenderResult{PixiToml: string(artifact.RenderManifest(form))}, nil
	}
	out, err := artifact.ApplyForm([]byte(req.BasePixiToml), form)
	if err != nil {
		return nil, &shared.ValidationError{Field: artifact.FieldManifest, Message: err.Error()}
	}
	return &shared.ManifestRenderResult{PixiToml: string(out)}, nil
}
