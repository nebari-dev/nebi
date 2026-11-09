package remote

// Owner: U5 (remote). Server project list, search, refresh, details.

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
)

// ListServerProjects lists server projects (GET /api/v1/catalog/projects).
// Every successful fetch is a sync of the list, so it records the time in
// store_config.last_synced_at ("Last synced"); refresh=true additionally
// drops cached server version lists so markers refresh too.
func (s *Service) ListServerProjects(ctx context.Context, search string, refresh bool) (*shared.ServerProjectList, error) {
	conn, err := s.connection(ctx)
	if err != nil {
		return nil, err
	}
	path := "/catalog/projects"
	if q := strings.TrimSpace(search); q != "" {
		path += "?search=" + url.QueryEscape(q)
	}
	var list []catalogProject
	if err := s.ServerJSON(ctx, http.MethodGet, path, nil, &list); err != nil {
		return nil, err
	}
	if refresh {
		resetServerCache()
	}
	now := time.Now().UTC()
	if err := s.local.MarkServerSynced(ctx, now); err != nil {
		return nil, err
	}
	linked, err := s.linkedProjects(s.deps.DB.WithContext(ctx), conn.URL)
	if err != nil {
		return nil, err
	}
	out := &shared.ServerProjectList{Projects: make([]shared.ServerProjectItem, 0, len(list)), LastSyncedAt: &now}
	for _, p := range list {
		item := serverProjectItem(p)
		if id, ok := linked[p.ID]; ok {
			lid := id
			item.LocalProjectID = &lid
		}
		out.Projects = append(out.Projects, item)
	}
	return out, nil
}

// GetRemoteDetails is the "Remote details" view of a server project: the
// catalog entry, its address on the server and its versions (newest first;
// ServerOnly marks versions this machine does not have).
func (s *Service) GetRemoteDetails(ctx context.Context, serverProjectID string) (*shared.RemoteDetails, error) {
	conn, err := s.connection(ctx)
	if err != nil {
		return nil, err
	}
	sp, err := s.catalogProject(ctx, serverProjectID)
	if err != nil {
		return nil, err
	}
	st, err := s.fetchServerState(ctx, conn.URL, serverProjectID, false)
	if err != nil {
		return nil, err
	}
	db := s.deps.DB.WithContext(ctx)
	item := serverProjectItem(*sp)
	have := map[int]string{}
	if p, err := s.localProjectFor(db, conn.URL, serverProjectID); err != nil {
		return nil, err
	} else if p != nil {
		id := p.ID
		item.LocalProjectID = &id
		locals, err := localVersions(db, p.ID)
		if err != nil {
			return nil, err
		}
		for _, v := range locals {
			have[v.row.VersionNumber] = v.hash
		}
	}
	versions := make([]shared.VersionItem, 0, len(st.versions))
	for i := len(st.versions) - 1; i >= 0; i-- {
		v := serverOnlyItem(&st.versions[i])
		if h, ok := have[v.Number]; ok && h == st.versions[i].ContentHash {
			v.ServerOnly = false
		}
		versions = append(versions, v)
	}
	if len(versions) > 0 {
		m := shared.SyncNewestOnServer
		versions[0].Sync = &m
	}
	return &shared.RemoteDetails{
		Project:  item,
		URL:      conn.URL + "/projects/" + url.PathEscape(serverProjectID),
		Versions: versions,
	}, nil
}

func serverProjectItem(p catalogProject) shared.ServerProjectItem {
	item := shared.ServerProjectItem{
		ID:            p.ID,
		Name:          p.Name,
		Owner:         p.Owner,
		Namespace:     p.Namespace,
		LatestVersion: p.LatestVersion,
		Role:          p.Role,
	}
	if !p.UpdatedAt.IsZero() {
		t := p.UpdatedAt
		item.UpdatedAt = &t
	}
	return item
}

// linkedProjects maps server project IDs to the local projects pulled from
// (or pushed to) the given server.
func (s *Service) linkedProjects(db *gorm.DB, base string) (map[string]uuid.UUID, error) {
	var rows []struct {
		ProjectID       uuid.UUID
		ServerProjectID string
		ServerURL       string
	}
	err := db.Model(&models.ProjectRemote{}).
		Select("project_remotes.project_id", "project_remotes.server_project_id", "project_remotes.server_url").
		Joins("JOIN projects ON projects.id = project_remotes.project_id AND projects.deleted_at IS NULL").
		Where("project_remotes.kind = ?", models.RemoteKindServer).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := map[string]uuid.UUID{}
	for _, r := range rows {
		if sameServer(r.ServerURL, base) {
			out[r.ServerProjectID] = r.ProjectID
		}
	}
	return out, nil
}
