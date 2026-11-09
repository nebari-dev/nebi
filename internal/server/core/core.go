// Package core is the team-server business logic added for the client
// rework (issue #649): version sync for push/pull, the project catalog,
// sharing with roles Owner / Can edit / Can view, and publication metadata
// for the "Published" badge. It must never import internal/client/...;
// pure helpers shared with the client live in internal/artifact.
//
// Legacy server logic still lives in internal/service; core may call it.
//
// File ownership: core.go, errors.go and types.go are frozen; sync.go
// belongs to U5, sharing.go to U6, publications.go to U7.
package core

import (
	"log/slog"

	"github.com/nebari-dev/nebi/internal/rbac"
	"github.com/nebari-dev/nebi/internal/service"
	"gorm.io/gorm"
)

// Deps are the server resources core is built on.
type Deps struct {
	DB       *gorm.DB
	RBAC     rbac.Provider
	Projects *service.ProjectService
	Logger   *slog.Logger
}

// Service is the server core.
type Service struct {
	deps Deps
}

// New builds the server core.
func New(deps Deps) *Service {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	return &Service{deps: deps}
}

// DB returns the server database.
func (s *Service) DB() *gorm.DB { return s.deps.DB }
