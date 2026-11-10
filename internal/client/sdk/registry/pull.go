package registry

// Owner: U7 (publish). Pull a project from a registry.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/artifact"
	"github.com/nebari-dev/nebi/internal/client/sdk/local"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/contenthash"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/oci"
	"gorm.io/gorm"
)

// Metadata keys of pull_registry jobs (CONTRACTS.md, job types).
const (
	MetaRegistryID = "registry_id"
	MetaRepository = "repository"
	MetaTag        = "tag"
	MetaInstall    = "install"
)

// maxPullBundleBytes caps what a registry pull may write to disk.
const maxPullBundleBytes int64 = 4 << 30

// ListRepositories lists repositories in a registry namespace whose name
// contains search (case-insensitive), sorted by name. Registries without
// the catalog API (Quay) fall back to the vendor listing of the namespace.
func (s *Service) ListRepositories(ctx context.Context, registryID uuid.UUID, search string) ([]string, error) {
	reg, err := s.loadRegistry(ctx, registryID)
	if err != nil {
		return nil, err
	}
	ep, err := s.ociEndpoint(ctx, *reg)
	if err != nil {
		return nil, err
	}
	_, namespace, _ := endpoint(*reg)
	repos, err := oci.ListRepositories(ctx, oci.BrowseOptions{
		RegistryHost: ep.Host,
		Username:     ep.Username,
		Password:     ep.Password,
		PlainHTTP:    ep.PlainHTTP,
	})
	if err != nil && namespace != "" && strings.Contains(ep.Host, "quay.io") {
		repos, err = oci.ListRepositoriesViaQuayAPI(ctx, ep.Host, namespace, "")
	}
	if err != nil {
		return nil, fmt.Errorf("couldn’t list repositories on %s: %w", reg.Name, err)
	}
	search = strings.ToLower(strings.TrimSpace(search))
	prefix := ""
	if namespace != "" {
		prefix = namespace + "/"
	}
	out := []string{}
	for _, r := range repos {
		if prefix != "" && !strings.HasPrefix(r.Name, prefix) {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(r.Name), search) {
			continue
		}
		out = append(out, r.Name)
	}
	sort.Strings(out)
	return out, nil
}

// ListTags lists tags of a repository: numeric tags (version numbers)
// newest first, then the others by name.
func (s *Service) ListTags(ctx context.Context, registryID uuid.UUID, repository string) ([]string, error) {
	repository = strings.Trim(strings.TrimSpace(repository), "/")
	if repository == "" || !repositoryRE.MatchString(repository) {
		return nil, &shared.ValidationError{Field: "repository", Message: "Choose a repository."}
	}
	reg, err := s.loadRegistry(ctx, registryID)
	if err != nil {
		return nil, err
	}
	ep, err := s.ociEndpoint(ctx, *reg)
	if err != nil {
		return nil, err
	}
	tags, err := oci.ListTags(ctx, ep.Host+"/"+repository, oci.BrowseOptions{
		Username:  ep.Username,
		Password:  ep.Password,
		PlainHTTP: ep.PlainHTTP,
	})
	if err != nil {
		return nil, fmt.Errorf("couldn’t list tags of %s: %w", repository, err)
	}
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		out = append(out, t.Name)
	}
	sortTags(out)
	return out, nil
}

func sortTags(tags []string) {
	sort.SliceStable(tags, func(i, j int) bool {
		a, aErr := strconv.Atoi(tags[i])
		b, bErr := strconv.Atoi(tags[j])
		switch {
		case aErr == nil && bErr == nil:
			return a > b
		case aErr == nil:
			return true
		case bErr == nil:
			return false
		default:
			return tags[i] < tags[j]
		}
	})
}

