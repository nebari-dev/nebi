package remote

// Owner: U5 (remote). Pull a server project (optionally installing it) and
// pull a server-only version.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/artifact"
	"github.com/nebari-dev/nebi/internal/client/sdk/exec"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
)

// Pull job metadata keys.
const (
	MetaServerProjectID = "server_project_id"
	MetaServerURL       = "server_url"
)

// Pull checks the local name (ConflictError "name_taken"), creates the
// local project row (status pending) with a server ProjectRemote, and
// submits a JobTypePull or JobTypePullInstall job with metadata
// {"server_project_id", "server_url"}.
func (s *Service) Pull(ctx context.Context, serverProjectID string, req shared.PullRequest) (*shared.PullResult, error) {
	conn, err := s.connection(ctx)
	if err != nil {
		return nil, err
	}
	serverProjectID = strings.TrimSpace(serverProjectID)
	if serverProjectID == "" {
		return nil, &shared.ValidationError{Field: "server_project_id", Message: "Choose a server project."}
	}
	sp, err := s.catalogProject(ctx, serverProjectID)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(req.LocalName)
	if req.LocalName == "" {
		name = sp.Name // CLI default: the server name
	}
	if name == "" {
		return nil, &shared.ValidationError{Field: "local_name", Message: "Give the project a name."}
	}
	if err := artifact.ValidateProjectName(name); err != nil {
		var verr *artifact.ValidationError
		if errors.As(err, &verr) {
			return nil, &shared.ValidationError{Field: "local_name", Message: verr.Message}
		}
		return nil, &shared.ValidationError{Field: "local_name", Message: err.Error()}
	}

	db := s.deps.DB.WithContext(ctx)
	if existing, err := s.localProjectFor(db, conn.URL, serverProjectID); err != nil {
		return nil, err
	} else if existing != nil {
		return nil, &shared.ConflictError{
			Code:    "already_pulled",
			Message: fmt.Sprintf("This project is already on this machine as %s.", existing.Name),
		}
	}
	if taken, err := nameTaken(db, name); err != nil {
		return nil, err
	} else if taken {
		return nil, &shared.ConflictError{
			Code:    "name_taken",
			Message: fmt.Sprintf("A project called %s is already on this machine. Pick another name.", name),
		}
	}

	path := strings.TrimSpace(req.Path)
	if path == "" {
		path = filepath.Join(s.deps.DefaultProjectsDir(), name)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, &shared.ValidationError{Field: "path", Message: "Choose a valid folder."}
	}
	if entries, err := os.ReadDir(abs); err == nil && len(entries) > 0 {
		return nil, &shared.ValidationError{Field: "path", Message: "That folder isn’t empty. Choose another location."}
	} else if err == nil || errors.Is(err, os.ErrNotExist) {
		// ok: empty or missing
	} else {
		return nil, &shared.ValidationError{Field: "path", Message: "That folder can’t be used: " + err.Error()}
	}

	project := models.Project{
		Name:    name,
		OwnerID: s.deps.LocalUserID,
		Status:  models.ProjectStatusPending,
		Source:  "local",
		Path:    abs,
	}
	remoteName := conn.Name
	if remoteName == "" {
		remoteName = ServerName(conn.URL)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&project).Error; err != nil {
			return fmt.Errorf("creating project: %w", err)
		}
		return tx.Create(&models.ProjectRemote{
			ProjectID:         project.ID,
			Kind:              models.RemoteKindServer,
			Name:              remoteName,
			ServerURL:         conn.URL,
			ServerProjectID:   serverProjectID,
			ServerProjectName: sp.Name,
		}).Error
	}); err != nil {
		return nil, err
	}

	jobType := models.JobTypePull
	if req.Install {
		jobType = models.JobTypePullInstall
	}
	job, err := s.deps.Runtime.Jobs().Submit(ctx, shared.JobRequest{
		ProjectID: project.ID,
		Type:      jobType,
		Metadata: map[string]any{
			MetaServerProjectID: serverProjectID,
			MetaServerURL:       conn.URL,
		},
	})
	if job == nil {
		// Nothing will ever run for this row: free the name again.
		s.discardPulledProject(context.WithoutCancel(ctx), project.ID)
		if err == nil {
			err = fmt.Errorf("pull job was not created")
		}
		return nil, err
	}
	res := &shared.PullResult{ProjectID: project.ID, Job: shared.JobSummaryFrom(*job, project.Name)}
	if err != nil {
		// Inline (CLI) jobs return the finished job and its error.
		return res, err
	}
	return res, nil
}

