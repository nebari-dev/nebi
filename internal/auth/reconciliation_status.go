package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/rbac"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type authReconciliationKind string

const (
	authReconciliationOIDCGroups authReconciliationKind = "oidc_groups"
	authReconciliationProxyAdmin authReconciliationKind = "proxy_admin"

	defaultAuthReconciliationStaleAfter = TokenDuration
	maxReconciliationErrorLength        = 2048
)

var authReconciliationStaleAfterNanos atomic.Int64

func init() {
	ConfigureAuthReconciliationStaleAfter(defaultAuthReconciliationStaleAfter)
}

// ConfigureAuthReconciliationStaleAfter sets the reconciliation freshness
// window used by bearer-token validation and the background monitor.
func ConfigureAuthReconciliationStaleAfter(staleAfter time.Duration) {
	if staleAfter <= 0 {
		staleAfter = defaultAuthReconciliationStaleAfter
	}
	authReconciliationStaleAfterNanos.Store(int64(staleAfter))
}

func authReconciliationStaleAfter() time.Duration {
	return time.Duration(authReconciliationStaleAfterNanos.Load())
}

func authReconciliationMonitorInterval() time.Duration {
	interval := authReconciliationStaleAfter() / 2
	if interval <= 0 {
		return time.Second
	}
	return interval
}

func authReconciliationSuccessRefreshAfter() time.Duration {
	return authReconciliationMonitorInterval()
}

type authReconciliationFailureSource string

const (
	authReconciliationFailureSourceIdentityProvider authReconciliationFailureSource = "identity_provider"
	authReconciliationFailureSourceLocal            authReconciliationFailureSource = "local_authorization_storage"
)

type authReconciliationState struct {
	desiredGroupsJSON *string
	desiredAdmin      *bool
}

func recordAuthReconciliationSuccessWithGroups(db *gorm.DB, userID uuid.UUID, kind authReconciliationKind, claimGroups []string) error {
	return recordAuthReconciliationSuccessWithState(db, userID, kind, newOIDCGroupReconciliationState(claimGroups))
}

func recordAuthReconciliationSuccessWithAdmin(db *gorm.DB, userID uuid.UUID, kind authReconciliationKind, shouldBeAdmin bool) error {
	return recordAuthReconciliationSuccessWithState(db, userID, kind, newProxyAdminReconciliationState(shouldBeAdmin))
}

func recordAuthReconciliationSuccessWithState(db *gorm.DB, userID uuid.UUID, kind authReconciliationKind, state authReconciliationState) error {
	if db == nil {
		return errors.New("database is not configured")
	}

	now := time.Now().UTC()
	var status models.AuthReconciliationStatus
	err := db.Where("user_id = ? AND kind = ?", userID, string(kind)).First(&status).Error
	switch {
	case err == nil:
		if !authReconciliationSuccessNeedsWrite(status, state, now) {
			return nil
		}
		status.LastSuccessAt = &now
		status.LastFailureAt = nil
		status.LastFailureSource = ""
		status.ConsecutiveFailures = 0
		status.LastError = ""
		applyAuthReconciliationState(&status, state)
		return db.Save(&status).Error
	case errors.Is(err, gorm.ErrRecordNotFound):
		status = models.AuthReconciliationStatus{
			UserID:              userID,
			Kind:                string(kind),
			LastSuccessAt:       &now,
			ConsecutiveFailures: 0,
		}
		applyAuthReconciliationState(&status, state)
		return upsertAuthReconciliationStatus(db, &status, []string{
			"last_success_at",
			"last_failure_at",
			"last_failure_source",
			"consecutive_failures",
			"last_error",
			"desired_groups_json",
			"desired_admin",
			"updated_at",
		})
	default:
		return err
	}
}

func clearAuthReconciliationFailureWithGroups(db *gorm.DB, userID uuid.UUID, kind authReconciliationKind, claimGroups []string) error {
	return clearAuthReconciliationFailureWithState(db, userID, kind, newOIDCGroupReconciliationState(claimGroups))
}

