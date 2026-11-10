// Package registry is the client SDK for OCI registries: configured
// registries (oci_registries in the local database, passwords in the
// keyring), publish a version to several registries with per-registry
// results, unpublish with a per-registry capability check (gap 6), pull a
// project from a registry, and the local "Published" badge data
// (issue #649).
//
// File ownership: service.go is frozen; registries.go, publish.go and
// pull.go belong to U7.
package registry

import (
	"github.com/nebari-dev/nebi/internal/client/sdk/local"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
)

// Service is the registry SDK.
type Service struct {
	deps  shared.Deps
	local *local.Service
}

// New builds the registry SDK.
func New(deps shared.Deps, l *local.Service) *Service {
	return &Service{deps: deps, local: l}
}
