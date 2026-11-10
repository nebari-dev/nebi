// Package exec is the client SDK for materializing environments: install a
// version (one installed at a time, replacing the current one) and
// uninstall (issue #649). Job submission goes through
// shared.Runtime.Jobs(); the job bodies (Run*) execute inside the worker or
// the CLI's inline runner.
//
// File ownership: service.go is frozen; install.go belongs to U2.
package exec

import (
	"github.com/nebari-dev/nebi/internal/client/sdk/local"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
)

// Service is the exec SDK.
type Service struct {
	deps  shared.Deps
	local *local.Service
}

// New builds the exec SDK.
func New(deps shared.Deps, l *local.Service) *Service {
	return &Service{deps: deps, local: l}
}
