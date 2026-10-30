package rbac

import (
	"testing"

	"github.com/google/uuid"
)

// In local mode the router skips InitEnforcer because middleware bypasses
// RBAC anyway. But ProjectService.Create still calls
// rbac.GrantProjectAccess unconditionally, so the data-layer functions
// must tolerate a nil enforcer instead of nil-deref panicking. These
// tests pin that contract.

func TestGrantProjectAccess_NilEnforcer_NoPanic(t *testing.T) {
	saved := enforcer
	enforcer = nil
	t.Cleanup(func() { enforcer = saved })

	if err := GrantProjectAccess(uuid.New(), uuid.New(), "owner"); err != nil {
		t.Fatalf("expected nil-safe no-op, got error: %v", err)
	}
}

func TestRevokeProjectAccess_NilEnforcer_NoPanic(t *testing.T) {
	saved := enforcer
	enforcer = nil
	t.Cleanup(func() { enforcer = saved })

	if err := RevokeProjectAccess(uuid.New(), uuid.New()); err != nil {
		t.Fatalf("expected nil-safe no-op, got error: %v", err)
	}
}

func TestCanReadProject_NilEnforcer_AllowsAll(t *testing.T) {
	saved := enforcer
	enforcer = nil
	t.Cleanup(func() { enforcer = saved })

	allowed, err := CanReadProject(uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatal("expected nil-enforcer (local mode) to allow read")
	}
}

func TestCanWriteProject_NilEnforcer_AllowsAll(t *testing.T) {
	saved := enforcer
	enforcer = nil
	t.Cleanup(func() { enforcer = saved })

	allowed, err := CanWriteProject(uuid.New(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatal("expected nil-enforcer (local mode) to allow write")
	}
}

func TestIsAdmin_NilEnforcer_AllowsAll(t *testing.T) {
	saved := enforcer
	enforcer = nil
	t.Cleanup(func() { enforcer = saved })

	allowed, err := IsAdmin(uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatal("expected nil-enforcer (local mode) to be admin")
	}
}
