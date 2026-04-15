package service

import (
	"testing"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
)

// --- ShareWorkspace tests ---

func TestShareWorkspace_GrantsAccess(t *testing.T) {
	svc, db := testSetup(t, false) // team mode
	alice := createTestUser(t, db, "alice")
	bob := createTestUser(t, db, "bob")
	ws := createReadyWorkspace(t, svc, db, "share-test", alice)

	// Seed roles
	db.Create(&models.Role{Name: "viewer", Description: "read-only"})
	db.Create(&models.Role{Name: "editor", Description: "read-write"})

	perm, err := svc.ShareWorkspace(ws.ID.String(), alice, bob, "editor")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if perm.UserID != bob {
		t.Errorf("expected user ID %s, got %s", bob, perm.UserID)
	}
	if perm.WorkspaceID != ws.ID {
		t.Errorf("expected workspace ID %s, got %s", ws.ID, perm.WorkspaceID)
	}

	// Verify permission record in DB
	var dbPerm models.Permission
	if err := db.Where("user_id = ? AND workspace_id = ?", bob, ws.ID).First(&dbPerm).Error; err != nil {
		t.Fatalf("permission not found in DB: %v", err)
	}

	// Verify audit log
	var auditCount int64
	db.Model(&models.AuditLog{}).Where("user_id = ? AND action = ?", alice, "grant_permission").Count(&auditCount)
	if auditCount != 1 {
		t.Errorf("expected 1 audit log, got %d", auditCount)
	}
}

func TestShareWorkspace_RejectsNonOwner(t *testing.T) {
	svc, db := testSetup(t, false)
	alice := createTestUser(t, db, "alice")
	bob := createTestUser(t, db, "bob")
	charlie := createTestUser(t, db, "charlie")
	ws := createReadyWorkspace(t, svc, db, "share-test", alice)

	db.Create(&models.Role{Name: "viewer"})

	// Bob is not the owner — should be forbidden
	_, err := svc.ShareWorkspace(ws.ID.String(), bob, charlie, "viewer")
	if err == nil {
		t.Fatal("expected error for non-owner share")
	}
	var fe *ForbiddenError
	if !isForbiddenError(err, &fe) {
		t.Fatalf("expected ForbiddenError, got %T: %v", err, err)
	}
}

func TestShareWorkspace_RejectsInvalidRole(t *testing.T) {
	svc, db := testSetup(t, false)
	alice := createTestUser(t, db, "alice")
	bob := createTestUser(t, db, "bob")
	ws := createReadyWorkspace(t, svc, db, "share-test", alice)

	_, err := svc.ShareWorkspace(ws.ID.String(), alice, bob, "superadmin")
	if err == nil {
		t.Fatal("expected error for invalid role")
	}
	var ve *ValidationError
	if !isValidationError(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
}

func TestShareWorkspace_RejectsNonExistentUser(t *testing.T) {
	svc, db := testSetup(t, false)
	alice := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "share-test", alice)

	_, err := svc.ShareWorkspace(ws.ID.String(), alice, uuid.New(), "viewer")
	if err == nil {
		t.Fatal("expected error for non-existent user")
	}
}

func TestShareWorkspace_NotFoundWorkspace(t *testing.T) {
	svc, db := testSetup(t, false)
	alice := createTestUser(t, db, "alice")

	_, err := svc.ShareWorkspace(uuid.New().String(), alice, uuid.New(), "viewer")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// --- UnshareWorkspace tests ---

func TestUnshareWorkspace_RevokesAccess(t *testing.T) {
	svc, db := testSetup(t, false)
	alice := createTestUser(t, db, "alice")
	bob := createTestUser(t, db, "bob")
	ws := createReadyWorkspace(t, svc, db, "unshare-test", alice)

	db.Create(&models.Role{Name: "viewer"})

	// Share first
	svc.ShareWorkspace(ws.ID.String(), alice, bob, "viewer")

	// Unshare
	err := svc.UnshareWorkspace(ws.ID.String(), alice, bob)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify permission removed from DB
	var count int64
	db.Model(&models.Permission{}).Where("user_id = ? AND workspace_id = ?", bob, ws.ID).Count(&count)
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

func TestUnshareWorkspace_RejectsNonOwner(t *testing.T) {
	svc, db := testSetup(t, false)
	alice := createTestUser(t, db, "alice")
	bob := createTestUser(t, db, "bob")
	charlie := createTestUser(t, db, "charlie")
	ws := createReadyWorkspace(t, svc, db, "unshare-test", alice)

	err := svc.UnshareWorkspace(ws.ID.String(), bob, charlie)
	var fe *ForbiddenError
	if !isForbiddenError(err, &fe) {
		t.Fatalf("expected ForbiddenError, got %T: %v", err, err)
	}
}

func TestUnshareWorkspace_CannotRemoveOwner(t *testing.T) {
	svc, db := testSetup(t, false)
	alice := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "unshare-test", alice)

	err := svc.UnshareWorkspace(ws.ID.String(), alice, alice)
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
	ws := createReadyWorkspace(t, svc, db, "collab-test", alice)

	collabs, err := svc.ListCollaborators(ws.ID.String())
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
	ws := createReadyWorkspace(t, svc, db, "collab-test", alice)

	db.Create(&models.Role{Name: "editor"})
	svc.ShareWorkspace(ws.ID.String(), alice, bob, "editor")

	collabs, err := svc.ListCollaborators(ws.ID.String())
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