// RunPull is the pull / pull_install job body: download every server
// version (keeping server numbers, authors, tags, publications), write the
// latest files into the project directory and, for pull_install, install
// the latest version through exec. When the pull itself fails or is
// cancelled the half-made project is removed; once the versions are on
// this machine they stay even if the install fails or is cancelled.
func (s *Service) RunPull(ctx context.Context, job *models.Job, log io.Writer) (err error) {
	serverProjectID, _ := job.Metadata[MetaServerProjectID].(string)
	serverURL, _ := job.Metadata[MetaServerURL].(string)
	if serverProjectID == "" {
		return fmt.Errorf("pull job has no %s", MetaServerProjectID)
	}
	pulled := false
	defer func() {
		if err != nil && !pulled {
			s.discardPulledProject(context.WithoutCancel(ctx), job.ProjectID)
		}
	}()
	db := s.deps.DB.WithContext(ctx)
	var project models.Project
	if err := s.deps.DB.Where("id = ?", job.ProjectID).First(&project).Error; err != nil {
		return fmt.Errorf("loading project: %w", err)
	}

	conn, err := s.connection(ctx)
	if err != nil {
		return err
	}
	if serverURL != "" && !sameServer(serverURL, conn.URL) {
		return fmt.Errorf("the server connection changed to %s while pulling from %s", displayURL(conn.URL), displayURL(serverURL))
	}
	fmt.Fprintf(log, "Pulling %s from %s\n", project.Name, displayURL(conn.URL))
	st, err := s.fetchServerState(ctx, conn.URL, serverProjectID, false)
	if err != nil {
		return fmt.Errorf("listing server versions: %w", err)
	}
	if len(st.versions) == 0 {
		return errors.New("this project has no versions on the server yet")
	}

	files := make([]*syncVersionFiles, 0, len(st.versions))
	for _, v := range st.versions {
		if err := ctx.Err(); err != nil {
			return err
		}
		fmt.Fprintf(log, "Downloading version %d\n", v.Number)
		f, err := s.fetchServerVersion(ctx, serverProjectID, v.Number)
		if err != nil {
			return fmt.Errorf("downloading version %d: %w", v.Number, err)
		}
		files = append(files, f)
	}

	latest := files[len(files)-1]
	now := time.Now().UTC()
	if err := db.Transaction(func(tx *gorm.DB) error {
		for _, f := range files {
			if _, err := s.storeServerVersion(tx, project.ID, f, now); err != nil {
				return err
			}
		}
		if err := tx.Model(&models.Project{}).Where("id = ?", project.ID).
			Update("status", models.ProjectStatusReady).Error; err != nil {
			return err
		}
		return tx.Model(&models.ProjectRemote{}).
			Where("project_id = ? AND kind = ?", project.ID, models.RemoteKindServer).
			Update("last_synced_at", now).Error
	}); err != nil {
		return fmt.Errorf("saving pulled versions: %w", err)
	}
	pulled = true
	if err := writeProjectFiles(project.Path, latest.PixiToml, latest.PixiLock); err != nil {
		return err
	}
	n := latest.Number
	job.VersionNumber = &n
	// Persist now so the UI can show "Pulled. Installing version N…" while
	// the install part of the job runs.
	_ = s.deps.DB.Model(&models.Job{}).Where("id = ?", job.ID).Update("version_number", n).Error
	fmt.Fprintf(log, "Pulled %d version(s); version %d is the latest\n", len(files), n)

	if job.Type != models.JobTypePullInstall {
		return nil
	}
	fmt.Fprintf(log, "Installing version %d\n", n)
	return exec.New(s.deps, s.local).RunInstallVersion(ctx, job, log)
}