func clearAuthReconciliationFailureWithState(db *gorm.DB, userID uuid.UUID, kind authReconciliationKind, state authReconciliationState) error {
	if db == nil {
		return errors.New("database is not configured")
	}

	var status models.AuthReconciliationStatus
	err := db.Where("user_id = ? AND kind = ?", userID, string(kind)).First(&status).Error
	switch {
	case err == nil:
		status.LastFailureAt = nil
		status.LastFailureSource = ""
		status.ConsecutiveFailures = 0
		status.LastError = ""
		applyAuthReconciliationState(&status, state)
		return db.Save(&status).Error
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil
	default:
		return err
	}
}

func authReconciliationSuccessNeedsWrite(status models.AuthReconciliationStatus, state authReconciliationState, now time.Time) bool {
	if status.LastFailureAt != nil || status.LastFailureSource != "" || status.ConsecutiveFailures != 0 || status.LastError != "" {
		return true
	}
	if state.desiredGroupsJSON != nil && status.DesiredGroupsJSON != *state.desiredGroupsJSON {
		return true
	}
	if state.desiredAdmin != nil {
		if status.DesiredAdmin == nil || *status.DesiredAdmin != *state.desiredAdmin {
			return true
		}
	}
	if status.LastSuccessAt == nil {
		return true
	}
	return now.Sub(*status.LastSuccessAt) >= authReconciliationSuccessRefreshAfter()
}

func upsertAuthReconciliationStatus(db *gorm.DB, status *models.AuthReconciliationStatus, updateColumns []string) error {
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "user_id"},
			{Name: "kind"},
		},
		DoUpdates: clause.AssignmentColumns(updateColumns),
	}).Create(status).Error
}

func recordAuthReconciliationFailureWithGroups(db *gorm.DB, userID uuid.UUID, kind authReconciliationKind, cause error, claimGroups []string) {
	recordAuthReconciliationFailureWithState(db, userID, kind, cause, authReconciliationFailureSourceLocal, newOIDCGroupReconciliationState(claimGroups))
}

func recordAuthReconciliationFailureWithAdmin(db *gorm.DB, userID uuid.UUID, kind authReconciliationKind, cause error, shouldBeAdmin bool) {
	recordAuthReconciliationFailureWithState(db, userID, kind, cause, authReconciliationFailureSourceLocal, newProxyAdminReconciliationState(shouldBeAdmin))
}

func recordAuthReconciliationFailureWithState(db *gorm.DB, userID uuid.UUID, kind authReconciliationKind, cause error, source authReconciliationFailureSource, state authReconciliationState) {
	now := time.Now().UTC()
	if cause == nil {
		cause = errors.New("unknown authorization reconciliation failure")
	}
	lastError := cause.Error()
	if len(lastError) > maxReconciliationErrorLength {
		lastError = lastError[:maxReconciliationErrorLength]
	}

	var status models.AuthReconciliationStatus
	var lastSuccessAt *time.Time
	var consecutiveFailures int
	if db != nil {
		err := db.Where("user_id = ? AND kind = ?", userID, string(kind)).First(&status).Error
		switch {
		case err == nil:
			lastSuccessAt = status.LastSuccessAt
			consecutiveFailures = status.ConsecutiveFailures + 1
			status.LastFailureAt = &now
			status.LastFailureSource = string(source)
			status.ConsecutiveFailures = consecutiveFailures
			status.LastError = lastError
			applyAuthReconciliationState(&status, state)
			if err := db.Save(&status).Error; err != nil {
				slog.Error("Failed to record authorization reconciliation failure",
					"user_id", userID, "kind", kind, "error", err)
			}
		case errors.Is(err, gorm.ErrRecordNotFound):
			consecutiveFailures = 1
			status = models.AuthReconciliationStatus{
				UserID:              userID,
				Kind:                string(kind),
				LastFailureAt:       &now,
				LastFailureSource:   string(source),
				ConsecutiveFailures: consecutiveFailures,
				LastError:           lastError,
			}
			applyAuthReconciliationState(&status, state)
			if err := upsertAuthReconciliationStatus(db, &status, []string{
				"last_failure_at",
				"last_failure_source",
				"consecutive_failures",
				"last_error",
				"desired_groups_json",
				"desired_admin",
				"updated_at",
			}); err != nil {
				slog.Error("Failed to record authorization reconciliation failure",
					"user_id", userID, "kind", kind, "error", err)
			}
		default:
			slog.Error("Failed to load authorization reconciliation status",
				"user_id", userID, "kind", kind, "error", err)
		}
	}

	var staleFor time.Duration
	if lastSuccessAt != nil {
		staleFor = now.Sub(*lastSuccessAt)
	}

	slog.Error("Authorization reconciliation failure alert",
		"user_id", userID,
		"kind", kind,
		"error", cause,
		"failure_source", source,
		"last_success_at", lastSuccessAt,
		"stale_for", staleFor.String(),
		"stale_after", authReconciliationStaleAfter().String(),
		"consecutive_failures", consecutiveFailures,
		"alert", true,
	)
}

