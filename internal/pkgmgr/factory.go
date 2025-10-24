package pkgmgr

import (
	"fmt"
)

// FactoryFunc is a function that creates a new package manager instance
type FactoryFunc func(customPath string) (PackageManager, error)

var registry = make(map[string]FactoryFunc)

// Register registers a package manager factory function
func Register(name string, factory FactoryFunc) {
	registry[name] = factory
}

// New creates a package manager instance based on type
func New(pmType string) (PackageManager, error) {
	return NewWithPath(pmType, "")
}

// NewWithPath creates a package manager instance with a custom binary path
func NewWithPath(pmType string, customPath string) (PackageManager, error) {
	factory, ok := registry[pmType]
	if !ok {
		return nil, fmt.Errorf("unsupported package manager: %s", pmType)
	}
	return factory(customPath)
}
