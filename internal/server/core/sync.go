package core

// Owner: U5 (remote). Project catalog and version sync.
//
// Version numbers are shared between a server project and the machines
// that pull it: a client pushes its versions with their numbers and the
// server stores them as is. A number that already exists is accepted again
// only with the same content hash (idempotent push); different content is
// a "version_conflict", and a number that would leave a gap is
// "server_ahead" (the client is missing versions the server has, or has
// not renumbered its local-only versions yet).

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/artifact"
	"github.com/nebari-dev/nebi/internal/audit"
	"github.com/nebari-dev/nebi/internal/contenthash"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
)

// Catalog role and namespace values.
const (
	roleOwner          = "owner"
	roleEdit           = "edit"
	roleView           = "view"
	namespacePersonal  = "Personal"
	latestTag          = "latest"
	maxPushVersions    = 500
	maxPushTagsPerItem = 50
)

// ListCatalogProjects lists projects the caller can read, with the caller's
// role, filtered by a case-insensitive name search. Admins see every
// project. Namespace is "Personal" for the caller's own projects, the team
// name when access comes through a group, and the owner's username for
// projects shared with the caller directly.
func (s *Service) ListCatalogProjects(ctx context.Context, userID uuid.UUID, search string) ([]CatalogProject, error) {
	db := s.deps.DB.WithContext(ctx)

	isAdmin := false
	if s.deps.RBAC != nil {
		ok, err := s.deps.RBAC.IsAdmin(userID)
		if err != nil {
			return nil, fmt.Errorf("checking admin: %w", err)
		}
		isAdmin = ok
	}

	roleNames, err := s.roleNames(db)
	if err != nil {
		return nil, err
	}

	// Direct grants.
	var perms []models.Permission
	if err := db.Where("user_id = ?", userID).Find(&perms).Error; err != nil {
		return nil, fmt.Errorf("listing permissions: %w", err)
	}
	direct := map[uuid.UUID]string{}
	for _, p := range perms {
		direct[p.ProjectID] = strongerRole(direct[p.ProjectID], catalogRole(roleNames[p.RoleID]))
	}

	// Group grants (casbin grouping rules are the membership source of truth).
	type groupGrant struct {
		role  string
		group string
	}
	viaGroup := map[uuid.UUID]groupGrant{}
	if s.deps.RBAC != nil {
		groups, err := s.deps.RBAC.GetUserGroups(userID)
		if err != nil {
			return nil, fmt.Errorf("listing groups: %w", err)
		}
		if len(groups) > 0 {
			var gperms []models.GroupPermission
			if err := db.Preload("Group").Where("group_id IN ?", groups).Find(&gperms).Error; err != nil {
				return nil, fmt.Errorf("listing group permissions: %w", err)
			}
			for _, gp := range gperms {
				r := catalogRole(roleNames[gp.RoleID])
				cur, ok := viaGroup[gp.ProjectID]
				if !ok || strongerRole(cur.role, r) != cur.role {
					viaGroup[gp.ProjectID] = groupGrant{role: r, group: gp.Group.Name}
				}
			}
		}
	}

	q := db.Preload("Owner").Where("status = ?", models.ProjectStatusReady)
	if !isAdmin {
		ids := make([]uuid.UUID, 0, len(direct)+len(viaGroup))
		for id := range direct {
			ids = append(ids, id)
		}
		for id := range viaGroup {
			ids = append(ids, id)
		}
		if len(ids) > 0 {
			q = q.Where("owner_id = ? OR id IN ?", userID, ids)
		} else {
			q = q.Where("owner_id = ?", userID)
		}
	}
	if s := strings.TrimSpace(search); s != "" {
		q = q.Where(`LOWER(name) LIKE ? ESCAPE '\'`, "%"+escapeLike(strings.ToLower(s))+"%")
	}
	var projects []models.Project
	if err := q.Find(&projects).Error; err != nil {
		return nil, fmt.Errorf("listing projects: %w", err)
	}

	latest, err := s.latestVersions(db, projects)
	if err != nil {
		return nil, err
	}

	out := make([]CatalogProject, 0, len(projects))
	for _, p := range projects {
		item := CatalogProject{
			ID:        p.ID,
			Name:      p.Name,
			Owner:     p.Owner.Username,
			UpdatedAt: p.UpdatedAt,
		}
		if lv, ok := latest[p.ID]; ok {
			n := lv.number
			item.LatestVersion = &n
			if lv.createdAt.After(item.UpdatedAt) {
				item.UpdatedAt = lv.createdAt
			}
		}
		switch {
		case p.OwnerID == userID:
			item.Role, item.Namespace = roleOwner, namespacePersonal
		case direct[p.ID] != "":
			item.Role, item.Namespace = direct[p.ID], p.Owner.Username
			if g, ok := viaGroup[p.ID]; ok && strongerRole(item.Role, g.role) != item.Role {
				item.Role = g.role
			}
		case viaGroup[p.ID].role != "":
			item.Role, item.Namespace = viaGroup[p.ID].role, viaGroup[p.ID].group
		default:
			// Admin without an explicit grant: admins can write any project.
			item.Role, item.Namespace = roleEdit, p.Owner.Username
		}
		if item.Namespace == "" {
			item.Namespace = namespacePersonal
		}
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := strings.ToLower(out[i].Name), strings.ToLower(out[j].Name)
		if a != b {
			return a < b
		}
		return out[i].ID.String() < out[j].ID.String()
	})
	return out, nil
}

