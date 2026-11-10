package core

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/config"
	"github.com/nebari-dev/nebi/internal/db"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/limits"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/queue"
	"github.com/nebari-dev/nebi/internal/rbac"
	"github.com/nebari-dev/nebi/internal/service"
	"gorm.io/gorm"
)

type sharingFixture struct {
	svc     *Service
	db      *gorm.DB
	rbac    rbac.Provider
	owner   uuid.UUID
	project uuid.UUID
}

// newSharingFixture builds a team-mode core on a fresh database with the
// global casbin enforcer (tests in this package must not run in parallel).
func newSharingFixture(t *testing.T) *sharingFixture {
	t.Helper()
	cfg := &config.Config{Mode: config.ModeTeam}
	cfg.Database.Driver = "sqlite"
	cfg.Database.DSN = filepath.Join(t.TempDir(), "core-sharing.db")
	cfg.Storage.ProjectsDir = t.TempDir()
	gdb, err := db.New(cfg.Database)
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	if err := db.Migrate(gdb, false); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := rbac.InitEnforcer(gdb, logger); err != nil {
		t.Fatalf("InitEnforcer: %v", err)
	}
	exec, err := executor.NewLocalExecutor(cfg)
	if err != nil {
		t.Fatalf("executor: %v", err)
	}
	q := queue.NewMemoryQueue(16)
	t.Cleanup(func() { q.Close() })
	provider := rbac.NewDefaultProvider()
	projects := service.New(gdb, q, exec, false, nil, provider, limits.Defaults())

	f := &sharingFixture{
		svc:  New(Deps{DB: gdb, RBAC: provider, Projects: projects, Logger: logger}),
		db:   gdb,
		rbac: provider,
	}
	f.owner = f.user(t, "olivia")
	p := models.Project{Name: "ml-baseline", OwnerID: f.owner, Status: models.ProjectStatusReady}
	if err := gdb.Create(&p).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := provider.GrantProjectAccess(f.owner, p.ID, "owner"); err != nil {
		t.Fatalf("grant owner: %v", err)
	}
	f.project = p.ID
	return f
}

