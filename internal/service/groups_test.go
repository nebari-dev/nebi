package service

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func groupTestSetup(t *testing.T) (*GroupService, *gorm.DB) {
	t.Helper()
	_, db := testSetup(t, false)
	return NewGroupService(db), db
}

func TestListGroups_IncludesMemberCounts(t *testing.T) {
	svc, db := groupTestSetup(t)
	alice := createTestUser(t, db, "alice")
	bob := createTestUser(t, db, "bob")
	ds := createTestGroup(t, db, "data-science")
	createTestGroup(t, db, "empty")
	addTestGroupMember(t, db, ds.ID, alice)
	addTestGroupMember(t, db, ds.ID, bob)

	groups, err := svc.ListGroups()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	counts := map[string]int64{}
	for _, g := range groups {
		counts[g.Name] = g.MemberCount
	}
	if counts["data-science"] != 2 || counts["empty"] != 0 || len(counts) != 2 {
		t.Fatalf("unexpected member counts %v", counts)
	}
}

func TestGetGroup(t *testing.T) {
	svc, db := groupTestSetup(t)
	g := createTestGroup(t, db, "ds")

	got, err := svc.GetGroup(g.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "ds" || got.MemberCount != 0 {
		t.Fatalf("unexpected group %+v", got)
	}
	if _, err := svc.GetGroup(uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestListMembersAndGroupsForUser(t *testing.T) {
	svc, db := groupTestSetup(t)
	alice := createTestUser(t, db, "alice")
	bob := createTestUser(t, db, "bob")
	g := createTestGroup(t, db, "ds")
	addTestGroupMember(t, db, g.ID, alice)

	members, err := svc.ListMembers(g.ID)
	if err != nil {
		t.Fatalf("list members: %v", err)
	}
	if len(members) != 1 || members[0].User.Username != "alice" {
		t.Fatalf("expected alice as the only member, got %+v", members)
	}

	aliceGroups, err := svc.ListGroupsForUser(alice)
	if err != nil || len(aliceGroups) != 1 || aliceGroups[0].ID != g.ID {
		t.Fatalf("expected alice in ds, got %+v err=%v", aliceGroups, err)
	}
	bobGroups, err := svc.ListGroupsForUser(bob)
	if err != nil || len(bobGroups) != 0 {
		t.Fatalf("expected bob in no groups, got %+v err=%v", bobGroups, err)
	}
}