// Pull creates the local project (status pending) and submits a
// JobTypePullRegistry job that downloads the artifact into
// DefaultProjectsDir()/<local name> and stores it as version 1. LocalName
// defaults to the last segment of the repository.
func (s *Service) Pull(ctx context.Context, registryID uuid.UUID, req shared.RegistryPullRequest) (*shared.PullResult, error) {
	reg, err := s.loadRegistry(ctx, registryID)
	if err != nil {
		return nil, err
	}
	repo := strings.Trim(strings.TrimSpace(req.Repository), "/")
	tag := strings.TrimSpace(req.Tag)
	if repo == "" || !repositoryRE.MatchString(repo) {
		return nil, &shared.ValidationError{Field: "repository", Message: "Choose a repository."}
	}
	if tag == "" || !tagRE.MatchString(tag) {
		return nil, &shared.ValidationError{Field: "tag", Message: "Choose a tag."}
	}
	name := strings.TrimSpace(req.LocalName)
	if name == "" {
		name = path.Base(repo)
	}
	if err := artifact.ValidateProjectName(name); err != nil {
		return nil, &shared.ValidationError{Field: "local_name", Message: err.Error()}
	}
	var taken int64
	if err := s.db(ctx).Model(&models.Project{}).Where("LOWER(name) = LOWER(?)", name).Count(&taken).Error; err != nil {
		return nil, err
	}
	if taken > 0 {
		return nil, &shared.ConflictError{Code: "name_taken", Message: fmt.Sprintf("A project called %s is already on this machine. Pick another name.", name)}
	}
	dest := filepath.Join(s.deps.DefaultProjectsDir(), name)
	if nonEmpty, err := dirNonEmpty(dest); err != nil {
		return nil, err
	} else if nonEmpty {
		return nil, &shared.ValidationError{Field: "local_name", Message: fmt.Sprintf("A folder called %s already exists in Nebi’s projects folder. Pick another name.", name)}
	}

	project := models.Project{
		Name:    name,
		OwnerID: s.deps.LocalUserID,
		Status:  models.ProjectStatusPending,
		Source:  "managed",
		Path:    dest,
	}
	if err := s.db(ctx).Omit("Owner").Create(&project).Error; err != nil {
		return nil, fmt.Errorf("creating project: %w", err)
	}
	job, err := s.deps.Runtime.Jobs().Submit(ctx, shared.JobRequest{
		ProjectID: project.ID,
		Type:      models.JobTypePullRegistry,
		Metadata: map[string]any{
			MetaRegistryID: reg.ID.String(),
			MetaRepository: repo,
			MetaTag:        tag,
			MetaInstall:    req.Install,
		},
	})
	if job == nil {
		if err == nil {
			err = fmt.Errorf("pull job was not created")
		}
		_ = s.db(context.WithoutCancel(ctx)).Delete(&project).Error
		return nil, err
	}
	res := &shared.PullResult{ProjectID: project.ID, Job: shared.JobSummaryFrom(*job, project.Name)}
	if err != nil {
		// Inline (CLI) jobs report their own failure.
		return res, err
	}
	return res, nil
}

