package auth

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	// Every connection to ":memory:" is a separate database.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(
		&models.User{},
		&models.FederatedIdentity{},
		&models.Group{},
		&models.GroupMember{},
		&models.AuditLog{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

const testIssuer = "https://issuer.example.com"

func TestFindOrCreateFederatedUser_CreatesNew(t *testing.T) {
	db := setupTestDB(t)

	user, err := findOrCreateFederatedUser(db, federatedUserClaims{
		Issuer:            testIssuer,
		Subject:           "sub-bob",
		PreferredUsername: "Bob",
		Email:             "bob@example.com",
		EmailVerified:     true,
		AvatarURL:         "https://example.com/bob.png",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.Username != "bob" || user.Email != "bob@example.com" || user.AvatarURL != "https://example.com/bob.png" {
		t.Fatalf("unexpected user %+v", user)
	}

	var count int64
	db.Model(&models.FederatedIdentity{}).Where("issuer = ? AND subject = ? AND user_id = ?", testIssuer, "sub-bob", user.ID).Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 federated identity, got %d", count)
	}
}

func TestFindOrCreateFederatedUser_FindsExistingBySubjectOnly(t *testing.T) {
	db := setupTestDB(t)

	existing, err := findOrCreateFederatedUser(db, federatedUserClaims{
		Issuer: testIssuer, Subject: "sub-carol", PreferredUsername: "carol",
		Email: "carol@example.com", EmailVerified: true, AvatarURL: "old-avatar",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	user, err := findOrCreateFederatedUser(db, federatedUserClaims{
		Issuer: testIssuer, Subject: "sub-carol", PreferredUsername: "changed-carol",
		Email: "changed@example.com", EmailVerified: true, AvatarURL: "new-avatar",
	})
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if user.ID != existing.ID {
		t.Fatal("expected the same user for the same issuer and subject")
	}
	if user.Username != "carol" {
		t.Errorf("expected username to stay carol, got %s", user.Username)
	}
	if user.AvatarURL != "new-avatar" {
		t.Errorf("expected avatar update, got %s", user.AvatarURL)
	}
	var identity models.FederatedIdentity
	db.First(&identity, "issuer = ? AND subject = ?", testIssuer, "sub-carol")
	if identity.Username != "changed-carol" || identity.Email != "changed@example.com" {
		t.Errorf("expected identity profile to track the latest claims, got %+v", identity)
	}
}

// Identities are matched only by (issuer, subject). A new identity whose
// username or verified email claims collide with an existing user must get
// its own suffixed account, never the existing one.
func TestFindOrCreateFederatedUser_DoesNotLinkByUsernameOrEmail(t *testing.T) {
	db := setupTestDB(t)

	original, err := findOrCreateFederatedUser(db, federatedUserClaims{
		Issuer: testIssuer, Subject: "sub-alice", PreferredUsername: "alice",
		Email: "alice@example.com", EmailVerified: true,
	})
	if err != nil {
		t.Fatalf("create original: %v", err)
	}

	for _, claims := range []federatedUserClaims{
		// Recreated in the IdP: new subject, same claims.
		{Issuer: testIssuer, Subject: "sub-alice-2", PreferredUsername: "alice", Email: "alice@example.com", EmailVerified: true},
		// Same subject at another issuer.
		{Issuer: "https://other-issuer.example.com", Subject: "sub-alice", PreferredUsername: "ALICE", Email: "alice@example.com", EmailVerified: true},
	} {
		user, err := findOrCreateFederatedUser(db, claims)
		if err != nil {
			t.Fatalf("create colliding user: %v", err)
		}
		if user.ID == original.ID {
			t.Fatalf("identity %s/%s was linked to the existing user", claims.Issuer, claims.Subject)
		}
		if !strings.HasPrefix(user.Username, "alice-") {
			t.Errorf("expected a suffixed username, got %s", user.Username)
		}
		if user.Email == original.Email {
			t.Errorf("expected a distinct email, got %s", user.Email)
		}
	}
}

func TestFindOrCreateFederatedUser_Fallbacks(t *testing.T) {
	db := setupTestDB(t)

	tests := []struct {
		name         string
		claims       federatedUserClaims
		wantUsername string
		wantEmail    string
	}{
		{"verified email", federatedUserClaims{Subject: "s1", Email: "dave@example.com", EmailVerified: true}, "dave@example.com", "dave@example.com"},
		{"subject only", federatedUserClaims{Subject: "sub-xyz"}, "sub-xyz", "sub-xyz@nebi.local"},
		{"username without email", federatedUserClaims{Subject: "s3", PreferredUsername: "frank"}, "frank", "frank@nebi.local"},
		{"unverified email is not trusted", federatedUserClaims{Subject: "s4", PreferredUsername: "gina", Email: "gina@example.com"}, "gina", "gina@nebi.local"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.claims.Issuer = testIssuer
			user, err := findOrCreateFederatedUser(db, tt.claims)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if user.Username != tt.wantUsername || user.Email != tt.wantEmail {
				t.Fatalf("got username %q email %q, want %q %q", user.Username, user.Email, tt.wantUsername, tt.wantEmail)
			}
		})
	}
}

func TestFindOrCreateFederatedUser_RequiresIssuerAndSubject(t *testing.T) {
	db := setupTestDB(t)
	if _, err := findOrCreateFederatedUser(db, federatedUserClaims{Subject: "s"}); err == nil {
		t.Error("expected error without issuer")
	}
	if _, err := findOrCreateFederatedUser(db, federatedUserClaims{Issuer: testIssuer}); err == nil {
		t.Error("expected error without subject")
	}
}

func TestIsUniqueConstraintErrorOnlyMatchesUniqueConstraints(t *testing.T) {
	db := setupTestDB(t)
	if err := db.Create(&models.User{Username: "dup", Email: "dup@example.com"}).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	err := db.Create(&models.User{Username: "dup", Email: "other@example.com"}).Error
	if !isUniqueConstraintError(err) {
		t.Fatalf("expected unique constraint error, got %v", err)
	}
	if isUniqueConstraintError(gorm.ErrRecordNotFound) {
		t.Fatal("record-not-found is not a unique constraint error")
	}
}
