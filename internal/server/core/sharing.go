package core

// Owner: U6 (sharing). Roles map onto the existing RBAC roles:
// owner -> project owner, edit -> "editor", view -> "viewer".
//
// Authorization (team mode only; local mode never reaches core):
//   - search: any authenticated user. Users match by username or email;
//     teams match by name, and non-admins only see the teams they belong
//     to (the same rule ProjectService.ShareProjectWithGroup enforces).
//   - list access: anyone who can read the project, the owner, or an admin.
//   - grant / change role / remove / delete from server: the project owner
//     or an admin. Non-admin owners can only share with their own teams.
//   - the owner's own access can never be changed or removed (409
//     "last_owner"): a project has exactly one owner.

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/audit"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/service"
	"gorm.io/gorm"
)

// Share roles on the wire.
const (
	RoleOwner = "owner"
	RoleEdit  = "edit"
	RoleView  = "view"
)

// Principal kinds on the wire.
const (
	PrincipalUser  = "user"
	PrincipalGroup = "group"
)

// maxPrincipalResults caps a principal search.
const maxPrincipalResults = 20

// rbacRole maps a share role to the RBAC role name.
func rbacRole(role string) (string, error) {
	switch role {
	case RoleEdit:
		return "editor", nil
	case RoleView:
		return "viewer", nil
	}
	return "", &ValidationError{Field: "role", Message: "Role must be 'edit' or 'view'"}
}

// shareRole maps an RBAC role name to a share role.
func shareRole(name string) string {
	switch name {
	case "viewer":
		return RoleView
	case "owner":
		return RoleOwner
	default:
		return RoleEdit
	}
}

// likePattern builds a case-insensitive LIKE pattern for a search query.
func likePattern(q string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(strings.ToLower(q)) + "%"
}

// SearchPrincipals finds users (username, email) and teams (name) for the
// share dialog. An empty query returns nothing (no directory listing).
func (s *Service) SearchPrincipals(ctx context.Context, callerID uuid.UUID, query string) ([]Principal, error) {
	q := strings.TrimSpace(query)
	out := []Principal{}
	if q == "" {
		return out, nil
	}
	db := s.deps.DB.WithContext(ctx)
	pattern := likePattern(q)

	var users []models.User
	if err := db.Where(`LOWER(username) LIKE ? ESCAPE '\' OR LOWER(email) LIKE ? ESCAPE '\'`, pattern, pattern).
		Order("username").Limit(maxPrincipalResults).Find(&users).Error; err != nil {
		return nil, fmt.Errorf("search users: %w", err)
	}
	for _, u := range users {
		out = append(out, Principal{Kind: PrincipalUser, ID: u.ID.String(), Name: u.Username, Email: u.Email})
	}

	if len(out) >= maxPrincipalResults {
		return out[:maxPrincipalResults], nil
	}

	isAdmin, err := s.isAdmin(callerID)
	if err != nil {
		return nil, err
	}
	gq := db.Where(`LOWER(name) LIKE ? ESCAPE '\'`, pattern)
	if !isAdmin {
		mine, err := s.deps.RBAC.GetUserGroups(callerID)
		if err != nil {
			return nil, fmt.Errorf("list caller teams: %w", err)
		}
		if len(mine) == 0 {
			return out, nil
		}
		gq = gq.Where("id IN ?", mine)
	}
	var groups []models.Group
	if err := gq.Order("name").Limit(maxPrincipalResults - len(out)).Find(&groups).Error; err != nil {
		return nil, fmt.Errorf("search teams: %w", err)
	}
	counts, err := s.memberCounts(ctx, groups)
	if err != nil {
		return nil, err
	}
	for _, g := range groups {
		out = append(out, Principal{Kind: PrincipalGroup, ID: g.ID.String(), Name: g.Name, Members: counts[g.ID]})
	}
	return out, nil
}

