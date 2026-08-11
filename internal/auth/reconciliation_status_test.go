package auth

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/nebari-dev/nebi/internal/models"
)

func TestAlertUnresolvedAuthReconciliationsReportsOnlyOpenFailures(t *testing.T) {
	db := syncTestDB(t)
	now := time.Now().UTC()

	u := models.User{Username: "alice", Email: "alice@test"}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	oldSuccess := now.Add(-time.Minute)
	newFailure := now
	if err := db.Create(&models.AuthReconciliationStatus{
		UserID:              u.ID,
		Kind:                string(authReconciliationOIDCGroups),
		LastSuccessAt:       &oldSuccess,
		LastFailureAt:       &newFailure,
		ConsecutiveFailures: 1,
		LastError:           "casbin unavailable",
	}).Error; err != nil {
		t.Fatalf("create unresolved status: %v", err)
	}

	oldFailure := now.Add(-time.Minute)
	newSuccess := now
	if err := db.Create(&models.AuthReconciliationStatus{
		UserID:              u.ID,
		Kind:                string(authReconciliationProxyAdmin),
		LastSuccessAt:       &newSuccess,
		LastFailureAt:       &oldFailure,
		ConsecutiveFailures: 0,
	}).Error; err != nil {
		t.Fatalf("create resolved status: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	count, err := alertUnresolvedAuthReconciliations(db, &stubRBACProvider{}, logger, now)
	if err != nil {
		t.Fatalf("alert unresolved reconciliations: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one unresolved reconciliation alert, got %d", count)
	}
}

func TestAlertUnresolvedAuthReconciliationsRetriesOIDCGroupState(t *testing.T) {
	db := syncTestDB(t)
	now := time.Now().UTC()

	u := models.User{Username: "alice", Email: "alice@test"}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	if err := db.Create(&models.AuthReconciliationStatus{
		UserID:              u.ID,
		Kind:                string(authReconciliationOIDCGroups),
		LastFailureAt:       &now,
		LastFailureSource:   string(authReconciliationFailureSourceLocal),
		ConsecutiveFailures: 1,
		LastError:           "casbin unavailable",
		DesiredGroupsJSON:   encodeOIDCGroupReconciliationState([]string{"/engineering", "engineering"}),
	}).Error; err != nil {
		t.Fatalf("create unresolved status: %v", err)
	}

	provider := &stubRBACProvider{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	count, err := alertUnresolvedAuthReconciliations(db, provider, logger, now)
	if err != nil {
		t.Fatalf("alert unresolved reconciliations: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected retry to resolve reconciliation, got %d unresolved", count)
	}
	if len(provider.addedGroups) != 1 {
		t.Fatalf("expected retry to add one casbin group, got %d", len(provider.addedGroups))
	}

	var status models.AuthReconciliationStatus
	if err := db.First(&status, "user_id = ? AND kind = ?", u.ID, string(authReconciliationOIDCGroups)).Error; err != nil {
		t.Fatalf("load status: %v", err)
	}
	if status.LastSuccessAt == nil {
		t.Fatal("expected retry to record success")
	}
	if status.LastFailureAt != nil {
		t.Fatal("expected retry success to clear failure timestamp")
	}
}

func TestAlertUnresolvedAuthReconciliationsRetriesPersistedFailuresRegardlessOfSource(t *testing.T) {
	db := syncTestDB(t)
	now := time.Now().UTC()

	u := models.User{Username: "alice", Email: "alice@test"}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	if err := db.Create(&models.AuthReconciliationStatus{
		UserID:              u.ID,
		Kind:                string(authReconciliationOIDCGroups),
		LastFailureAt:       &now,
		LastFailureSource:   string(authReconciliationFailureSourceIdentityProvider),
		ConsecutiveFailures: 1,
		LastError:           "token endpoint unavailable",
		DesiredGroupsJSON:   encodeOIDCGroupReconciliationState([]string{"engineering"}),
	}).Error; err != nil {
		t.Fatalf("create unresolved status: %v", err)
	}

	provider := &stubRBACProvider{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	count, err := alertUnresolvedAuthReconciliations(db, provider, logger, now)
	if err != nil {
		t.Fatalf("alert unresolved reconciliations: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected retry to resolve reconciliation, got %d unresolved", count)
	}
	if len(provider.addedGroups) != 1 {
		t.Fatalf("expected retry to add one casbin group, got %d", len(provider.addedGroups))
	}
}

func TestAlertUnresolvedAuthReconciliationsRetriesProxyAdminState(t *testing.T) {
	db := syncTestDB(t)
	now := time.Now().UTC()

	u := models.User{Username: "alice", Email: "alice@test"}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	if err := db.Create(&models.AuthReconciliationStatus{
		UserID:              u.ID,
		Kind:                string(authReconciliationProxyAdmin),
		LastFailureAt:       &now,
		LastFailureSource:   string(authReconciliationFailureSourceLocal),
		ConsecutiveFailures: 1,
		LastError:           "revoke failed",
		DesiredAdmin:        boolPtr(false),
	}).Error; err != nil {
		t.Fatalf("create unresolved status: %v", err)
	}

	provider := &stubRBACProvider{isAdmin: true}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	count, err := alertUnresolvedAuthReconciliations(db, provider, logger, now)
	if err != nil {
		t.Fatalf("alert unresolved reconciliations: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected retry to resolve reconciliation, got %d unresolved", count)
	}
	if !provider.revokedAdmin {
		t.Fatal("expected retry to revoke admin")
	}

	var status models.AuthReconciliationStatus
	if err := db.First(&status, "user_id = ? AND kind = ?", u.ID, string(authReconciliationProxyAdmin)).Error; err != nil {
		t.Fatalf("load status: %v", err)
	}
	if status.LastSuccessAt == nil {
		t.Fatal("expected retry to record success")
	}
	if status.LastFailureAt != nil {
		t.Fatal("expected retry success to clear failure timestamp")
	}
}

func TestAlertUnresolvedAuthReconciliationsDoesNotRetryProxyAdminGrants(t *testing.T) {
	db := syncTestDB(t)
	now := time.Now().UTC()

	u := models.User{Username: "alice", Email: "alice@test"}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	if err := db.Create(&models.AuthReconciliationStatus{
		UserID:              u.ID,
		Kind:                string(authReconciliationProxyAdmin),
		LastFailureAt:       &now,
		LastFailureSource:   string(authReconciliationFailureSourceLocal),
		ConsecutiveFailures: 1,
		LastError:           "grant failed",
		DesiredAdmin:        boolPtr(true),
	}).Error; err != nil {
		t.Fatalf("create unresolved status: %v", err)
	}

	provider := &stubRBACProvider{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	count, err := alertUnresolvedAuthReconciliations(db, provider, logger, now)
	if err != nil {
		t.Fatalf("alert unresolved reconciliations: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected stale grant to remain unresolved, got %d unresolved", count)
	}
	if provider.madeAdmin {
		t.Fatal("expected monitor not to grant admin from cached desired state")
	}
}

func boolPtr(value bool) *bool {
	return &value
}