// CreateCatalogProject creates an empty, ready server project owned by the
// caller (the first push of a local project).
func (s *Service) CreateCatalogProject(ctx context.Context, userID uuid.UUID, req CreateCatalogProjectRequest) (*CatalogProject, error) {
	name := strings.TrimSpace(req.Name)
	if err := artifact.ValidateWorkspaceName(name); err != nil {
		return nil, &ValidationError{Field: "name", Message: err.Error()}
	}
	db := s.deps.DB.WithContext(ctx)
	var owner models.User
	if err := db.Where("id = ?", userID).First(&owner).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrForbidden
		}
		return nil, err
	}
	project := models.Project{
		Name:    name,
		OwnerID: userID,
		Status:  models.ProjectStatusReady,
		Source:  "managed",
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&project).Error; err != nil {
			return fmt.Errorf("create project: %w", err)
		}
		return audit.LogAction(tx, userID, audit.ActionCreateProject, fmt.Sprintf("project:%s", project.ID), map[string]interface{}{
			"name":   project.Name,
			"origin": "client_push",
		})
	}); err != nil {
		return nil, err
	}
	// Casbin uses its own connection: grant outside the transaction.
	if s.deps.RBAC != nil {
		if err := s.deps.RBAC.GrantProjectAccess(userID, project.ID, "owner"); err != nil {
			return nil, fmt.Errorf("grant owner access: %w", err)
		}
	}
	return &CatalogProject{
		ID:        project.ID,
		Name:      project.Name,
		Owner:     owner.Username,
		Namespace: namespacePersonal,
		UpdatedAt: project.UpdatedAt,
		Role:      roleOwner,
	}, nil
}

// ListSyncVersions lists a project's versions (oldest first) with tags and
// publication records. ContentHash is always computed from the stored
// files (contenthash.Hash), so versions created by older server code
// compare correctly with pushed ones.
func (s *Service) ListSyncVersions(ctx context.Context, projectID uuid.UUID) ([]SyncVersion, error) {
	db := s.deps.DB.WithContext(ctx)
	if err := s.requireProject(db, projectID); err != nil {
		return nil, err
	}
	var rows []models.ProjectVersion
	if err := db.Preload("CreatedByUser").
		Where("project_id = ?", projectID).
		Order("version_number ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("listing versions: %w", err)
	}
	tags, err := s.tagsByVersion(db, projectID)
	if err != nil {
		return nil, err
	}
	pubs, err := s.PublicationsByVersion(ctx, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]SyncVersion, 0, len(rows))
	for i := range rows {
		out = append(out, syncVersionFrom(&rows[i], tags[rows[i].VersionNumber], pubs[rows[i].VersionNumber]))
	}
	return out, nil
}