func logIdentityProviderAuthFailure(kind authReconciliationKind, cause error) {
	if cause == nil {
		return
	}
	slog.Error("Identity provider authorization reconciliation failure",
		"kind", kind,
		"failure_source", authReconciliationFailureSourceIdentityProvider,
		"error", cause,
		"alert", true,
	)
}

// StartAuthReconciliationMonitor periodically retries unresolved
// authorization reconciliation failures and alerts until a later success clears them.
func StartAuthReconciliationMonitor(ctx context.Context, db *gorm.DB, rbacProvider rbac.Provider, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	if db == nil {
		logger.Error("Authorization reconciliation monitor disabled; database is not configured")
		return
	}

	go monitorAuthReconciliations(ctx, db, rbacProvider, logger, authReconciliationMonitorInterval())
}

func monitorAuthReconciliations(ctx context.Context, db *gorm.DB, rbacProvider rbac.Provider, logger *slog.Logger, interval time.Duration) {
	if interval <= 0 {
		interval = authReconciliationMonitorInterval()
	}

	report := func() {
		if _, err := alertUnresolvedAuthReconciliations(db, rbacProvider, logger, time.Now().UTC()); err != nil {
			logger.Error("Failed to monitor authorization reconciliation status", "error", err)
		}
	}

	report()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			report()
		}
	}
}

// alertUnresolvedAuthReconciliations accepts "now" so tests can exercise stale
// alert behavior deterministically. Runtime callers pass the current UTC time.
func alertUnresolvedAuthReconciliations(db *gorm.DB, rbacProvider rbac.Provider, logger *slog.Logger, now time.Time) (int, error) {
	if logger == nil {
		logger = slog.Default()
	}

	var statuses []models.AuthReconciliationStatus
	err := db.
		Where("last_failure_at IS NOT NULL").
		Where("last_success_at IS NULL OR last_failure_at > last_success_at").
		Find(&statuses).Error
	if err != nil {
		return 0, err
	}

	unresolved := 0
	for _, status := range statuses {
		reconciled, retryErr := retryAuthReconciliationStatus(db, rbacProvider, status)
		if retryErr != nil {
			logger.Error("Authorization reconciliation retry failed",
				"user_id", status.UserID,
				"kind", status.Kind,
				"error", retryErr,
				"failure_source", authReconciliationFailureSourceLocal,
			)
		}
		if reconciled {
			continue
		}

		unresolved++
		staleFor := time.Duration(0)
		if status.LastSuccessAt != nil {
			staleFor = now.Sub(*status.LastSuccessAt)
		} else if status.LastFailureAt != nil {
			staleFor = now.Sub(*status.LastFailureAt)
		}

		logger.Error("Authorization reconciliation stale alert",
			"user_id", status.UserID,
			"kind", status.Kind,
			"error", status.LastError,
			"failure_source", status.LastFailureSource,
			"last_success_at", status.LastSuccessAt,
			"last_failure_at", status.LastFailureAt,
			"stale_for", staleFor.String(),
			"stale_after", authReconciliationStaleAfter().String(),
			"consecutive_failures", status.ConsecutiveFailures,
			"alert", true,
			"monitor", true,
		)
	}

	return unresolved, nil
}

