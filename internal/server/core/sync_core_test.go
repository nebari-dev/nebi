package core

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/contenthash"
	"github.com/nebari-dev/nebi/internal/db"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// fakeRBAC is a minimal rbac.Provider for core tests.
type fakeRBAC struct {
	admins  map[uuid.UUID]bool
	groups  map[uuid.UUID][]uuid.UUID
	granted []string
}

func (f *fakeRBAC) CanReadProject(uuid.UUID, uuid.UUID) (bool, error)  { return true, nil }
func (f *fakeRBAC) CanWriteProject(uuid.UUID, uuid.UUID) (bool, error) { return true, nil }
func (f *fakeRBAC) CanReadRegistry(uuid.UUID, uuid.UUID) (bool, error) { return true, nil }
func (f *fakeRBAC) CanWriteRegistry(uuid.UUID, uuid.UUID) (bool, error) {
	return true, nil
}
func (f *fakeRBAC) IsAdmin(u uuid.UUID) (bool, error) { return f.admins[u], nil }
func (f *fakeRBAC) GrantProjectAccess(u, p uuid.UUID, role string) error {
	f.granted = append(f.granted, u.String()+":"+p.String()+":"+role)
	return nil
}
func (f *fakeRBAC) RevokeProjectAccess(uuid.UUID, uuid.UUID) error { return nil }
func (f *fakeRBAC) MakeAdmin(uuid.UUID) error                      { return nil }
func (f *fakeRBAC) RevokeAdmin(uuid.UUID) error                    { return nil }
func (f *fakeRBAC) GetAllAdminUserIDs() (map[uuid.UUID]bool, error) {
	return f.admins, nil
}
func (f *fakeRBAC) AddUserToGroup(uuid.UUID, uuid.UUID) error      { return nil }
func (f *fakeRBAC) RemoveUserFromGroup(uuid.UUID, uuid.UUID) error { return nil }
func (f *fakeRBAC) GetUserGroups(u uuid.UUID) ([]uuid.UUID, error) {
	return f.groups[u], nil
}
func (f *fakeRBAC) GrantGroupProjectAccess(uuid.UUID, uuid.UUID, string) error  { return nil }
func (f *fakeRBAC) RevokeGroupProjectAccess(uuid.UUID, uuid.UUID) error         { return nil }
func (f *fakeRBAC) GrantGroupRegistryAccess(uuid.UUID, uuid.UUID, string) error { return nil }
func (f *fakeRBAC) RevokeGroupRegistryAccess(uuid.UUID, uuid.UUID) error        { return nil }
func (f *fakeRBAC) MakeGroupAdmin(uuid.UUID) error                              { return nil }
func (f *fakeRBAC) RevokeGroupAdmin(uuid.UUID) error                            { return nil }
func (f *fakeRBAC) RemoveAllGroupPolicies(uuid.UUID) error                      { return nil }