func (f *sharingFixture) user(t *testing.T, name string) uuid.UUID {
	t.Helper()
	u := models.User{Username: name, Email: name + "@example.com"}
	if err := f.db.Create(&u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return u.ID
}

func (f *sharingFixture) group(t *testing.T, name string, members ...uuid.UUID) uuid.UUID {
	t.Helper()
	g := models.Group{Name: name}
	if err := f.db.Create(&g).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	for _, m := range members {
		if err := f.db.Create(&models.GroupMember{GroupID: g.ID, UserID: m}).Error; err != nil {
			t.Fatalf("add member: %v", err)
		}
		if err := f.rbac.AddUserToGroup(m, g.ID); err != nil {
			t.Fatalf("rbac add member: %v", err)
		}
	}
	return g.ID
}

func userRef(id uuid.UUID) PrincipalRef  { return PrincipalRef{Kind: PrincipalUser, ID: id.String()} }
func groupRef(id uuid.UUID) PrincipalRef { return PrincipalRef{Kind: PrincipalGroup, ID: id.String()} }

func roleOf(list *AccessList, id uuid.UUID) string {
	for _, e := range list.Entries {
		if e.Principal.ID == id.String() {
			return e.Role
		}
	}
	return ""
}

func (f *sharingFixture) canRead(t *testing.T, id uuid.UUID) bool {
	t.Helper()
	ok, err := f.rbac.CanReadProject(id, f.project)
	if err != nil {
		t.Fatal(err)
	}
	return ok
}

func (f *sharingFixture) canWrite(t *testing.T, id uuid.UUID) bool {
	t.Helper()
	ok, err := f.rbac.CanWriteProject(id, f.project)
	if err != nil {
		t.Fatal(err)
	}
	return ok
}

func TestSharingGrantUpdateRevokeUser(t *testing.T) {
	f := newSharingFixture(t)
	ctx := context.Background()
	bob := f.user(t, "bob")

	if f.canRead(t, bob) {
		t.Fatal("bob must not read before sharing")
	}
	list, err := f.svc.GrantAccess(ctx, f.owner, f.project, GrantAccessRequest{Principals: []PrincipalRef{userRef(bob)}, Role: RoleEdit})
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	if !list.CanManage || len(list.Entries) != 2 || list.Entries[0].Role != RoleOwner || roleOf(list, bob) != RoleEdit {
		t.Fatalf("unexpected list after grant: %+v", list)
	}
	if list.Entries[1].Principal.Email != "bob@example.com" || list.Entries[1].Principal.Name != "bob" {
		t.Fatalf("principal fields: %+v", list.Entries[1].Principal)
	}
	if !f.canWrite(t, bob) {
		t.Fatal("editor must write")
	}

	list, err = f.svc.UpdateAccess(ctx, f.owner, f.project, userRef(bob), UpdateAccessRequest{Role: RoleView})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if roleOf(list, bob) != RoleView {
		t.Fatalf("role after update: %q", roleOf(list, bob))
	}
	if f.canWrite(t, bob) || !f.canRead(t, bob) {
		t.Fatal("viewer must read but not write")
	}

	list, err = f.svc.RevokeAccess(ctx, f.owner, f.project, userRef(bob))
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if roleOf(list, bob) != "" || len(list.Entries) != 1 {
		t.Fatalf("bob still listed: %+v", list)
	}
	if f.canRead(t, bob) {
		t.Fatal("bob must not read after revoke")
	}

	// Undo of a removal is a grant again.
	list, err = f.svc.GrantAccess(ctx, f.owner, f.project, GrantAccessRequest{Principals: []PrincipalRef{userRef(bob)}, Role: RoleView})
	if err != nil || roleOf(list, bob) != RoleView {
		t.Fatalf("regrant: %v %+v", err, list)
	}

	var audits int64
	f.db.Model(&models.AuditLog{}).Where("user_id = ?", f.owner).Count(&audits)
	if audits < 4 {
		t.Fatalf("expected audit entries, got %d", audits)
	}
}

func TestSharingGrantIsIdempotentAndChangesRole(t *testing.T) {
	f := newSharingFixture(t)
	ctx := context.Background()
	bob := f.user(t, "bob")
	for _, role := range []string{RoleView, RoleEdit} {
		if _, err := f.svc.GrantAccess(ctx, f.owner, f.project, GrantAccessRequest{Principals: []PrincipalRef{userRef(bob), userRef(bob)}, Role: role}); err != nil {
			t.Fatalf("grant %s: %v", role, err)
		}
	}
	var n int64
	f.db.Model(&models.Permission{}).Where("user_id = ? AND project_id = ?", bob, f.project).Count(&n)
	if n != 1 {
		t.Fatalf("want one permission row, got %d", n)
	}
	if !f.canWrite(t, bob) {
		t.Fatal("second grant must upgrade to edit")
	}
}

func TestSharingOnlyOwnerOrAdminManages(t *testing.T) {
	f := newSharingFixture(t)
	ctx := context.Background()
	editor := f.user(t, "eddie")
	viewer := f.user(t, "vera")
	stranger := f.user(t, "stan")
	admin := f.user(t, "ada")
	if err := f.rbac.MakeAdmin(admin); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.GrantAccess(ctx, f.owner, f.project, GrantAccessRequest{Principals: []PrincipalRef{userRef(editor)}, Role: RoleEdit}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.GrantAccess(ctx, f.owner, f.project, GrantAccessRequest{Principals: []PrincipalRef{userRef(viewer)}, Role: RoleView}); err != nil {
		t.Fatal(err)
	}

	for name, caller := range map[string]uuid.UUID{"editor": editor, "viewer": viewer, "stranger": stranger, "anonymous": uuid.Nil} {
		if _, err := f.svc.GrantAccess(ctx, caller, f.project, GrantAccessRequest{Principals: []PrincipalRef{userRef(stranger)}, Role: RoleView}); !errors.Is(err, ErrForbidden) {
			t.Errorf("%s grant: want forbidden, got %v", name, err)
		}
		if _, err := f.svc.UpdateAccess(ctx, caller, f.project, userRef(viewer), UpdateAccessRequest{Role: RoleEdit}); !errors.Is(err, ErrForbidden) {
			t.Errorf("%s update: want forbidden, got %v", name, err)
		}
		if _, err := f.svc.RevokeAccess(ctx, caller, f.project, userRef(viewer)); !errors.Is(err, ErrForbidden) {
			t.Errorf("%s revoke: want forbidden, got %v", name, err)
		}
		if err := f.svc.DeleteProject(ctx, caller, f.project); !errors.Is(err, ErrForbidden) {
			t.Errorf("%s delete: want forbidden, got %v", name, err)
		}
	}
	if f.canRead(t, stranger) {
		t.Fatal("stranger gained access")
	}

	// Readers see the list but cannot manage; strangers cannot see it.
	for _, caller := range []uuid.UUID{editor, viewer} {
		list, err := f.svc.ListAccess(ctx, caller, f.project)
		if err != nil || list.CanManage || len(list.Entries) != 3 {
			t.Fatalf("reader list: %v %+v", err, list)
		}
	}
	if _, err := f.svc.ListAccess(ctx, stranger, f.project); !errors.Is(err, ErrForbidden) {
		t.Fatalf("stranger list: want forbidden, got %v", err)
	}

	// Admins manage any project, even without a project policy.
	list, err := f.svc.ListAccess(ctx, admin, f.project)
	if err != nil || !list.CanManage {
		t.Fatalf("admin list: %v %+v", err, list)
	}
	if _, err := f.svc.GrantAccess(ctx, admin, f.project, GrantAccessRequest{Principals: []PrincipalRef{userRef(stranger)}, Role: RoleView}); err != nil {
		t.Fatalf("admin grant: %v", err)
	}
	if !f.canRead(t, stranger) {
		t.Fatal("admin grant did not apply")
	}
}

func TestSharingOwnerCannotBeChanged(t *testing.T) {
	f := newSharingFixture(t)
	ctx := context.Background()
	var cerr *ConflictError
	if _, err := f.svc.RevokeAccess(ctx, f.owner, f.project, userRef(f.owner)); !errors.As(err, &cerr) || cerr.Code != "last_owner" {
		t.Fatalf("revoke owner: want last_owner, got %v", err)
	}
	if _, err := f.svc.UpdateAccess(ctx, f.owner, f.project, userRef(f.owner), UpdateAccessRequest{Role: RoleView}); !errors.As(err, &cerr) || cerr.Code != "last_owner" {
		t.Fatalf("update owner: want last_owner, got %v", err)
	}
	var verr *ValidationError
	if _, err := f.svc.GrantAccess(ctx, f.owner, f.project, GrantAccessRequest{Principals: []PrincipalRef{userRef(f.owner)}, Role: RoleView}); !errors.As(err, &verr) {
		t.Fatalf("grant owner: want validation, got %v", err)
	}
	if !f.canWrite(t, f.owner) {
		t.Fatal("owner lost write access")
	}
}

func TestSharingValidation(t *testing.T) {
	f := newSharingFixture(t)
	ctx := context.Background()
	bob := f.user(t, "bob")
	cases := []GrantAccessRequest{
		{Principals: []PrincipalRef{userRef(bob)}, Role: "owner"},
		{Principals: []PrincipalRef{userRef(bob)}, Role: "admin"},
		{Principals: nil, Role: RoleView},
		{Principals: []PrincipalRef{{Kind: "robot", ID: bob.String()}}, Role: RoleView},
		{Principals: []PrincipalRef{{Kind: PrincipalUser, ID: "not-a-uuid"}}, Role: RoleView},
		{Principals: []PrincipalRef{userRef(uuid.New())}, Role: RoleView},
		{Principals: []PrincipalRef{groupRef(uuid.New())}, Role: RoleView},
	}
	for i, req := range cases {
		var verr *ValidationError
		if _, err := f.svc.GrantAccess(ctx, f.owner, f.project, req); !errors.As(err, &verr) {
			t.Errorf("case %d: want validation error, got %v", i, err)
		}
	}
	if _, err := f.svc.UpdateAccess(ctx, f.owner, f.project, userRef(bob), UpdateAccessRequest{Role: RoleEdit}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update without grant: want not found, got %v", err)
	}
	if _, err := f.svc.RevokeAccess(ctx, f.owner, f.project, userRef(bob)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoke without grant: want not found, got %v", err)
	}
	if _, err := f.svc.ListAccess(ctx, f.owner, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown project: want not found, got %v", err)
	}
}

func TestSharingTeams(t *testing.T) {
	f := newSharingFixture(t)
	ctx := context.Background()
	member := f.user(t, "mia")
	outsider := f.user(t, "otto")
	mine := f.group(t, "team-jatic", f.owner, member)
	other := f.group(t, "team-jobs", outsider)

	// A non-admin owner can only share with teams they belong to.
	if _, err := f.svc.GrantAccess(ctx, f.owner, f.project, GrantAccessRequest{Principals: []PrincipalRef{groupRef(other)}, Role: RoleView}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("share with foreign team: want forbidden, got %v", err)
	}
	list, err := f.svc.GrantAccess(ctx, f.owner, f.project, GrantAccessRequest{Principals: []PrincipalRef{groupRef(mine)}, Role: RoleEdit})
	if err != nil {
		t.Fatalf("share with own team: %v", err)
	}
	var team *AccessEntry
	for i := range list.Entries {
		if list.Entries[i].Principal.Kind == PrincipalGroup {
			team = &list.Entries[i]
		}
	}
	if team == nil || team.Role != RoleEdit || team.Principal.Members != 2 || team.Principal.Name != "team-jatic" {
		t.Fatalf("team entry: %+v", team)
	}
	if !f.canWrite(t, member) || f.canRead(t, outsider) {
		t.Fatal("team grant must apply to members only")
	}

	if _, err := f.svc.UpdateAccess(ctx, f.owner, f.project, groupRef(mine), UpdateAccessRequest{Role: RoleView}); err != nil {
		t.Fatalf("update team: %v", err)
	}
	if f.canWrite(t, member) || !f.canRead(t, member) {
		t.Fatal("team role change must apply")
	}
	if _, err := f.svc.RevokeAccess(ctx, f.owner, f.project, groupRef(mine)); err != nil {
		t.Fatalf("revoke team: %v", err)
	}
	if f.canRead(t, member) {
		t.Fatal("team revoke must apply")
	}
}

func TestSearchPrincipals(t *testing.T) {
	f := newSharingFixture(t)
	ctx := context.Background()
	alex := f.user(t, "alex")
	f.user(t, "jordan")
	admin := f.user(t, "root_admin")
	if err := f.rbac.MakeAdmin(admin); err != nil {
		t.Fatal(err)
	}
	f.group(t, "team-alpha", alex)
	f.group(t, "team-beta")

	got, err := f.svc.SearchPrincipals(ctx, alex, "  ")
	if err != nil || len(got) != 0 {
		t.Fatalf("empty query: %v %+v", err, got)
	}
	got, err = f.svc.SearchPrincipals(ctx, alex, "JORDAN@example")
	if err != nil || len(got) != 1 || got[0].Name != "jordan" || got[0].Kind != PrincipalUser || got[0].Email != "jordan@example.com" {
		t.Fatalf("email search: %v %+v", err, got)
	}
	// Non-admins only see the teams they belong to.
	got, err = f.svc.SearchPrincipals(ctx, alex, "team")
	if err != nil || len(got) != 1 || got[0].Name != "team-alpha" || got[0].Members != 1 {
		t.Fatalf("member team search: %v %+v", err, got)
	}
	got, err = f.svc.SearchPrincipals(ctx, admin, "team")
	if err != nil || len(got) != 2 {
		t.Fatalf("admin team search: %v %+v", err, got)
	}
	// LIKE wildcards are literal.
	got, err = f.svc.SearchPrincipals(ctx, admin, "%")
	if err != nil || len(got) != 0 {
		t.Fatalf("wildcard search: %v %+v", err, got)
	}
	got, err = f.svc.SearchPrincipals(ctx, admin, "t_admin")
	if err != nil || len(got) != 1 || got[0].Name != "root_admin" {
		t.Fatalf("underscore search: %v %+v", err, got)
	}
}

func TestDeleteProjectFromServer(t *testing.T) {
	f := newSharingFixture(t)
	ctx := context.Background()
	if err := f.svc.DeleteProject(ctx, f.owner, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown project: want not found, got %v", err)
	}
	if err := f.svc.DeleteProject(ctx, f.owner, f.project); err != nil {
		t.Fatalf("owner delete: %v", err)
	}
	var job models.Job
	if err := f.db.Where("project_id = ? AND type = ?", f.project, models.JobTypeDelete).First(&job).Error; err != nil {
		t.Fatalf("delete job not created: %v", err)
	}
}
