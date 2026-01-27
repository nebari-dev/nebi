package uv

import (
	"context"
	"errors"

	"github.com/nebari-dev/nebi/internal/pkgmgr"
)

func init() {
	pkgmgr.Register("uv", func(customPath string) (pkgmgr.PackageManager, error) {
		return NewWithPath(customPath)
	})
}

// ErrNotImplemented is returned for all UV operations (to be implemented in future)
var ErrNotImplemented = errors.New("uv support not yet implemented")

// UvManager is a stub implementation for the uv package manager
type UvManager struct{}

// New creates a new UvManager instance
func New() (*UvManager, error) {
	return &UvManager{}, nil
}

// NewWithPath creates a new UvManager with a custom uv binary path
func NewWithPath(customPath string) (*UvManager, error) {
	return &UvManager{}, nil
}

// Name returns the package manager name
func (u *UvManager) Name() string {
	return "uv"
}

// Init creates a new environment (not yet implemented)
func (u *UvManager) Init(ctx context.Context, opts pkgmgr.InitOptions) error {
	return ErrNotImplemented
}

// Install adds packages to an environment (not yet implemented)
func (u *UvManager) Install(ctx context.Context, opts pkgmgr.InstallOptions) error {
	return ErrNotImplemented
}

// Remove removes packages from an environment (not yet implemented)
func (u *UvManager) Remove(ctx context.Context, opts pkgmgr.RemoveOptions) error {
	return ErrNotImplemented
}

// List returns installed packages (not yet implemented)
func (u *UvManager) List(ctx context.Context, opts pkgmgr.ListOptions) ([]pkgmgr.Package, error) {
	return nil, ErrNotImplemented
}

// Update updates packages in an environment (not yet implemented)
func (u *UvManager) Update(ctx context.Context, opts pkgmgr.UpdateOptions) error {
	return ErrNotImplemented
}

// GetManifest returns the parsed manifest file (not yet implemented)
func (u *UvManager) GetManifest(ctx context.Context, envPath string) (*pkgmgr.Manifest, error) {
	return nil, ErrNotImplemented
}
