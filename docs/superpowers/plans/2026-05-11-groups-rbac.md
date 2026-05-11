# Groups: share workspaces/registries with groups (native + OIDC) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a first-class `Group` primitive so workspace shares, registry access, and the admin role can be granted to a set of users at once. Works in both deployment modes — natively (admins create/manage groups in the UI) and via OIDC (groups + members come from the IdP `groups` claim and reconcile on every login).

**Architecture:**
- Three new GORM models (`Group`, `GroupMember`, `GroupPermission`) sibling to the existing per-user `Permission` table. Registry-by-group and admin-by-group are Casbin policies only — no extra tables.
- Casbin already declares `g = _, _` in `model.conf`. The matcher currently uses `r.sub == p.sub`, which **bypasses grouping**. The plan changes it to `g(r.sub, p.sub)` so existing direct policies still match (Casbin treats `g(x, x)` as true) **and** new group policies are resolved transitively. All existing `IsAdmin` / `CanReadWorkspace` / `CanWriteWorkspace` helpers continue to work unchanged.
- DB writes use the existing pattern: write rows inside a `db.Transaction`, then mutate Casbin **after** the tx commits. Casbin mutations use `AddPolicy`/`AddGroupingPolicy`/`SavePolicy` to match the existing helper style in `internal/rbac/rbac.go`. On group delete, Casbin rules are hard-removed via `RemoveFilteredGroupingPolicy` + `RemoveFilteredPolicy` (Casbin doesn't honor GORM soft-delete; the `casbin_rule` table also has no `DeletedAt`).
- OIDC sync is a JIT full reconcile on every login: parse the `groups` claim from the ID token, upsert each group + membership, then remove any `source=oidc` memberships for this user that aren't in this login's claim.
- Frontend mirrors existing patterns: a new admin page `Groups.tsx` (parallel to `UserManagement.tsx`), and the existing `ShareDialog.tsx` grows a User/Group tabbed selector.

**Tech Stack:** Go 1.23 + Gin + GORM + Casbin v2 (`github.com/casbin/casbin/v2`) + `coreos/go-oidc/v3`. Frontend: React 19 + TanStack Query 5 + Zustand + Tailwind 4 + Radix UI + vitest + MSW.

**Deviations from the issue (each verified against existing code):**

1. **Matcher rewrite is required.** The issue states the matcher needs no change. That is **incorrect** — the current matcher (`internal/rbac/model.conf:14`) uses `r.sub == p.sub`, which never invokes the `g` grouping function. Task 1 changes the matcher to `g(r.sub, p.sub)`. This is fully backward compatible: Casbin's RBAC role manager treats `g(x, x)` as true for any subject `x`, so every existing direct user-to-object policy still matches.
2. **DB + Casbin writes are sequential, not transactional.** The issue says "DB write + Casbin write must happen in a single transaction (matches today's per-user pattern)." Inspection of `internal/service/workspace_permissions.go:51-75` shows the *actual* per-user pattern is: commit a DB transaction, then call the Casbin helper outside the tx (with the comment "RBAC outside transaction — Casbin uses its own DB connection"). We match that pattern. There is a tiny crash window where DB succeeded but Casbin didn't; the existing codebase has accepted this for the per-user case, so we accept it here too. Tightening this is a follow-up refactor across all permission code, not part of this feature.
3. **Group-as-admin policy uses `("admin", "admin")` not `("admin", "*")`.** The issue text says `p(group_id, "admin", "*")`. The current admin policy in `internal/rbac/rbac.go:75-79` and `:121-128` uses object `"admin"` and action `"admin"`. We mirror the existing scheme so `IsAdmin(user)` resolves transitively via the matcher change alone.
4. **`GET /workspaces/{id}/collaborators` returns a flat list with a `kind` discriminator** (`kind: "user" | "group"`) rather than separate `users[]` and `groups[]` arrays. Functionally equivalent, but easier for the frontend to render with a single mapped component. The TypeScript type is a discriminated union.

**Run order:** Tasks 1–11 are backend (each commits independently; the backend ships a working API after Task 11). Tasks 12–16 are frontend and depend on Task 11.

**Test command throughout:** `make test` for the full suite, or `go test -v ./internal/<pkg>/...` for a specific package. Frontend: `cd frontend && npm run test` (vitest). Lint: `cd frontend && npm run lint`. Type-check: `cd frontend && npx tsc -b`.

---

## File Structure

**New files:**
- `internal/models/group.go` — `Group` struct
- `internal/models/group_member.go` — `GroupMember` struct
- `internal/models/group_permission.go` — `GroupPermission` struct
- `internal/service/groups.go` — `GroupService`: native CRUD, member ops, workspace share, registry/admin grants
- `internal/service/groups_test.go` — `GroupService` unit tests
- `internal/api/handlers/group.go` — HTTP handlers for `/admin/groups/*`, `/groups/me`
- `internal/api/handlers/group_test.go` — handler tests
- `frontend/src/pages/admin/Groups.tsx` — admin Groups page
- `frontend/src/components/admin/CreateGroupDialog.tsx`
- `frontend/src/components/admin/GroupMembersDialog.tsx`
- `frontend/src/hooks/useGroups.ts` — React Query hooks for groups
- `frontend/src/api/groups.ts` — group API client

**Modified files:**
- `internal/rbac/model.conf` — matcher uses `g(r.sub, p.sub)`
- `internal/rbac/rbac.go` — new helpers: `AddUserToGroup`, `RemoveUserFromGroup`, `GetUserGroups`, `GrantGroupWorkspaceAccess`, `RevokeGroupWorkspaceAccess`, `GrantGroupRegistryAccess`, `RevokeGroupRegistryAccess`, `MakeGroupAdmin`, `RevokeGroupAdmin`, `RemoveAllGroupPolicies`, `CanReadRegistry`, `CanWriteRegistry`
- `internal/rbac/provider.go` — extend `Provider` interface with the above
- `internal/db/db.go` — add `Group`, `GroupMember`, `GroupPermission` to `AutoMigrate`
- `internal/service/workspace_permissions.go` — `ShareWorkspaceWithGroup`, `UnshareWorkspaceFromGroup`; extend `ListCollaborators` to include groups
- `internal/service/workspace_permissions_test.go` — new tests
- `internal/service/types.go` — extend `CollaboratorResult` with `Source` + `Kind`; add `GroupCollaboratorResult`
- `internal/service/admin.go` — registry-group grants
- `internal/api/handlers/workspace.go` — `ShareWorkspaceWithGroup`, `UnshareWorkspaceWithGroup`
- `internal/api/handlers/registry.go` — `GrantRegistryToGroup`, `RevokeRegistryFromGroup`
- `internal/api/router.go` — wire new routes
- `internal/auth/oidc.go` — request `groups` scope, parse claim, JIT sync
- `internal/auth/basic.go` — `ExchangeIDToken` mirrors the same sync path
- `frontend/src/types/models.ts` — `Group`, `GroupMember`, `GroupPermission`, request types, extended `Collaborator`
- `frontend/src/api/admin.ts` — group-related calls (or split into `api/groups.ts`)
- `frontend/src/components/sharing/ShareDialog.tsx` — User/Group tabs
- `frontend/src/pages/admin/UserManagement.tsx` — show groups per user
- `frontend/src/App.tsx` — add `/admin/groups` route
- `frontend/src/components/admin/AdminLayout.tsx` (or wherever the admin nav lives) — add Groups nav link

---

## Task 1: Casbin matcher + group helpers

**Files:**
- Modify: `internal/rbac/model.conf:14` (matcher line)
- Modify: `internal/rbac/rbac.go` (add helpers)
- Modify: `internal/rbac/provider.go` (extend interface + default impl)
- Test: `internal/rbac/rbac_test.go` (new file)

- [ ] **Step 1.1: Write the failing test for transitive group reads**

Create `internal/rbac/rbac_test.go`:

```go
package rbac

import (
	"log/slog"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	return db
}

func TestGroupGrantsWorkspaceAccessTransitively(t *testing.T) {
	db := newTestDB(t)
	if err := InitEnforcer(db, slog.Default()); err != nil {
		t.Fatalf("init enforcer: %v", err)
	}
	defer func() { enforcer = nil }()

	user := uuid.New()
	group := uuid.New()
	ws := uuid.New()

	if err := AddUserToGroup(user, group); err != nil {
		t.Fatalf("add user to group: %v", err)
	}
	if err := GrantGroupWorkspaceAccess(group, ws, "viewer"); err != nil {
		t.Fatalf("grant group: %v", err)
	}

	canRead, err := CanReadWorkspace(user, ws)
	if err != nil {
		t.Fatalf("enforce: %v", err)
	}
	if !canRead {
		t.Fatalf("expected user to read workspace via group, got false")
	}
}

func TestGroupAdminTransitive(t *testing.T) {
	db := newTestDB(t)
	if err := InitEnforcer(db, slog.Default()); err != nil {
		t.Fatalf("init enforcer: %v", err)
	}
	defer func() { enforcer = nil }()

	user := uuid.New()
	group := uuid.New()

	if err := AddUserToGroup(user, group); err != nil {
		t.Fatalf("add user to group: %v", err)
	}
	if err := MakeGroupAdmin(group); err != nil {
		t.Fatalf("make group admin: %v", err)
	}

	isAdmin, err := IsAdmin(user)
	if err != nil {
		t.Fatalf("is admin: %v", err)
	}
	if !isAdmin {
		t.Fatalf("expected user to be admin via group, got false")
	}
}

func TestDirectUserPolicyStillWorksAfterMatcherChange(t *testing.T) {
	db := newTestDB(t)
	if err := InitEnforcer(db, slog.Default()); err != nil {
		t.Fatalf("init enforcer: %v", err)
	}
	defer func() { enforcer = nil }()

	user := uuid.New()
	ws := uuid.New()

	if err := GrantWorkspaceAccess(user, ws, "editor"); err != nil {
		t.Fatalf("grant workspace: %v", err)
	}

	canWrite, err := CanWriteWorkspace(user, ws)
	if err != nil {
		t.Fatalf("enforce: %v", err)
	}
	if !canWrite {
		t.Fatalf("expected direct write policy to still match, got false")
	}
}

func TestRemoveAllGroupPoliciesCleansEverything(t *testing.T) {
	db := newTestDB(t)
	if err := InitEnforcer(db, slog.Default()); err != nil {
		t.Fatalf("init enforcer: %v", err)
	}
	defer func() { enforcer = nil }()

	user := uuid.New()
	group := uuid.New()
	ws := uuid.New()
	reg := uuid.New()

	_ = AddUserToGroup(user, group)
	_ = GrantGroupWorkspaceAccess(group, ws, "editor")
	_ = GrantGroupRegistryAccess(group, reg, "write")
	_ = MakeGroupAdmin(group)

	if err := RemoveAllGroupPolicies(group); err != nil {
		t.Fatalf("remove all: %v", err)
	}

	canRead, _ := CanReadWorkspace(user, ws)
	if canRead {
		t.Fatalf("expected workspace access to be revoked")
	}
	isAdmin, _ := IsAdmin(user)
	if isAdmin {
		t.Fatalf("expected admin to be revoked")
	}
}
```

- [ ] **Step 1.2: Run the tests to verify they fail**

Run: `go test -v -run 'TestGroup|TestDirectUser|TestRemoveAllGroup' ./internal/rbac/`
Expected: FAIL — symbols `AddUserToGroup`, `GrantGroupWorkspaceAccess`, `MakeGroupAdmin`, `GrantGroupRegistryAccess`, `RemoveAllGroupPolicies` are undefined.

- [ ] **Step 1.3: Update the matcher in `model.conf`**

Edit `internal/rbac/model.conf` line 14 from:

```
m = r.sub == p.sub && r.obj == p.obj && (r.act == p.act || (r.act == "read" && p.act == "write"))
```

to:

```
m = g(r.sub, p.sub) && r.obj == p.obj && (r.act == p.act || (r.act == "read" && p.act == "write"))
```

Why this is safe: Casbin's built-in `RoleManager` treats `g(x, x)` as true for any subject `x`, even when no explicit grouping rule exists. So every existing direct `(user_id, ws:<id>, read|write)` policy still matches. New `(group_id, ws:<id>, read|write)` policies match when an explicit `g, user_id, group_id` rule exists.

- [ ] **Step 1.4: Add group helpers to `internal/rbac/rbac.go`**

Append to `internal/rbac/rbac.go` (after `GetUserWorkspaces`):

```go
// AddUserToGroup creates a grouping rule g(userID, groupID).
func AddUserToGroup(userID, groupID uuid.UUID) error {
	if enforcer == nil {
		return nil
	}
	if _, err := enforcer.AddGroupingPolicy(userID.String(), groupID.String()); err != nil {
		return err
	}
	return enforcer.SavePolicy()
}

// RemoveUserFromGroup removes the grouping rule g(userID, groupID).
func RemoveUserFromGroup(userID, groupID uuid.UUID) error {
	if enforcer == nil {
		return nil
	}
	if _, err := enforcer.RemoveGroupingPolicy(userID.String(), groupID.String()); err != nil {
		return err
	}
	return enforcer.SavePolicy()
}

// GetUserGroups returns the group IDs the user belongs to via Casbin grouping rules.
func GetUserGroups(userID uuid.UUID) ([]uuid.UUID, error) {
	if enforcer == nil {
		return nil, nil
	}
	roles, err := enforcer.GetRolesForUser(userID.String())
	if err != nil {
		return nil, err
	}
	groups := make([]uuid.UUID, 0, len(roles))
	for _, r := range roles {
		if id, err := uuid.Parse(r); err == nil {
			groups = append(groups, id)
		}
	}
	return groups, nil
}

// GrantGroupWorkspaceAccess grants a group access to a workspace.
func GrantGroupWorkspaceAccess(groupID, wsID uuid.UUID, role string) error {
	var action string
	switch role {
	case "owner", "editor":
		action = "write"
	case "viewer":
		action = "read"
	default:
		return fmt.Errorf("invalid role: %s", role)
	}
	if enforcer == nil {
		return nil
	}
	if _, err := enforcer.AddPolicy(groupID.String(), fmt.Sprintf("ws:%s", wsID.String()), action); err != nil {
		return err
	}
	return enforcer.SavePolicy()
}

// RevokeGroupWorkspaceAccess revokes a group's access to a workspace.
func RevokeGroupWorkspaceAccess(groupID, wsID uuid.UUID) error {
	if enforcer == nil {
		return nil
	}
	obj := fmt.Sprintf("ws:%s", wsID.String())
	enforcer.RemovePolicy(groupID.String(), obj, "read")
	enforcer.RemovePolicy(groupID.String(), obj, "write")
	return enforcer.SavePolicy()
}

// GrantGroupRegistryAccess grants a group access to a registry (read or write).
func GrantGroupRegistryAccess(groupID, regID uuid.UUID, action string) error {
	if action != "read" && action != "write" {
		return fmt.Errorf("invalid registry action: %s", action)
	}
	if enforcer == nil {
		return nil
	}
	if _, err := enforcer.AddPolicy(groupID.String(), fmt.Sprintf("reg:%s", regID.String()), action); err != nil {
		return err
	}
	return enforcer.SavePolicy()
}

// RevokeGroupRegistryAccess revokes a group's access to a registry.
func RevokeGroupRegistryAccess(groupID, regID uuid.UUID) error {
	if enforcer == nil {
		return nil
	}
	obj := fmt.Sprintf("reg:%s", regID.String())
	enforcer.RemovePolicy(groupID.String(), obj, "read")
	enforcer.RemovePolicy(groupID.String(), obj, "write")
	return enforcer.SavePolicy()
}

// CanReadRegistry checks if a subject can read a registry (transitive via groups).
func CanReadRegistry(userID, regID uuid.UUID) (bool, error) {
	if enforcer == nil {
		return true, nil
	}
	return enforcer.Enforce(userID.String(), fmt.Sprintf("reg:%s", regID.String()), "read")
}

// CanWriteRegistry checks if a subject can write to a registry (transitive via groups).
func CanWriteRegistry(userID, regID uuid.UUID) (bool, error) {
	if enforcer == nil {
		return true, nil
	}
	return enforcer.Enforce(userID.String(), fmt.Sprintf("reg:%s", regID.String()), "write")
}

// MakeGroupAdmin grants admin privileges to every member of a group.
func MakeGroupAdmin(groupID uuid.UUID) error {
	if enforcer == nil {
		return nil
	}
	if _, err := enforcer.AddPolicy(groupID.String(), "admin", "admin"); err != nil {
		return err
	}
	return enforcer.SavePolicy()
}

// RevokeGroupAdmin removes group-level admin privilege.
func RevokeGroupAdmin(groupID uuid.UUID) error {
	if enforcer == nil {
		return nil
	}
	if _, err := enforcer.RemovePolicy(groupID.String(), "admin", "admin"); err != nil {
		return err
	}
	return enforcer.SavePolicy()
}

// RemoveAllGroupPolicies removes every Casbin rule that involves a group:
//   - All `p` policies where the group is the subject (workspace, registry, admin grants).
//   - All `g` grouping rules where the group is the role (memberships).
// Casbin doesn't honor GORM soft-delete, so this is a hard remove.
func RemoveAllGroupPolicies(groupID uuid.UUID) error {
	if enforcer == nil {
		return nil
	}
	if _, err := enforcer.RemoveFilteredPolicy(0, groupID.String()); err != nil {
		return err
	}
	if _, err := enforcer.RemoveFilteredGroupingPolicy(1, groupID.String()); err != nil {
		return err
	}
	return enforcer.SavePolicy()
}
```

- [ ] **Step 1.5: Extend the `Provider` interface in `internal/rbac/provider.go`**

Replace `internal/rbac/provider.go` contents:

```go
package rbac

import "github.com/google/uuid"

// Provider abstracts RBAC operations so callers can use dependency injection
// instead of the global enforcer. This enables per-test isolation and mocking.
type Provider interface {
	CanReadWorkspace(userID, wsID uuid.UUID) (bool, error)
	CanWriteWorkspace(userID, wsID uuid.UUID) (bool, error)
	CanReadRegistry(userID, regID uuid.UUID) (bool, error)
	CanWriteRegistry(userID, regID uuid.UUID) (bool, error)
	IsAdmin(userID uuid.UUID) (bool, error)
	GrantWorkspaceAccess(userID, wsID uuid.UUID, role string) error
	RevokeWorkspaceAccess(userID, wsID uuid.UUID) error
	MakeAdmin(userID uuid.UUID) error
	RevokeAdmin(userID uuid.UUID) error
	GetAllAdminUserIDs() (map[uuid.UUID]bool, error)

	// Group operations
	AddUserToGroup(userID, groupID uuid.UUID) error
	RemoveUserFromGroup(userID, groupID uuid.UUID) error
	GetUserGroups(userID uuid.UUID) ([]uuid.UUID, error)
	GrantGroupWorkspaceAccess(groupID, wsID uuid.UUID, role string) error
	RevokeGroupWorkspaceAccess(groupID, wsID uuid.UUID) error
	GrantGroupRegistryAccess(groupID, regID uuid.UUID, action string) error
	RevokeGroupRegistryAccess(groupID, regID uuid.UUID) error
	MakeGroupAdmin(groupID uuid.UUID) error
	RevokeGroupAdmin(groupID uuid.UUID) error
	RemoveAllGroupPolicies(groupID uuid.UUID) error
}

// DefaultProvider wraps the global Casbin enforcer as an rbac.Provider.
type DefaultProvider struct{}

func NewDefaultProvider() *DefaultProvider { return &DefaultProvider{} }

func (DefaultProvider) CanReadWorkspace(userID, wsID uuid.UUID) (bool, error) {
	return CanReadWorkspace(userID, wsID)
}
func (DefaultProvider) CanWriteWorkspace(userID, wsID uuid.UUID) (bool, error) {
	return CanWriteWorkspace(userID, wsID)
}
func (DefaultProvider) CanReadRegistry(userID, regID uuid.UUID) (bool, error) {
	return CanReadRegistry(userID, regID)
}
func (DefaultProvider) CanWriteRegistry(userID, regID uuid.UUID) (bool, error) {
	return CanWriteRegistry(userID, regID)
}
func (DefaultProvider) IsAdmin(userID uuid.UUID) (bool, error) { return IsAdmin(userID) }
func (DefaultProvider) GrantWorkspaceAccess(userID, wsID uuid.UUID, role string) error {
	return GrantWorkspaceAccess(userID, wsID, role)
}
func (DefaultProvider) RevokeWorkspaceAccess(userID, wsID uuid.UUID) error {
	return RevokeWorkspaceAccess(userID, wsID)
}
func (DefaultProvider) MakeAdmin(userID uuid.UUID) error  { return MakeAdmin(userID) }
func (DefaultProvider) RevokeAdmin(userID uuid.UUID) error { return RevokeAdmin(userID) }
func (DefaultProvider) GetAllAdminUserIDs() (map[uuid.UUID]bool, error) {
	return GetAllAdminUserIDs()
}
func (DefaultProvider) AddUserToGroup(userID, groupID uuid.UUID) error {
	return AddUserToGroup(userID, groupID)
}
func (DefaultProvider) RemoveUserFromGroup(userID, groupID uuid.UUID) error {
	return RemoveUserFromGroup(userID, groupID)
}
func (DefaultProvider) GetUserGroups(userID uuid.UUID) ([]uuid.UUID, error) {
	return GetUserGroups(userID)
}
func (DefaultProvider) GrantGroupWorkspaceAccess(groupID, wsID uuid.UUID, role string) error {
	return GrantGroupWorkspaceAccess(groupID, wsID, role)
}
func (DefaultProvider) RevokeGroupWorkspaceAccess(groupID, wsID uuid.UUID) error {
	return RevokeGroupWorkspaceAccess(groupID, wsID)
}
func (DefaultProvider) GrantGroupRegistryAccess(groupID, regID uuid.UUID, action string) error {
	return GrantGroupRegistryAccess(groupID, regID, action)
}
func (DefaultProvider) RevokeGroupRegistryAccess(groupID, regID uuid.UUID) error {
	return RevokeGroupRegistryAccess(groupID, regID)
}
func (DefaultProvider) MakeGroupAdmin(groupID uuid.UUID) error  { return MakeGroupAdmin(groupID) }
func (DefaultProvider) RevokeGroupAdmin(groupID uuid.UUID) error { return RevokeGroupAdmin(groupID) }
func (DefaultProvider) RemoveAllGroupPolicies(groupID uuid.UUID) error {
	return RemoveAllGroupPolicies(groupID)
}
```

- [ ] **Step 1.6: Run the tests to verify they pass**

Run: `go test -v -run 'TestGroup|TestDirectUser|TestRemoveAllGroup' ./internal/rbac/`
Expected: PASS (4 tests).

- [ ] **Step 1.7: Run the existing rbac test to make sure nothing regressed**

Run: `go test -v ./internal/rbac/...`
Expected: PASS (all tests, including the existing `nil_enforcer_test.go`).

- [ ] **Step 1.8: Run the whole backend suite to confirm the matcher change broke nothing**

Run: `make test`
Expected: PASS. If anything fails, the matcher change is the prime suspect — re-read the failing test before touching code.

- [ ] **Step 1.9: Commit**

```bash
git add internal/rbac/model.conf internal/rbac/rbac.go internal/rbac/provider.go internal/rbac/rbac_test.go
git commit -m "rbac: add Casbin group helpers and switch matcher to g(r.sub, p.sub)"
```

---

## Task 2: Group / GroupMember / GroupPermission models + AutoMigrate

**Files:**
- Create: `internal/models/group.go`
- Create: `internal/models/group_member.go`
- Create: `internal/models/group_permission.go`
- Modify: `internal/db/db.go:100-113` (AutoMigrate list)

- [ ] **Step 2.1: Create `internal/models/group.go`**

```go
package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GroupSource identifies how a group entered the system.
type GroupSource string

const (
	GroupSourceNative GroupSource = "native"
	GroupSourceOIDC   GroupSource = "oidc"
)

// Group represents a named collection of users for permission grants.
type Group struct {
	ID          uuid.UUID      `gorm:"type:text;primary_key" json:"id"`
	Name        string         `gorm:"uniqueIndex;not null" json:"name"`
	Description string         `json:"description"`
	Source      GroupSource    `gorm:"type:text;not null;default:native;index" json:"source"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

// BeforeCreate hook to generate UUID
func (g *Group) BeforeCreate(tx *gorm.DB) error {
	if g.ID == uuid.Nil {
		g.ID = uuid.New()
	}
	return nil
}
```

- [ ] **Step 2.2: Create `internal/models/group_member.go`**

```go
package models

import (
	"time"

	"github.com/google/uuid"
)

// GroupMember is the join table linking users to groups. Composite primary key
// (group_id, user_id) — a user appears at most once per group.
type GroupMember struct {
	GroupID   uuid.UUID `gorm:"type:text;primaryKey" json:"group_id"`
	UserID    uuid.UUID `gorm:"type:text;primaryKey" json:"user_id"`
	Group     Group     `gorm:"foreignKey:GroupID" json:"group,omitempty"`
	User      User      `gorm:"foreignKey:UserID" json:"user,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
```

- [ ] **Step 2.3: Create `internal/models/group_permission.go`**

```go
package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GroupPermission is the sibling of `Permission` (which is per-user) and represents
// a group's access to a workspace. Registry and admin grants live in Casbin only.
type GroupPermission struct {
	ID          uint           `gorm:"primarykey" json:"id"`
	GroupID     uuid.UUID      `gorm:"type:text;not null;index" json:"group_id"`
	Group       Group          `gorm:"foreignKey:GroupID" json:"group,omitempty"`
	WorkspaceID uuid.UUID      `gorm:"type:text;not null;index" json:"workspace_id"`
	Workspace   Workspace      `gorm:"foreignKey:WorkspaceID" json:"workspace,omitempty"`
	RoleID      uint           `gorm:"not null" json:"role_id"`
	Role        Role           `gorm:"foreignKey:RoleID" json:"role,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}
```

- [ ] **Step 2.4: Add models to `AutoMigrate` in `internal/db/db.go`**

Edit `internal/db/db.go`, replace the `AutoMigrate` block (currently `internal/db/db.go:100-113`) with:

```go
	err := db.AutoMigrate(
		&models.User{},
		&models.Role{},
		&models.Workspace{},
		&models.Job{},
		&models.Permission{},
		&models.Template{},
		&models.Package{},
		&models.AuditLog{},
		&models.WorkspaceVersion{},
		&models.OCIRegistry{},
		&models.Publication{},
		&models.WorkspaceTag{},
		&models.Group{},
		&models.GroupMember{},
		&models.GroupPermission{},
	)
```

- [ ] **Step 2.5: Add the same three models to the per-test migrator**

Edit `internal/service/workspace_test.go` — find `testSetup` (around line 23) and add the three new models to its `AutoMigrate` list. The migration list there is hand-maintained and must mirror `db.Migrate` for tests to work.

Locate the existing block (currently 11 models) and replace with:

```go
	if err := db.AutoMigrate(
		&models.User{},
		&models.Role{},
		&models.Workspace{},
		&models.Job{},
		&models.Permission{},
		&models.WorkspaceVersion{},
		&models.WorkspaceTag{},
		&models.AuditLog{},
		&models.Package{},
		&models.OCIRegistry{},
		&models.Publication{},
		&models.Group{},
		&models.GroupMember{},
		&models.GroupPermission{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
```

- [ ] **Step 2.6: Verify the build**

Run: `go build ./...`
Expected: success, no compile errors.

- [ ] **Step 2.7: Run the full suite to make sure migrations still succeed**

Run: `make test`
Expected: PASS. Failures here usually mean a missing model in `testSetup` or a typo in a tag.

- [ ] **Step 2.8: Commit**

```bash
git add internal/models/group.go internal/models/group_member.go internal/models/group_permission.go internal/db/db.go internal/service/workspace_test.go
git commit -m "models: add Group, GroupMember, GroupPermission with AutoMigrate"
```

---

## Task 3: GroupService — native CRUD (groups + members)

**Files:**
- Create: `internal/service/groups.go`
- Create: `internal/service/groups_test.go`
- Modify: `internal/audit/audit.go` — add new action constants

- [ ] **Step 3.1: Add audit action constants**

Edit `internal/audit/audit.go`. Inside the `const` block where `ActionGrantPermission` / `ActionRevokePermission` live (around lines 37–38), append:

```go
	ActionCreateGroup       = "create_group"
	ActionDeleteGroup       = "delete_group"
	ActionUpdateGroup       = "update_group"
	ActionAddGroupMember    = "add_group_member"
	ActionRemoveGroupMember = "remove_group_member"
	ActionGrantGroupPerm    = "grant_group_permission"
	ActionRevokeGroupPerm   = "revoke_group_permission"
	ActionGrantGroupAdmin   = "grant_group_admin"
	ActionRevokeGroupAdmin  = "revoke_group_admin"
```

- [ ] **Step 3.2: Write the failing GroupService test file**

Create `internal/service/groups_test.go`:

```go
package service

import (
	"errors"
	"testing"

	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/rbac"
	"gorm.io/gorm"
)

func groupTestSetup(t *testing.T) (*GroupService, *gorm.DB) {
	t.Helper()
	_, db := testSetup(t, false)
	return NewGroupService(db, rbac.NewDefaultProvider()), db
}

func TestCreateGroup_NativeSucceeds(t *testing.T) {
	svc, db := groupTestSetup(t)
	admin := createTestUser(t, db, "admin")

	g, err := svc.CreateGroup(CreateGroupRequest{Name: "data-science", Description: "DS team"}, admin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g.Name != "data-science" {
		t.Errorf("expected name 'data-science', got %q", g.Name)
	}
	if g.Source != models.GroupSourceNative {
		t.Errorf("expected source native, got %q", g.Source)
	}

	var count int64
	db.Model(&models.AuditLog{}).Where("user_id = ? AND action = ?", admin, "create_group").Count(&count)
	if count != 1 {
		t.Errorf("expected 1 audit log, got %d", count)
	}
}

func TestCreateGroup_DuplicateNameFails(t *testing.T) {
	svc, db := groupTestSetup(t)
	admin := createTestUser(t, db, "admin")
	_, _ = svc.CreateGroup(CreateGroupRequest{Name: "dup"}, admin)

	_, err := svc.CreateGroup(CreateGroupRequest{Name: "dup"}, admin)
	if err == nil {
		t.Fatal("expected duplicate-name error, got nil")
	}
}

func TestAddGroupMember_GrantsCasbinMembership(t *testing.T) {
	svc, db := groupTestSetup(t)
	admin := createTestUser(t, db, "admin")
	alice := createTestUser(t, db, "alice")
	g, _ := svc.CreateGroup(CreateGroupRequest{Name: "ds"}, admin)

	if err := svc.AddMember(g.ID, alice, admin); err != nil {
		t.Fatalf("add member: %v", err)
	}

	groups, err := rbac.GetUserGroups(alice)
	if err != nil {
		t.Fatalf("get user groups: %v", err)
	}
	found := false
	for _, gid := range groups {
		if gid == g.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected alice to be in group %s, got %v", g.ID, groups)
	}
}

func TestRemoveGroupMember_RevokesCasbinMembership(t *testing.T) {
	svc, db := groupTestSetup(t)
	admin := createTestUser(t, db, "admin")
	alice := createTestUser(t, db, "alice")
	g, _ := svc.CreateGroup(CreateGroupRequest{Name: "ds"}, admin)
	_ = svc.AddMember(g.ID, alice, admin)

	if err := svc.RemoveMember(g.ID, alice, admin); err != nil {
		t.Fatalf("remove member: %v", err)
	}

	groups, _ := rbac.GetUserGroups(alice)
	for _, gid := range groups {
		if gid == g.ID {
			t.Fatalf("expected alice removed from group, still present")
		}
	}
}

func TestDeleteGroup_HardRemovesCasbinRules(t *testing.T) {
	svc, db := groupTestSetup(t)
	admin := createTestUser(t, db, "admin")
	alice := createTestUser(t, db, "alice")
	g, _ := svc.CreateGroup(CreateGroupRequest{Name: "ds"}, admin)
	_ = svc.AddMember(g.ID, alice, admin)

	if err := svc.DeleteGroup(g.ID, admin); err != nil {
		t.Fatalf("delete group: %v", err)
	}

	groups, _ := rbac.GetUserGroups(alice)
	if len(groups) != 0 {
		t.Fatalf("expected casbin membership removed, got %v", groups)
	}

	var dbGroup models.Group
	err := db.Unscoped().First(&dbGroup, "id = ?", g.ID).Error
	if err != nil {
		t.Fatalf("expected soft-deleted row to still exist, got error: %v", err)
	}
	if !dbGroup.DeletedAt.Valid {
		t.Fatalf("expected DeletedAt to be set")
	}
}

func TestDeleteGroup_OIDCRejected(t *testing.T) {
	svc, db := groupTestSetup(t)
	admin := createTestUser(t, db, "admin")
	g := models.Group{Name: "synced", Source: models.GroupSourceOIDC}
	if err := db.Create(&g).Error; err != nil {
		t.Fatalf("seed group: %v", err)
	}

	err := svc.DeleteGroup(g.ID, admin)
	if err == nil {
		t.Fatal("expected ConflictError for OIDC group, got nil")
	}
	var ce *ConflictError
	if !isConflictError(err, &ce) {
		t.Fatalf("expected ConflictError, got %T: %v", err, err)
	}
}

func TestUpdateGroup_OIDCRejected(t *testing.T) {
	svc, db := groupTestSetup(t)
	admin := createTestUser(t, db, "admin")
	g := models.Group{Name: "synced", Source: models.GroupSourceOIDC}
	if err := db.Create(&g).Error; err != nil {
		t.Fatalf("seed group: %v", err)
	}

	_, err := svc.UpdateGroup(g.ID, UpdateGroupRequest{Description: ptr("new")}, admin)
	if err == nil {
		t.Fatal("expected ConflictError for OIDC group, got nil")
	}
}

func ptr[T any](v T) *T { return &v }

func isConflictError(err error, target **ConflictError) bool {
	return errors.As(err, target)
}
```

Before running tests, grep for an existing `isConflictError` helper: `grep -rn "isConflictError" /Users/tylerman/gh/nebi/internal/service/`. If one already exists (e.g. in `workspace_permissions_test.go`), delete the local copy above to avoid a duplicate-symbol compile error.

- [ ] **Step 3.3: Run the tests to confirm they fail with "undefined" errors**

Run: `go test -v -run 'TestCreateGroup|TestAddGroupMember|TestRemoveGroupMember|TestDeleteGroup|TestUpdateGroup' ./internal/service/`
Expected: FAIL with "undefined: NewGroupService" / "CreateGroupRequest" / "UpdateGroupRequest".

- [ ] **Step 3.4: Implement `GroupService` in `internal/service/groups.go`**

```go
package service

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/audit"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/rbac"
	"gorm.io/gorm"
)

// GroupService owns CRUD + membership ops for groups. Workspace shares and
// registry/admin grants for groups live in WorkspaceService and AdminService
// to keep their existing locality of behaviour.
type GroupService struct {
	db   *gorm.DB
	rbac rbac.Provider
}

func NewGroupService(db *gorm.DB, rbacProvider rbac.Provider) *GroupService {
	return &GroupService{db: db, rbac: rbacProvider}
}

// CreateGroupRequest is the input for CreateGroup.
type CreateGroupRequest struct {
	Name        string
	Description string
}

// UpdateGroupRequest holds optional fields for PATCH.
type UpdateGroupRequest struct {
	Name        *string
	Description *string
}

// GroupWithMemberCount denormalises member_count for list views.
type GroupWithMemberCount struct {
	models.Group
	MemberCount int64 `json:"member_count"`
}

// CreateGroup creates a native group. Admin-only; the handler is responsible
// for the admin check.
func (s *GroupService) CreateGroup(req CreateGroupRequest, actorID uuid.UUID) (*models.Group, error) {
	if req.Name == "" {
		return nil, &ValidationError{Message: "Group name is required"}
	}

	var existing models.Group
	if err := s.db.Where("name = ?", req.Name).First(&existing).Error; err == nil {
		return nil, &ConflictError{Message: "Group with this name already exists"}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	g := models.Group{
		Name:        req.Name,
		Description: req.Description,
		Source:      models.GroupSourceNative,
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&g).Error; err != nil {
			return fmt.Errorf("create group: %w", err)
		}
		audit.LogAction(tx, actorID, audit.ActionCreateGroup, fmt.Sprintf("group:%s", g.ID), map[string]interface{}{
			"name": g.Name,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &g, nil
}

// ListGroups returns every group with denormalised member count.
func (s *GroupService) ListGroups() ([]GroupWithMemberCount, error) {
	var groups []models.Group
	if err := s.db.Find(&groups).Error; err != nil {
		return nil, fmt.Errorf("list groups: %w", err)
	}

	out := make([]GroupWithMemberCount, len(groups))
	for i, g := range groups {
		var count int64
		s.db.Model(&models.GroupMember{}).Where("group_id = ?", g.ID).Count(&count)
		out[i] = GroupWithMemberCount{Group: g, MemberCount: count}
	}
	return out, nil
}

// GetGroup returns one group + member count.
func (s *GroupService) GetGroup(id uuid.UUID) (*GroupWithMemberCount, error) {
	var g models.Group
	if err := s.db.First(&g, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var count int64
	s.db.Model(&models.GroupMember{}).Where("group_id = ?", g.ID).Count(&count)
	return &GroupWithMemberCount{Group: g, MemberCount: count}, nil
}

// UpdateGroup updates name/description on a native group. OIDC groups return ConflictError.
func (s *GroupService) UpdateGroup(id uuid.UUID, req UpdateGroupRequest, actorID uuid.UUID) (*models.Group, error) {
	var g models.Group
	if err := s.db.First(&g, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if g.Source == models.GroupSourceOIDC {
		return nil, &ConflictError{Message: "OIDC-synced groups are read-only"}
	}

	updates := map[string]interface{}{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if len(updates) == 0 {
		return &g, nil
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&g).Updates(updates).Error; err != nil {
			return fmt.Errorf("update group: %w", err)
		}
		audit.LogAction(tx, actorID, audit.ActionUpdateGroup, fmt.Sprintf("group:%s", g.ID), updates)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &g, nil
}

// DeleteGroup soft-deletes the group row (GORM) and hard-removes every Casbin
// policy + grouping rule that referenced it (Casbin does not honour
// gorm.DeletedAt). OIDC groups return ConflictError — they are reconciled by
// login flow only.
func (s *GroupService) DeleteGroup(id uuid.UUID, actorID uuid.UUID) error {
	var g models.Group
	if err := s.db.First(&g, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	if g.Source == models.GroupSourceOIDC {
		return &ConflictError{Message: "OIDC-synced groups are read-only"}
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("group_id = ?", g.ID).Delete(&models.GroupMember{}).Error; err != nil {
			return fmt.Errorf("delete members: %w", err)
		}
		if err := tx.Where("group_id = ?", g.ID).Delete(&models.GroupPermission{}).Error; err != nil {
			return fmt.Errorf("delete group permissions: %w", err)
		}
		if err := tx.Delete(&g).Error; err != nil {
			return fmt.Errorf("delete group: %w", err)
		}
		audit.LogAction(tx, actorID, audit.ActionDeleteGroup, fmt.Sprintf("group:%s", g.ID), map[string]interface{}{"name": g.Name})
		return nil
	})
	if err != nil {
		return err
	}

	if err := s.rbac.RemoveAllGroupPolicies(g.ID); err != nil {
		return fmt.Errorf("remove casbin policies: %w", err)
	}
	return nil
}

// ListMembers returns every user in a group with denormalised user fields.
func (s *GroupService) ListMembers(groupID uuid.UUID) ([]models.GroupMember, error) {
	var members []models.GroupMember
	if err := s.db.Preload("User").Where("group_id = ?", groupID).Find(&members).Error; err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	return members, nil
}

// AddMember adds a user to a group. Idempotent: existing membership is a no-op.
func (s *GroupService) AddMember(groupID, userID, actorID uuid.UUID) error {
	var g models.Group
	if err := s.db.First(&g, "id = ?", groupID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	if g.Source == models.GroupSourceOIDC {
		return &ConflictError{Message: "OIDC-synced groups manage members from the IdP"}
	}

	var existing models.GroupMember
	err := s.db.Where("group_id = ? AND user_id = ?", groupID, userID).First(&existing).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	var u models.User
	if err := s.db.First(&u, "id = ?", userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &ValidationError{Message: "User not found"}
		}
		return err
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		m := models.GroupMember{GroupID: groupID, UserID: userID}
		if err := tx.Create(&m).Error; err != nil {
			return fmt.Errorf("create member: %w", err)
		}
		audit.LogAction(tx, actorID, audit.ActionAddGroupMember, fmt.Sprintf("group:%s", groupID), map[string]interface{}{
			"user_id": userID,
		})
		return nil
	})
	if err != nil {
		return err
	}

	return s.rbac.AddUserToGroup(userID, groupID)
}

// RemoveMember removes a user from a group.
func (s *GroupService) RemoveMember(groupID, userID, actorID uuid.UUID) error {
	var g models.Group
	if err := s.db.First(&g, "id = ?", groupID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	if g.Source == models.GroupSourceOIDC {
		return &ConflictError{Message: "OIDC-synced groups manage members from the IdP"}
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Where("group_id = ? AND user_id = ?", groupID, userID).Delete(&models.GroupMember{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		audit.LogAction(tx, actorID, audit.ActionRemoveGroupMember, fmt.Sprintf("group:%s", groupID), map[string]interface{}{
			"user_id": userID,
		})
		return nil
	})
	if err != nil {
		return err
	}

	return s.rbac.RemoveUserFromGroup(userID, groupID)
}

// ListGroupsForUser returns groups the user belongs to (used by /groups/me).
func (s *GroupService) ListGroupsForUser(userID uuid.UUID) ([]models.Group, error) {
	var groups []models.Group
	err := s.db.
		Joins("JOIN group_members gm ON gm.group_id = groups.id").
		Where("gm.user_id = ?", userID).
		Find(&groups).Error
	if err != nil {
		return nil, fmt.Errorf("list user groups: %w", err)
	}
	return groups, nil
}
```

- [ ] **Step 3.5: Run the GroupService tests**

Run: `go test -v -run 'TestCreateGroup|TestAddGroupMember|TestRemoveGroupMember|TestDeleteGroup|TestUpdateGroup' ./internal/service/`
Expected: PASS (7 tests).

- [ ] **Step 3.6: Commit**

```bash
git add internal/audit/audit.go internal/service/groups.go internal/service/groups_test.go
git commit -m "service: add GroupService with native CRUD and member ops"
```

---

## Task 4: Share workspace with a group + list-collaborators extension

**Files:**
- Modify: `internal/service/types.go` — extend `CollaboratorResult`, add `GroupCollaboratorResult`
- Modify: `internal/service/workspace_permissions.go` — add `ShareWorkspaceWithGroup`, `UnshareWorkspaceFromGroup`; extend `ListCollaborators`
- Modify: `internal/service/workspace_permissions_test.go` — add tests

- [ ] **Step 4.1: Extend `CollaboratorResult` and add `GroupCollaboratorResult`**

In `internal/service/types.go`, replace the `CollaboratorResult` block:

```go
// CollaboratorKind identifies whether a collaborator entry is a user or a group.
type CollaboratorKind string

const (
	CollaboratorKindUser  CollaboratorKind = "user"
	CollaboratorKindGroup CollaboratorKind = "group"
)

// CollaboratorResult is the result type for ListCollaborators.
type CollaboratorResult struct {
	Kind     CollaboratorKind `json:"kind"`
	UserID   uuid.UUID        `json:"user_id,omitempty"`
	Username string           `json:"username,omitempty"`
	Email    string           `json:"email,omitempty"`
	GroupID  uuid.UUID        `json:"group_id,omitempty"`
	Name     string           `json:"name,omitempty"`
	Source   string           `json:"source,omitempty"` // "" for users, "native"/"oidc" for groups
	Role     string           `json:"role"`
	IsOwner  bool             `json:"is_owner"`
}
```

**Important**: any code that reads `CollaboratorResult.UserID` for groups will get `uuid.Nil` — that's fine for new code, and the existing frontend has not yet been updated, so we'll keep the field shape backward compatible via `omitempty`. Existing user entries unchanged.

Search for callers: `grep -rn "CollaboratorResult" /Users/tylerman/gh/nebi/internal/` — adjust any consumer that expects `UserID` to be non-nil. After this task, callers should branch on `Kind`.

- [ ] **Step 4.2: Write failing tests for `ShareWorkspaceWithGroup` and the extended collaborator listing**

Append to `internal/service/workspace_permissions_test.go`:

```go
func TestShareWorkspaceWithGroup_GrantsTransitiveAccess(t *testing.T) {
	svc, db := testSetup(t, false)
	groupSvc := NewGroupService(db, rbac.NewDefaultProvider())

	alice := createTestUser(t, db, "alice")
	bob := createTestUser(t, db, "bob")
	ws := createReadyWorkspace(t, svc, db, "share-grp", alice)

	db.Create(&models.Role{Name: "viewer", Description: "read"})
	db.Create(&models.Role{Name: "editor", Description: "write"})

	g, _ := groupSvc.CreateGroup(CreateGroupRequest{Name: "team"}, alice)
	_ = groupSvc.AddMember(g.ID, bob, alice)

	perm, err := svc.ShareWorkspaceWithGroup(ws.ID.String(), alice, g.ID, "editor", groupSvc)
	if err != nil {
		t.Fatalf("share with group: %v", err)
	}
	if perm.GroupID != g.ID || perm.WorkspaceID != ws.ID {
		t.Fatalf("permission shape wrong: %+v", perm)
	}

	canWrite, err := svc.rbac.CanWriteWorkspace(bob, ws.ID)
	if err != nil || !canWrite {
		t.Fatalf("bob should have transitive write access, err=%v can=%v", err, canWrite)
	}
}

func TestShareWorkspaceWithGroup_OwnerNotInGroupRejected(t *testing.T) {
	svc, db := testSetup(t, false)
	groupSvc := NewGroupService(db, rbac.NewDefaultProvider())

	alice := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "share-grp", alice)
	db.Create(&models.Role{Name: "viewer"})
	db.Create(&models.Role{Name: "editor"})

	g, _ := groupSvc.CreateGroup(CreateGroupRequest{Name: "outsiders"}, alice)
	// Note: alice is NOT a member of `g`.

	_, err := svc.ShareWorkspaceWithGroup(ws.ID.String(), alice, g.ID, "viewer", groupSvc)
	if err == nil {
		t.Fatal("expected ForbiddenError when owner is not a member of the group")
	}
	var fe *ForbiddenError
	if !errors.As(err, &fe) {
		t.Fatalf("expected ForbiddenError, got %T: %v", err, err)
	}
}

func TestListCollaborators_IncludesGroups(t *testing.T) {
	svc, db := testSetup(t, false)
	groupSvc := NewGroupService(db, rbac.NewDefaultProvider())
	alice := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "x", alice)
	db.Create(&models.Role{Name: "viewer"})
	db.Create(&models.Role{Name: "editor"})

	g, _ := groupSvc.CreateGroup(CreateGroupRequest{Name: "ds"}, alice)
	_ = groupSvc.AddMember(g.ID, alice, alice)
	_, _ = svc.ShareWorkspaceWithGroup(ws.ID.String(), alice, g.ID, "viewer", groupSvc)

	cs, err := svc.ListCollaborators(ws.ID.String())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var sawGroup bool
	for _, c := range cs {
		if c.Kind == CollaboratorKindGroup && c.GroupID == g.ID {
			sawGroup = true
			if c.Role != "viewer" {
				t.Errorf("expected role viewer, got %q", c.Role)
			}
			if c.Source != "native" {
				t.Errorf("expected source native, got %q", c.Source)
			}
		}
	}
	if !sawGroup {
		t.Fatalf("expected group collaborator in list, got %+v", cs)
	}
}
```

If `"errors"` isn't already imported in this file, add it.

- [ ] **Step 4.3: Run the new tests to confirm they fail**

Run: `go test -v -run 'TestShareWorkspaceWithGroup|TestListCollaborators_Includes' ./internal/service/`
Expected: FAIL — `ShareWorkspaceWithGroup` and `CollaboratorKindGroup` undefined.

- [ ] **Step 4.4: Implement `ShareWorkspaceWithGroup` and `UnshareWorkspaceFromGroup`**

Append to `internal/service/workspace_permissions.go`:

```go
// ShareWorkspaceWithGroup grants a group access to a workspace.
// Authorization: caller must be admin OR (owner AND member of the group).
// Membership is resolved via Casbin grouping rules, so the GroupService
// dependency is not needed here.
func (s *WorkspaceService) ShareWorkspaceWithGroup(wsID string, callerID uuid.UUID, groupID uuid.UUID, role string) (*models.GroupPermission, error) {
	wsUUID, err := uuid.Parse(wsID)
	if err != nil {
		return nil, &ValidationError{Message: "Invalid workspace ID"}
	}

	var ws models.Workspace
	if err := s.db.Where("id = ?", wsUUID).First(&ws).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	var g models.Group
	if err := s.db.First(&g, "id = ?", groupID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, &ValidationError{Message: "Group not found"}
		}
		return nil, err
	}

	// Admin bypass: if caller is admin, allow regardless of membership.
	isAdmin, err := s.rbac.IsAdmin(callerID)
	if err != nil {
		return nil, fmt.Errorf("check admin: %w", err)
	}
	if !isAdmin {
		if ws.OwnerID != callerID {
			return nil, &ForbiddenError{Message: "Only the owner or an admin can share this workspace"}
		}
		// Owner must be a member of the group they're sharing to.
		userGroups, err := s.rbac.GetUserGroups(callerID)
		if err != nil {
			return nil, fmt.Errorf("check group membership: %w", err)
		}
		var member bool
		for _, gid := range userGroups {
			if gid == groupID {
				member = true
				break
			}
		}
		if !member {
			return nil, &ForbiddenError{Message: "You can only share with groups you belong to"}
		}
	}

	if role != "viewer" && role != "editor" {
		return nil, &ValidationError{Message: "Role must be 'viewer' or 'editor'"}
	}
	var roleRecord models.Role
	if err := s.db.Where("name = ?", role).First(&roleRecord).Error; err != nil {
		return nil, fmt.Errorf("role not found: %w", err)
	}

	// Reject duplicate.
	var existing models.GroupPermission
	if err := s.db.Where("group_id = ? AND workspace_id = ?", groupID, wsUUID).First(&existing).Error; err == nil {
		return nil, &ConflictError{Message: "Group already has permission on this workspace"}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	var permission models.GroupPermission
	err = s.db.Transaction(func(tx *gorm.DB) error {
		permission = models.GroupPermission{
			GroupID:     groupID,
			WorkspaceID: wsUUID,
			RoleID:      roleRecord.ID,
		}
		if err := tx.Create(&permission).Error; err != nil {
			return fmt.Errorf("create group permission: %w", err)
		}
		audit.LogAction(tx, callerID, audit.ActionGrantGroupPerm, fmt.Sprintf("ws:%s", wsUUID.String()), map[string]interface{}{
			"group_id": groupID,
			"role":     role,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	if err := s.rbac.GrantGroupWorkspaceAccess(groupID, wsUUID, role); err != nil {
		return nil, fmt.Errorf("grant RBAC: %w", err)
	}
	return &permission, nil
}

// UnshareWorkspaceFromGroup revokes a group's access. Same auth rules as ShareWorkspaceWithGroup.
func (s *WorkspaceService) UnshareWorkspaceFromGroup(wsID string, callerID uuid.UUID, groupID uuid.UUID) error {
	wsUUID, err := uuid.Parse(wsID)
	if err != nil {
		return &ValidationError{Message: "Invalid workspace ID"}
	}

	var ws models.Workspace
	if err := s.db.Where("id = ?", wsUUID).First(&ws).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}

	isAdmin, err := s.rbac.IsAdmin(callerID)
	if err != nil {
		return fmt.Errorf("check admin: %w", err)
	}
	if !isAdmin && ws.OwnerID != callerID {
		return &ForbiddenError{Message: "Only the owner or an admin can unshare this workspace"}
	}

	var permission models.GroupPermission
	if err := s.db.Where("group_id = ? AND workspace_id = ?", groupID, wsUUID).First(&permission).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&permission).Error; err != nil {
			return fmt.Errorf("delete group permission: %w", err)
		}
		audit.LogAction(tx, callerID, audit.ActionRevokeGroupPerm, fmt.Sprintf("ws:%s", wsUUID.String()), map[string]interface{}{
			"group_id": groupID,
		})
		return nil
	})
	if err != nil {
		return err
	}

	return s.rbac.RevokeGroupWorkspaceAccess(groupID, wsUUID)
}
```

You will need to add `"errors"` to the import group at the top of `workspace_permissions.go` if not already present.

- [ ] **Step 4.5: Extend `ListCollaborators` to include groups**

In `internal/service/workspace_permissions.go`, replace the existing `ListCollaborators` (currently lines 138–186) with:

```go
// ListCollaborators returns every user (Kind=user) and group (Kind=group) with
// access to the workspace, plus the owner. Groups are tagged with their source.
func (s *WorkspaceService) ListCollaborators(wsID string) ([]CollaboratorResult, error) {
	wsUUID, err := uuid.Parse(wsID)
	if err != nil {
		return nil, &ValidationError{Message: "Invalid workspace ID"}
	}

	var ws models.Workspace
	if err := s.db.Where("id = ?", wsUUID).First(&ws).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	collaborators := []CollaboratorResult{}

	// Owner.
	var owner models.User
	if err := s.db.First(&owner, "id = ?", ws.OwnerID).Error; err == nil {
		collaborators = append(collaborators, CollaboratorResult{
			Kind:     CollaboratorKindUser,
			UserID:   ws.OwnerID,
			Username: owner.Username,
			Email:    owner.Email,
			Role:     "owner",
			IsOwner:  true,
		})
	}

	// Per-user permissions.
	var userPerms []models.Permission
	if err := s.db.Preload("User").Preload("Role").Where("workspace_id = ?", wsUUID).Find(&userPerms).Error; err != nil {
		return nil, fmt.Errorf("fetch user collaborators: %w", err)
	}
	for _, p := range userPerms {
		if p.UserID == ws.OwnerID {
			continue
		}
		collaborators = append(collaborators, CollaboratorResult{
			Kind:     CollaboratorKindUser,
			UserID:   p.UserID,
			Username: p.User.Username,
			Email:    p.User.Email,
			Role:     p.Role.Name,
			IsOwner:  false,
		})
	}

	// Per-group permissions.
	var groupPerms []models.GroupPermission
	if err := s.db.Preload("Group").Preload("Role").Where("workspace_id = ?", wsUUID).Find(&groupPerms).Error; err != nil {
		return nil, fmt.Errorf("fetch group collaborators: %w", err)
	}
	for _, gp := range groupPerms {
		collaborators = append(collaborators, CollaboratorResult{
			Kind:    CollaboratorKindGroup,
			GroupID: gp.GroupID,
			Name:    gp.Group.Name,
			Source:  string(gp.Group.Source),
			Role:    gp.Role.Name,
			IsOwner: false,
		})
	}

	return collaborators, nil
}
```

- [ ] **Step 4.6: Run the workspace permission tests**

Run: `go test -v -run 'TestShareWorkspaceWithGroup|TestListCollaborators|TestShareWorkspace_|TestUnshareWorkspace' ./internal/service/`
Expected: PASS (existing user-share tests still pass; new group tests pass).

- [ ] **Step 4.7: Run the full backend suite**

Run: `make test`
Expected: PASS.

- [ ] **Step 4.8: Commit**

```bash
git add internal/service/types.go internal/service/workspace_permissions.go internal/service/workspace_permissions_test.go
git commit -m "service: share workspaces with groups and surface them in collaborators"
```

---

## Task 5: Admin grants — group as admin, group on registry

**Files:**
- Modify: `internal/service/admin.go` — add `GrantGroupAdmin`, `RevokeGroupAdmin`, `GrantRegistryToGroup`, `RevokeRegistryFromGroup`
- Modify: `internal/service/admin_test.go` — add tests

- [ ] **Step 5.1: Write the failing tests**

Append to `internal/service/admin_test.go`:

```go
func TestGrantGroupAdmin_MembersBecomeEffectiveAdmins(t *testing.T) {
	svc, _, db := adminTestSetup(t)
	groupSvc := NewGroupService(db, rbac.NewDefaultProvider())
	admin := createTestUser(t, db, "admin")
	alice := createTestUser(t, db, "alice")
	g, _ := groupSvc.CreateGroup(CreateGroupRequest{Name: "admins"}, admin)
	_ = groupSvc.AddMember(g.ID, alice, admin)

	if err := svc.GrantGroupAdmin(g.ID, admin); err != nil {
		t.Fatalf("grant group admin: %v", err)
	}

	provider := rbac.NewDefaultProvider()
	isAdmin, err := provider.IsAdmin(alice)
	if err != nil || !isAdmin {
		t.Fatalf("alice should be admin via group, err=%v admin=%v", err, isAdmin)
	}
}

func TestRevokeGroupAdmin_RemovesEffectiveAdmin(t *testing.T) {
	svc, _, db := adminTestSetup(t)
	groupSvc := NewGroupService(db, rbac.NewDefaultProvider())
	admin := createTestUser(t, db, "admin")
	alice := createTestUser(t, db, "alice")
	g, _ := groupSvc.CreateGroup(CreateGroupRequest{Name: "admins"}, admin)
	_ = groupSvc.AddMember(g.ID, alice, admin)
	_ = svc.GrantGroupAdmin(g.ID, admin)

	if err := svc.RevokeGroupAdmin(g.ID, admin); err != nil {
		t.Fatalf("revoke group admin: %v", err)
	}

	isAdmin, _ := rbac.NewDefaultProvider().IsAdmin(alice)
	if isAdmin {
		t.Fatalf("alice should no longer be admin")
	}
}

func TestGrantRegistryToGroup_GivesTransitiveAccess(t *testing.T) {
	svc, _, db := adminTestSetup(t)
	groupSvc := NewGroupService(db, rbac.NewDefaultProvider())
	admin := createTestUser(t, db, "admin")
	alice := createTestUser(t, db, "alice")
	g, _ := groupSvc.CreateGroup(CreateGroupRequest{Name: "reg-team"}, admin)
	_ = groupSvc.AddMember(g.ID, alice, admin)

	reg := models.OCIRegistry{Name: "private", URL: "ghcr.io", Namespace: "ns"}
	if err := db.Create(&reg).Error; err != nil {
		t.Fatalf("seed registry: %v", err)
	}

	if err := svc.GrantRegistryToGroup(reg.ID, g.ID, "write", admin); err != nil {
		t.Fatalf("grant registry: %v", err)
	}

	can, err := rbac.NewDefaultProvider().CanWriteRegistry(alice, reg.ID)
	if err != nil || !can {
		t.Fatalf("alice should have write on registry, err=%v can=%v", err, can)
	}
}
```

Imports needed (add if missing): `"github.com/nebari-dev/nebi/internal/rbac"`, `"github.com/nebari-dev/nebi/internal/models"`.

The `models.OCIRegistry` struct uses `uuid.UUID` as its ID; double-check `internal/models/registry.go` to be sure the field name and JSON tags are right when seeding in the test.

- [ ] **Step 5.2: Run the failing tests**

Run: `go test -v -run 'TestGrantGroupAdmin|TestRevokeGroupAdmin|TestGrantRegistryToGroup' ./internal/service/`
Expected: FAIL — undefined service methods.

- [ ] **Step 5.3: Implement the new admin methods**

Append to `internal/service/admin.go` (before the closing of the file or alongside `RevokePermission`):

```go
// GrantGroupAdmin promotes a group to admin. Every current and future member
// of the group becomes an effective admin via Casbin g + p rules.
func (s *AdminService) GrantGroupAdmin(groupID uuid.UUID, actorID uuid.UUID) error {
	var g models.Group
	if err := s.db.First(&g, "id = ?", groupID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	if err := s.rbac.MakeGroupAdmin(groupID); err != nil {
		return fmt.Errorf("make group admin: %w", err)
	}
	audit.LogAction(s.db, actorID, audit.ActionGrantGroupAdmin, fmt.Sprintf("group:%s", groupID), map[string]interface{}{
		"group_name": g.Name,
	})
	return nil
}

// RevokeGroupAdmin removes the group's admin grant.
func (s *AdminService) RevokeGroupAdmin(groupID uuid.UUID, actorID uuid.UUID) error {
	var g models.Group
	if err := s.db.First(&g, "id = ?", groupID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}
	if err := s.rbac.RevokeGroupAdmin(groupID); err != nil {
		return fmt.Errorf("revoke group admin: %w", err)
	}
	audit.LogAction(s.db, actorID, audit.ActionRevokeGroupAdmin, fmt.Sprintf("group:%s", groupID), map[string]interface{}{
		"group_name": g.Name,
	})
	return nil
}

// GrantRegistryToGroup grants a group access to a registry (read or write).
func (s *AdminService) GrantRegistryToGroup(regID, groupID uuid.UUID, action string, actorID uuid.UUID) error {
	if action != "read" && action != "write" {
		return &ValidationError{Message: "action must be 'read' or 'write'"}
	}
	var reg models.OCIRegistry
	if err := s.db.First(&reg, "id = ?", regID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &ValidationError{Message: "Registry not found"}
		}
		return err
	}
	var g models.Group
	if err := s.db.First(&g, "id = ?", groupID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &ValidationError{Message: "Group not found"}
		}
		return err
	}
	if err := s.rbac.GrantGroupRegistryAccess(groupID, regID, action); err != nil {
		return fmt.Errorf("grant registry: %w", err)
	}
	audit.LogAction(s.db, actorID, audit.ActionGrantGroupPerm, fmt.Sprintf("reg:%s", regID), map[string]interface{}{
		"group_id": groupID,
		"action":   action,
	})
	return nil
}

// RevokeRegistryFromGroup removes a group's access to a registry.
func (s *AdminService) RevokeRegistryFromGroup(regID, groupID uuid.UUID, actorID uuid.UUID) error {
	if err := s.rbac.RevokeGroupRegistryAccess(groupID, regID); err != nil {
		return fmt.Errorf("revoke registry: %w", err)
	}
	audit.LogAction(s.db, actorID, audit.ActionRevokeGroupPerm, fmt.Sprintf("reg:%s", regID), map[string]interface{}{
		"group_id": groupID,
	})
	return nil
}
```

Add `"errors"` to imports if not already there.

- [ ] **Step 5.4: Run the new tests**

Run: `go test -v -run 'TestGrantGroupAdmin|TestRevokeGroupAdmin|TestGrantRegistryToGroup' ./internal/service/`
Expected: PASS (3 tests).

- [ ] **Step 5.5: Commit**

```bash
git add internal/service/admin.go internal/service/admin_test.go
git commit -m "service: admin grants for group admin role and registry access"
```

---

## Task 6: HTTP handlers — `/admin/groups/*`, `/groups/me`

**Files:**
- Create: `internal/api/handlers/group.go`
- Create: `internal/api/handlers/group_test.go`

- [ ] **Step 6.1: Write the failing handler tests**

Create `internal/api/handlers/group_test.go`:

```go
package handlers

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/rbac"
	"github.com/nebari-dev/nebi/internal/service"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupGroupTestRouter(t *testing.T) (*gin.Engine, *gorm.DB, uuid.UUID) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.User{}, &models.Role{}, &models.Group{},
		&models.GroupMember{}, &models.GroupPermission{},
		&models.AuditLog{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := rbac.InitEnforcer(db, slog.Default()); err != nil {
		t.Fatalf("rbac: %v", err)
	}
	t.Cleanup(func() {})

	groupSvc := service.NewGroupService(db, rbac.NewDefaultProvider())
	h := NewGroupHandler(groupSvc)

	user := models.User{Username: "admin", Email: "admin@test"}
	db.Create(&user)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user", &user)
		c.Next()
	})
	admin := r.Group("/api/v1/admin")
	{
		admin.POST("/groups", h.CreateGroup)
		admin.GET("/groups", h.ListGroups)
		admin.GET("/groups/:id", h.GetGroup)
		admin.PATCH("/groups/:id", h.UpdateGroup)
		admin.DELETE("/groups/:id", h.DeleteGroup)
		admin.POST("/groups/:id/members", h.AddMember)
		admin.DELETE("/groups/:id/members/:user_id", h.RemoveMember)
	}
	r.GET("/api/v1/groups/me", h.MyGroups)
	return r, db, user.ID
}

func TestCreateGroup_Handler201(t *testing.T) {
	r, _, _ := setupGroupTestRouter(t)
	body, _ := json.Marshal(map[string]string{"name": "team-a"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/groups", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", w.Code, w.Body.String())
	}
	var out models.Group
	json.Unmarshal(w.Body.Bytes(), &out)
	if out.Name != "team-a" {
		t.Errorf("expected name 'team-a', got %q", out.Name)
	}
}

func TestPatchGroup_OIDCReturns409(t *testing.T) {
	r, db, _ := setupGroupTestRouter(t)
	g := models.Group{Name: "synced", Source: models.GroupSourceOIDC}
	db.Create(&g)

	body, _ := json.Marshal(map[string]string{"description": "x"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/groups/"+g.ID.String(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}

func TestMyGroups_ReturnsOnlyCallersGroups(t *testing.T) {
	r, db, callerID := setupGroupTestRouter(t)
	groupSvc := service.NewGroupService(db, rbac.NewDefaultProvider())

	mine, _ := groupSvc.CreateGroup(service.CreateGroupRequest{Name: "mine"}, callerID)
	_ = groupSvc.AddMember(mine.ID, callerID, callerID)
	_, _ = groupSvc.CreateGroup(service.CreateGroupRequest{Name: "theirs"}, callerID)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/groups/me", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var out []models.Group
	json.Unmarshal(w.Body.Bytes(), &out)
	if len(out) != 1 || out[0].Name != "mine" {
		t.Fatalf("expected single 'mine' group, got %+v", out)
	}
}
```

- [ ] **Step 6.2: Run the failing tests**

Run: `go test -v -run 'TestCreateGroup_Handler|TestPatchGroup_OIDC|TestMyGroups_' ./internal/api/handlers/`
Expected: FAIL — `NewGroupHandler` and handler methods undefined.

- [ ] **Step 6.3: Implement the handler**

Create `internal/api/handlers/group.go`:

```go
package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/service"
)

type GroupHandler struct {
	svc *service.GroupService
}

func NewGroupHandler(svc *service.GroupService) *GroupHandler {
	return &GroupHandler{svc: svc}
}

type CreateGroupRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
}

type UpdateGroupRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

type AddMemberRequest struct {
	UserID uuid.UUID `json:"user_id" binding:"required"`
}

// ListGroups returns all groups with member counts. Admin-only.
// @Router /admin/groups [get]
func (h *GroupHandler) ListGroups(c *gin.Context) {
	groups, err := h.svc.ListGroups()
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, groups)
}

// CreateGroup creates a native group. Admin-only.
// @Router /admin/groups [post]
func (h *GroupHandler) CreateGroup(c *gin.Context) {
	var req CreateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	g, err := h.svc.CreateGroup(service.CreateGroupRequest{
		Name:        req.Name,
		Description: req.Description,
	}, getUserID(c))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, g)
}

// GetGroup returns one group + member count. Admin-only.
// @Router /admin/groups/{id} [get]
func (h *GroupHandler) GetGroup(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid group ID"})
		return
	}
	g, err := h.svc.GetGroup(id)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, g)
}

// UpdateGroup updates a native group; OIDC groups return 409. Admin-only.
// @Router /admin/groups/{id} [patch]
func (h *GroupHandler) UpdateGroup(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid group ID"})
		return
	}
	var req UpdateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	g, err := h.svc.UpdateGroup(id, service.UpdateGroupRequest{
		Name:        req.Name,
		Description: req.Description,
	}, getUserID(c))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, g)
}

// DeleteGroup soft-deletes a native group + hard-removes Casbin rules. Admin-only.
// @Router /admin/groups/{id} [delete]
func (h *GroupHandler) DeleteGroup(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid group ID"})
		return
	}
	if err := h.svc.DeleteGroup(id, getUserID(c)); err != nil {
		handleServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// AddMember adds a user to a native group. Admin-only.
// @Router /admin/groups/{id}/members [post]
func (h *GroupHandler) AddMember(c *gin.Context) {
	groupID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid group ID"})
		return
	}
	var req AddMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	if err := h.svc.AddMember(groupID, req.UserID, getUserID(c)); err != nil {
		handleServiceError(c, err)
		return
	}
	c.Status(http.StatusCreated)
}

// RemoveMember removes a user from a native group. Admin-only.
// @Router /admin/groups/{id}/members/{user_id} [delete]
func (h *GroupHandler) RemoveMember(c *gin.Context) {
	groupID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid group ID"})
		return
	}
	userID, err := uuid.Parse(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid user ID"})
		return
	}
	if err := h.svc.RemoveMember(groupID, userID, getUserID(c)); err != nil {
		handleServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// ListMembers returns every user in a group. Admin-only.
// @Router /admin/groups/{id}/members [get]
func (h *GroupHandler) ListMembers(c *gin.Context) {
	groupID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid group ID"})
		return
	}
	members, err := h.svc.ListMembers(groupID)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, members)
}

// MyGroups returns the caller's groups (used by the ShareDialog picker).
// @Router /groups/me [get]
func (h *GroupHandler) MyGroups(c *gin.Context) {
	uid := getUserID(c)
	if uid == uuid.Nil {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "Unauthorized"})
		return
	}
	groups, err := h.svc.ListGroupsForUser(uid)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, groups)
}
```

- [ ] **Step 6.4: Run the handler tests**

Run: `go test -v -run 'TestCreateGroup_Handler|TestPatchGroup_OIDC|TestMyGroups_' ./internal/api/handlers/`
Expected: PASS (3 tests).

- [ ] **Step 6.5: Commit**

```bash
git add internal/api/handlers/group.go internal/api/handlers/group_test.go
git commit -m "api: handlers for /admin/groups and /groups/me"
```

---

## Task 7: HTTP handlers — workspace share-group, registry-group, group-as-admin

**Files:**
- Modify: `internal/api/handlers/workspace.go` — add `ShareWorkspaceWithGroup`, `UnshareWorkspaceWithGroup`
- Modify: `internal/api/handlers/registry.go` — add `GrantRegistryToGroup`, `RevokeRegistryFromGroup`
- Modify: `internal/api/handlers/admin.go` — add `GrantGroupAdmin`, `RevokeGroupAdmin`

- [ ] **Step 7.1: Workspace share-group handlers**

Append to `internal/api/handlers/workspace.go`:

```go
type ShareWorkspaceWithGroupRequest struct {
	GroupID uuid.UUID `json:"group_id" binding:"required"`
	Role    string    `json:"role" binding:"required"` // "viewer" or "editor"
}

// ShareWorkspaceWithGroup grants a group access to a workspace.
// @Router /workspaces/{id}/share-group [post]
func (h *WorkspaceHandler) ShareWorkspaceWithGroup(c *gin.Context) {
	var req ShareWorkspaceWithGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	perm, err := h.svc.ShareWorkspaceWithGroup(c.Param("id"), getUserID(c), req.GroupID, req.Role)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, perm)
}

// UnshareWorkspaceWithGroup revokes a group's access.
// @Router /workspaces/{id}/share-group/{group_id} [delete]
func (h *WorkspaceHandler) UnshareWorkspaceWithGroup(c *gin.Context) {
	groupID, err := uuid.Parse(c.Param("group_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid group ID"})
		return
	}
	if err := h.svc.UnshareWorkspaceFromGroup(c.Param("id"), getUserID(c), groupID); err != nil {
		handleServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
```

`ShareWorkspaceWithGroup` resolves membership through Casbin grouping rules, so the handler does NOT need to hold a `*service.GroupService` for this method. Leave `WorkspaceHandler` and `NewWorkspaceHandler` unchanged — no extra dependency, no new constructor argument, no call-site updates required for this method.

- [ ] **Step 7.2: Registry-group handlers**

Append to `internal/api/handlers/registry.go`:

```go
type GrantRegistryToGroupRequest struct {
	GroupID uuid.UUID `json:"group_id" binding:"required"`
	Action  string    `json:"action" binding:"required"` // "read" or "write"
}

// GrantRegistryToGroup grants a group access to a registry. Admin-only.
// @Router /admin/registries/{id}/grant-group [post]
func (h *RegistryHandler) GrantRegistryToGroup(c *gin.Context) {
	regID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid registry ID"})
		return
	}
	var req GrantRegistryToGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	if err := adminSvc.GrantRegistryToGroup(regID, req.GroupID, req.Action, getUserID(c)); err != nil {
		handleServiceError(c, err)
		return
	}
	c.Status(http.StatusCreated)
}

// RevokeRegistryFromGroup revokes a group's access to a registry. Admin-only.
// @Router /admin/registries/{id}/grant-group/{group_id} [delete]
func (h *RegistryHandler) RevokeRegistryFromGroup(c *gin.Context) {
	regID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid registry ID"})
		return
	}
	groupID, err := uuid.Parse(c.Param("group_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid group ID"})
		return
	}
	if err := adminSvc.RevokeRegistryFromGroup(regID, groupID, getUserID(c)); err != nil {
		handleServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
```

Same DI approach as workspace: extend `RegistryHandler` to hold `adminSvc *service.AdminService` and update `NewRegistryHandler`. (Inspect `registry.go` first to see if `RegistryHandler` already holds an AdminService — many handlers in this repo already inject sibling services.)

- [ ] **Step 7.3: Admin group-as-admin handlers**

Append to `internal/api/handlers/admin.go`:

```go
// GrantGroupAdmin promotes a group to admin. Admin-only.
// @Router /admin/groups/{id}/grant-admin [post]
func (h *AdminHandler) GrantGroupAdmin(c *gin.Context) {
	groupID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid group ID"})
		return
	}
	if err := h.svc.GrantGroupAdmin(groupID, getUserID(c)); err != nil {
		handleServiceError(c, err)
		return
	}
	c.Status(http.StatusCreated)
}

// RevokeGroupAdmin removes the group's admin grant. Admin-only.
// @Router /admin/groups/{id}/grant-admin [delete]
func (h *AdminHandler) RevokeGroupAdmin(c *gin.Context) {
	groupID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid group ID"})
		return
	}
	if err := h.svc.RevokeGroupAdmin(groupID, getUserID(c)); err != nil {
		handleServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
```

- [ ] **Step 7.4: Verify the build**

Run: `go build ./...`
Expected: success. If the workspace/registry handler constructor changes broke call sites, fix each one to pass the new dependency.

- [ ] **Step 7.5: Run handler tests**

Run: `go test -v ./internal/api/handlers/...`
Expected: PASS — all previous handler tests, plus the new group handler tests from Task 6, plus any new tests for share-group if you choose to add them (optional at this layer, since the service layer is tested).

- [ ] **Step 7.6: Commit**

```bash
git add internal/api/handlers/workspace.go internal/api/handlers/registry.go internal/api/handlers/admin.go
git commit -m "api: handlers for share-group, registry-group grants, and group-as-admin"
```

---

## Task 8: Wire all new routes

**Files:**
- Modify: `internal/api/router.go`

- [ ] **Step 8.1: Construct the GroupService and pass it to the handlers**

In `internal/api/router.go`, find where `WorkspaceService` and `AdminService` are constructed (search for `service.New(...)` and `service.NewAdminService(...)`). After those, add:

```go
	groupSvc := service.NewGroupService(db, rbacProvider)
	groupHandler := handlers.NewGroupHandler(groupSvc)
```

The existing `wsHandler := handlers.NewWorkspaceHandler(wsSvc)` call does NOT need to change — `ShareWorkspaceWithGroup` does not need the GroupService.

For `registryHandler`, if you adopted Option A (passing `adminSvc`), update its constructor call accordingly.

- [ ] **Step 8.2: Register `/admin/groups/*` routes**

Inside the admin route group (find the block `admin := protected.Group("/admin")` and the lines after `admin.Use(middleware.RequireAdmin(...))`), append:

```go
		// Groups
		admin.GET("/groups", groupHandler.ListGroups)
		admin.POST("/groups", groupHandler.CreateGroup)
		admin.GET("/groups/:id", groupHandler.GetGroup)
		admin.PATCH("/groups/:id", groupHandler.UpdateGroup)
		admin.DELETE("/groups/:id", groupHandler.DeleteGroup)
		admin.GET("/groups/:id/members", groupHandler.ListMembers)
		admin.POST("/groups/:id/members", groupHandler.AddMember)
		admin.DELETE("/groups/:id/members/:user_id", groupHandler.RemoveMember)
		admin.POST("/groups/:id/grant-admin", adminHandler.GrantGroupAdmin)
		admin.DELETE("/groups/:id/grant-admin", adminHandler.RevokeGroupAdmin)
		admin.POST("/registries/:id/grant-group", registryHandler.GrantRegistryToGroup)
		admin.DELETE("/registries/:id/grant-group/:group_id", registryHandler.RevokeRegistryFromGroup)
```

(Replace `adminHandler` / `registryHandler` with the real local variable names — inspect the surrounding code to be sure.)

- [ ] **Step 8.3: Register `/groups/me` (caller picker)**

Inside the `protected` group (already authenticated, no admin check), add:

```go
		protected.GET("/groups/me", groupHandler.MyGroups)
```

- [ ] **Step 8.4: Register workspace share-group routes**

Inside the `ws := protected.Group("/workspaces/:id")` block, add:

```go
			ws.POST("/share-group", wsHandler.ShareWorkspaceWithGroup)
			ws.DELETE("/share-group/:group_id", wsHandler.UnshareWorkspaceWithGroup)
```

(No `RequireWorkspaceAccess` middleware — the service layer enforces "admin OR (owner AND member)".)

- [ ] **Step 8.5: Verify the build**

Run: `go build ./...`
Expected: success.

- [ ] **Step 8.6: Smoke-test the routes**

Run: `make test`
Expected: PASS. The router file itself has no tests, but the handlers and services are covered. If anything fails, the wiring is wrong — re-check route paths, parameter names (`:id` vs `:group_id`), and handler/service constructor arities.

- [ ] **Step 8.7: Commit**

```bash
git add internal/api/router.go
git commit -m "api: wire group, share-group, and group-as-admin routes"
```

---

## Task 9: OIDC group sync (scope, claim, JIT reconcile)

**Files:**
- Modify: `internal/auth/oidc.go` — request `groups` scope, parse claim, JIT sync
- Modify: `internal/auth/basic.go` — `ExchangeIDToken` mirrors same sync (proxy flow)
- Create: `internal/auth/group_sync.go` — shared sync function
- Create: `internal/auth/group_sync_test.go` — direct test of the sync function

- [ ] **Step 9.1: Write the failing test for the sync function**

Create `internal/auth/group_sync_test.go`:

```go
package auth

import (
	"log/slog"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/rbac"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func syncTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Group{}, &models.GroupMember{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := rbac.InitEnforcer(db, slog.Default()); err != nil {
		t.Fatalf("rbac: %v", err)
	}
	t.Cleanup(func() {})
	return db
}

func TestSyncOIDCGroups_CreatesGroupAndMembership(t *testing.T) {
	db := syncTestDB(t)
	u := models.User{Username: "alice", Email: "alice@test"}
	db.Create(&u)

	if err := SyncOIDCGroups(db, u.ID, []string{"data-science", "admins"}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	var groups []models.Group
	db.Find(&groups)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	for _, g := range groups {
		if g.Source != models.GroupSourceOIDC {
			t.Errorf("group %q expected source oidc, got %q", g.Name, g.Source)
		}
	}

	memberships, _ := rbac.GetUserGroups(u.ID)
	if len(memberships) != 2 {
		t.Fatalf("expected 2 casbin memberships, got %d", len(memberships))
	}
}

func TestSyncOIDCGroups_RemovesStaleMemberships(t *testing.T) {
	db := syncTestDB(t)
	u := models.User{Username: "alice", Email: "alice@test"}
	db.Create(&u)
	_ = SyncOIDCGroups(db, u.ID, []string{"x", "y"})

	if err := SyncOIDCGroups(db, u.ID, []string{"x"}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	memberships, _ := rbac.GetUserGroups(u.ID)
	if len(memberships) != 1 {
		t.Fatalf("expected 1 membership after reconcile, got %d", len(memberships))
	}
}

func TestSyncOIDCGroups_KeepsZeroMemberGroups(t *testing.T) {
	db := syncTestDB(t)
	u := models.User{Username: "alice", Email: "alice@test"}
	db.Create(&u)
	_ = SyncOIDCGroups(db, u.ID, []string{"keep-me"})
	_ = SyncOIDCGroups(db, u.ID, []string{}) // user dropped from the group

	var g models.Group
	if err := db.First(&g, "name = ?", "keep-me").Error; err != nil {
		t.Fatalf("expected group 'keep-me' to still exist, err=%v", err)
	}
}

func TestSyncOIDCGroups_DoesNotTouchNativeMemberships(t *testing.T) {
	db := syncTestDB(t)
	u := models.User{Username: "alice", Email: "alice@test"}
	db.Create(&u)
	native := models.Group{Name: "native-grp", Source: models.GroupSourceNative}
	db.Create(&native)
	db.Create(&models.GroupMember{GroupID: native.ID, UserID: u.ID})
	_ = rbac.AddUserToGroup(u.ID, native.ID)

	_ = SyncOIDCGroups(db, u.ID, []string{"x"})

	var mem models.GroupMember
	if err := db.Where("group_id = ? AND user_id = ?", native.ID, u.ID).First(&mem).Error; err != nil {
		t.Fatalf("native membership should be untouched, err=%v", err)
	}
	memberships, _ := rbac.GetUserGroups(u.ID)
	if len(memberships) != 2 {
		t.Fatalf("expected 2 memberships (1 native + 1 oidc), got %d: %v", len(memberships), memberships)
	}
	nativeStillPresent := false
	for _, id := range memberships {
		if id == native.ID {
			nativeStillPresent = true
			break
		}
	}
	if !nativeStillPresent {
		t.Fatalf("native group %s missing from casbin memberships: %v", native.ID, memberships)
	}
}

// Regression test for the silent-merge security issue: if an OIDC claim names
// a group that already exists as `source=native`, we must refuse to add the
// user to it. Phase 2 reconcile only sees source=oidc rows, so any membership
// created here would become permanent untracked access from external IdP data.
func TestSyncOIDCGroups_RefusesToMergeIntoNativeGroup(t *testing.T) {
	db := syncTestDB(t)
	u := models.User{Username: "alice", Email: "alice@test"}
	db.Create(&u)

	// Operator pre-creates a native group with a name that an IdP could collide with.
	native := models.Group{Name: "engineering", Source: models.GroupSourceNative}
	if err := db.Create(&native).Error; err != nil {
		t.Fatalf("seed native: %v", err)
	}

	// OIDC claim arrives with the same name.
	if err := SyncOIDCGroups(db, u.ID, []string{"engineering"}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Alice must NOT be a member of the native group via DB.
	var mem models.GroupMember
	err := db.Where("group_id = ? AND user_id = ?", native.ID, u.ID).First(&mem).Error
	if err == nil {
		t.Fatalf("expected no GroupMember row for native group, found one")
	}

	// And NOT a member via Casbin.
	memberships, _ := rbac.GetUserGroups(u.ID)
	for _, id := range memberships {
		if id == native.ID {
			t.Fatalf("expected user NOT to be in casbin grouping rule for native group, got %v", memberships)
		}
	}

	// The native group's source must remain unchanged.
	var refetched models.Group
	if err := db.First(&refetched, "id = ?", native.ID).Error; err != nil {
		t.Fatalf("refetch native: %v", err)
	}
	if refetched.Source != models.GroupSourceNative {
		t.Fatalf("native group's source was reclassified to %q", refetched.Source)
	}
}

func _testUUID() uuid.UUID { return uuid.New() }
```

- [ ] **Step 9.2: Run failing tests**

Run: `go test -v -run TestSyncOIDCGroups ./internal/auth/`
Expected: FAIL — `SyncOIDCGroups` undefined.

- [ ] **Step 9.3: Implement the sync function**

Create `internal/auth/group_sync.go`:

```go
package auth

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/rbac"
	"gorm.io/gorm"
)

// SyncOIDCGroups reconciles the user's OIDC group memberships with the names
// in the latest ID token's `groups` claim. Idempotent: safe to call on every
// login. Only affects groups with source=oidc; native memberships are
// untouched. Zero-member OIDC groups are preserved so existing workspace
// shares survive churn.
//
// Name collision with native groups: If an OIDC claim names a group that
// already exists with source=native, the membership is NOT added — native
// groups are administered explicitly in nebi, and silently merging IdP claims
// into them would create permanent untracked grants (phase-2 reconcile only
// considers source=oidc memberships).
func SyncOIDCGroups(db *gorm.DB, userID uuid.UUID, claimGroups []string) error {
	desired := make(map[string]struct{}, len(claimGroups))
	for _, name := range claimGroups {
		if name == "" {
			continue
		}
		desired[name] = struct{}{}
	}

	// Phase 1: upsert each desired group + membership.
	for name := range desired {
		var g models.Group
		err := db.Where("name = ?", name).First(&g).Error
		switch {
		case err == nil:
			// If this name already exists as a native group, do NOT merge OIDC claims
			// into it. Native group membership is administered explicitly in nebi; an
			// OIDC claim that happens to share the name must not silently grant
			// permanent access (phase-2 reconcile only looks at source=oidc, so any
			// membership added here would never be removed).
			if g.Source == models.GroupSourceNative {
				slog.Warn("OIDC claim names a native group; skipping membership",
					"group_name", name, "group_id", g.ID, "user_id", userID)
				continue
			}
		case errors.Is(err, gorm.ErrRecordNotFound):
			g = models.Group{Name: name, Source: models.GroupSourceOIDC}
			if err := db.Create(&g).Error; err != nil {
				return fmt.Errorf("create oidc group %q: %w", name, err)
			}
		default:
			return fmt.Errorf("lookup group %q: %w", name, err)
		}

		var existing models.GroupMember
		err = db.Where("group_id = ? AND user_id = ?", g.ID, userID).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := db.Create(&models.GroupMember{GroupID: g.ID, UserID: userID}).Error; err != nil {
				return fmt.Errorf("create membership for %q: %w", name, err)
			}
		} else if err != nil {
			return fmt.Errorf("lookup membership for %q: %w", name, err)
		}

		if err := rbac.AddUserToGroup(userID, g.ID); err != nil {
			return fmt.Errorf("casbin add %q: %w", name, err)
		}
	}

	// Phase 2: remove stale OIDC memberships not in claim.
	var current []models.GroupMember
	err := db.
		Joins("JOIN groups g ON g.id = group_members.group_id").
		Where("group_members.user_id = ? AND g.source = ?", userID, models.GroupSourceOIDC).
		Preload("Group").
		Find(&current).Error
	if err != nil {
		return fmt.Errorf("list current oidc memberships: %w", err)
	}

	for _, m := range current {
		if _, ok := desired[m.Group.Name]; ok {
			continue
		}
		if err := db.Where("group_id = ? AND user_id = ?", m.GroupID, userID).Delete(&models.GroupMember{}).Error; err != nil {
			return fmt.Errorf("delete stale membership: %w", err)
		}
		if err := rbac.RemoveUserFromGroup(userID, m.GroupID); err != nil {
			return fmt.Errorf("casbin remove stale: %w", err)
		}
	}

	slog.Debug("OIDC groups synced", "user_id", userID, "claim_count", len(desired))
	return nil
}
```

- [ ] **Step 9.4: Hook the sync into the OIDC callback**

Edit `internal/auth/oidc.go`:

1. Default scopes (line ~53): include `"groups"`:

```go
		scopes = []string{oidc.ScopeOpenID, "profile", "email", "groups"}
```

If `cfg.Scopes` is non-empty, the caller is opting in or out — leave that branch alone.

2. Claims struct (lines ~113-120): add `Groups`:

```go
		var claims struct {
			Email             string   `json:"email"`
			EmailVerified     bool     `json:"email_verified"`
			Name              string   `json:"name"`
			PreferredUsername string   `json:"preferred_username"`
			Sub               string   `json:"sub"`
			Picture           string   `json:"picture"`
			Groups            []string `json:"groups"`
		}
```

3. After `findOrCreateUser` succeeds and before `generateToken`, add the sync (still inside `HandleCallback`):

```go
		if err := SyncOIDCGroups(a.db, user.ID, claims.Groups); err != nil {
			slog.Warn("OIDC group sync failed; continuing login", "user_id", user.ID, "err", err)
		}
```

We log-and-continue rather than fail the whole login: a partial group sync should not lock a user out. If your codebase has a stricter policy, escalate to `return nil, err` after discussion with the team.

- [ ] **Step 9.5: Mirror the sync in the proxy / ID-token-exchange path**

Edit `internal/auth/basic.go` — find `ExchangeIDToken` (around line 274). Where it parses claims today, ensure `Groups []string` is part of the struct, then after the user is upserted call `SyncOIDCGroups(a.db, user.ID, claims.Groups)` with the same log-and-continue pattern.

Read the existing `ExchangeIDToken` body first, then make the minimal addition.

- [ ] **Step 9.6: Run all auth tests**

Run: `go test -v ./internal/auth/...`
Expected: PASS — including the new `SyncOIDCGroups` tests.

- [ ] **Step 9.7: Run the full suite**

Run: `make test`
Expected: PASS.

- [ ] **Step 9.8: Commit**

```bash
git add internal/auth/oidc.go internal/auth/basic.go internal/auth/group_sync.go internal/auth/group_sync_test.go
git commit -m "auth: OIDC group sync on login (JIT upsert + stale reconcile)"
```

---

## Task 10: Backend smoke check — manual HTTP run

**Files:** none.

- [ ] **Step 10.1: Run the server in native mode**

```bash
make build && ./bin/nebi serve --config config.yaml.example &
SERVER_PID=$!
sleep 2
```

- [ ] **Step 10.2: Create a user and authenticate**

Use the existing `auth/login` endpoint to obtain a token. Inspect `cmd/` for the bootstrap path if a seed admin doesn't exist yet, or use the local-mode bypass for the smoke test.

- [ ] **Step 10.3: Exercise the new endpoints**

```bash
# Create a group
curl -X POST localhost:8080/api/v1/admin/groups \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"name":"smoke-test","description":"smoke"}'

# List groups
curl localhost:8080/api/v1/admin/groups -H "Authorization: Bearer $TOKEN"

# Add a member, share a workspace, list collaborators … etc.
```

Document what you tested in the commit message.

- [ ] **Step 10.4: Kill the server**

```bash
kill $SERVER_PID
```

- [ ] **Step 10.5: Commit a stub note if you fixed anything during the smoke test**

(Often no commit is needed; this task exists to catch wiring drift before frontend work begins.)

---

## Task 11: Frontend types + API client

**Files:**
- Modify: `frontend/src/types/models.ts`
- Create: `frontend/src/api/groups.ts`

- [ ] **Step 11.1: Add group types to `frontend/src/types/models.ts`**

Append to the existing `models.ts`:

```ts
export type GroupSource = 'native' | 'oidc';

export interface Group {
  id: string;
  name: string;
  description: string;
  source: GroupSource;
  created_at: string;
  updated_at: string;
}

export interface GroupWithMemberCount extends Group {
  member_count: number;
}

export interface GroupMember {
  group_id: string;
  user_id: string;
  created_at: string;
  user?: User;
}

export interface GroupCollaborator {
  kind: 'group';
  group_id: string;
  name: string;
  source: GroupSource;
  role: 'editor' | 'viewer';
  is_owner: false;
}

// Extend the existing Collaborator type to a discriminated union.
// Find the current `Collaborator` interface (around line 96) and replace it with:
export type Collaborator =
  | {
      kind: 'user';
      user_id: string;
      username: string;
      email: string;
      role: 'owner' | 'editor' | 'viewer';
      is_owner: boolean;
    }
  | GroupCollaborator;

export interface CreateGroupRequest {
  name: string;
  description?: string;
}

export interface UpdateGroupRequest {
  name?: string;
  description?: string;
}

export interface ShareWorkspaceWithGroupRequest {
  group_id: string;
  role: 'editor' | 'viewer';
}
```

If TypeScript complains that consumers of the old flat `Collaborator` shape now break, that's expected: you'll fix call sites in subsequent tasks (12-14). Plan order: types → ShareDialog → admin page → user-page.

- [ ] **Step 11.2: Create `frontend/src/api/groups.ts`**

```ts
import { apiClient } from './client';
import type {
  Group,
  GroupWithMemberCount,
  GroupMember,
  CreateGroupRequest,
  UpdateGroupRequest,
  ShareWorkspaceWithGroupRequest,
} from '@/types/models';

export const groupsApi = {
  list: async (): Promise<GroupWithMemberCount[]> => {
    const r = await apiClient.get('/admin/groups');
    return r.data;
  },
  get: async (id: string): Promise<GroupWithMemberCount> => {
    const r = await apiClient.get(`/admin/groups/${id}`);
    return r.data;
  },
  create: async (data: CreateGroupRequest): Promise<Group> => {
    const r = await apiClient.post('/admin/groups', data);
    return r.data;
  },
  update: async (id: string, data: UpdateGroupRequest): Promise<Group> => {
    const r = await apiClient.patch(`/admin/groups/${id}`, data);
    return r.data;
  },
  remove: async (id: string): Promise<void> => {
    await apiClient.delete(`/admin/groups/${id}`);
  },

  listMembers: async (id: string): Promise<GroupMember[]> => {
    const r = await apiClient.get(`/admin/groups/${id}/members`);
    return r.data;
  },
  addMember: async (id: string, userId: string): Promise<void> => {
    await apiClient.post(`/admin/groups/${id}/members`, { user_id: userId });
  },
  removeMember: async (id: string, userId: string): Promise<void> => {
    await apiClient.delete(`/admin/groups/${id}/members/${userId}`);
  },

  grantAdmin: async (id: string): Promise<void> => {
    await apiClient.post(`/admin/groups/${id}/grant-admin`);
  },
  revokeAdmin: async (id: string): Promise<void> => {
    await apiClient.delete(`/admin/groups/${id}/grant-admin`);
  },

  myGroups: async (): Promise<Group[]> => {
    const r = await apiClient.get('/groups/me');
    return r.data;
  },

  shareWorkspace: async (workspaceId: string, body: ShareWorkspaceWithGroupRequest): Promise<void> => {
    await apiClient.post(`/workspaces/${workspaceId}/share-group`, body);
  },
  unshareWorkspace: async (workspaceId: string, groupId: string): Promise<void> => {
    await apiClient.delete(`/workspaces/${workspaceId}/share-group/${groupId}`);
  },
};
```

- [ ] **Step 11.3: Type-check**

Run: `cd frontend && npx tsc -b`
Expected: errors will surface in consumers of the old flat `Collaborator` type. Note the file:line where each error appears — you'll fix them inline as part of Task 12 (`ShareDialog`) and Task 14 (UserManagement). If the error list is short, you can either fix them now in tiny edits, or commit the types now and fix consumers per page.

For now, commit the types + API client. Consumers will be fixed in the next tasks.

- [ ] **Step 11.4: Commit**

```bash
git add frontend/src/types/models.ts frontend/src/api/groups.ts
git commit -m "frontend: add Group types and API client"
```

---

## Task 12: Frontend `useGroups` hook + Admin Groups page

**Files:**
- Create: `frontend/src/hooks/useGroups.ts`
- Create: `frontend/src/pages/admin/Groups.tsx`
- Create: `frontend/src/components/admin/CreateGroupDialog.tsx`
- Create: `frontend/src/components/admin/GroupMembersDialog.tsx`
- Modify: `frontend/src/App.tsx` — add `/admin/groups` route
- Modify: `frontend/src/components/admin/AdminLayout.tsx` (or wherever the admin nav lives) — add nav link

- [ ] **Step 12.1: Inspect the existing admin nav structure**

Run: `grep -rn "admin/users\|AdminLayout" /Users/tylerman/gh/nebi/frontend/src/`
Identify the component that renders the admin sidebar/nav. You'll add a Groups link there in Step 12.6.

- [ ] **Step 12.2: Create the React Query hook**

Create `frontend/src/hooks/useGroups.ts`:

```ts
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { groupsApi } from '@/api/groups';
import type { CreateGroupRequest, UpdateGroupRequest } from '@/types/models';

const groupsKey = ['groups'] as const;

export const useGroups = () =>
  useQuery({ queryKey: groupsKey, queryFn: groupsApi.list });

export const useGroup = (id: string | undefined) =>
  useQuery({
    queryKey: ['group', id],
    queryFn: () => groupsApi.get(id!),
    enabled: !!id,
  });

export const useGroupMembers = (id: string | undefined) =>
  useQuery({
    queryKey: ['group', id, 'members'],
    queryFn: () => groupsApi.listMembers(id!),
    enabled: !!id,
  });

export const useMyGroups = (enabled = true) =>
  useQuery({ queryKey: ['groups', 'me'], queryFn: groupsApi.myGroups, enabled });

export const useCreateGroup = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: CreateGroupRequest) => groupsApi.create(data),
    onSuccess: () => qc.invalidateQueries({ queryKey: groupsKey }),
  });
};

export const useUpdateGroup = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: UpdateGroupRequest }) =>
      groupsApi.update(id, data),
    onSuccess: (_, { id }) => {
      qc.invalidateQueries({ queryKey: groupsKey });
      qc.invalidateQueries({ queryKey: ['group', id] });
    },
  });
};

export const useDeleteGroup = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => groupsApi.remove(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: groupsKey }),
  });
};

export const useAddGroupMember = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, userId }: { id: string; userId: string }) =>
      groupsApi.addMember(id, userId),
    onSuccess: (_, { id }) => {
      qc.invalidateQueries({ queryKey: ['group', id, 'members'] });
      qc.invalidateQueries({ queryKey: groupsKey });
    },
  });
};

export const useRemoveGroupMember = () => {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, userId }: { id: string; userId: string }) =>
      groupsApi.removeMember(id, userId),
    onSuccess: (_, { id }) => {
      qc.invalidateQueries({ queryKey: ['group', id, 'members'] });
      qc.invalidateQueries({ queryKey: groupsKey });
    },
  });
};
```

- [ ] **Step 12.3: Create `CreateGroupDialog.tsx`**

Mirror `frontend/src/components/admin/CreateUserDialog.tsx` (read it first to copy the structure exactly):

```tsx
import { useState } from 'react';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger, DialogFooter } from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { useCreateGroup } from '@/hooks/useGroups';

export const CreateGroupDialog = () => {
  const [open, setOpen] = useState(false);
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [error, setError] = useState('');
  const createMutation = useCreateGroup();

  const onSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    try {
      await createMutation.mutateAsync({ name, description });
      setName('');
      setDescription('');
      setOpen(false);
    } catch (err) {
      const message = (err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? 'Failed to create group';
      setError(message);
    }
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button>Create group</Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Create group</DialogTitle>
        </DialogHeader>
        <form onSubmit={onSubmit} className="space-y-4">
          {error && <div className="bg-red-500/10 border border-red-500/20 text-red-500 px-3 py-2 rounded text-sm">{error}</div>}
          <div className="space-y-2">
            <Label htmlFor="grp-name">Name</Label>
            <Input id="grp-name" required value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="grp-desc">Description</Label>
            <Input id="grp-desc" value={description} onChange={(e) => setDescription(e.target.value)} />
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setOpen(false)}>Cancel</Button>
            <Button type="submit" disabled={createMutation.isPending}>Create</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
};
```

If the `Dialog` primitives used by `CreateUserDialog.tsx` differ (the repo uses Radix UI for some dialogs), match that exactly. Read `CreateUserDialog.tsx` end-to-end before writing this one.

- [ ] **Step 12.4: Create `GroupMembersDialog.tsx`**

```tsx
import { useState } from 'react';
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Trash2, UserPlus } from 'lucide-react';
import { SelectRoot, SelectTrigger, SelectValue, SelectContent, SelectItem } from '@/components/ui/select';
import { useGroupMembers, useAddGroupMember, useRemoveGroupMember } from '@/hooks/useGroups';
import { useUsers } from '@/hooks/useAdmin';
import type { GroupWithMemberCount } from '@/types/models';

interface Props {
  group: GroupWithMemberCount;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export const GroupMembersDialog = ({ group, open, onOpenChange }: Props) => {
  const { data: members, isLoading } = useGroupMembers(group.id);
  const { data: users } = useUsers();
  const addMutation = useAddGroupMember();
  const removeMutation = useRemoveGroupMember();
  const [selectedUser, setSelectedUser] = useState('');
  const [error, setError] = useState('');

  const isOIDC = group.source === 'oidc';
  const availableUsers = (users ?? []).filter(
    (u) => !members?.some((m) => m.user_id === u.id),
  );

  const handleAdd = async () => {
    if (!selectedUser) return;
    setError('');
    try {
      await addMutation.mutateAsync({ id: group.id, userId: selectedUser });
      setSelectedUser('');
    } catch (err) {
      setError(
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
          'Failed to add member',
      );
    }
  };

  const handleRemove = async (userId: string) => {
    setError('');
    try {
      await removeMutation.mutateAsync({ id: group.id, userId });
    } catch (err) {
      setError(
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
          'Failed to remove member',
      );
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            {group.name}
            {isOIDC && (
              <Badge variant="outline" className="border-blue-500/40 text-blue-500">
                OIDC-synced
              </Badge>
            )}
          </DialogTitle>
        </DialogHeader>

        {error && (
          <div className="bg-red-500/10 border border-red-500/20 text-red-500 px-3 py-2 rounded text-sm">
            {error}
          </div>
        )}

        <div className="space-y-4">
          <div>
            <h3 className="text-sm font-medium mb-2">Members</h3>
            {isLoading ? (
              <div className="text-sm text-muted-foreground">Loading…</div>
            ) : members && members.length > 0 ? (
              <ul className="divide-y border rounded">
                {members.map((m) => (
                  <li key={m.user_id} className="flex items-center justify-between p-2">
                    <div>
                      <div className="font-medium text-sm">{m.user?.username ?? m.user_id}</div>
                      {m.user?.email && (
                        <div className="text-xs text-muted-foreground">{m.user.email}</div>
                      )}
                    </div>
                    {!isOIDC && (
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => handleRemove(m.user_id)}
                        disabled={removeMutation.isPending}
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    )}
                  </li>
                ))}
              </ul>
            ) : (
              <div className="text-sm text-muted-foreground">No members.</div>
            )}
          </div>

          {!isOIDC && availableUsers.length > 0 && (
            <div className="space-y-2">
              <h3 className="text-sm font-medium">Add member</h3>
              <div className="flex gap-2">
                <SelectRoot value={selectedUser} onValueChange={setSelectedUser}>
                  <SelectTrigger className="flex-1">
                    <SelectValue placeholder="Select a user" />
                  </SelectTrigger>
                  <SelectContent>
                    {availableUsers.map((u) => (
                      <SelectItem key={u.id} value={u.id}>
                        {u.username} — {u.email}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </SelectRoot>
                <Button onClick={handleAdd} disabled={!selectedUser || addMutation.isPending}>
                  <UserPlus className="h-4 w-4 mr-1" /> Add
                </Button>
              </div>
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
};
```

Notes for the implementer:
- The exact Select primitive names (`SelectRoot` / `SelectTrigger` / …) must match what `ShareDialog.tsx` already uses — open it first and copy the import line verbatim. If the codebase uses different aliases (e.g. plain `Select`), adjust.
- `useUsers` comes from `@/hooks/useAdmin` — grep `frontend/src/hooks/` to confirm the export name before relying on it.

- [ ] **Step 12.5: Create `frontend/src/pages/admin/Groups.tsx`**

Mirror `UserManagement.tsx` (table layout, CreateXDialog header, ConfirmDialog for destructive ops, error banner). Columns: Name | Description | Source (badge: native / oidc) | Member count | Created | Actions (Members, Edit, Delete, Toggle-Admin).

```tsx
import { useMemo, useState } from 'react';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Users, Trash2, Shield, ShieldOff, Pencil } from 'lucide-react';
import { ConfirmDialog } from '@/components/ConfirmDialog';
import { CreateGroupDialog } from '@/components/admin/CreateGroupDialog';
import { GroupMembersDialog } from '@/components/admin/GroupMembersDialog';
import { useGroups, useDeleteGroup } from '@/hooks/useGroups';
import { groupsApi } from '@/api/groups';
import type { GroupWithMemberCount } from '@/types/models';

export const Groups = () => {
  const { data: groups, isLoading } = useGroups();
  const deleteMutation = useDeleteGroup();
  const [confirm, setConfirm] = useState<{ id: string; name: string } | null>(null);
  const [membersOf, setMembersOf] = useState<GroupWithMemberCount | null>(null);
  const [error, setError] = useState('');

  const rows = useMemo(() => groups ?? [], [groups]);

  const handleDelete = async () => {
    if (!confirm) return;
    setError('');
    try {
      await deleteMutation.mutateAsync(confirm.id);
      setConfirm(null);
    } catch (err) {
      setError((err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? 'Failed to delete group');
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <div>
          <h1 className="text-3xl font-bold">Groups</h1>
          <p className="text-muted-foreground">
            Manage groups and grant permission to workspaces and registries.
          </p>
        </div>
        <CreateGroupDialog />
      </div>

      {error && (
        <div className="bg-red-500/10 border border-red-500/20 text-red-500 px-4 py-3 rounded">{error}</div>
      )}

      <Card>
        <CardContent className="p-0">
          {isLoading ? (
            <div className="p-8 text-center text-muted-foreground">Loading…</div>
          ) : rows.length === 0 ? (
            <div className="p-8 text-center text-muted-foreground">No groups yet.</div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full">
                <thead className="border-b bg-muted/50">
                  <tr>
                    <th className="text-left p-4 font-medium">Name</th>
                    <th className="text-left p-4 font-medium">Description</th>
                    <th className="text-left p-4 font-medium">Source</th>
                    <th className="text-left p-4 font-medium">Members</th>
                    <th className="text-left p-4 font-medium">Created</th>
                    <th className="text-right p-4 font-medium">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((g) => (
                    <tr key={g.id} className="border-b last:border-0 hover:bg-muted/50">
                      <td className="p-4 font-medium">{g.name}</td>
                      <td className="p-4 text-sm text-muted-foreground">{g.description}</td>
                      <td className="p-4">
                        <Badge variant="outline" className={g.source === 'oidc' ? 'border-blue-500/40 text-blue-500' : ''}>
                          {g.source}
                        </Badge>
                      </td>
                      <td className="p-4">{g.member_count}</td>
                      <td className="p-4 text-sm text-muted-foreground">
                        {new Date(g.created_at).toLocaleDateString()}
                      </td>
                      <td className="p-4">
                        <div className="flex justify-end gap-2">
                          <Button variant="ghost" size="sm" onClick={() => setMembersOf(g)}>
                            <Users className="h-4 w-4" />
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            disabled={g.source === 'oidc'}
                            onClick={() => setConfirm({ id: g.id, name: g.name })}
                          >
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>

      <ConfirmDialog
        open={!!confirm}
        onOpenChange={(o) => !o && setConfirm(null)}
        onConfirm={handleDelete}
        title="Delete group"
        description={`Delete group "${confirm?.name}"? Members lose all permissions granted via this group.`}
        confirmText="Delete"
        cancelText="Cancel"
        variant="destructive"
      />

      {membersOf && (
        <GroupMembersDialog
          group={membersOf}
          open={!!membersOf}
          onOpenChange={(o) => !o && setMembersOf(null)}
        />
      )}
    </div>
  );
};
```

Adjust the import path of `ConfirmDialog` to match what `UserManagement.tsx` uses.

- [ ] **Step 12.6: Add the route + nav link**

Edit `frontend/src/App.tsx`. Locate the existing admin route block:

```tsx
<Route element={<AdminRoute />}>
  <Route element={<AdminLayout />}>
    <Route path="admin" element={<AdminDashboard />} />
    <Route path="admin/users" element={<UserManagement />} />
    ...
  </Route>
</Route>
```

Add:

```tsx
    <Route path="admin/groups" element={<Groups />} />
```

(and import `Groups`).

In whichever component renders the admin sidebar/nav (identified in Step 12.1), add a Groups link next to Users. Match the existing link style exactly.

- [ ] **Step 12.7: Type-check + lint**

Run: `cd frontend && npx tsc -b && npm run lint`
Expected: no new errors. (Old Collaborator usages may still be broken — that's fine until Task 13.)

- [ ] **Step 12.8: Run the frontend dev server and click through**

```bash
cd frontend && npm run dev
```

Manually verify:
- Navigating to `/admin/groups` shows the empty state.
- "Create group" succeeds and adds a row.
- Member dialog opens, search shows users, add/remove works.
- Delete asks for confirmation, then removes the row.

- [ ] **Step 12.9: Commit**

```bash
git add frontend/src/hooks/useGroups.ts frontend/src/pages/admin/Groups.tsx frontend/src/components/admin/CreateGroupDialog.tsx frontend/src/components/admin/GroupMembersDialog.tsx frontend/src/App.tsx
git commit -m "frontend: admin Groups page with CRUD and member management"
```

(Add the admin layout file if you modified it.)

---

## Task 13: Frontend ShareDialog — User / Group tabs

**Files:**
- Modify: `frontend/src/components/sharing/ShareDialog.tsx`
- Modify: `frontend/src/components/sharing/ShareDialog.test.tsx` — add group tests
- Modify: `frontend/src/pages/WorkspaceDetail.tsx` — fix stale `collaborators?.length` counts (Task 11 narrowing filters groups out, so the count badges currently disagree with rendered rows once groups exist)
- Modify: `frontend/src/hooks/` — if no `useCollaborators` group mutation exists, add it (see Step 13.4)

**Stale counts to fix in `WorkspaceDetail.tsx`** (Task 11 introduced `userCollaborators` filter but left these reading the raw `collaborators?.length`):
- Line ~324: collaborators tab badge count
- Line ~329: sidebar collaborators heading count
- Line ~336: "+N more" math
- Line ~704: collaborators tab count

Replace each with either `userCollaborators?.length` (if the count should reflect only user rows) or with a combined "X users + Y groups" display.

- [ ] **Step 13.1: Read the current ShareDialog end-to-end**

Run: `cat /Users/tylerman/gh/nebi/frontend/src/components/sharing/ShareDialog.tsx`

You're extending the existing component, not rewriting it.

- [ ] **Step 13.2: Add a tab/segmented control switching between User and Group**

The codebase uses Tailwind + custom UI primitives. The simplest pattern is a two-button toggle (no Radix tabs needed):

```tsx
const [mode, setMode] = useState<'user' | 'group'>('user');
// inside the "Add Collaborator" section, before the user select:
<div className="flex gap-2">
  <Button variant={mode === 'user' ? 'default' : 'outline'} size="sm" onClick={() => setMode('user')}>User</Button>
  <Button variant={mode === 'group' ? 'default' : 'outline'} size="sm" onClick={() => setMode('group')}>Group</Button>
</div>
```

Then below the toggle render either the existing user dropdown or a group dropdown sourced from `useMyGroups()`.

- [ ] **Step 13.3: Add group-share + group-unshare mutations**

Either add to an existing hook file (e.g. `hooks/useWorkspaces.ts`) or define inline in `ShareDialog`:

```ts
const shareGroupMutation = useMutation({
  mutationFn: (data: { group_id: string; role: 'editor' | 'viewer' }) =>
    groupsApi.shareWorkspace(environmentId, data),
  onSuccess: () => qc.invalidateQueries({ queryKey: ['workspaces', environmentId, 'collaborators'] }),
});

const unshareGroupMutation = useMutation({
  mutationFn: (groupId: string) => groupsApi.unshareWorkspace(environmentId, groupId),
  onSuccess: () => qc.invalidateQueries({ queryKey: ['workspaces', environmentId, 'collaborators'] }),
});
```

(`qc` = `useQueryClient()`.)

- [ ] **Step 13.4: Render group collaborators in the "Current Access" list**

The collaborators query already includes groups (Task 4 extended the backend). Update the list rendering to switch on `c.kind`:

```tsx
{collaborators?.map((c) => (
  c.kind === 'user' ? (
    <CollaboratorRow
      key={`u-${c.user_id}`}
      title={c.username}
      subtitle={c.email}
      role={c.role}
      isOwner={c.is_owner}
      onRemove={() => setConfirmRemove({ kind: 'user', id: c.user_id, label: c.username })}
    />
  ) : (
    <CollaboratorRow
      key={`g-${c.group_id}`}
      title={c.name}
      subtitle={c.source === 'oidc' ? 'OIDC group' : 'Native group'}
      role={c.role}
      isOwner={false}
      onRemove={() => setConfirmRemove({ kind: 'group', id: c.group_id, label: c.name })}
    />
  )
))}
```

Extract a `CollaboratorRow` subcomponent if the row markup grows beyond a few lines.

`confirmRemove` becomes a discriminated union; the existing single-shape `{ userId, username }` becomes `{ kind: 'user' | 'group', id, label }`. Update `handleConfirmRemove` to call the right mutation by kind.

- [ ] **Step 13.5: Update existing ShareDialog tests to use the new collaborator shape**

The test file `frontend/src/components/sharing/ShareDialog.test.tsx` (read it first) imports mocks like `mockOwnerCollaborator` from `@/test/handlers`. Find those mocks (likely `frontend/src/test/handlers.ts`) and update their shape to `{ kind: 'user', ... }`. Add at least one mock group collaborator.

- [ ] **Step 13.6: Add new tests for group sharing**

Append to `ShareDialog.test.tsx`:

```ts
it('switches to group mode and shares with a group', async () => {
  server.use(
    http.get('/api/v1/groups/me', () => HttpResponse.json([{
      id: 'g-1', name: 'data-science', description: '', source: 'native',
      created_at: '', updated_at: '',
    }])),
    http.post('/api/v1/workspaces/:id/share-group', () => HttpResponse.json({}, { status: 201 })),
  );
  renderWithProviders(<ShareDialog {...defaultProps} />);
  await userEvent.click(screen.getByRole('button', { name: 'Group' }));
  // pick the group, pick a role, submit
  // … etc. Mirror the existing user-share test asserts.
});

it('renders a group collaborator with its source badge', async () => {
  server.use(
    http.get('/api/v1/workspaces/:id/collaborators', () =>
      HttpResponse.json([
        mockOwnerCollaborator,
        { kind: 'group', group_id: 'g-1', name: 'data-science', source: 'oidc', role: 'editor', is_owner: false },
      ]),
    ),
  );
  renderWithProviders(<ShareDialog {...defaultProps} />);
  await waitFor(() => screen.getByText('data-science'));
  expect(screen.getByText(/OIDC group/i)).toBeInTheDocument();
});
```

- [ ] **Step 13.7: Run frontend tests**

Run: `cd frontend && npm run test`
Expected: PASS (all existing tests + the two new ones).

- [ ] **Step 13.8: Commit**

```bash
git add frontend/src/components/sharing/ShareDialog.tsx frontend/src/components/sharing/ShareDialog.test.tsx frontend/src/test/handlers.ts
git commit -m "frontend: ShareDialog supports sharing with groups"
```

---

## Task 14: Frontend user-management — show user's groups

**Files:**
- Modify: `frontend/src/pages/admin/UserManagement.tsx`
- Optionally extend: `frontend/src/api/admin.ts` — add `getUserGroups(userId)`
- Backend prerequisite check: `internal/api/handlers/admin.go` — does `GET /admin/users/:id` already include groups? If not, add a small extension.

- [ ] **Step 14.1: Add `ListUserGroups` to the admin service + a route**

The existing `AdminService.GetUser` at `internal/service/admin.go:101` returns `*UserWithAdmin` — no groups. Rather than mutate that shape (which would ripple through the existing UserManagement page), expose a sibling endpoint `GET /admin/users/:id/groups` that returns `[]models.Group`. This is the smallest possible change.

Add to `internal/service/admin.go`:

```go
// ListUserGroups returns every group the given user belongs to (native + OIDC).
func (s *AdminService) ListUserGroups(userID uuid.UUID) ([]models.Group, error) {
	var groups []models.Group
	err := s.db.
		Joins("JOIN group_members gm ON gm.group_id = groups.id").
		Where("gm.user_id = ?", userID).
		Find(&groups).Error
	if err != nil {
		return nil, fmt.Errorf("list user groups: %w", err)
	}
	return groups, nil
}
```

Add the handler in `internal/api/handlers/admin.go`:

```go
// ListUserGroups returns the groups a user belongs to. Admin-only.
// @Router /admin/users/{id}/groups [get]
func (h *AdminHandler) ListUserGroups(c *gin.Context) {
	uid, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid user ID"})
		return
	}
	groups, err := h.svc.ListUserGroups(uid)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, groups)
}
```

Wire the route in `internal/api/router.go` inside the admin group:

```go
		admin.GET("/users/:id/groups", adminHandler.ListUserGroups)
```

Add a service-level test alongside the existing admin tests:

```go
func TestListUserGroups_ReturnsOnlyTheirs(t *testing.T) {
	svc, _, db := adminTestSetup(t)
	groupSvc := NewGroupService(db, rbac.NewDefaultProvider())
	admin := createTestUser(t, db, "admin")
	alice := createTestUser(t, db, "alice")
	bob := createTestUser(t, db, "bob")

	g, _ := groupSvc.CreateGroup(CreateGroupRequest{Name: "ds"}, admin)
	_ = groupSvc.AddMember(g.ID, alice, admin)

	aliceGroups, err := svc.ListUserGroups(alice)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(aliceGroups) != 1 || aliceGroups[0].ID != g.ID {
		t.Fatalf("expected alice in 1 group %s, got %+v", g.ID, aliceGroups)
	}

	bobGroups, _ := svc.ListUserGroups(bob)
	if len(bobGroups) != 0 {
		t.Fatalf("expected bob in 0 groups, got %d", len(bobGroups))
	}
}
```

Run: `go test -v -run TestListUserGroups ./internal/service/`
Expected: PASS.

- [ ] **Step 14.2: Render a "Groups" column in `UserManagement.tsx`**

Add a new column between Role and Created showing the comma-separated group names or a chip list. Use the existing `Badge` primitive.

- [ ] **Step 14.3: Type-check + run frontend tests**

Run: `cd frontend && npx tsc -b && npm run lint && npm run test`
Expected: PASS.

- [ ] **Step 14.4: Commit**

```bash
git add internal/api/handlers/admin.go internal/service/admin.go frontend/src/pages/admin/UserManagement.tsx frontend/src/api/admin.ts
git commit -m "frontend: surface user's groups in admin user-management"
```

---

## Task 15: End-to-end manual walkthrough

**Files:** none.

- [ ] **Step 15.1: Reset to a clean dev DB and start the server**

```bash
rm -f nebi.db
make build
./bin/nebi serve --config config.yaml.example &
SERVER_PID=$!
cd frontend && npm run dev &
DEV_PID=$!
```

- [ ] **Step 15.2: Native flow**

1. Log in as the seed admin (or bootstrap one).
2. `/admin/groups`: create `data-science` with two members.
3. Open a workspace as the owner who is a member of `data-science`.
4. Use the ShareDialog → Group tab → share with `data-science` as Viewer.
5. Verify Collaborators list shows the group with a "native" badge.
6. Log in as one of the group members (different browser/incognito) and confirm the shared workspace appears.

- [ ] **Step 15.3: OIDC flow**

If a Keycloak/Dex sandbox is wired up in `docker-compose.dev.yml`, restart the server in OIDC mode and log in. Verify:
- The `groups` claim from the IdP creates rows in `groups` with `source=oidc`.
- A second login that drops a group removes the membership but keeps the group row.
- The admin Groups page renders OIDC groups read-only.

If no OIDC sandbox exists, document the manual claim payload you used to drive `ExchangeIDToken` (proxy header path) and call it out in the PR description.

- [ ] **Step 15.4: Stop everything**

```bash
kill $DEV_PID $SERVER_PID
```

- [ ] **Step 15.5: If issues found, fix and commit per task**

(One commit per logical fix; no batch "fix walkthrough issues" commit.)

---

## Task 16: Documentation + PR

**Files:**
- Modify: `docs/docs/ui.md` — short note on the new Groups admin page
- Modify: `docs/docs/server-setup.md` — note the `groups` scope is requested at OIDC login

- [ ] **Step 16.1: Add UI docs**

In `docs/docs/ui.md`, add a short Groups section under the Admin docs (mirror the User-management section).

- [ ] **Step 16.2: Note OIDC scope change**

In `docs/docs/server-setup.md`, mention that nebi now requests the `groups` scope, that the IdP must return a `groups` claim in the ID token, and that group reconciliation happens on every login.

- [ ] **Step 16.3: Sweep Swagger godoc blocks on new handlers**

Tasks 6 and 7 added handler methods with only minimal `@Router` annotations. Other handlers in the package (e.g. `workspace.go`'s `ShareWorkspace`, `admin.go`'s `CreateUser`) use full godoc blocks: `@Summary`, `@Tags`, `@Security BearerAuth`, `@Accept json`, `@Produce json`, `@Param`, `@Success`, `@Failure`. Bring the new handlers up to parity so generated Swagger UI shows useful tags + schemas.

Files to update (each function in each file):
- `internal/api/handlers/group.go` — 9 methods.
- `internal/api/handlers/workspace.go` — `ShareWorkspaceWithGroup`, `UnshareWorkspaceWithGroup`.
- `internal/api/handlers/registry.go` — `GrantRegistryToGroup`, `RevokeRegistryFromGroup`.
- `internal/api/handlers/admin.go` — `GrantGroupAdmin`, `RevokeGroupAdmin`, `ListUserGroups`.

Use `internal/api/handlers/workspace.go::ShareWorkspace` (around line 406) as the template. Then run `make swagger` (or whatever the project's regen target is) to refresh `internal/swagger/docs.go`. Commit with `docs(swagger): annotate group handlers and regenerate`.

- [ ] **Step 16.4: Commit docs**

```bash
git add docs/docs/ui.md docs/docs/server-setup.md
git commit -m "docs: document groups feature and OIDC scope"
```

- [ ] **Step 16.5: Open the PR**

```bash
git push -u origin <branch>
gh pr create --title "Groups: share workspaces and registries with groups (native + OIDC)" --body "$(cat <<'EOF'
## Summary

Closes https://github.com/nebari-dev/nebi/issues/293.

- New `Group` primitive with native (admin-managed) and OIDC-synced sources.
- Workspace share, registry access, and the admin role can now be granted to groups.
- Casbin matcher switched from `r.sub == p.sub` to `g(r.sub, p.sub)` so existing direct policies still match and group memberships resolve transitively.
- OIDC `groups` claim drives JIT membership reconciliation on every login.
- Admin Groups page, ShareDialog gains a User/Group tabbed selector, user-management shows each user's groups.

## Test plan
- [ ] `make test` passes.
- [ ] `cd frontend && npm run test && npm run lint && npx tsc -b` passes.
- [ ] Manual: native group sharing flow described in Task 15.
- [ ] Manual: OIDC sync — confirm a login that drops a claim removes the membership.
EOF
)"
```

---

## Self-review notes (author's pre-handoff check)

1. **Matcher change is the most fragile step.** If Task 1 fails the full suite, the `g(r.sub, p.sub)` change is the suspect — verify the existing per-user policies still match in `TestDirectUserPolicyStillWorksAfterMatcherChange`. Casbin treats `g(x, x)` as true by default, so this should work, but pin it down before moving on.
2. **`workspace_test.go` migration list is hand-maintained.** Step 2.5 keeps it in sync with `db.Migrate`. If you add new models in the future, remember both places.
3. **OIDC sync is best-effort by design** (Step 9.4 — log and continue). If your team prefers strict-failure, change the policy and add a test.
4. **`ShareWorkspaceWithGroup` resolves group membership via Casbin grouping rules** (`s.rbac.GetUserGroups`), so it does not need a `*GroupService` dependency. The handler stays slim — no extra service to inject.
5. **Frontend `Collaborator` type becomes a discriminated union.** Every consumer must branch on `kind`. The TypeScript compiler will surface every site that needs updating.
6. **Casbin `SavePolicy` rewrites the whole policy table on every helper call.** That mirrors the pre-existing pattern in `rbac.go` and is fine for the volumes involved. If profiling shows it becomes a bottleneck, switch the rbac module to `enforcer.AddPolicy` + per-call adapter writes (a separate refactor).