func newSyncTestCore(t *testing.T) (*Service, *gorm.DB, *fakeRBAC) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "core.db")
	gdb, err := gorm.Open(sqlite.Open(path+"?_pragma=busy_timeout(10000)&_pragma=foreign_keys(ON)"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := gdb.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.Migrate(gdb, false); err != nil {
		t.Fatal(err)
	}
	f := &fakeRBAC{admins: map[uuid.UUID]bool{}, groups: map[uuid.UUID][]uuid.UUID{}}
	return New(Deps{DB: gdb, RBAC: f}), gdb, f
}

func mkUser(t *testing.T, gdb *gorm.DB, name string) models.User {
	t.Helper()
	u := models.User{ID: uuid.New(), Username: name, Email: name + "@example.com"}
	if err := gdb.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	return u
}

func pushOne(n int, toml string, tags ...string) PushVersion {
	return PushVersion{Number: n, Description: "v", PixiToml: toml, PixiLock: "lock-" + toml, Tags: tags}
}

func TestCatalogCreateAndPushVersions(t *testing.T) {
	ctx := context.Background()
	c, gdb, rb := newSyncTestCore(t)
	sam := mkUser(t, gdb, "sam")

	proj, err := c.CreateCatalogProject(ctx, sam.ID, CreateCatalogProjectRequest{Name: "ml-baseline"})
	if err != nil {
		t.Fatal(err)
	}
	if proj.Role != "owner" || proj.Namespace != "Personal" || proj.Owner != "sam" || proj.LatestVersion != nil {
		t.Fatalf("unexpected catalog project: %+v", proj)
	}
	if len(rb.granted) != 1 {
		t.Fatalf("owner access not granted: %v", rb.granted)
	}

	base := 1
	res, err := c.PushVersions(ctx, sam.ID, proj.ID, PushVersionsRequest{Versions: []PushVersion{
		pushOne(1, "a", "stable"),
		{Number: 2, BaseVersion: &base, Description: "second", Author: "Sam Garcia", PixiToml: "b", PixiLock: "lb",
			Publications: []PublicationRecord{{RegistryHost: "ghcr.io", Repository: "org/ml", Tag: "v2", Digest: "sha256:1"}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Accepted) != 2 || res.LatestVersion != 2 {
		t.Fatalf("unexpected result %+v", res)
	}

	// Idempotent: the same content again is accepted, nothing duplicated.
	if _, err := c.PushVersions(ctx, sam.ID, proj.ID, PushVersionsRequest{Versions: []PushVersion{pushOne(1, "a")}}); err != nil {
		t.Fatalf("idempotent push failed: %v", err)
	}
	var count int64
	gdb.Model(&models.ProjectVersion{}).Where("project_id = ?", proj.ID).Count(&count)
	if count != 2 {
		t.Fatalf("want 2 versions, got %d", count)
	}

	versions, err := c.ListSyncVersions(ctx, proj.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 || versions[0].Number != 1 || versions[1].Number != 2 {
		t.Fatalf("unexpected versions %+v", versions)
	}
	if versions[0].ContentHash != contenthash.Hash("a", "lock-a") {
		t.Fatalf("hash mismatch: %s", versions[0].ContentHash)
	}
	if got := versions[0].Tags; len(got) != 1 || got[0] != "stable" {
		t.Fatalf("v1 tags = %v", got)
	}
	if got := versions[1].Tags; len(got) != 1 || got[0] != "latest" {
		t.Fatalf("v2 tags = %v", got)
	}
	if versions[1].Author != "Sam Garcia" || versions[0].Author != "sam" {
		t.Fatalf("authors = %q, %q", versions[0].Author, versions[1].Author)
	}
	if versions[1].BaseVersion == nil || *versions[1].BaseVersion != 1 {
		t.Fatalf("base version not kept")
	}
	var pubs []models.VersionPublication
	gdb.Where("project_id = ?", proj.ID).Find(&pubs)
	if len(pubs) != 1 || pubs[0].VersionNumber != 2 || pubs[0].PublishedBy != "sam" {
		t.Fatalf("publications = %+v", pubs)
	}

	files, err := c.GetSyncVersion(ctx, proj.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if files.PixiToml != "b" || files.PixiLock != "lb" {
		t.Fatalf("files = %+v", files)
	}
	if _, err := c.GetSyncVersion(ctx, proj.ID, 9); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing version: %v", err)
	}

	list, err := c.ListCatalogProjects(ctx, sam.ID, "BASE")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].LatestVersion == nil || *list[0].LatestVersion != 2 {
		t.Fatalf("catalog = %+v", list)
	}
	if list, _ := c.ListCatalogProjects(ctx, sam.ID, "nothing"); len(list) != 0 {
		t.Fatalf("search should filter: %+v", list)
	}
}

func TestPushVersionsConflicts(t *testing.T) {
	ctx := context.Background()
	c, gdb, _ := newSyncTestCore(t)
	sam := mkUser(t, gdb, "sam")
	proj, err := c.CreateCatalogProject(ctx, sam.ID, CreateCatalogProjectRequest{Name: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.PushVersions(ctx, sam.ID, proj.ID, PushVersionsRequest{Versions: []PushVersion{pushOne(1, "a")}}); err != nil {
		t.Fatal(err)
	}
	var cerr *ConflictError
	_, err = c.PushVersions(ctx, sam.ID, proj.ID, PushVersionsRequest{Versions: []PushVersion{pushOne(1, "other")}})
	if !errors.As(err, &cerr) || cerr.Code != "version_conflict" {
		t.Fatalf("want version_conflict, got %v", err)
	}
	_, err = c.PushVersions(ctx, sam.ID, proj.ID, PushVersionsRequest{Versions: []PushVersion{pushOne(3, "c")}})
	if !errors.As(err, &cerr) || cerr.Code != "server_ahead" {
		t.Fatalf("want server_ahead, got %v", err)
	}
	// A failed push stores nothing.
	_, err = c.PushVersions(ctx, sam.ID, proj.ID, PushVersionsRequest{Versions: []PushVersion{pushOne(2, "b"), pushOne(4, "d")}})
	if !errors.As(err, &cerr) {
		t.Fatalf("want conflict, got %v", err)
	}
	var count int64
	gdb.Model(&models.ProjectVersion{}).Where("project_id = ?", proj.ID).Count(&count)
	if count != 1 {
		t.Fatalf("partial push stored versions: %d", count)
	}
	var verr *ValidationError
	_, err = c.PushVersions(ctx, sam.ID, proj.ID, PushVersionsRequest{Versions: []PushVersion{pushOne(2, "b"), pushOne(2, "b")}})
	if !errors.As(err, &verr) {
		t.Fatalf("want validation error for repeats, got %v", err)
	}
	_, err = c.PushVersions(ctx, sam.ID, proj.ID, PushVersionsRequest{Versions: []PushVersion{pushOne(2, "b", "1bad")}})
	if !errors.As(err, &verr) {
		t.Fatalf("want validation error for tag, got %v", err)
	}
	if _, err := c.PushVersions(ctx, sam.ID, uuid.New(), PushVersionsRequest{Versions: []PushVersion{pushOne(1, "a")}}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown project: %v", err)
	}
}

func TestCatalogVisibilityAndRoles(t *testing.T) {
	ctx := context.Background()
	c, gdb, rb := newSyncTestCore(t)
	sam := mkUser(t, gdb, "sam")
	alex := mkUser(t, gdb, "alex")
	jo := mkUser(t, gdb, "jo")
	admin := mkUser(t, gdb, "root")
	rb.admins[admin.ID] = true

	own, _ := c.CreateCatalogProject(ctx, sam.ID, CreateCatalogProjectRequest{Name: "sams"})
	shared, _ := c.CreateCatalogProject(ctx, alex.ID, CreateCatalogProjectRequest{Name: "alexs"})
	team, _ := c.CreateCatalogProject(ctx, jo.ID, CreateCatalogProjectRequest{Name: "team-proj"})
	_, _ = c.CreateCatalogProject(ctx, jo.ID, CreateCatalogProjectRequest{Name: "private"})

	var viewer, editor models.Role
	gdb.Where("name = ?", "viewer").First(&viewer)
	gdb.Where("name = ?", "editor").First(&editor)
	gdb.Create(&models.Permission{UserID: sam.ID, ProjectID: shared.ID, RoleID: viewer.ID})
	g := models.Group{Name: "team-jatic"}
	gdb.Create(&g)
	gdb.Create(&models.GroupPermission{GroupID: g.ID, ProjectID: team.ID, RoleID: editor.ID})
	rb.groups[sam.ID] = []uuid.UUID{g.ID}

	list, err := c.ListCatalogProjects(ctx, sam.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	got := map[uuid.UUID]CatalogProject{}
	for _, p := range list {
		got[p.ID] = p
	}
	if len(list) != 3 {
		t.Fatalf("sam should see 3 projects, got %+v", list)
	}
	if p := got[own.ID]; p.Role != "owner" || p.Namespace != "Personal" {
		t.Fatalf("own = %+v", p)
	}
	if p := got[shared.ID]; p.Role != "view" || p.Namespace != "alex" {
		t.Fatalf("shared = %+v", p)
	}
	if p := got[team.ID]; p.Role != "edit" || p.Namespace != "team-jatic" {
		t.Fatalf("team = %+v", p)
	}

	all, err := c.ListCatalogProjects(ctx, admin.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 4 {
		t.Fatalf("admin should see every project, got %d", len(all))
	}
}
