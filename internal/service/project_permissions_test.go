package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/rbac"
)

// --- ShareProject tests ---

func TestShareProject_GrantsAccess(t *testing.T) {
	svc, db := testSetup(t, false) // team mode
	alice := createTestUser(t, db, "alice")
	bob := createTestUser(t, db, "bob")
	project := createReadyProject(t, svc, db, "share-test", alice)

	// Seed roles
	db.Create(&models.Role{Name: "viewer", Description: "read-only"})
	db.Create(&models.Role{Name: "editor", Description: "read-write"})

	perm, err := svc.ShareProject(project.ID.String(), alice, bob, "editor")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if perm.UserID != bob {
		t.Errorf("expected user ID %s, got %s", bob, perm.UserID)
	}
	if perm.ProjectID != project.ID {
		t.Errorf("expected project ID %s, got %s", project.ID, perm.ProjectID)
	}

	// Verify permission record in DB
	var dbPerm models.Permission
	if err := db.Where("user_id = ? AND project_id = ?", bob, project.ID).First(&dbPerm).Error; err != nil {
		t.Fatalf("permission not found in DB: %v", err)
	}

	// Verify audit log
	var auditCount int64
	db.Model(&models.AuditLog{}).Where("user_id = ? AND action = ?", alice, "grant_permission").Count(&auditCount)
	if auditCount != 1 {
		t.Errorf("expected 1 audit log, got %d", auditCount)
	}
}

func TestShareProject_RejectsNonOwner(t *testing.T) {
	svc, db := testSetup(t, false)
	alice := createTestUser(t, db, "alice")
	bob := createTestUser(t, db, "bob")
	charlie := createTestUser(t, db, "charlie")
	project := createReadyProject(t, svc, db, "share-test", alice)

	db.Create(&models.Role{Name: "viewer"})

	// Bob is not the owner — should be forbidden
	_, err := svc.ShareProject(project.ID.String(), bob, charlie, "viewer")
	if err == nil {
		t.Fatal("expected error for non-owner share")
	}
	var fe *ForbiddenError
	if !isForbiddenError(err, &fe) {
		t.Fatalf("expected ForbiddenError, got %T: %v", err, err)
	}
}

