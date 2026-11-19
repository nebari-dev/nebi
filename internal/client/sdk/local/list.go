package local

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client/jobs"
)

// State is a project's environment state. It is derived on every read from
// the version count, the project's jobs, InstalledVersionID and the presence
// of .pixi/envs, and never stored.
type State string

const (
	// StateCreating: the create job is pending or running.
	StateCreating State = "creating"
	// StateLockFailed: the project has no versions because the create job
	// failed or was cancelled (or did not survive a restart). JobID is that
	// job, when known.
	StateLockFailed State = "lock_failed"
	// StateInstalling: an install is pending or running.
	StateInstalling State = "installing"
	// StateUninstalling: an uninstall is pending or running.
	StateUninstalling State = "uninstalling"
	// StateInstallFailed: the last install failed or was cancelled. JobID
	// is that job.
	StateInstallFailed State = "install_failed"
	// StateInstalled: nebi installed a version (InstalledVersionID is set)
	// and its environment is on disk (.pixi/envs exists).
	StateInstalled State = "installed"
	// StateNotInstalled: there are versions but no environment that nebi
	// installed. This includes a .pixi/envs left behind by a failed or
	// interrupted install whose job did not survive a restart, and one
	// created by running pixi outside nebi: in both cases nebi does not
	// know which version, if any, is on disk.
	StateNotInstalled State = "not_installed"
)

// RemoteRef is a link from a local project to a project on a nebi server.
// Links are owned by sdk/remote; until it lands, Remotes is always empty.
type RemoteRef struct {
	ServerURL       string `json:"server_url"`
	RemoteProjectID string `json:"remote_project_id"`
}

// ProjectSummary is one row of the project list.
type ProjectSummary struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	Path string    `json:"path"`

	State State `json:"state"`
	// JobID is the job behind State: the running create or install job, or
	// the failed one for StateLockFailed and StateInstallFailed.
	JobID *uuid.UUID `json:"job_id,omitempty"`

	// InstalledVersion and LatestVersion are version numbers; nil when
	// nothing is installed or there are no versions.
	InstalledVersion *int `json:"installed_version"`
	LatestVersion    *int `json:"latest_version"`

	// SizeBytes is the cached size of Path on disk. Nil until it has been
	// computed: sizes are computed in the background, so a project's first
	// listing has no size.
	SizeBytes *int64 `json:"size_bytes"`

	// Remotes are the server projects this project is linked to. Never nil.
	Remotes []RemoteRef `json:"remotes"`

	CreatedAt time.Time `json:"created_at"`
}

// ProjectDetail is one project with everything the list shows and more.
type ProjectDetail struct {
	ProjectSummary
	ManagedDir         bool       `json:"managed_dir"`
	InstalledVersionID *uuid.UUID `json:"installed_version_id,omitempty"`
	VersionCount       int        `json:"version_count"`
}

// ListProjects returns every local project, ordered by name. It only reads:
// sizes come from the cache, and missing or stale ones are recomputed in
// the background, so listing never waits for a walk of .pixi.
func (s *Service) ListProjects(ctx context.Context) ([]ProjectSummary, error) {
	var projects []Project
	if err := s.db.WithContext(ctx).Order("name COLLATE NOCASE ASC").Find(&projects).Error; err != nil {
		return nil, fmt.Errorf("listing projects: %w", err)
	}
	out := make([]ProjectSummary, 0, len(projects))
	for i := range projects {
		d, err := s.describe(ctx, &projects[i])
		if err != nil {
			return nil, err
		}
		out = append(out, d.ProjectSummary)
	}
	s.refreshSizes(projects)
	return out, nil
}

// GetProject returns one project. Like ListProjects it only reads, and its
// size comes from the cache.
func (s *Service) GetProject(ctx context.Context, id uuid.UUID) (*ProjectDetail, error) {
	p, err := s.loadProject(ctx, id)
	if err != nil {
		return nil, err
	}
	d, err := s.describe(ctx, p)
	if err != nil {
		return nil, err
	}
	s.refreshSizes([]Project{*p})
	return d, nil
}

