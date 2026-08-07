package pkgmgr

import (
	"context"
	"fmt"
)

// FactoryFunc is a function that creates a new package manager instance
type FactoryFunc func(ctx context.Context, customPath string) (PackageManager, error)

var registry = make(map[string]FactoryFunc)

// Register registers a package manager factory function
func Register(name string, factory FactoryFunc) {
	registry[name] = factory
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
