package sdk

// Owner: U4 (compare). Compare two versions of a project; either may be a
// server-only version (fetched through c.Remote).

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/artifact"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
)

// compareServerLabelTimeout bounds the lookup of the server's latest
// version, which only labels the pickers and must never hold the diff up
// for the full HTTP timeout when the server is slow or unreachable.
const compareServerLabelTimeout = 5 * time.Second

// machinePlatform is this machine's pixi platform ("" when pixi has none).
// A variable so tests can pin it.
var machinePlatform = func() string {
	return artifact.DefaultPlatform(runtime.GOOS, runtime.GOARCH)
}

// Compare diffs the requested packages, channels and platforms of base and
// target (internal/artifact does the diff).
//
// A side that is not on this machine is fetched from the linked server
// (c.Remote.ServerVersionFiles). When the project has no server remote, no
// server is connected, or the remote SDK cannot serve version files, that
// side is reported as shared.ErrNotFound; server failures (unreachable,
// forbidden, missing) are returned as the remote SDK reports them.
//
// Resolved versions come from this machine's platform when either lock has
// it, else from the platform artifact.DiffVersions picks.
func (c *Client) Compare(ctx context.Context, projectID uuid.UUID, base, target int) (*shared.CompareResult, error) {
	if base < 1 {
		return nil, &shared.ValidationError{Field: "base", Message: "Choose a base version."}
	}
	if target < 1 {
		return nil, &shared.ValidationError{Field: "target", Message: "Choose a version to compare to."}
	}

	var p models.Project
	err := c.deps.DB.WithContext(ctx).Where("id = ?", projectID).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("project %s: %w", projectID, shared.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	linked, err := c.hasServerRemote(ctx, projectID)
	if err != nil {
		return nil, err
	}

	baseContent, baseServerOnly, err := c.compareSide(ctx, projectID, base, linked)
	if err != nil {
		return nil, err
	}
	targetContent, targetServerOnly, err := c.compareSide(ctx, projectID, target, linked)
	if err != nil {
		return nil, err
	}

	opts := artifact.DiffOptions{Platform: pickComparePlatform(baseContent, targetContent)}
	diff, err := artifact.DiffVersions(baseContent, targetContent, opts)
	if err != nil {
		return nil, fmt.Errorf("compare version %d with version %d: %w", base, target, err)
	}

	var serverLatest *int
	if linked {
		serverLatest = c.serverLatestVersion(ctx, p)
	}
	ref := func(n int, serverOnly bool) shared.CompareVersionRef {
		return shared.CompareVersionRef{
			Number:       n,
			ServerOnly:   serverOnly,
			Installed:    p.InstalledVersion != nil && *p.InstalledVersion == n,
			ServerLatest: serverLatest != nil && *serverLatest == n,
		}
	}
	return &shared.CompareResult{
		Base:        ref(base, baseServerOnly),
		Target:      ref(target, targetServerOnly),
		VersionDiff: *diff,
	}, nil
}

// compareSide loads one version's files: from the local database, else
// from the linked server. serverOnly reports the second case.
func (c *Client) compareSide(ctx context.Context, projectID uuid.UUID, n int, linked bool) (artifact.VersionContent, bool, error) {
	v, err := c.Local.LoadVersion(ctx, projectID, n)
	if err == nil {
		return artifact.VersionContent{Toml: []byte(v.ManifestContent), Lock: []byte(v.LockFileContent)}, false, nil
	}
	if !errors.Is(err, shared.ErrNotFound) {
		return artifact.VersionContent{}, false, err
	}
	if !linked {
		return artifact.VersionContent{}, false, fmt.Errorf("version %d: %w", n, shared.ErrNotFound)
	}
	toml, lock, err := c.Remote.ServerVersionFiles(ctx, projectID, n)
	switch {
	case err == nil:
		return artifact.VersionContent{Toml: []byte(toml), Lock: []byte(lock)}, true, nil
	case errors.Is(err, shared.ErrNotConnected):
		return artifact.VersionContent{}, false, fmt.Errorf("version %d is not on this machine and no server is connected: %w", n, shared.ErrNotFound)
	case errors.Is(err, shared.ErrNotImplemented):
		return artifact.VersionContent{}, false, fmt.Errorf("version %d is not on this machine: %w", n, shared.ErrNotFound)
	default:
		return artifact.VersionContent{}, false, err
	}
}

// hasServerRemote reports whether the project is linked to a server
// project, so compare only goes to the network for linked projects.
func (c *Client) hasServerRemote(ctx context.Context, projectID uuid.UUID) (bool, error) {
	var count int64
	err := c.deps.DB.WithContext(ctx).Model(&models.ProjectRemote{}).
		Where("project_id = ? AND kind = ?", projectID, models.RemoteKindServer).
		Count(&count).Error
	return count > 0, err
}

// serverLatestVersion asks the remote project decorator for the linked
// server's newest version. nil when unknown (not connected, unreachable).
func (c *Client) serverLatestVersion(ctx context.Context, p models.Project) *int {
	ctx, cancel := context.WithTimeout(ctx, compareServerLabelTimeout)
	defer cancel()
	items := []shared.ProjectListItem{{
		ID:                   p.ID,
		Name:                 p.Name,
		Path:                 p.Path,
		Status:               p.Status,
		InstalledVersion:     p.InstalledVersion,
		LastInstalledVersion: p.LastInstalledVersion,
		Remotes:              []shared.RemoteRef{},
		CreatedAt:            p.CreatedAt,
		UpdatedAt:            p.UpdatedAt,
	}}
	if err := c.Remote.DecorateProjects(ctx, items); err != nil {
		c.deps.Logger.Debug("compare: server latest version unavailable", "project", p.ID, "error", err)
		return nil
	}
	return items[0].ServerLatestVersion
}

// pickComparePlatform returns this machine's platform when either lock
// resolves it, else "" (artifact.DiffVersions then picks one). Lock parse
// errors are left for DiffVersions to report.
func pickComparePlatform(sides ...artifact.VersionContent) string {
	mp := machinePlatform()
	if mp == "" {
		return ""
	}
	for _, s := range sides {
		l, err := artifact.ParseLock(s.Lock)
		if err != nil {
			continue
		}
		if env := l.Environment(""); env != nil {
			if _, ok := env.Packages[mp]; ok {
				return mp
			}
		}
	}
	return ""
}