// ListAccess returns who has access; CanManage is true for owners/admins.
func (s *Service) ListAccess(ctx context.Context, callerID, projectID uuid.UUID) (*AccessList, error) {
	project, err := s.loadProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	canManage, err := s.canManage(callerID, project)
	if err != nil {
		return nil, err
	}
	if !canManage {
		ok, err := s.deps.RBAC.CanReadProject(callerID, projectID)
		if err != nil {
			return nil, fmt.Errorf("check read access: %w", err)
		}
		if !ok {
			return nil, forbidden("You do not have access to this project")
		}
	}
	return s.accessList(ctx, project, canManage)
}

// GrantAccess adds principals with a role (owners/admins only). A principal
// that already has access gets the new role, so a grant is idempotent.
func (s *Service) GrantAccess(ctx context.Context, callerID, projectID uuid.UUID, req GrantAccessRequest) (*AccessList, error) {
	project, isAdmin, err := s.authorizeManage(ctx, callerID, projectID)
	if err != nil {
		return nil, err
	}
	role, err := rbacRole(req.Role)
	if err != nil {
		return nil, err
	}
	if len(req.Principals) == 0 {
		return nil, &ValidationError{Field: "principals", Message: "Add at least one person or team"}
	}
	roleRec, err := s.roleRecord(ctx, role)
	if err != nil {
		return nil, err
	}

	type target struct {
		kind string
		id   uuid.UUID
	}
	targets := make([]target, 0, len(req.Principals))
	seen := map[target]bool{}
	var callerGroups map[uuid.UUID]bool
	for _, p := range req.Principals {
		kind, id, err := parsePrincipal(p)
		if err != nil {
			return nil, err
		}
		t := target{kind, id}
		if seen[t] {
			continue
		}
		seen[t] = true
		switch kind {
		case PrincipalUser:
			if id == project.OwnerID {
				return nil, &ValidationError{Field: "principals", Message: "The owner already has access to this project"}
			}
			if err := s.mustExist(ctx, &models.User{}, id, "User not found"); err != nil {
				return nil, err
			}
		case PrincipalGroup:
			if err := s.mustExist(ctx, &models.Group{}, id, "Team not found"); err != nil {
				return nil, err
			}
			if !isAdmin {
				if callerGroups == nil {
					callerGroups, err = s.groupSet(callerID)
					if err != nil {
						return nil, err
					}
				}
				if !callerGroups[id] {
					return nil, forbidden("You can only share with teams you belong to")
				}
			}
		}
		targets = append(targets, t)
	}

	err = s.deps.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, t := range targets {
			if err := upsertGrant(tx, t.kind, t.id, projectID, roleRec.ID); err != nil {
				return err
			}
			logGrant(tx, callerID, projectID, t.kind, t.id, role)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, t := range targets {
		if err := s.setPolicy(t.kind, t.id, projectID, role); err != nil {
			return nil, err
		}
	}
	return s.accessList(ctx, project, true)
}

// UpdateAccess changes a principal's role (owners/admins only).
func (s *Service) UpdateAccess(ctx context.Context, callerID, projectID uuid.UUID, p PrincipalRef, req UpdateAccessRequest) (*AccessList, error) {
	project, _, err := s.authorizeManage(ctx, callerID, projectID)
	if err != nil {
		return nil, err
	}
	kind, id, err := parsePrincipal(p)
	if err != nil {
		return nil, err
	}
	if kind == PrincipalUser && id == project.OwnerID {
		return nil, &ConflictError{Code: "last_owner", Message: "The owner's access can't be changed"}
	}
	role, err := rbacRole(req.Role)
	if err != nil {
		return nil, err
	}
	roleRec, err := s.roleRecord(ctx, role)
	if err != nil {
		return nil, err
	}
	err = s.deps.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(grantModel(kind)).Where(grantColumn(kind)+" = ? AND project_id = ?", id, projectID).Update("role_id", roleRec.ID)
		if res.Error != nil {
			return fmt.Errorf("update access: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			return notFound("This " + principalNoun(kind) + " has no access to change")
		}
		logGrant(tx, callerID, projectID, kind, id, role)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := s.setPolicy(kind, id, projectID, role); err != nil {
		return nil, err
	}
	return s.accessList(ctx, project, true)
}

// RevokeAccess removes a principal (owners/admins only).
func (s *Service) RevokeAccess(ctx context.Context, callerID, projectID uuid.UUID, p PrincipalRef) (*AccessList, error) {
	project, _, err := s.authorizeManage(ctx, callerID, projectID)
	if err != nil {
		return nil, err
	}
	kind, id, err := parsePrincipal(p)
	if err != nil {
		return nil, err
	}
	if kind == PrincipalUser && id == project.OwnerID {
		return nil, &ConflictError{Code: "last_owner", Message: "The owner can't be removed from the project"}
	}
	err = s.deps.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Where(grantColumn(kind)+" = ? AND project_id = ?", id, projectID).Delete(grantModel(kind))
		if res.Error != nil {
			return fmt.Errorf("remove access: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			return notFound("This " + principalNoun(kind) + " has no access to the project")
		}
		action, key := audit.ActionRevokePermission, "target_user_id"
		if kind == PrincipalGroup {
			action, key = audit.ActionRevokeGroupPerm, "group_id"
		}
		audit.LogAction(tx, callerID, action, projectResource(projectID), map[string]interface{}{key: id})
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := s.revokePolicy(kind, id, projectID); err != nil {
		return nil, err
	}
	return s.accessList(ctx, project, true)
}

// DeleteProject deletes a project from the server (owners/admins only; the
// legacy DELETE /projects/:id lets any editor do it). The removal runs as a
// server job, like the legacy route.
func (s *Service) DeleteProject(ctx context.Context, callerID, projectID uuid.UUID) error {
	if _, _, err := s.authorizeManageWith(ctx, callerID, projectID, "Only the project owner or an admin can delete it from the server"); err != nil {
		return err
	}
	if s.deps.Projects == nil {
		return errors.New("project service unavailable")
	}
	err := s.deps.Projects.Delete(ctx, projectID.String(), callerID)
	var verr *service.ValidationError
	var cerr *service.ConflictError
	var ferr *service.ForbiddenError
	switch {
	case err == nil:
		return nil
	case errors.Is(err, service.ErrNotFound):
		return ErrNotFound
	case errors.As(err, &verr):
		return &ValidationError{Message: verr.Message}
	case errors.As(err, &cerr):
		return &ConflictError{Code: "job_active", Message: cerr.Message}
	case errors.As(err, &ferr):
		return forbidden(ferr.Message)
	}
	return err
}

// ---------------------------------------------------------------- helpers

// messageError carries a user-facing message and unwraps to a core
// sentinel, so writeError maps it while the body keeps a clean message.
type messageError struct {
	msg      string
	sentinel error
}

func (e *messageError) Error() string { return e.msg }
func (e *messageError) Unwrap() error { return e.sentinel }

func forbidden(msg string) error { return &messageError{msg: msg, sentinel: ErrForbidden} }
func notFound(msg string) error  { return &messageError{msg: msg, sentinel: ErrNotFound} }

func (s *Service) loadProject(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	var p models.Project
	if err := s.deps.DB.WithContext(ctx).First(&p, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &p, nil
}

func (s *Service) isAdmin(userID uuid.UUID) (bool, error) {
	ok, err := s.deps.RBAC.IsAdmin(userID)
	if err != nil {
		return false, fmt.Errorf("check admin: %w", err)
	}
	return ok, nil
}

func (s *Service) canManage(callerID uuid.UUID, p *models.Project) (bool, error) {
	if callerID != uuid.Nil && p.OwnerID == callerID {
		return true, nil
	}
	return s.isAdmin(callerID)
}

// authorizeManage loads the project and requires the owner or an admin.
func (s *Service) authorizeManage(ctx context.Context, callerID, projectID uuid.UUID) (*models.Project, bool, error) {
	return s.authorizeManageWith(ctx, callerID, projectID, "Only the project owner or an admin can manage access")
}

func (s *Service) authorizeManageWith(ctx context.Context, callerID, projectID uuid.UUID, denied string) (*models.Project, bool, error) {
	if callerID == uuid.Nil {
		return nil, false, forbidden(denied)
	}
	project, err := s.loadProject(ctx, projectID)
	if err != nil {
		return nil, false, err
	}
	isAdmin, err := s.isAdmin(callerID)
	if err != nil {
		return nil, false, err
	}
	if !isAdmin && project.OwnerID != callerID {
		return nil, false, forbidden(denied)
	}
	return project, isAdmin, nil
}

func (s *Service) groupSet(userID uuid.UUID) (map[uuid.UUID]bool, error) {
	ids, err := s.deps.RBAC.GetUserGroups(userID)
	if err != nil {
		return nil, fmt.Errorf("list caller teams: %w", err)
	}
	set := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return set, nil
}

func (s *Service) roleRecord(ctx context.Context, name string) (*models.Role, error) {
	var r models.Role
	if err := s.deps.DB.WithContext(ctx).Where("name = ?", name).First(&r).Error; err != nil {
		return nil, fmt.Errorf("role %q not found: %w", name, err)
	}
	return &r, nil
}

func (s *Service) mustExist(ctx context.Context, model any, id uuid.UUID, msg string) error {
	var n int64
	if err := s.deps.DB.WithContext(ctx).Model(model).Where("id = ?", id).Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return &ValidationError{Field: "principals", Message: msg}
	}
	return nil
}

func (s *Service) memberCounts(ctx context.Context, groups []models.Group) (map[uuid.UUID]int, error) {
	counts := map[uuid.UUID]int{}
	if len(groups) == 0 {
		return counts, nil
	}
	ids := make([]uuid.UUID, len(groups))
	for i, g := range groups {
		ids[i] = g.ID
	}
	var rows []struct {
		GroupID uuid.UUID
		N       int
	}
	if err := s.deps.DB.WithContext(ctx).Model(&models.GroupMember{}).
		Select("group_id, COUNT(*) AS n").Where("group_id IN ?", ids).Group("group_id").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("count team members: %w", err)
	}
	for _, r := range rows {
		counts[r.GroupID] = r.N
	}
	return counts, nil
}

// accessList builds "Who has access": the owner first, then everyone else
// by name (so an undo puts a row back where it was).
func (s *Service) accessList(ctx context.Context, project *models.Project, canManage bool) (*AccessList, error) {
	db := s.deps.DB.WithContext(ctx)
	entries := []AccessEntry{}

	var owner models.User
	if err := db.First(&owner, "id = ?", project.OwnerID).Error; err == nil {
		entries = append(entries, AccessEntry{
			Principal: Principal{Kind: PrincipalUser, ID: owner.ID.String(), Name: owner.Username, Email: owner.Email},
			Role:      RoleOwner,
		})
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("load owner: %w", err)
	}

	var rest []AccessEntry
	seen := map[string]bool{}

	var perms []models.Permission
	if err := db.Preload("User").Preload("Role").Where("project_id = ?", project.ID).Find(&perms).Error; err != nil {
		return nil, fmt.Errorf("load user access: %w", err)
	}
	for _, p := range perms {
		if p.UserID == project.OwnerID || p.User.ID == uuid.Nil || seen["u"+p.UserID.String()] {
			continue
		}
		seen["u"+p.UserID.String()] = true
		role := shareRole(p.Role.Name)
		if role == RoleOwner {
			role = RoleEdit
		}
		rest = append(rest, AccessEntry{
			Principal: Principal{Kind: PrincipalUser, ID: p.UserID.String(), Name: p.User.Username, Email: p.User.Email},
			Role:      role,
		})
	}

	var gperms []models.GroupPermission
	if err := db.Preload("Group").Preload("Role").Where("project_id = ?", project.ID).Find(&gperms).Error; err != nil {
		return nil, fmt.Errorf("load team access: %w", err)
	}
	var groups []models.Group
	for _, gp := range gperms {
		if gp.Group.ID != uuid.Nil {
			groups = append(groups, gp.Group)
		}
	}
	counts, err := s.memberCounts(ctx, groups)
	if err != nil {
		return nil, err
	}
	for _, gp := range gperms {
		if gp.Group.ID == uuid.Nil || seen["g"+gp.GroupID.String()] {
			continue
		}
		seen["g"+gp.GroupID.String()] = true
		role := shareRole(gp.Role.Name)
		if role == RoleOwner {
			role = RoleEdit
		}
		rest = append(rest, AccessEntry{
			Principal: Principal{Kind: PrincipalGroup, ID: gp.GroupID.String(), Name: gp.Group.Name, Members: counts[gp.GroupID]},
			Role:      role,
		})
	}

	sort.SliceStable(rest, func(i, j int) bool {
		a, b := strings.ToLower(rest[i].Principal.Name), strings.ToLower(rest[j].Principal.Name)
		if a != b {
			return a < b
		}
		return rest[i].Principal.Kind > rest[j].Principal.Kind // users before teams
	})
	return &AccessList{Entries: append(entries, rest...), CanManage: canManage}, nil
}

func parsePrincipal(p PrincipalRef) (string, uuid.UUID, error) {
	if p.Kind != PrincipalUser && p.Kind != PrincipalGroup {
		return "", uuid.Nil, &ValidationError{Field: "kind", Message: "Kind must be 'user' or 'group'"}
	}
	id, err := uuid.Parse(p.ID)
	if err != nil {
		return "", uuid.Nil, &ValidationError{Field: "principal_id", Message: "Invalid " + principalNoun(p.Kind) + " ID"}
	}
	return p.Kind, id, nil
}

func principalNoun(kind string) string {
	if kind == PrincipalGroup {
		return "team"
	}
	return "user"
}

func grantModel(kind string) any {
	if kind == PrincipalGroup {
		return &models.GroupPermission{}
	}
	return &models.Permission{}
}

func grantColumn(kind string) string {
	if kind == PrincipalGroup {
		return "group_id"
	}
	return "user_id"
}

func projectResource(id uuid.UUID) string { return "project:" + id.String() }

// upsertGrant sets the role of an existing grant or creates one.
func upsertGrant(tx *gorm.DB, kind string, id, projectID uuid.UUID, roleID uint) error {
	res := tx.Model(grantModel(kind)).Where(grantColumn(kind)+" = ? AND project_id = ?", id, projectID).Update("role_id", roleID)
	if res.Error != nil {
		return fmt.Errorf("update access: %w", res.Error)
	}
	if res.RowsAffected > 0 {
		return nil
	}
	var err error
	if kind == PrincipalGroup {
		err = tx.Create(&models.GroupPermission{GroupID: id, ProjectID: projectID, RoleID: roleID}).Error
	} else {
		err = tx.Create(&models.Permission{UserID: id, ProjectID: projectID, RoleID: roleID}).Error
	}
	if err != nil {
		return fmt.Errorf("grant access: %w", err)
	}
	return nil
}

func logGrant(tx *gorm.DB, callerID, projectID uuid.UUID, kind string, id uuid.UUID, role string) {
	if kind == PrincipalGroup {
		audit.LogAction(tx, callerID, audit.ActionGrantGroupPerm, projectResource(projectID), map[string]interface{}{"group_id": id, "role": role})
		return
	}
	audit.LogAction(tx, callerID, audit.ActionGrantPermission, projectResource(projectID), map[string]interface{}{"target_user_id": id, "role": role})
}

// setPolicy replaces the principal's casbin policy on the project. Casbin
// writes outside the DB transaction (it has its own adapter), like the
// legacy share functions.
func (s *Service) setPolicy(kind string, id, projectID uuid.UUID, role string) error {
	if err := s.revokePolicy(kind, id, projectID); err != nil {
		return err
	}
	var err error
	if kind == PrincipalGroup {
		err = s.deps.RBAC.GrantGroupProjectAccess(id, projectID, role)
	} else {
		err = s.deps.RBAC.GrantProjectAccess(id, projectID, role)
	}
	if err != nil {
		return fmt.Errorf("grant RBAC permission: %w", err)
	}
	return nil
}

func (s *Service) revokePolicy(kind string, id, projectID uuid.UUID) error {
	var err error
	if kind == PrincipalGroup {
		err = s.deps.RBAC.RevokeGroupProjectAccess(id, projectID)
	} else {
		err = s.deps.RBAC.RevokeProjectAccess(id, projectID)
	}
	if err != nil {
		return fmt.Errorf("revoke RBAC permission: %w", err)
	}
	return nil
}