// GetSyncVersion returns one version with its files.
func (s *Service) GetSyncVersion(ctx context.Context, projectID uuid.UUID, number int) (*SyncVersionFiles, error) {
	db := s.deps.DB.WithContext(ctx)
	if err := s.requireProject(db, projectID); err != nil {
		return nil, err
	}
	var row models.ProjectVersion
	err := db.Preload("CreatedByUser").
		Where("project_id = ? AND version_number = ?", projectID, number).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("version %d: %w", number, ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	tags, err := s.tagsByVersion(db, projectID)
	if err != nil {
		return nil, err
	}
	pubs, err := s.PublicationsByVersion(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return &SyncVersionFiles{
		SyncVersion: syncVersionFrom(&row, tags[number], pubs[number]),
		PixiToml:    row.ManifestContent,
		PixiLock:    row.LockFileContent,
	}, nil
}

// PushVersions stores pushed versions with their numbers, moves tags
// ("latest" always to the newest) and records publications. A number that
// exists with the same content hash is accepted idempotently; with other
// content it is a ConflictError "version_conflict". A number above the
// next free one is a ConflictError "server_ahead".
func (s *Service) PushVersions(ctx context.Context, userID, projectID uuid.UUID, req PushVersionsRequest) (*PushVersionsResult, error) {
	if err := validatePush(req); err != nil {
		return nil, err
	}
	db := s.deps.DB.WithContext(ctx)
	var pusher models.User
	if err := db.Where("id = ?", userID).First(&pusher).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrForbidden
		}
		return nil, err
	}

	result := &PushVersionsResult{Accepted: []int{}}
	created := []int{}
	err := db.Transaction(func(tx *gorm.DB) error {
		var project models.Project
		if err := tx.Where("id = ?", projectID).First(&project).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if project.Status != models.ProjectStatusReady {
			return &ConflictError{Code: "project_not_ready", Message: "The server project is not ready to receive versions."}
		}
		// Serialize pushes to one project: the update takes the row lock
		// (PostgreSQL) or the database write lock (SQLite) for the rest of
		// the transaction.
		now := time.Now().UTC()
		if err := tx.Model(&models.Project{}).Where("id = ?", projectID).Update("updated_at", now).Error; err != nil {
			return fmt.Errorf("lock project: %w", err)
		}

		var existing []models.ProjectVersion
		if err := tx.Select("id", "version_number", "manifest_content", "lock_file_content").
			Where("project_id = ?", projectID).Find(&existing).Error; err != nil {
			return fmt.Errorf("loading versions: %w", err)
		}
		hashes := make(map[int]string, len(existing))
		maxNumber := 0
		for _, v := range existing {
			hashes[v.VersionNumber] = contenthash.Hash(v.ManifestContent, v.LockFileContent)
			if v.VersionNumber > maxNumber {
				maxNumber = v.VersionNumber
			}
		}

		for _, pv := range req.Versions {
			hash := contenthash.Hash(pv.PixiToml, pv.PixiLock)
			if have, ok := hashes[pv.Number]; ok {
				if have != hash {
					return &ConflictError{
						Code:    "version_conflict",
						Message: fmt.Sprintf("Version %d on the server has different content.", pv.Number),
					}
				}
			} else {
				if pv.Number > maxNumber+1 {
					return &ConflictError{
						Code:    "server_ahead",
						Message: fmt.Sprintf("Version %d would skip versions: the server's newest version is %d.", pv.Number, maxNumber),
					}
				}
				author := strings.TrimSpace(pv.Author)
				if author == "" {
					author = pusher.Username
				}
				row := models.ProjectVersion{
					ProjectID:       projectID,
					VersionNumber:   pv.Number,
					ManifestContent: pv.PixiToml,
					LockFileContent: pv.PixiLock,
					PackageMetadata: "[]",
					ContentHash:     hash,
					CreatedBy:       userID,
					Description:     pv.Description,
					BaseVersion:     validBase(pv.BaseVersion, pv.Number),
					AuthorName:      author,
				}
				if err := tx.Create(&row).Error; err != nil {
					return fmt.Errorf("create version %d: %w", pv.Number, err)
				}
				hashes[pv.Number] = hash
				if pv.Number > maxNumber {
					maxNumber = pv.Number
				}
				created = append(created, pv.Number)
				// Keep the legacy content-hash tag so `nebi pull name:sha-...`
				// and legacy push deduplication keep working.
				if err := ensureTag(tx, projectID, hash, pv.Number, userID, false); err != nil {
					return err
				}
			}
			for _, tag := range pv.Tags {
				tag = strings.TrimSpace(tag)
				if tag == "" || tag == latestTag || isContentHashTag(tag) {
					continue
				}
				if err := ensureTag(tx, projectID, tag, pv.Number, userID, true); err != nil {
					return err
				}
			}
			if err := recordPushedPublications(tx, projectID, pv, pusher.Username, now); err != nil {
				return err
			}
			result.Accepted = append(result.Accepted, pv.Number)
		}
		if maxNumber > 0 {
			if err := ensureTag(tx, projectID, latestTag, maxNumber, userID, true); err != nil {
				return err
			}
		}
		result.LatestVersion = maxNumber
		return nil
	})
	if err != nil {
		return nil, err
	}
	_ = audit.Log(s.deps.DB, userID, audit.ActionPush, audit.ResourceProject, projectID, map[string]interface{}{
		"accepted": result.Accepted,
		"created":  created,
		"latest":   result.LatestVersion,
		"origin":   "client_sync",
	})
	return result, nil
}

