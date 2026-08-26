package service

import (
	"context"
	"fmt"

	"github.com/nebari-dev/nebi/internal/pixi"
)

// pixiListPackages lists installed packages via the pixi CLI. It is a
// package-level variable so tests can stub out the pixi binary dependency.
var pixiListPackages = func(ctx context.Context, opts pixi.ListOptions) ([]pixi.Package, error) {
	pm, err := pixi.NewWithPathContext(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create pixi manager: %w", err)
	}
	return pm.List(ctx, opts)
}

// SetPixiListPackagesForTests replaces the pixi list implementation and
// returns a function that restores the previous one. Test use only.
func SetPixiListPackagesForTests(f func(context.Context, pixi.ListOptions) ([]pixi.Package, error)) func() {
	prev := pixiListPackages
	pixiListPackages = f
	return func() { pixiListPackages = prev }
}
