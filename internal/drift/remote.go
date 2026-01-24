package drift

import (
	"context"
	"fmt"

	"github.com/aktech/darb/internal/cliclient"
	"github.com/aktech/darb/internal/nebifile"
)

// RemoteStatus represents the result of a remote comparison.
type RemoteStatus struct {
	// TagHasMoved is true if the tag now points to a different digest
	// than what was originally pulled.
	TagHasMoved bool `json:"tag_has_moved"`

	// OriginDigest is the manifest digest from when the repo was pulled.
	OriginDigest string `json:"origin_digest"`

	// CurrentTagDigest is what the tag currently resolves to on the server.
	CurrentTagDigest string `json:"current_tag_digest,omitempty"`

	// CurrentVersionID is the version ID the tag currently points to.
	CurrentVersionID int32 `json:"current_version_id,omitempty"`

	// Error is set if the remote check failed.
	Error string `json:"error,omitempty"`
}

// VersionContent holds the content fetched from the server for a specific version.
type VersionContent struct {
	PixiToml  string `json:"pixi_toml"`
	PixiLock  string `json:"pixi_lock"`
	VersionID int32  `json:"version_id"`
}

// ThreeWayStatus represents the full three-state comparison:
// LOCAL ↔ ORIGIN ↔ CURRENT TAG
type ThreeWayStatus struct {
	// Local is the drift status of local files vs origin
	Local *RepoStatus `json:"local"`

	// Remote is the status of the tag in the registry
	Remote *RemoteStatus `json:"remote"`

	// Summary combines local and remote status into a human-readable summary
	Summary string `json:"summary"`
}

// CheckRemote checks if the remote tag has moved since the repo was pulled.
// It compares the stored manifest digest against the current tag resolution.
func CheckRemote(ctx context.Context, client *cliclient.Client, nf *nebifile.NebiFile) *RemoteStatus {
	rs := &RemoteStatus{
		OriginDigest: nf.Origin.ManifestDigest,
	}

	// Find repo by listing environments
	envs, err := client.ListEnvironments(ctx)
	if err != nil {
		rs.Error = fmt.Sprintf("failed to list environments: %v", err)
		return rs
	}

	var envID string
	for _, env := range envs {
		if env.Name == nf.Origin.Repo {
			envID = env.ID
			break
		}
	}
	if envID == "" {
		rs.Error = fmt.Sprintf("repo %q not found on server", nf.Origin.Repo)
		return rs
	}

	// Get publications to find current digest for the tag
	pubs, err := client.GetEnvironmentPublications(ctx, envID)
	if err != nil {
		rs.Error = fmt.Sprintf("failed to get publications: %v", err)
		return rs
	}

	found := false
	for _, pub := range pubs {
		if pub.Tag == nf.Origin.Tag {
			rs.CurrentTagDigest = pub.Digest
			rs.CurrentVersionID = int32(pub.VersionNumber)
			found = true
			break
		}
	}

	if !found {
		rs.Error = fmt.Sprintf("tag %q not found in registry", nf.Origin.Tag)
		return rs
	}

	// Compare digests
	if rs.OriginDigest != "" && rs.CurrentTagDigest != "" {
		rs.TagHasMoved = rs.OriginDigest != rs.CurrentTagDigest
	}

	return rs
}

// FetchVersionContent fetches the pixi.toml and pixi.lock content for a specific
// version from the server. Used by nebi diff to get the original content.
func FetchVersionContent(ctx context.Context, client *cliclient.Client, envID string, versionID int32) (*VersionContent, error) {
	pixiToml, err := client.GetVersionPixiToml(ctx, envID, versionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get pixi.toml for version %d: %w", versionID, err)
	}

	pixiLock, err := client.GetVersionPixiLock(ctx, envID, versionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get pixi.lock for version %d: %w", versionID, err)
	}

	return &VersionContent{
		PixiToml:  pixiToml,
		PixiLock:  pixiLock,
		VersionID: versionID,
	}, nil
}

// FetchByTag resolves a tag to a version and fetches its content.
// This is used by nebi diff --remote.
func FetchByTag(ctx context.Context, client *cliclient.Client, repo, tag string) (*VersionContent, error) {
	// Find repo
	envs, err := client.ListEnvironments(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}

	var envID string
	for _, env := range envs {
		if env.Name == repo {
			envID = env.ID
			break
		}
	}
	if envID == "" {
		return nil, fmt.Errorf("repo %q not found", repo)
	}

	// Find publication for tag
	pubs, err := client.GetEnvironmentPublications(ctx, envID)
	if err != nil {
		return nil, fmt.Errorf("failed to get publications: %w", err)
	}

	var versionNumber int32
	found := false
	for _, pub := range pubs {
		if pub.Tag == tag {
			versionNumber = int32(pub.VersionNumber)
			found = true
			break
		}
	}

	if !found {
		return nil, fmt.Errorf("tag %q not found for repo %q", tag, repo)
	}

	return FetchVersionContent(ctx, client, envID, versionNumber)
}

// CheckThreeWay performs the full three-way comparison:
// 1. LOCAL vs ORIGIN (offline drift detection)
// 2. ORIGIN vs CURRENT TAG (remote tag mutation detection)
func CheckThreeWay(ctx context.Context, client *cliclient.Client, dir string, nf *nebifile.NebiFile) *ThreeWayStatus {
	ts := &ThreeWayStatus{}

	// Local drift check (offline)
	ts.Local = CheckWithNebiFile(dir, nf)

	// Remote check (network)
	ts.Remote = CheckRemote(ctx, client, nf)

	// Generate summary
	ts.Summary = generateSummary(ts)

	return ts
}

// generateSummary creates a human-readable summary of the three-way status.
func generateSummary(ts *ThreeWayStatus) string {
	localModified := ts.Local.Overall == StatusModified
	remoteError := ts.Remote.Error != ""
	tagMoved := ts.Remote.TagHasMoved

	switch {
	case remoteError:
		if localModified {
			return "modified locally (remote check failed)"
		}
		return "clean locally (remote check failed)"
	case localModified && tagMoved:
		return "modified locally AND remote tag has moved"
	case localModified && !tagMoved:
		return "modified locally, remote unchanged"
	case !localModified && tagMoved:
		return "clean locally, but remote tag has moved"
	default:
		return "clean (local matches origin, tag unchanged)"
	}
}
