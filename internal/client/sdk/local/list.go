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

	// InstalledVersion is the number of the version nebi last recorded as
	// installed (InstalledVersionID); nil when none is recorded. It is
	// kept when the environment is no longer on disk, so check State to
	// know whether it is installed now.
	InstalledVersion *int `json:"installed_version"`
	// LatestVersion is the newest version number; nil when there are no
	// versions.
	LatestVersion *int `json:"latest_version"`

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
//
// Projects are read with readProjects, so a job that starts or finishes
// during the list does not tear the state the job writes, within the
// limits readProjects describes. A project created after the list starts
// is left out; the next list shows it.
func (s *Service) ListProjects(ctx context.Context) ([]ProjectSummary, error) {
	var ids []uuid.UUID
	if err := s.db.WithContext(ctx).Model(&Project{}).Pluck("id", &ids).Error; err != nil {
		return nil, fmt.Errorf("listing projects: %w", err)
	}
	reads, err := s.readProjects(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]ProjectSummary, len(reads))
	projects := make([]Project, len(reads))
	for i, r := range reads {
		out[i] = r.detail.ProjectSummary
		projects[i] = r.project
	}
	s.refreshSizes(projects)
	return out, nil
}

// GetProject returns one project. Like ListProjects it only reads, its
// size comes from the cache, and it is read with readProjects.
func (s *Service) GetProject(ctx context.Context, id uuid.UUID) (*ProjectDetail, error) {
	reads, err := s.readProjects(ctx, []uuid.UUID{id})
	if err != nil {
		return nil, err
	}
	if len(reads) == 0 {
		return nil, fmt.Errorf("%w: project %s", ErrNotFound, id)
	}
	s.refreshSizes([]Project{reads[0].project})
	return reads[0].detail, nil
}

// maxReadAttempts bounds how many times readProjects reads a project whose
// jobs keep changing during the read.
const maxReadAttempts = 5

// projectRead is a project row and the detail readProjects built from it.
type projectRead struct {
	project Project
	detail  *ProjectDetail
}

// readProjects reads the projects with the given IDs, ordered by name as
// of their first read, and leaves out those that do not exist. It is the
// only way project state is read, so every read path gets the same
// guarantee for the state a job writes.
//
// A project's state comes from two sources that change independently: its
// jobs, in the JobRunner, and its row, versions and .pixi/envs, which a
// job writes. Reading them one after the other can tear in either
// direction. A job that finishes between the reads pairs a row from before
// its writes with the job already finished (not_installed for a project
// that was just installed); a job that starts between them pairs a row it
// has already changed (an install clears InstalledVersionID first) with
// jobs that do not show it (not_installed for a project being installed).
//
// So each project's jobs are read before and after its row, versions and
// disk. The job list changes only when a job is submitted, before its body
// writes anything, and when it finishes, after its body has written
// everything (see JobRunner.ProjectJobs). When the two job reads match, no
// job started or finished during the row read, and the row is consistent
// with the jobs. When they differ, the project is read again, up to
// maxReadAttempts times; if its jobs still change on the last attempt,
// that attempt is returned as read, unchecked.
//
// The guarantee covers only what a job writes. State written outside a
// job is not bracketed by the two job reads:
//   - CreateProject inserts the row before it submits the create job, so a
//     read in between sees the row with no job.
//   - DeleteProject deletes the row and versions in a transaction, not a
//     job, so a read that loads the row before the delete commits and the
//     versions after it reports lock_failed linked to the create job; the
//     next read returns ErrNotFound.
func (s *Service) readProjects(ctx context.Context, ids []uuid.UUID) ([]projectRead, error) {
	var out []projectRead
	index := make(map[uuid.UUID]int, len(ids))
	gone := map[uuid.UUID]bool{}
	pending := ids
	for attempt := 1; len(pending) > 0; attempt++ {
		before, err := s.projectJobs(ctx, pending)
		if err != nil {
			return nil, err
		}
		var rows []Project
		if err := s.db.WithContext(ctx).Where("id IN ?", pending).Order("name COLLATE NOCASE ASC").Find(&rows).Error; err != nil {
			return nil, fmt.Errorf("loading projects: %w", err)
		}
		details := make([]*ProjectDetail, len(rows))
		for i := range rows {
			if details[i], err = s.describe(ctx, &rows[i], before[rows[i].ID]); err != nil {
				return nil, err
			}
		}
		after, err := s.projectJobs(ctx, pending)
		if err != nil {
			return nil, err
		}

		found := make(map[uuid.UUID]bool, len(rows))
		var retry []uuid.UUID
		for i := range rows {
			id := rows[i].ID
			found[id] = true
			r := projectRead{project: rows[i], detail: details[i]}
			if j, ok := index[id]; ok {
				out[j] = r
			} else {
				index[id] = len(out)
				out = append(out, r)
			}
			if attempt < maxReadAttempts && !sameJobs(before[id], after[id]) {
				retry = append(retry, id)
			}
		}
		// A project deleted since an earlier attempt read it is gone.
		for _, id := range pending {
			if !found[id] {
				gone[id] = true
			}
		}
		pending = retry
	}
	if len(gone) == 0 {
		return out, nil
	}
	kept := out[:0]
	for _, r := range out {
		if !gone[r.project.ID] {
			kept = append(kept, r)
		}
	}
	return kept, nil
}

// projectJobs reads the jobs of each project.
func (s *Service) projectJobs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID][]jobs.Job, error) {
	out := make(map[uuid.UUID][]jobs.Job, len(ids))
	for _, id := range ids {
		list, err := s.jobs.ProjectJobs(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("reading jobs of project %s: %w", id, err)
		}
		out[id] = list
	}
	return out, nil
}

// sameJobs reports whether two reads of a project's jobs list the same jobs
// with the same statuses.
func sameJobs(a, b []jobs.Job) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].Status != b[i].Status {
			return false
		}
	}
	return true
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

// describe builds a project's detail from its row p and projectJobs, the
// project's jobs. Call it through readProjects, which reads the jobs on
// both sides of the row so the two are consistent.
func (s *Service) describe(ctx context.Context, p *Project, projectJobs []jobs.Job) (*ProjectDetail, error) {
	info, err := s.versionCounts(ctx, p.ID, p.InstalledVersionID)
	if err != nil {
		return nil, fmt.Errorf("reading versions of %s: %w", p.Name, err)
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

	// The environment jobs: installs, uninstalls, and create and
	// create_version jobs that got as far as installing.
	env := latestJob(projectJobs, func(j jobs.Job) bool {
		if !j.Active() && !j.Started() {
			return false // cancelled while pending: it changed nothing
		}
		return j.Type == jobs.TypeInstall || j.Type == jobs.TypeUninstall ||
			((j.Type == jobs.TypeCreate || j.Type == jobs.TypeCreateVersion) && j.Stage == StageInstall)
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