// RunPull is the pull_registry job body: extract the artifact into the
// project folder, store version 1 (the OCI tag becomes a version tag when
// it is a valid one), record the registry remote and mark the project
// ready. With install=true it then submits an install_version job. On
// failure or cancellation it removes what it created, including the
// pending project.
func (s *Service) RunPull(ctx context.Context, job *models.Job, log io.Writer) (err error) {
	project, err := s.loadProject(ctx, job.ProjectID)
	if err != nil {
		return err
	}
	regID, err := uuid.Parse(metaString(job.Metadata, MetaRegistryID))
	if err != nil {
		return fmt.Errorf("pull job has no registry")
	}
	repo := metaString(job.Metadata, MetaRepository)
	tag := metaString(job.Metadata, MetaTag)
	install := metaBool(job.Metadata, MetaInstall)

	createdDir := false
	defer func() {
		if err == nil {
			return
		}
		cleanup := context.WithoutCancel(ctx)
		if createdDir {
			_ = os.RemoveAll(project.Path)
		}
		_ = s.db(cleanup).Where("project_id = ?", project.ID).Delete(&models.ProjectVersion{}).Error
		_ = s.db(cleanup).Where("project_id = ?", project.ID).Delete(&models.ProjectTag{}).Error
		_ = s.db(cleanup).Where("project_id = ?", project.ID).Delete(&models.ProjectRemote{}).Error
		_ = s.db(cleanup).Delete(&models.Project{}, "id = ?", project.ID).Error
		fmt.Fprintf(log, "Removed the partially pulled project %s.\n", project.Name)
	}()

	reg, err := s.loadRegistry(ctx, regID)
	if err != nil {
		return err
	}
	ep, err := s.ociEndpoint(ctx, *reg)
	if err != nil {
		return err
	}
	ref := ep.Host + "/" + repo + ":" + tag
	fmt.Fprintf(log, "Pulling %s into %s\n", ref, project.Path)

	if nonEmpty, derr := dirNonEmpty(project.Path); derr != nil {
		return derr
	} else if nonEmpty {
		return fmt.Errorf("%s already exists and is not empty", project.Path)
	}
	if _, statErr := os.Stat(project.Path); os.IsNotExist(statErr) {
		createdDir = true
	}
	res, err := oci.ExtractBundle(ctx, ep.Host+"/"+repo, tag, project.Path, oci.PullOptions{
		Username:       ep.Username,
		Password:       ep.Password,
		PlainHTTP:      ep.PlainHTTP,
		MaxBundleBytes: maxPullBundleBytes,
	})
	createdDir = true
	if err != nil {
		return fmt.Errorf("pulling %s: %w", ref, err)
	}
	fmt.Fprintf(log, "Downloaded %s (%d extra file(s))\n", res.Digest, len(res.Assets))

	toml, err := os.ReadFile(filepath.Join(project.Path, artifact.ManifestFile))
	if err != nil {
		return fmt.Errorf("reading pixi.toml: %w", err)
	}
	lock, err := os.ReadFile(filepath.Join(project.Path, artifact.LockFile))
	if err != nil {
		return fmt.Errorf("reading pixi.lock: %w", err)
	}
	if _, perr := artifact.ParseManifest(toml); perr != nil {
		return fmt.Errorf("the pulled pixi.toml is not valid: %w", perr)
	}

	var tags []string
	if tag != "latest" && artifact.ValidateTag(tag) == nil {
		tags = []string{tag}
	}
	one := 1
	err = s.db(ctx).Transaction(func(tx *gorm.DB) error {
		v := models.ProjectVersion{
			ProjectID:       project.ID,
			VersionNumber:   1,
			ManifestContent: string(toml),
			LockFileContent: string(lock),
			PackageMetadata: "[]",
			ContentHash:     contenthash.Hash(string(toml), string(lock)),
			CreatedBy:       s.deps.LocalUserID,
			Description:     "Pulled from " + ref,
			JobID:           &job.ID,
		}
		if err := tx.Omit("Project", "Job", "CreatedByUser").Create(&v).Error; err != nil {
			return err
		}
		if err := local.ApplyTags(tx, project.ID, 1, tags, s.deps.LocalUserID); err != nil {
			return err
		}
		if err := upsertRegistryRemote(tx, project.ID, reg.ID, ep.Host, repo); err != nil {
			return err
		}
		return tx.Model(&models.Project{}).Where("id = ?", project.ID).Update("status", models.ProjectStatusReady).Error
	})
	if err != nil {
		return fmt.Errorf("saving version 1: %w", err)
	}
	job.VersionNumber = &one
	fmt.Fprintf(log, "Saved as version 1 of %s.\n", project.Name)

	if install {
		inst, ierr := s.deps.Runtime.Jobs().Submit(ctx, shared.JobRequest{
			ProjectID:     project.ID,
			Type:          models.JobTypeInstallVersion,
			VersionNumber: &one,
		})
		switch {
		case inst != nil && ierr == nil:
			fmt.Fprintf(log, "Installing version 1 (job %s).\n", inst.ID)
		case ierr != nil:
			// The pull itself succeeded; the install job reports its
			// own failure.
			fmt.Fprintf(log, "Couldn’t install version 1: %v\n", ierr)
		}
	}
	return nil
}

func dirNonEmpty(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("reading %s: %w", dir, err)
	}
	return len(entries) > 0, nil
}

func metaString(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
		return fmt.Sprint(v)
	}
	return ""
}

func metaBool(m map[string]any, key string) bool {
	switch v := m[key].(type) {
	case bool:
		return v
	case string:
		b, _ := strconv.ParseBool(v)
		return b
	}
	return false
}
