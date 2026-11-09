// Package sdk is the client SDK facade (issue #649). The client REST API
// (internal/client/api), the job handlers (internal/client/jobs) and the
// CLI (cmd/nebi-cli) all call the SDK through a *Client; none of them talk
// to the database, pixi or the team server directly.
//
// Sub-packages: local (this machine's projects, versions, tags, manifests,
// persistence), exec (install / uninstall), remote (team server), registry
// (OCI registries). Shared types and errors are in sdk/shared.
//
// Dependency direction: internal/client/jobs imports the SDK, never the
// reverse; the SDK starts jobs through shared.JobSubmitter
// (Deps.Runtime.Jobs()).
//
// File ownership: sdk.go is frozen; compare.go belongs to U4.
package sdk

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/auth"
	"github.com/nebari-dev/nebi/internal/client/sdk/exec"
	"github.com/nebari-dev/nebi/internal/client/sdk/local"
	"github.com/nebari-dev/nebi/internal/client/sdk/registry"
	"github.com/nebari-dev/nebi/internal/client/sdk/remote"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/store"
)

// Client is the SDK facade.
type Client struct {
	Local    *local.Service
	Exec     *exec.Service
	Remote   *remote.Service
	Registry *registry.Service

	deps shared.Deps
}

// New builds the SDK on deps (DB, Executor and DataDir are required).
// It resolves the local user, defaults Credentials, Logger and Runtime, and
// registers the cross-unit decorators.
func New(deps shared.Deps) (*Client, error) {
	if deps.DB == nil {
		return nil, fmt.Errorf("sdk: DB is required")
	}
	if deps.Runtime == nil {
		deps.Runtime = &shared.Runtime{}
	}
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	if deps.DataDir == "" {
		dir, err := store.DefaultDataDir()
		if err != nil {
			return nil, fmt.Errorf("sdk: data dir: %w", err)
		}
		deps.DataDir = dir
	}
	if deps.Credentials == nil {
		deps.Credentials = &lazyCredentials{dataDir: deps.DataDir}
	}
	if deps.LocalUserID == uuid.Nil {
		la, err := auth.NewLocalAuthenticator(deps.DB)
		if err != nil {
			return nil, fmt.Errorf("sdk: local user: %w", err)
		}
		deps.LocalUserID = la.User().ID
	}

	l := local.New(deps)
	c := &Client{
		Local:    l,
		Exec:     exec.New(deps, l),
		Remote:   remote.New(deps, l),
		Registry: registry.New(deps, l),
		deps:     deps,
	}
	// Order matters: local publications first, then the server view
	// (which may append server-only versions and server publications).
	deps.Runtime.AddVersionDecorator(c.Registry)
	deps.Runtime.AddVersionDecorator(c.Remote)
	deps.Runtime.AddProjectDecorator(c.Remote)
	return c, nil
}

// Deps returns the resolved dependencies.
func (c *Client) Deps() shared.Deps { return c.deps }

// Runtime returns the late-bound hooks (job submitter, decorators).
func (c *Client) Runtime() *shared.Runtime { return c.deps.Runtime }

// lazyCredentials defers store.NewCredentialStore (which probes the OS
// keyring) until a credential is actually used, so starting the app or the
// CLI never touches the keychain.
type lazyCredentials struct {
	dataDir string
	once    sync.Once
	store   store.CredentialStore
}

func (l *lazyCredentials) get() store.CredentialStore {
	l.once.Do(func() { l.store = store.NewCredentialStore(l.dataDir) })
	return l.store
}

func (l *lazyCredentials) SetPassword(name, password string) error {
	return l.get().SetPassword(name, password)
}

func (l *lazyCredentials) GetPassword(name string) (string, error) {
	return l.get().GetPassword(name)
}

func (l *lazyCredentials) DeletePassword(name string) error {
	return l.get().DeletePassword(name)
}