// ProjectSize walks the project directory, caches its size and returns it.
// A missing directory has size 0.
func (s *Service) ProjectSize(ctx context.Context, id uuid.UUID) (int64, error) {
	p, err := s.loadProject(ctx, id)
	if err != nil {
		return 0, err
	}
	size, err := s.measureDir(ctx, p.Path)
	if err != nil {
		return 0, fmt.Errorf("measuring %s: %w", p.Path, err)
	}
	s.sizes.store(p.ID, size, s.now())
	return size, nil
}

func (s *Service) describe(ctx context.Context, p *Project) (*ProjectDetail, error) {
	info, err := s.versions.VersionInfo(ctx, p.ID, p.InstalledVersionID)
	if err != nil {
		return nil, fmt.Errorf("reading versions of %s: %w", p.Name, err)
	}
	projectJobs, err := s.jobs.ProjectJobs(ctx, p.ID)
	if err != nil {
		return nil, fmt.Errorf("reading jobs of %s: %w", p.Name, err)
	}
	state, jobID := deriveState(info.Count, projectJobs, p.InstalledVersionID != nil, envInstalled(p.Path))

	var size *int64
	if bytes, _, ok := s.sizes.get(p.ID); ok {
		size = &bytes
	}
	return &ProjectDetail{
		ProjectSummary: ProjectSummary{
			ID:               p.ID,
			Name:             p.Name,
			Path:             p.Path,
			State:            state,
			JobID:            jobID,
			InstalledVersion: info.Installed,
			LatestVersion:    info.Latest,
			SizeBytes:        size,
			Remotes:          []RemoteRef{},
			CreatedAt:        p.CreatedAt,
		},
		ManagedDir:         p.ManagedDir,
		InstalledVersionID: p.InstalledVersionID,
		VersionCount:       info.Count,
	}, nil
}

// envInstalled reports whether the project has an environment on disk.
func envInstalled(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".pixi", "envs"))
	return err == nil && info.IsDir()
}

// deriveState computes a project's State from its version count, its jobs
// (oldest first), whether InstalledVersionID is set (hasInstalledVersion)
// and whether .pixi/envs exists (envOnDisk), and returns the job the state
// refers to.
//
// An active (pending or running) create job wins. A project with no
// versions is lock_failed. An active install or uninstall comes next; then
// a failed or cancelled last install wins over what is on disk, because
// pixi can leave a partial .pixi/envs behind when it stops early. An
// install or uninstall cancelled while pending never ran pixi, so it is
// skipped and the job before it decides. Otherwise the project is installed
// only when both hasInstalledVersion and envOnDisk hold. Jobs live in
// memory, so after a restart a failed install leaves no job behind, only
// its partial .pixi/envs; InstalledVersionID, which is set only after an
// install succeeds, is what tells that apart from a real install.
func deriveState(versionCount int, projectJobs []jobs.Job, hasInstalledVersion, envOnDisk bool) (State, *uuid.UUID) {
	create := latestJob(projectJobs, func(j jobs.Job) bool { return j.Type == jobs.TypeCreate })
	if create != nil && create.Active() {
		return StateCreating, &create.ID
	}
	if versionCount == 0 {
		if create != nil {
			return StateLockFailed, &create.ID
		}
		return StateLockFailed, nil
	}

	// The environment jobs: installs, uninstalls, and create jobs that got
	// as far as installing.
	env := latestJob(projectJobs, func(j jobs.Job) bool {
		if !j.Active() && !j.Started() {
			return false // cancelled while pending: it changed nothing
		}
		return j.Type == jobs.TypeInstall || j.Type == jobs.TypeUninstall ||
			(j.Type == jobs.TypeCreate && j.Stage == StageInstall)
	})
	if env != nil && env.Active() {
		if env.Type == jobs.TypeUninstall {
			return StateUninstalling, &env.ID
		}
		return StateInstalling, &env.ID
	}
	if env != nil && (env.Status == jobs.StatusFailed || env.Status == jobs.StatusCancelled) && env.Type != jobs.TypeUninstall {
		return StateInstallFailed, &env.ID
	}
	if hasInstalledVersion && envOnDisk {
		return StateInstalled, nil
	}
	return StateNotInstalled, nil
}

func latestJob(list []jobs.Job, match func(jobs.Job) bool) *jobs.Job {
	for i := len(list) - 1; i >= 0; i-- {
		if match(list[i]) {
			return &list[i]
		}
	}
	return nil
}