// PullVersion stores a server-only version locally (synchronous).
func (s *Service) PullVersion(ctx context.Context, projectID uuid.UUID, number int) (*shared.VersionItem, error) {
	remote, conn, err := s.linkedServer(ctx, projectID)
	if err != nil {
		return nil, err
	}
	db := s.deps.DB.WithContext(ctx)
	if _, err := s.local.LoadVersion(ctx, projectID, number); err == nil {
		// Collisions with local-only versions are renumbered first.
		st, err := s.fetchServerState(ctx, conn.URL, remote.ServerProjectID, false)
		if err != nil {
			return nil, err
		}
		if _, err := s.reconcile(ctx, projectID, st); err != nil {
			return nil, err
		}
		if v, err := s.local.LoadVersion(ctx, projectID, number); err == nil {
			return s.versionItem(db, projectID, v)
		}
	} else if !errors.Is(err, shared.ErrNotFound) {
		return nil, err
	}
	f, err := s.fetchServerVersion(ctx, remote.ServerProjectID, number)
	if err != nil {
		return nil, err
	}
	var row *models.ProjectVersion
	if err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		row, err = s.storeServerVersion(tx, projectID, f, time.Now().UTC())
		return err
	}); err != nil {
		return nil, err
	}
	forgetServerProject(conn.URL, remote.ServerProjectID)
	return s.versionItem(db, projectID, row)
}

// catalogProject finds a server project visible to the caller.
func (s *Service) catalogProject(ctx context.Context, serverProjectID string) (*catalogProject, error) {
	var list []catalogProject
	if err := s.ServerJSON(ctx, "GET", "/catalog/projects", nil, &list); err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == serverProjectID {
			return &list[i], nil
		}
	}
	return nil, &shared.RemoteError{Status: 404, Message: "That project isn’t on the server, or it isn’t shared with you."}
}

// localProjectFor returns the local project linked to a server project.
func (s *Service) localProjectFor(db *gorm.DB, base, serverProjectID string) (*models.Project, error) {
	var remotes []models.ProjectRemote
	if err := db.Where("kind = ? AND server_project_id = ?", models.RemoteKindServer, serverProjectID).Find(&remotes).Error; err != nil {
		return nil, err
	}
	for _, r := range remotes {
		if !sameServer(r.ServerURL, base) {
			continue
		}
		var p models.Project
		err := db.Where("id = ?", r.ProjectID).First(&p).Error
		if err == nil {
			return &p, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	return nil, nil
}

// nameTaken reports whether a local project already uses name (ignoring
// case, as the prototype does).
func nameTaken(db *gorm.DB, name string) (bool, error) {
	var count int64
	err := db.Model(&models.Project{}).Where("LOWER(name) = ?", strings.ToLower(name)).Count(&count).Error
	return count > 0, err
}

// discardPulledProject removes a project whose pull never completed so its
// name can be used again.
func (s *Service) discardPulledProject(ctx context.Context, projectID uuid.UUID) {
	db := s.deps.DB.WithContext(ctx)
	_ = db.Where("project_id = ?", projectID).Delete(&models.ProjectRemote{}).Error
	_ = db.Where("id = ?", projectID).Delete(&models.Project{}).Error
}

// writeProjectFiles writes the latest version's files into the project
// directory so pixi and editors see them.
func writeProjectFiles(dir, pixiToml, pixiLock string) error {
	if dir == "" {
		return errors.New("the project has no folder")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating project folder: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, artifact.ManifestFile), []byte(pixiToml), 0o644); err != nil {
		return fmt.Errorf("writing pixi.toml: %w", err)
	}
	lockPath := filepath.Join(dir, artifact.LockFile)
	if pixiLock == "" {
		_ = os.Remove(lockPath)
		return nil
	}
	if err := os.WriteFile(lockPath, []byte(pixiLock), 0o644); err != nil {
		return fmt.Errorf("writing pixi.lock: %w", err)
	}
	return nil
}
