package pkgmgr

import (
	"context"
	"fmt"
)

// FactoryFunc is a function that creates a new package manager instance
type FactoryFunc func(ctx context.Context, customPath string) (PackageManager, error)

var registry = make(map[string]FactoryFunc)
var manifestParsers = make(map[string]ManifestContentParser)

// ManifestContentParser validates and inspects manifest content without
// requiring a package-manager binary instance.
type ManifestContentParser struct {
	PackageNames           func(content string) ([]string, error)
	DefaultDependencyNames func(content string) ([]string, error)
}

// Register registers a package manager factory function
func Register(name string, factory FactoryFunc) {
	registry[name] = factory
}

// RegisterManifestContentParser registers manifest parsing hooks for a package
// manager. Package managers with a manifest format should register one from
// their init function.
func RegisterManifestContentParser(name string, parser ManifestContentParser) {
	manifestParsers[name] = parser
}

// ManifestPackageNames returns every package key declared in a manifest for
// the selected package manager.
func ManifestPackageNames(pmType string, content string) ([]string, error) {
	if content == "" {
		return nil, nil
	}
	parser, ok := manifestParsers[pmType]
	if !ok || parser.PackageNames == nil {
		return nil, fmt.Errorf("manifest validation is not supported for package manager %q", pmType)
	}
	return parser.PackageNames(content)
}

// ManifestDefaultDependencyNames returns package keys declared in the default
// dependency section for the selected package manager.
func ManifestDefaultDependencyNames(pmType string, content string) ([]string, error) {
	if content == "" {
		return nil, nil
	}
	parser, ok := manifestParsers[pmType]
	if !ok || parser.DefaultDependencyNames == nil {
		return nil, fmt.Errorf("manifest validation is not supported for package manager %q", pmType)
	}
	return parser.DefaultDependencyNames(content)
}

// New creates a package manager instance based on type
func New(pmType string) (PackageManager, error) {
	return NewWithContext(context.Background(), pmType)
}

// NewWithContext creates a package manager instance using the provided context
// for setup work such as binary verification or auto-installation.
func NewWithContext(ctx context.Context, pmType string) (PackageManager, error) {
	return NewWithPathContext(ctx, pmType, "")
}

// NewWithPath creates a package manager instance with a custom binary path
func NewWithPath(pmType string, customPath string) (PackageManager, error) {
	return NewWithPathContext(context.Background(), pmType, customPath)
}

// NewWithPathContext creates a package manager instance with a custom binary
// path, using the provided context for setup work.
func NewWithPathContext(ctx context.Context, pmType string, customPath string) (PackageManager, error) {
	factory, ok := registry[pmType]
	if !ok {
		return nil, fmt.Errorf("unsupported package manager: %s", pmType)
	}
	return factory(ctx, customPath)
}