// ---------------------------------------------------------------- helpers

type latestVersion struct {
	number    int
	createdAt time.Time
}

func (s *Service) latestVersions(db *gorm.DB, projects []models.Project) (map[uuid.UUID]latestVersion, error) {
	out := map[uuid.UUID]latestVersion{}
	if len(projects) == 0 {
		return out, nil
	}
	ids := make([]uuid.UUID, len(projects))
	for i, p := range projects {
		ids[i] = p.ID
	}
	var rows []struct {
		ProjectID     uuid.UUID
		VersionNumber int
		CreatedAt     time.Time
	}
	if err := db.Model(&models.ProjectVersion{}).
		Select("project_id", "version_number", "created_at").
		Where("project_id IN ?", ids).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("listing latest versions: %w", err)
	}
	for _, r := range rows {
		cur, ok := out[r.ProjectID]
		if !ok || r.VersionNumber > cur.number {
			out[r.ProjectID] = latestVersion{number: r.VersionNumber, createdAt: r.CreatedAt}
		}
	}
	return out, nil
}

func (s *Service) roleNames(db *gorm.DB) (map[uint]string, error) {
	var roles []models.Role
	if err := db.Find(&roles).Error; err != nil {
		return nil, fmt.Errorf("listing roles: %w", err)
	}
	out := make(map[uint]string, len(roles))
	for _, r := range roles {
		out[r.ID] = r.Name
	}
	return out, nil
}