func TestShareProject_RejectsInvalidRole(t *testing.T) {
	svc, db := testSetup(t, false)
	alice := createTestUser(t, db, "alice")
	bob := createTestUser(t, db, "bob")
	project := createReadyProject(t, svc, db, "share-test", alice)

	_, err := svc.ShareProject(project.ID.String(), alice, bob, "superadmin")
	if err == nil {
		t.Fatal("expected error for invalid role")
	}
	var ve *ValidationError
	if !isValidationError(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
}

func TestShareProject_RejectsNonExistentUser(t *testing.T) {
	svc, db := testSetup(t, false)
	alice := createTestUser(t, db, "alice")
	project := createReadyProject(t, svc, db, "share-test", alice)

	_, err := svc.ShareProject(project.ID.String(), alice, uuid.New(), "viewer")
	if err == nil {
		t.Fatal("expected error for non-existent user")
	}
}

func TestShareProject_NotFoundProject(t *testing.T) {
	svc, db := testSetup(t, false)
	alice := createTestUser(t, db, "alice")

	_, err := svc.ShareProject(uuid.New().String(), alice, uuid.New(), "viewer")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// --- UnshareProject tests ---

func TestUnshareProject_RevokesAccess(t *testing.T) {
	svc, db := testSetup(t, false)
	alice := createTestUser(t, db, "alice")
	bob := createTestUser(t, db, "bob")
	project := createReadyProject(t, svc, db, "unshare-test", alice)

	db.Create(&models.Role{Name: "viewer"})

	// Share first
	svc.ShareProject(project.ID.String(), alice, bob, "viewer")

	// Unshare
	err := svc.UnshareProject(project.ID.String(), alice, bob)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify permission removed from DB
	var count int64
	db.Model(&models.Permission{}).Where("user_id = ? AND project_id = ?", bob, project.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected permission to be deleted, got %d", count)
	}

	// Verify audit log
	var auditCount int64
	db.Model(&models.AuditLog{}).Where("user_id = ? AND action = ?", alice, "revoke_permission").Count(&auditCount)
	if auditCount != 1 {
		t.Errorf("expected 1 revoke audit log, got %d", auditCount)
	}
}

func TestUnshareProject_RejectsNonOwner(t *testing.T) {
	svc, db := testSetup(t, false)
	alice := createTestUser(t, db, "alice")
	bob := createTestUser(t, db, "bob")
	charlie := createTestUser(t, db, "charlie")
	project := createReadyProject(t, svc, db, "unshare-test", alice)

	err := svc.UnshareProject(project.ID.String(), bob, charlie)
	var fe *ForbiddenError
	if !isForbiddenError(err, &fe) {
		t.Fatalf("expected ForbiddenError, got %T: %v", err, err)
	}
}

func TestUnshareProject_CannotRemoveOwner(t *testing.T) {
	svc, db := testSetup(t, false)
	alice := createTestUser(t, db, "alice")
	project := createReadyProject(t, svc, db, "unshare-test", alice)

	err := svc.UnshareProject(project.ID.String(), alice, alice)
	if err == nil {
		t.Fatal("expected error when removing owner's access")
	}
	var ve *ValidationError
	if !isValidationError(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
}

// --- ListCollaborators tests ---

func TestListCollaborators_OwnerOnly(t *testing.T) {
	svc, db := testSetup(t, false)
	alice := createTestUser(t, db, "alice")
	project := createReadyProject(t, svc, db, "collab-test", alice)

	collabs, err := svc.ListCollaborators(project.ID.String())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(collabs) != 1 {
		t.Fatalf("expected 1 collaborator (owner), got %d", len(collabs))
	}
	if collabs[0].Username != "alice" {
		t.Errorf("expected alice, got %q", collabs[0].Username)
	}
	if collabs[0].Role != "owner" {
		t.Errorf("expected role=owner, got %q", collabs[0].Role)
	}
	if !collabs[0].IsOwner {
		t.Error("expected IsOwner=true")
	}
}

func TestListCollaborators_WithSharedUsers(t *testing.T) {
	svc, db := testSetup(t, false)
	alice := createTestUser(t, db, "alice")
	bob := createTestUser(t, db, "bob")
	project := createReadyProject(t, svc, db, "collab-test", alice)

	db.Create(&models.Role{Name: "editor"})
	svc.ShareProject(project.ID.String(), alice, bob, "editor")

	collabs, err := svc.ListCollaborators(project.ID.String())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(collabs) != 2 {
		t.Fatalf("expected 2 collaborators, got %d", len(collabs))
	}

	// Owner should be first
	if collabs[0].Username != "alice" || !collabs[0].IsOwner {
		t.Errorf("expected alice as owner first, got %+v", collabs[0])
	}
	if collabs[1].Username != "bob" || collabs[1].Role != "editor" {
		t.Errorf("expected bob as editor, got %+v", collabs[1])
	}
}

func TestListCollaborators_NotFound(t *testing.T) {
	svc, _ := testSetup(t, false)

	_, err := svc.ListCollaborators(uuid.New().String())
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// --- helper ---

func isForbiddenError(err error, target **ForbiddenError) bool {
	fe, ok := err.(*ForbiddenError)
	if ok && target != nil {
		*target = fe
	}
	return ok
}

// --- ShareProjectWithGroup / UnshareProjectFromGroup tests ---

func TestShareProjectWithGroup_GrantsTransitiveAccess(t *testing.T) {
	svc, db := testSetup(t, false)
	groupSvc := NewGroupService(db, rbac.NewDefaultProvider())

	alice := createTestUser(t, db, "alice")
	bob := createTestUser(t, db, "bob")
	project := createReadyProject(t, svc, db, "share-grp", alice)

	db.Create(&models.Role{Name: "viewer", Description: "read"})
	db.Create(&models.Role{Name: "editor", Description: "write"})

	g, _ := groupSvc.CreateGroup(CreateGroupRequest{Name: "team"}, alice)
	// alice (owner) must be a member of the group to share with it.
	_ = groupSvc.AddMember(g.ID, alice, alice)
	_ = groupSvc.AddMember(g.ID, bob, alice)

	perm, err := svc.ShareProjectWithGroup(project.ID.String(), alice, g.ID, "editor")
	if err != nil {
		t.Fatalf("share with group: %v", err)
	}
	if perm.GroupID != g.ID || perm.ProjectID != project.ID {
		t.Fatalf("permission shape wrong: %+v", perm)
	}

	canWrite, err := svc.rbac.CanWriteProject(bob, project.ID)
	if err != nil || !canWrite {
		t.Fatalf("bob should have transitive write access, err=%v can=%v", err, canWrite)
	}
}

func TestShareProjectWithGroup_OwnerNotInGroupRejected(t *testing.T) {
	svc, db := testSetup(t, false)
	groupSvc := NewGroupService(db, rbac.NewDefaultProvider())

	alice := createTestUser(t, db, "alice")
	project := createReadyProject(t, svc, db, "share-grp", alice)
	db.Create(&models.Role{Name: "viewer"})
	db.Create(&models.Role{Name: "editor"})

	g, _ := groupSvc.CreateGroup(CreateGroupRequest{Name: "outsiders"}, alice)
	// Note: alice is NOT a member of `g`.

	_, err := svc.ShareProjectWithGroup(project.ID.String(), alice, g.ID, "viewer")
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
	project := createReadyProject(t, svc, db, "x", alice)
	db.Create(&models.Role{Name: "viewer"})
	db.Create(&models.Role{Name: "editor"})

	g, _ := groupSvc.CreateGroup(CreateGroupRequest{Name: "ds"}, alice)
	_ = groupSvc.AddMember(g.ID, alice, alice)
	_, _ = svc.ShareProjectWithGroup(project.ID.String(), alice, g.ID, "viewer")

	cs, err := svc.ListCollaborators(project.ID.String())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var sawGroup bool
	for _, c := range cs {
		if c.Kind == CollaboratorKindGroup && c.GroupID != nil && *c.GroupID == g.ID {
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

func TestCollaboratorResult_JSON_OmitsIrrelevantIDs(t *testing.T) {
	userID := uuid.New()
	userEntry := CollaboratorResult{
		Kind: CollaboratorKindUser, UserID: &userID, Username: "alice", Role: "viewer",
	}
	userJSON, err := json.Marshal(userEntry)
	if err != nil {
		t.Fatalf("marshal user: %v", err)
	}
	if bytes.Contains(userJSON, []byte("group_id")) {
		t.Errorf("user JSON leaked group_id: %s", userJSON)
	}

	groupID := uuid.New()
	groupEntry := CollaboratorResult{
		Kind: CollaboratorKindGroup, GroupID: &groupID, Name: "ds", Source: "native", Role: "viewer",
	}
	groupJSON, err := json.Marshal(groupEntry)
	if err != nil {
		t.Fatalf("marshal group: %v", err)
	}
	if bytes.Contains(groupJSON, []byte("user_id")) {
		t.Errorf("group JSON leaked user_id: %s", groupJSON)
	}
}

func TestListProjects_IncludesGroupShared(t *testing.T) {
	svc, db := testSetup(t, false)
	groupSvc := NewGroupService(db, rbac.NewDefaultProvider())

	alice := createTestUser(t, db, "alice")
	bob := createTestUser(t, db, "bob")
	project := createReadyProject(t, svc, db, "shared-via-group", alice)

	db.Create(&models.Role{Name: "viewer"})
	db.Create(&models.Role{Name: "editor"})

	g, _ := groupSvc.CreateGroup(CreateGroupRequest{Name: "team"}, alice)
	_ = groupSvc.AddMember(g.ID, alice, alice)
	_ = groupSvc.AddMember(g.ID, bob, alice)
	if _, err := svc.ShareProjectWithGroup(project.ID.String(), alice, g.ID, "viewer"); err != nil {
		t.Fatalf("share with group: %v", err)
	}

	results, err := svc.List(bob)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var sawIt bool
	for _, r := range results {
		if r.ID == project.ID {
			sawIt = true
			break
		}
	}
	if !sawIt {
		t.Fatalf("bob should see project %s shared via group, got %+v", project.ID, results)
	}
}