func retryAuthReconciliationStatus(db *gorm.DB, rbacProvider rbac.Provider, status models.AuthReconciliationStatus) (bool, error) {
	kind := authReconciliationKind(status.Kind)
	switch kind {
	case authReconciliationOIDCGroups:
		if status.DesiredGroupsJSON == "" {
			return false, nil
		}
		claimGroups, err := decodeOIDCGroupReconciliationState(status.DesiredGroupsJSON)
		if err != nil {
			return false, err
		}
		if err := syncOIDCGroupRemovalsOnly(db, status.UserID, claimGroups, rbacProvider); err != nil {
			recordAuthReconciliationFailureWithGroups(db, status.UserID, kind, err, claimGroups)
			return false, err
		}
		if err := clearAuthReconciliationFailureWithGroups(db, status.UserID, kind, claimGroups); err != nil {
			recordAuthReconciliationFailureWithGroups(db, status.UserID, kind, err, claimGroups)
			return false, err
		}
		return true, nil
	case authReconciliationProxyAdmin:
		if status.DesiredAdmin == nil {
			return false, nil
		}
		if *status.DesiredAdmin {
			return false, nil
		}
		if err := syncAdminRoleToDesired(status.UserID, *status.DesiredAdmin, rbacProvider); err != nil {
			recordAuthReconciliationFailureWithAdmin(db, status.UserID, kind, err, *status.DesiredAdmin)
			return false, err
		}
		state := newProxyAdminReconciliationState(*status.DesiredAdmin)
		// Monitor retries are cached-state repairs, so do not refresh
		// LastSuccessAt, which gates bearer-token freshness at the user level.
		if err := clearAuthReconciliationFailureWithState(db, status.UserID, kind, state); err != nil {
			recordAuthReconciliationFailureWithAdmin(db, status.UserID, kind, err, *status.DesiredAdmin)
			return false, err
		}
		return true, nil
	default:
		return false, nil
	}
}

func newOIDCGroupReconciliationState(claimGroups []string) authReconciliationState {
	desiredGroupsJSON := encodeOIDCGroupReconciliationState(claimGroups)
	return authReconciliationState{desiredGroupsJSON: &desiredGroupsJSON}
}

func newProxyAdminReconciliationState(shouldBeAdmin bool) authReconciliationState {
	return authReconciliationState{desiredAdmin: &shouldBeAdmin}
}

func applyAuthReconciliationState(status *models.AuthReconciliationStatus, state authReconciliationState) {
	if state.desiredGroupsJSON != nil {
		status.DesiredGroupsJSON = *state.desiredGroupsJSON
	}
	if state.desiredAdmin != nil {
		desiredAdmin := *state.desiredAdmin
		status.DesiredAdmin = &desiredAdmin
	}
}

func encodeOIDCGroupReconciliationState(claimGroups []string) string {
	encoded, err := json.Marshal(normalizeOIDCGroupNames(claimGroups))
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

func decodeOIDCGroupReconciliationState(value string) ([]string, error) {
	var claimGroups []string
	if err := json.Unmarshal([]byte(value), &claimGroups); err != nil {
		return nil, fmt.Errorf("decode desired oidc groups: %w", err)
	}
	return claimGroups, nil
}