func (s *Service) requireProject(db *gorm.DB, projectID uuid.UUID) error {
	var count int64
	if err := db.Model(&models.Project{}).Where("id = ?", projectID).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

// tagsByVersion returns the user-visible tags (no content-hash tags) per
// version number, sorted with "latest" first.
func (s *Service) tagsByVersion(db *gorm.DB, projectID uuid.UUID) (map[int][]string, error) {
	var tags []models.ProjectTag
	if err := db.Where("project_id = ?", projectID).Find(&tags).Error; err != nil {
		return nil, fmt.Errorf("listing tags: %w", err)
	}
	out := map[int][]string{}
	for _, t := range tags {
		if isContentHashTag(t.Tag) {
			continue
		}
		out[t.VersionNumber] = append(out[t.VersionNumber], t.Tag)
	}
	for n := range out {
		sortTags(out[n])
	}
	return out, nil
}

func sortTags(tags []string) {
	sort.Slice(tags, func(i, j int) bool {
		if tags[i] == latestTag || tags[j] == latestTag {
			return tags[i] == latestTag && tags[j] != latestTag
		}
		return tags[i] < tags[j]
	})
}

func syncVersionFrom(v *models.ProjectVersion, tags []string, pubs []PublicationRecord) SyncVersion {
	author := v.AuthorName
	if author == "" && v.CreatedByUser != nil {
		author = v.CreatedByUser.Username
	}
	if tags == nil {
		tags = []string{}
	}
	if pubs == nil {
		pubs = []PublicationRecord{}
	}
	return SyncVersion{
		Number:       v.VersionNumber,
		Description:  v.Description,
		Author:       author,
		CreatedAt:    v.CreatedAt,
		ContentHash:  contenthash.Hash(v.ManifestContent, v.LockFileContent),
		BaseVersion:  v.BaseVersion,
		Tags:         tags,
		Publications: pubs,
	}
}

func validatePush(req PushVersionsRequest) error {
	if len(req.Versions) == 0 {
		return &ValidationError{Field: "versions", Message: "Send at least one version."}
	}
	if len(req.Versions) > maxPushVersions {
		return &ValidationError{Field: "versions", Message: fmt.Sprintf("Push at most %d versions at a time.", maxPushVersions)}
	}
	prev := 0
	for _, v := range req.Versions {
		if v.Number < 1 {
			return &ValidationError{Field: "versions", Message: "Version numbers start at 1."}
		}
		if v.Number <= prev {
			return &ValidationError{Field: "versions", Message: "Versions must be sent in ascending order without repeats."}
		}
		prev = v.Number
		if strings.TrimSpace(v.PixiToml) == "" {
			return &ValidationError{Field: "versions", Message: fmt.Sprintf("Version %d has no pixi.toml.", v.Number)}
		}
		if len(v.Tags) > maxPushTagsPerItem {
			return &ValidationError{Field: "versions", Message: fmt.Sprintf("Version %d has too many tags.", v.Number)}
		}
		for _, tag := range v.Tags {
			tag = strings.TrimSpace(tag)
			if tag == "" || tag == latestTag || isContentHashTag(tag) {
				continue
			}
			if err := artifact.ValidateTag(tag); err != nil {
				return &ValidationError{Field: "versions", Message: err.Error()}
			}
		}
		for _, p := range v.Publications {
			if strings.TrimSpace(p.RegistryHost) == "" || strings.TrimSpace(p.Repository) == "" || strings.TrimSpace(p.Tag) == "" {
				return &ValidationError{Field: "versions", Message: fmt.Sprintf("Version %d has an incomplete publication record.", v.Number)}
			}
		}
	}
	return nil
}

// validBase keeps a base version only when it points at an earlier version.
func validBase(base *int, number int) *int {
	if base == nil || *base < 1 || *base >= number {
		return nil
	}
	b := *base
	return &b
}

// ensureTag points tag at version. With move=false an existing tag is left
// where it is (content-hash tags keep the first version with that content).
func ensureTag(tx *gorm.DB, projectID uuid.UUID, tag string, version int, userID uuid.UUID, move bool) error {
	var existing models.ProjectTag
	err := tx.Where("project_id = ? AND tag = ?", projectID, tag).First(&existing).Error
	switch {
	case err == nil:
		if !move || existing.VersionNumber == version {
			return nil
		}
		existing.VersionNumber = version
		if err := tx.Save(&existing).Error; err != nil {
			return fmt.Errorf("move tag %q: %w", tag, err)
		}
		return nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		if err := tx.Create(&models.ProjectTag{ProjectID: projectID, Tag: tag, VersionNumber: version, CreatedBy: userID}).Error; err != nil {
			return fmt.Errorf("create tag %q: %w", tag, err)
		}
		return nil
	default:
		return err
	}
}

// recordPushedPublications stores publication metadata sent with a pushed
// version, skipping records the server already has (same registry host,
// repository and tag on that version and not unpublished).
func recordPushedPublications(tx *gorm.DB, projectID uuid.UUID, pv PushVersion, pusher string, now time.Time) error {
	for _, rec := range pv.Publications {
		host := strings.TrimSpace(rec.RegistryHost)
		repo := strings.TrimSpace(rec.Repository)
		tag := strings.TrimSpace(rec.Tag)
		var count int64
		if err := tx.Model(&models.VersionPublication{}).
			Where("project_id = ? AND version_number = ? AND registry_host = ? AND repository = ? AND tag = ? AND unpublished_at IS NULL",
				projectID, pv.Number, host, repo, tag).
			Count(&count).Error; err != nil {
			return fmt.Errorf("checking publication: %w", err)
		}
		if count > 0 {
			continue
		}
		by := strings.TrimSpace(rec.PublishedBy)
		if by == "" {
			by = pusher
		}
		at := now
		if rec.PublishedAt != nil && !rec.PublishedAt.IsZero() {
			at = rec.PublishedAt.UTC()
		}
		row := models.VersionPublication{
			ProjectID:     projectID,
			VersionNumber: pv.Number,
			RegistryHost:  host,
			Repository:    repo,
			Tag:           tag,
			Digest:        strings.TrimSpace(rec.Digest),
			PublishedBy:   by,
			PublishedAt:   at,
		}
		if err := tx.Create(&row).Error; err != nil {
			return fmt.Errorf("recording publication: %w", err)
		}
	}
	return nil
}

// isContentHashTag reports whether tag is an internal content-hash tag
// ("sha-" + 12 lowercase hex), which the client never shows.
func isContentHashTag(tag string) bool {
	if len(tag) != 16 || !strings.HasPrefix(tag, "sha-") {
		return false
	}
	for _, r := range tag[4:] {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

func catalogRole(rbacRole string) string {
	switch rbacRole {
	case "owner", "admin":
		return roleOwner
	case "editor":
		return roleEdit
	case "viewer":
		return roleView
	default:
		return ""
	}
}

// strongerRole returns the more permissive of two catalog roles.
func strongerRole(a, b string) string {
	rank := map[string]int{"": 0, roleView: 1, roleEdit: 2, roleOwner: 3}
	if rank[b] > rank[a] {
		return b
	}
	return a
}

func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}
