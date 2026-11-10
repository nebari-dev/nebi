package registry_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	stdlog "log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	ggcr "github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client"
	"github.com/nebari-dev/nebi/internal/client/sdk/local"
	"github.com/nebari-dev/nebi/internal/client/sdk/registry"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/config"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/oci"
	"github.com/nebari-dev/nebi/internal/store"
	"github.com/zalando/go-keyring"
)

func init() { keyring.MockInit() }

const (
	testToml = "[workspace]\nname = \"project_1\"\nchannels = [\"conda-forge\"]\nplatforms = [\"linux-64\"]\n\n[dependencies]\npython = \"3.12.*\"\n"
	testLock = "version: 6\nenvironments: {}\npackages: []\n"
)

// newStack builds a CLI-style stack (jobs run inline) on a fresh data dir.
func newStack(t *testing.T) *client.Stack {
	t.Helper()
	dataDir := t.TempDir()
	gdb, err := local.OpenDB(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := gdb.DB(); err == nil {
			sqlDB.Close()
		}
	})
	if err := local.Migrate(gdb); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Mode: config.ModeLocal}
	cfg.Storage.ProjectsDir = filepath.Join(dataDir, "projects")
	exec, err := executor.NewLocalExecutor(cfg)
	if err != nil {
		t.Fatal(err)
	}
	s, err := client.NewStack(client.StackOptions{
		DB:          gdb,
		Executor:    exec,
		DataDir:     dataDir,
		ProjectsDir: cfg.Storage.ProjectsDir,
		InlineLog:   io.Discard,
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func startRegistry(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(ggcr.New(ggcr.Logger(stdlog.New(io.Discard, "", 0))))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	return u.Host
}

func createProject(t *testing.T, s *client.Stack, name string, versions int) models.Project {
	t.Helper()
	gdb := s.SDK.Deps().DB
	user := s.SDK.Deps().LocalUserID
	p := models.Project{Name: name, OwnerID: user, Status: models.ProjectStatusReady, Path: t.TempDir()}
	if err := gdb.Omit("Owner").Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	for n := 1; n <= versions; n++ {
		v := models.ProjectVersion{ProjectID: p.ID, VersionNumber: n, ManifestContent: testToml, LockFileContent: testLock + "# v" + string(rune('0'+n)) + "\n", PackageMetadata: "[]", CreatedBy: user}
		if err := gdb.Omit("Project", "Job", "CreatedByUser").Create(&v).Error; err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func TestRegistryCRUD(t *testing.T) {
	ctx := context.Background()
	s := newStack(t)
	reg := s.SDK.Registry

	items, err := reg.ListRegistries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "nebari-environments" || !items[0].IsDefault || !items[0].CanUnpublish {
		t.Fatalf("seeded registries = %+v", items)
	}

	var verr *shared.ValidationError
	if _, err := reg.AddRegistry(ctx, shared.AddRegistryRequest{URL: "ghcr.io"}); !errors.As(err, &verr) || verr.Field != "name" {
		t.Fatalf("missing name: %v", err)
	}
	if _, err := reg.AddRegistry(ctx, shared.AddRegistryRequest{Name: "x", URL: "not a url"}); !errors.As(err, &verr) || verr.Field != "url" {
		t.Fatalf("bad url: %v", err)
	}
	if _, err := reg.AddRegistry(ctx, shared.AddRegistryRequest{Name: "x", URL: "ghcr.io", Password: "pw"}); !errors.As(err, &verr) || verr.Field != "username" {
		t.Fatalf("password without username: %v", err)
	}
	if _, err := reg.AddRegistry(ctx, shared.AddRegistryRequest{Name: "NEBARI-environments", URL: "quay.io"}); !errors.As(err, &verr) || verr.Field != "name" {
		t.Fatalf("duplicate name: %v", err)
	}

	gh, err := reg.AddRegistry(ctx, shared.AddRegistryRequest{
		Name: "GitHub Container Registry", URL: "https://ghcr.io/", Namespace: "example-org",
		Username: "sam", Password: "secret", IsDefault: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if gh.URL != "ghcr.io" || !gh.HasCredentials || !gh.IsDefault || gh.CanUnpublish || gh.UnpublishReason == "" {
		t.Fatalf("added registry = %+v", gh)
	}
	if pw, _ := s.SDK.Local.RegistryPassword(ctx, gh.Name); pw != "secret" {
		t.Fatalf("password not in keyring: %q", pw)
	}
	var row models.OCIRegistry
	s.SDK.Deps().DB.First(&row, "id = ?", gh.ID)
	if row.Password != "" {
		t.Fatal("password stored in SQLite")
	}
	items, _ = reg.ListRegistries(ctx)
	if items[0].ID != gh.ID {
		t.Fatalf("default registry must be listed first: %+v", items)
	}
	defaults := 0
	for _, it := range items {
		if it.IsDefault {
			defaults++
		}
	}
	if defaults != 1 {
		t.Fatalf("want exactly one default, got %d", defaults)
	}

	// Rename moves the password to the new name.
	newName := "GHCR"
	if _, err := reg.UpdateRegistry(ctx, gh.ID, registry.UpdateRegistryRequest{Name: &newName}); err != nil {
		t.Fatal(err)
	}
	if pw, _ := s.SDK.Local.RegistryPassword(ctx, "GHCR"); pw != "secret" {
		t.Fatalf("password did not follow the rename: %q", pw)
	}
	if pw, _ := s.SDK.Local.RegistryPassword(ctx, "GitHub Container Registry"); pw != "" {
		t.Fatal("old keyring entry left behind")
	}
	resolved, err := reg.ResolveRegistry(ctx, "ghcr")
	if err != nil || resolved.ID != gh.ID {
		t.Fatalf("ResolveRegistry by name: %v %+v", err, resolved)
	}

	// A legacy CLI row must not come back after removal.
	legacy := store.LocalRegistry{ID: gh.ID, Name: "GHCR", URL: "ghcr.io"}
	if err := s.SDK.Deps().DB.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	if err := reg.RemoveRegistry(ctx, gh.ID); err != nil {
		t.Fatal(err)
	}
	if err := local.Migrate(s.SDK.Deps().DB); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.GetRegistry(ctx, gh.ID); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("removed registry still there: %v", err)
	}
	if pw, _ := s.SDK.Local.RegistryPassword(ctx, "GHCR"); pw != "" {
		t.Fatal("password left in keyring after remove")
	}
	items, _ = reg.ListRegistries(ctx)
	if len(items) != 1 || !items[0].IsDefault {
		t.Fatalf("the remaining registry must become the default: %+v", items)
	}
}

func TestPublishUnpublishAndBadge(t *testing.T) {
	ctx := context.Background()
	s := newStack(t)
	reg := s.SDK.Registry
	host := startRegistry(t)
	local1, err := reg.AddRegistry(ctx, shared.AddRegistryRequest{Name: "Local", URL: "http://" + host, Namespace: "team"})
	if err != nil {
		t.Fatal(err)
	}
	p := createProject(t, s, "Project_1", 3)

	defaults, err := reg.PublishDefaults(ctx, p.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	var d shared.PublishDefault
	for _, x := range defaults {
		if x.RegistryID == local1.ID {
			d = x
		}
	}
	if d.Repository != "team/project_1" || d.Tag != "2" {
		t.Fatalf("defaults = %+v", defaults)
	}

	var verr *shared.ValidationError
	if _, err := reg.Publish(ctx, p.ID, 2, shared.PublishRequest{Targets: []shared.PublishTarget{{RegistryID: local1.ID, Repository: "Team/Bad Name", Tag: "2"}}}); !errors.As(err, &verr) || verr.Message != registry.ReferenceHint {
		t.Fatalf("invalid reference: %v", err)
	}
	if _, err := reg.Publish(ctx, p.ID, 9, shared.PublishRequest{Targets: []shared.PublishTarget{{RegistryID: local1.ID, Repository: "team/p", Tag: "9"}}}); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("missing version: %v", err)
	}

	// One reachable registry, one that is down: per-registry results.
	down, err := reg.AddRegistry(ctx, shared.AddRegistryRequest{Name: "Down", URL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	res, err := reg.Publish(ctx, p.ID, 2, shared.PublishRequest{Targets: []shared.PublishTarget{
		{RegistryID: local1.ID, Repository: "team/project_1", Tag: "2"},
		{RegistryID: local1.ID, Repository: "team/project_1", Tag: "stable"},
		{RegistryID: down.ID, Repository: "project_1", Tag: "2"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Results) != 3 {
		t.Fatalf("results = %+v", res.Results)
	}
	if r := res.Results[0]; r.Status != registry.StatusPublished || r.Digest == "" || r.PublicationID == nil || r.Reference != host+"/team/project_1:2" {
		t.Fatalf("result 0 = %+v", r)
	}
	if r := res.Results[2]; r.Status != registry.StatusFailed || r.Error == "" || r.PublicationID != nil {
		t.Fatalf("result 2 = %+v", r)
	}
	pulled, err := oci.PullBundle(ctx, host+"/team/project_1", "2", oci.PullOptions{PlainHTTP: true})
	if err != nil {
		t.Fatalf("published artifact not pullable: %v", err)
	}
	if !strings.Contains(pulled.PixiLock, "# v2") {
		t.Fatalf("published the wrong version: %q", pulled.PixiLock)
	}

	// The badge decorator reports both local publications of version 2.
	items, err := reg.DecorateVersions(ctx, p.ID, []shared.VersionItem{{Number: 3}, {Number: 2}, {Number: 1}, {Number: 4, ServerOnly: true}})
	if err != nil {
		t.Fatal(err)
	}
	if len(items[1].Published) != 2 || len(items[0].Published) != 0 || len(items[3].Published) != 0 {
		t.Fatalf("badges = %+v", items)
	}
	b := items[1].Published[0]
	if b.Source != "local" || !b.CanUnpublish || b.Registry != host || b.Reference != host+"/team/project_1:2" || b.RegistryID == nil || *b.RegistryID != local1.ID {
		t.Fatalf("badge = %+v", b)
	}

	// Publishing "stable" on version 3 moves the tag.
	if _, err := reg.Publish(ctx, p.ID, 3, shared.PublishRequest{Targets: []shared.PublishTarget{{RegistryID: local1.ID, Repository: "team/project_1", Tag: "stable"}}}); err != nil {
		t.Fatal(err)
	}
	refs, _ := reg.PublishedRefs(ctx, p.ID)
	if len(refs[2]) != 1 || len(refs[3]) != 1 || refs[3][0].Tag != "stable" {
		t.Fatalf("tag did not move: %+v", refs)
	}
	// Defaults for version 2 now show its active publication.
	defaults, _ = reg.PublishDefaults(ctx, p.ID, 2)
	for _, x := range defaults {
		if x.RegistryID == local1.ID && (x.Repository != "team/project_1" || x.Tag != "2") {
			t.Fatalf("defaults after publish = %+v", x)
		}
	}
	// The registry is recorded as a remote of the project.
	var remotes []models.ProjectRemote
	s.SDK.Deps().DB.Where("project_id = ? AND kind = ?", p.ID, models.RemoteKindRegistry).Find(&remotes)
	if len(remotes) != 1 || remotes[0].Name != host+"/team" {
		t.Fatalf("remotes = %+v", remotes)
	}

	// Unpublish removes the tag from the registry and the badge.
	if err := reg.Unpublish(ctx, p.ID, *refs[2][0].PublicationID); err != nil {
		t.Fatalf("Unpublish: %v", err)
	}
	if _, err := oci.PullBundle(ctx, host+"/team/project_1", "2", oci.PullOptions{PlainHTTP: true}); err == nil {
		t.Fatal("tag 2 still in the registry")
	}
	if _, err := oci.PullBundle(ctx, host+"/team/project_1", "stable", oci.PullOptions{PlainHTTP: true}); err != nil {
		t.Fatalf("unpublishing tag 2 removed stable: %v", err)
	}
	refs, _ = reg.PublishedRefs(ctx, p.ID)
	if len(refs[2]) != 0 {
		t.Fatalf("badge still there: %+v", refs)
	}
	if err := reg.Unpublish(ctx, p.ID, uuid.New()); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("unknown publication: %v", err)
	}
}

func TestUnpublishUnsupportedRegistry(t *testing.T) {
	ctx := context.Background()
	s := newStack(t)
	gh, err := s.SDK.Registry.AddRegistry(ctx, shared.AddRegistryRequest{Name: "GitHub", URL: "ghcr.io", Namespace: "org"})
	if err != nil {
		t.Fatal(err)
	}
	p := createProject(t, s, "p", 1)
	pub := models.Publication{ProjectID: p.ID, VersionNumber: 1, RegistryID: gh.ID, Repository: "org/p", Tag: "1", RegistryHost: "ghcr.io", PublishedBy: s.SDK.Deps().LocalUserID}
	if err := s.SDK.Deps().DB.Omit("Project", "Registry", "PublishedByUser").Create(&pub).Error; err != nil {
		t.Fatal(err)
	}
	refs, _ := s.SDK.Registry.PublishedRefs(ctx, p.ID)
	if len(refs[1]) != 1 || refs[1][0].CanUnpublish {
		t.Fatalf("ghcr badge must not offer unpublish: %+v", refs)
	}
	err = s.SDK.Registry.Unpublish(ctx, p.ID, pub.ID)
	if !errors.Is(err, shared.ErrUnavailable) || !strings.Contains(err.Error(), "GitHub") {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

func TestPullFromRegistry(t *testing.T) {
	ctx := context.Background()
	s := newStack(t)
	reg := s.SDK.Registry
	host := startRegistry(t)
	r, err := reg.AddRegistry(ctx, shared.AddRegistryRequest{Name: "Local", URL: "http://" + host, Namespace: "team"})
	if err != nil {
		t.Fatal(err)
	}
	ep := oci.Registry{Host: host, PlainHTTP: true}
	if _, err := oci.PublishFiles(ctx, []byte(testToml), []byte(testLock), ep, "team/ml-baseline", "stable"); err != nil {
		t.Fatal(err)
	}
	if _, err := oci.PublishFiles(ctx, []byte(testToml), []byte(testLock), ep, "other/thing", "1"); err != nil {
		t.Fatal(err)
	}

	repos, err := reg.ListRepositories(ctx, r.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || repos[0] != "team/ml-baseline" {
		t.Fatalf("repositories outside the namespace leaked: %v", repos)
	}
	if repos, _ := reg.ListRepositories(ctx, r.ID, "nothing"); len(repos) != 0 {
		t.Fatalf("search: %v", repos)
	}
	tags, err := reg.ListTags(ctx, r.ID, "team/ml-baseline")
	if err != nil || len(tags) != 1 || tags[0] != "stable" {
		t.Fatalf("tags = %v, %v", tags, err)
	}

	res, err := reg.Pull(ctx, r.ID, shared.RegistryPullRequest{Repository: "team/ml-baseline", Tag: "stable"})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if res.Job.Status != models.JobStatusCompleted || res.Job.Type != models.JobTypePullRegistry {
		t.Fatalf("job = %+v", res.Job)
	}
	var p models.Project
	s.SDK.Deps().DB.First(&p, "id = ?", res.ProjectID)
	if p.Name != "ml-baseline" || p.Status != models.ProjectStatusReady {
		t.Fatalf("project = %+v", p)
	}
	if b, err := os.ReadFile(filepath.Join(p.Path, "pixi.toml")); err != nil || string(b) != testToml {
		t.Fatalf("pixi.toml on disk = %q, %v", b, err)
	}
	v, err := s.SDK.Local.LoadVersion(ctx, p.ID, 1)
	if err != nil || v.LockFileContent != testLock {
		t.Fatalf("version 1 = %+v, %v", v, err)
	}
	var tagRows []models.ProjectTag
	s.SDK.Deps().DB.Where("project_id = ?", p.ID).Find(&tagRows)
	got := map[string]int{}
	for _, tg := range tagRows {
		got[tg.Tag] = tg.VersionNumber
	}
	if got["stable"] != 1 || got["latest"] != 1 {
		t.Fatalf("tags = %v", got)
	}
	var job models.Job
	s.SDK.Deps().DB.First(&job, "id = ?", res.Job.ID)
	if job.VersionNumber == nil || *job.VersionNumber != 1 {
		t.Fatalf("job version = %v", job.VersionNumber)
	}

	var cerr *shared.ConflictError
	if _, err := reg.Pull(ctx, r.ID, shared.RegistryPullRequest{Repository: "team/ml-baseline", Tag: "stable", LocalName: "ML-Baseline"}); !errors.As(err, &cerr) || cerr.Code != "name_taken" {
		t.Fatalf("name taken: %v", err)
	}
	var verr *shared.ValidationError
	if _, err := reg.Pull(ctx, r.ID, shared.RegistryPullRequest{Repository: "team/ml-baseline", Tag: "stable", LocalName: "bad name"}); !errors.As(err, &verr) || verr.Field != "local_name" {
		t.Fatalf("bad name: %v", err)
	}

	// A failed pull removes the pending project and its folder.
	res, err = reg.Pull(ctx, r.ID, shared.RegistryPullRequest{Repository: "team/ml-baseline", Tag: "missing", LocalName: "gone"})
	if err == nil {
		t.Fatal("pull of a missing tag succeeded")
	}
	var count int64
	s.SDK.Deps().DB.Model(&models.Project{}).Where("name = ?", "gone").Count(&count)
	if count != 0 {
		t.Fatal("failed pull left the project behind")
	}
	if _, statErr := os.Stat(filepath.Join(s.SDK.Deps().DefaultProjectsDir(), "gone")); !os.IsNotExist(statErr) {
		t.Fatal("failed pull left the folder behind")
	}
	if res == nil || res.Job.Status != models.JobStatusFailed {
		t.Fatalf("failed pull result = %+v", res)
	}
}

func TestPublishReportsToLinkedServer(t *testing.T) {
	ctx := context.Background()
	s := newStack(t)
	reg := s.SDK.Registry
	host := startRegistry(t)
	r, err := reg.AddRegistry(ctx, shared.AddRegistryRequest{Name: "Local", URL: "http://" + host, Username: "u", Password: "registry-secret"})
	if err != nil {
		t.Fatal(err)
	}

	type call struct{ method, path, body, auth string }
	var mu sync.Mutex
	var calls []call
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		b, _ := io.ReadAll(req.Body)
		mu.Lock()
		calls = append(calls, call{req.Method, req.URL.Path, string(b), req.Header.Get("Authorization")})
		mu.Unlock()
		if req.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write(b)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	gdb := s.SDK.Deps().DB
	gdb.Save(&store.Config{ID: 1, ServerURL: srv.URL})
	gdb.Save(&store.Credentials{ID: 1, Token: "tok"})
	p := createProject(t, s, "linked", 2)
	gdb.Create(&models.ProjectRemote{ProjectID: p.ID, Kind: models.RemoteKindServer, Name: "team", ServerURL: srv.URL, ServerProjectID: "srv-1"})
	now := time.Now()
	gdb.Model(&models.ProjectVersion{}).Where("project_id = ? AND version_number = 1", p.ID).Update("synced_at", now)

	// Version 2 is not on the server yet: nothing is reported.
	if _, err := reg.Publish(ctx, p.ID, 2, shared.PublishRequest{Targets: []shared.PublishTarget{{RegistryID: r.ID, Repository: "linked", Tag: "2"}}}); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 0 {
		t.Fatalf("reported an unsynced version: %+v", calls)
	}
	res, err := reg.Publish(ctx, p.ID, 1, shared.PublishRequest{Targets: []shared.PublishTarget{{RegistryID: r.ID, Repository: "linked", Tag: "1"}}})
	if err != nil || res.Results[0].Status != registry.StatusPublished {
		t.Fatalf("publish: %v %+v", err, res)
	}
	if len(calls) != 1 || calls[0].method != http.MethodPost || calls[0].path != "/api/v1/projects/srv-1/sync/versions/1/publications" || calls[0].auth != "Bearer tok" {
		t.Fatalf("calls = %+v", calls)
	}
	if strings.Contains(calls[0].body, "registry-secret") || strings.Contains(calls[0].body, "password") {
		t.Fatalf("credentials sent to the server: %s", calls[0].body)
	}
	var rec registry.ServerPublication
	if err := json.Unmarshal([]byte(calls[0].body), &rec); err != nil || rec.ID == nil || *rec.ID != *res.Results[0].PublicationID || rec.RegistryHost != host || rec.Repository != "linked" {
		t.Fatalf("record = %+v (%v)", rec, err)
	}

	if err := reg.Unpublish(ctx, p.ID, *res.Results[0].PublicationID); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[1].method != http.MethodDelete || calls[1].path != "/api/v1/projects/srv-1/sync/versions/1/publications/"+res.Results[0].PublicationID.String() {
		t.Fatalf("calls = %+v", calls)
	}
}

// Registry rejections and unreachable registries get plain messages.
func TestPublishFailureMessages(t *testing.T) {
	ctx := context.Background()
	s := newStack(t)
	reg := s.SDK.Registry
	inner := ggcr.New(ggcr.Logger(stdlog.New(io.Discard, "", 0)))
	locked := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if u, p, ok := req.BasicAuth(); !ok || u != "sam" || p != "right" {
			w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		inner.ServeHTTP(w, req)
	}))
	t.Cleanup(locked.Close)
	lockedHost := strings.TrimPrefix(locked.URL, "http://")
	closed := httptest.NewServer(http.NotFoundHandler())
	closedHost := strings.TrimPrefix(closed.URL, "http://")
	closed.Close()

	anon, err := reg.AddRegistry(ctx, shared.AddRegistryRequest{Name: "Locked", URL: "http://" + lockedHost})
	if err != nil {
		t.Fatal(err)
	}
	wrong, err := reg.AddRegistry(ctx, shared.AddRegistryRequest{Name: "Wrong", URL: "http://" + lockedHost, Username: "sam", Password: "wrong"})
	if err != nil {
		t.Fatal(err)
	}
	right, err := reg.AddRegistry(ctx, shared.AddRegistryRequest{Name: "Right", URL: "http://" + lockedHost, Username: "sam", Password: "right"})
	if err != nil {
		t.Fatal(err)
	}
	gone, err := reg.AddRegistry(ctx, shared.AddRegistryRequest{Name: "Gone", URL: "http://" + closedHost})
	if err != nil {
		t.Fatal(err)
	}
	p := createProject(t, s, "msgs", 1)
	res, err := reg.Publish(ctx, p.ID, 1, shared.PublishRequest{Targets: []shared.PublishTarget{
		{RegistryID: anon.ID, Repository: "msgs", Tag: "1"},
		{RegistryID: wrong.ID, Repository: "msgs", Tag: "1"},
		{RegistryID: right.ID, Repository: "msgs", Tag: "1"},
		{RegistryID: gone.ID, Repository: "msgs", Tag: "1"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := []struct{ status, msg string }{
		{registry.StatusFailed, "Locked needs a username and password to publish. Add them on the Registries page."},
		{registry.StatusFailed, "Wrong didn’t accept the username and password. Check them on the Registries page."},
		{registry.StatusPublished, ""},
		{registry.StatusFailed, "Couldn’t reach " + closedHost + ". Check the registry URL and your connection."},
	}
	for i, w := range want {
		got := res.Results[i]
		if got.Status != w.status || got.Error != w.msg {
			t.Errorf("result %d = %s %q, want %s %q", i, got.Status, got.Error, w.status, w.msg)
		}
	}
}

// A publication that reached the server with a pushed version has a server
// ID; unpublishing it removes the server record with the same reference.
func TestUnpublishRemovesPushedServerRecord(t *testing.T) {
	ctx := context.Background()
	s := newStack(t)
	reg := s.SDK.Registry
	host := startRegistry(t)
	r, err := reg.AddRegistry(ctx, shared.AddRegistryRequest{Name: "Local", URL: "http://" + host})
	if err != nil {
		t.Fatal(err)
	}
	p := createProject(t, s, "pushed", 1)
	res, err := reg.Publish(ctx, p.ID, 1, shared.PublishRequest{Targets: []shared.PublishTarget{{RegistryID: r.ID, Repository: "pushed", Tag: "stable"}}})
	if err != nil || res.Results[0].Status != registry.StatusPublished {
		t.Fatalf("publish: %v %+v", err, res)
	}

	serverID := uuid.New().String()
	otherID := uuid.New().String()
	var mu sync.Mutex
	var deleted []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		const prefix = "/api/v1/projects/srv-1/sync/versions/1"
		switch {
		case req.Method == http.MethodGet && req.URL.Path == prefix:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"number": 1,
				"publications": []map[string]any{
					{"id": serverID, "registry_host": host, "repository": "pushed", "tag": "stable"},
					{"id": otherID, "registry_host": host, "repository": "pushed", "tag": "1"},
				},
			})
		case req.Method == http.MethodDelete && strings.HasPrefix(req.URL.Path, prefix+"/publications/"):
			id := strings.TrimPrefix(req.URL.Path, prefix+"/publications/")
			if id != serverID {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			deleted = append(deleted, id)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusTeapot)
		}
	}))
	t.Cleanup(srv.Close)
	gdb := s.SDK.Deps().DB
	gdb.Save(&store.Config{ID: 1, ServerURL: srv.URL})
	gdb.Save(&store.Credentials{ID: 1, Token: "tok"})
	gdb.Create(&models.ProjectRemote{ProjectID: p.ID, Kind: models.RemoteKindServer, Name: "team", ServerURL: srv.URL, ServerProjectID: "srv-1"})

	if err := reg.Unpublish(ctx, p.ID, *res.Results[0].PublicationID); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(deleted) != 1 || deleted[0] != serverID {
		t.Fatalf("deleted server records = %v, want only %s", deleted, serverID)
	}
}
