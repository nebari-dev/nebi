package pkgmgr

import (
	"context"
	"io"
)

// PackageManager is the interface that all package managers must implement
type PackageManager interface {
	// Name returns the package manager name (e.g., "pixi", "uv")
	Name() string

	// Init creates a new environment
	Init(ctx context.Context, opts InitOptions) error

	// Install adds packages to an environment
	Install(ctx context.Context, opts InstallOptions) error

	// Remove removes packages from an environment
	Remove(ctx context.Context, opts RemoveOptions) error

	// List returns installed packages
	List(ctx context.Context, opts ListOptions) ([]Package, error)

	// Update updates packages in an environment
	Update(ctx context.Context, opts UpdateOptions) error

	// GetManifest returns the parsed manifest file
	GetManifest(ctx context.Context, envPath string) (*Manifest, error)
}

// InitOptions contains parameters for initializing a new environment
type InitOptions struct {
	EnvPath   string   // Path where environment will be created
	Name      string   // Environment name
	Python    string   // Python version (if applicable)
	Channels  []string // Conda channels (pixi only)
	LogWriter io.Writer // Optional writer for streaming command output
}

// InstallOptions contains parameters for installing packages
type InstallOptions struct {
	EnvPath   string    // Path to environment
	Packages  []string  // Package names (e.g., "numpy==1.24.0")
	LogWriter io.Writer // Optional writer for streaming command output
}

// RemoveOptions contains parameters for removing packages
type RemoveOptions struct {
	EnvPath   string    // Path to environment
	Packages  []string  // Package names to remove
	LogWriter io.Writer // Optional writer for streaming command output
}

// ListOptions contains parameters for listing packages
type ListOptions struct {
	EnvPath string // Path to environment
}

// UpdateOptions contains parameters for updating packages
type UpdateOptions struct {
	EnvPath  string   // Path to environment
	Packages []string // Packages to update (empty = update all)
}

// Package represents an installed package
type Package struct {
	Name    string
	Version string
	Channel string // For conda-based managers
}

// Manifest represents a package manager manifest file
type Manifest struct {
	Name     string
	Packages map[string]string // name -> version
	Channels []string
	Raw      []byte // Raw manifest content
}
