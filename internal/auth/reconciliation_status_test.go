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

func TestAlertUnresolvedAuthReconciliationsDoesNotReplayOIDCGroupAdditions(t *testing.T) {
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
		t.Fatalf("expected removal-only retry to clear reconciliation failure, got %d unresolved", count)
	}
	if len(provider.addedGroups) != 0 {
		t.Fatalf("expected monitor not to add cached groups, got %d additions", len(provider.addedGroups))
	}

	var groupCount int64
	if err := db.Model(&models.Group{}).Where("name = ?", "engineering").Count(&groupCount).Error; err != nil {
		t.Fatalf("count groups: %v", err)
	}
	if groupCount != 0 {
		t.Fatalf("expected monitor not to create cached group, got %d groups", groupCount)
	}

	var status models.AuthReconciliationStatus
	if err := db.First(&status, "user_id = ? AND kind = ?", u.ID, string(authReconciliationOIDCGroups)).Error; err != nil {
		t.Fatalf("load status: %v", err)
	}
	if status.LastSuccessAt != nil {
		t.Fatal("expected removal-only retry not to record a fresh success")
	}
	if status.LastFailureAt != nil {
		t.Fatal("expected removal-only retry to clear failure timestamp")
	}
	if status.LastFailureSource != "" {
		t.Fatalf("expected removal-only retry to clear failure source, got %q", status.LastFailureSource)
	}
	if status.ConsecutiveFailures != 0 {
		t.Fatalf("expected removal-only retry to reset failure count, got %d", status.ConsecutiveFailures)
	}
}

func TestAlertUnresolvedAuthReconciliationsRetriesOIDCGroupRemovalsWithoutRefreshingSuccess(t *testing.T) {
	db := syncTestDB(t)
	now := time.Now().UTC()
	oldSuccess := now.Add(-2 * authReconciliationStaleAfter())

	u := models.User{Username: "alice", Email: "alice@test"}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	kept := models.Group{Name: "engineering", Source: models.GroupSourceOIDC}
	removed := models.Group{Name: "analytics", Source: models.GroupSourceOIDC}
	if err := db.Create(&kept).Error; err != nil {
		t.Fatalf("create kept group: %v", err)
	}
	if err := db.Create(&removed).Error; err != nil {
		t.Fatalf("create removed group: %v", err)
	}
	if err := db.Create(&models.GroupMember{GroupID: kept.ID, UserID: u.ID}).Error; err != nil {
		t.Fatalf("create kept membership: %v", err)
	}
	if err := db.Create(&models.GroupMember{GroupID: removed.ID, UserID: u.ID}).Error; err != nil {
		t.Fatalf("create removed membership: %v", err)
	}

	if err := db.Create(&models.AuthReconciliationStatus{
		UserID:              u.ID,
		Kind:                string(authReconciliationOIDCGroups),
		LastSuccessAt:       &oldSuccess,
		LastFailureAt:       &now,
		LastFailureSource:   string(authReconciliationFailureSourceLocal),
		ConsecutiveFailures: 1,
		LastError:           "casbin unavailable",
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
	if len(provider.addedGroups) != 0 {
		t.Fatalf("expected monitor not to add cached groups, got %d additions", len(provider.addedGroups))
	}
	if len(provider.removedGroups) != 1 || provider.removedGroups[0] != removed.ID {
		t.Fatalf("expected monitor to remove stale group %s, got %v", removed.ID, provider.removedGroups)
	}

	var keptCount int64
	if err := db.Model(&models.GroupMember{}).
		Where("group_id = ? AND user_id = ?", kept.ID, u.ID).
		Count(&keptCount).Error; err != nil {
		t.Fatalf("count kept membership: %v", err)
	}
	if keptCount != 1 {
		t.Fatalf("expected kept membership to remain, got %d rows", keptCount)
	}

	var removedCount int64
	if err := db.Model(&models.GroupMember{}).
		Where("group_id = ? AND user_id = ?", removed.ID, u.ID).
		Count(&removedCount).Error; err != nil {
		t.Fatalf("count removed membership: %v", err)
	}
	if removedCount != 0 {
		t.Fatalf("expected stale membership to be deleted, got %d rows", removedCount)
	}

	var status models.AuthReconciliationStatus
	if err := db.First(&status, "user_id = ? AND kind = ?", u.ID, string(authReconciliationOIDCGroups)).Error; err != nil {
		t.Fatalf("load status: %v", err)
	}
	if status.LastSuccessAt == nil || !status.LastSuccessAt.Equal(oldSuccess) {
		t.Fatalf("expected retry not to refresh last_success_at, got %v want %v", status.LastSuccessAt, oldSuccess)
	}
	if status.LastFailureAt != nil {
		t.Fatal("expected retry to clear failure timestamp")
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
	if len(provider.addedGroups) != 0 {
		t.Fatalf("expected monitor not to add cached groups, got %d additions", len(provider.addedGroups))
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
	if status.LastSuccessAt != nil {
		t.Fatal("expected retry not to record fresh success")
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
