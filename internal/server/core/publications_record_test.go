package core

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/db"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newPublicationsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gdb, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "core.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(gdb, false); err != nil {
		t.Fatal(err)
	}
	return gdb
}

func seedProjectVersions(t *testing.T, gdb *gorm.DB, n int) (models.User, models.Project) {
	t.Helper()
	u := models.User{Username: "sam", Email: "sam@example.com"}
	if err := gdb.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	p := models.Project{Name: "project_1", OwnerID: u.ID, Status: models.ProjectStatusReady}
	if err := gdb.Omit("Owner").Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= n; i++ {
		v := models.ProjectVersion{ProjectID: p.ID, VersionNumber: i, ManifestContent: "x", LockFileContent: "y", PackageMetadata: "[]", CreatedBy: u.ID}
		if err := gdb.Omit("Project", "Job", "CreatedByUser").Create(&v).Error; err != nil {
			t.Fatal(err)
		}
	}
	return u, p
}

func TestRecordPublicationLifecycle(t *testing.T) {
	ctx := context.Background()
	gdb := newPublicationsTestDB(t)
	svc := New(Deps{DB: gdb})
	u, p := seedProjectVersions(t, gdb, 3)

	clientID := uuid.New()
	rec, err := svc.RecordPublication(ctx, u.ID, p.ID, 2, PublicationRecord{
		ID:           &clientID,
		RegistryHost: "ghcr.io",
		Repository:   "example-org/project_1",
		Tag:          "stable",
		Digest:       "sha256:aaa",
	})
	if err != nil {
		t.Fatalf("RecordPublication: %v", err)
	}
	if rec.ID == nil || *rec.ID != clientID {
		t.Fatalf("client ID not kept: %+v", rec)
	}
	if rec.PublishedBy != "sam" {
		t.Fatalf("published_by = %q, want the caller's username", rec.PublishedBy)
	}

	// Recording the same ID again is an update, not a duplicate.
	if _, err := svc.RecordPublication(ctx, u.ID, p.ID, 2, PublicationRecord{ID: &clientID, RegistryHost: "ghcr.io", Repository: "example-org/project_1", Tag: "stable", Digest: "sha256:bbb"}); err != nil {
		t.Fatal(err)
	}
	byVersion, err := svc.PublicationsByVersion(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(byVersion[2]) != 1 || byVersion[2][0].Digest != "sha256:bbb" {
		t.Fatalf("after re-record: %+v", byVersion)
	}

	// The same tag on version 3 moves the tag: version 2's record is retired.
	if _, err := svc.RecordPublication(ctx, u.ID, p.ID, 3, PublicationRecord{RegistryHost: "ghcr.io", Repository: "example-org/project_1", Tag: "stable"}); err != nil {
		t.Fatal(err)
	}
	byVersion, _ = svc.PublicationsByVersion(ctx, p.ID)
	if len(byVersion[2]) != 0 || len(byVersion[3]) != 1 {
		t.Fatalf("tag did not move: %+v", byVersion)
	}
	v3ID := *byVersion[3][0].ID

	if err := svc.RemovePublication(ctx, u.ID, p.ID, 3, v3ID); err != nil {
		t.Fatalf("RemovePublication: %v", err)
	}
	if err := svc.RemovePublication(ctx, u.ID, p.ID, 3, v3ID); err != nil {
		t.Fatalf("RemovePublication twice: %v", err)
	}
	byVersion, _ = svc.PublicationsByVersion(ctx, p.ID)
	if len(byVersion) != 0 {
		t.Fatalf("expected no active publications, got %+v", byVersion)
	}
	if err := svc.RemovePublication(ctx, u.ID, p.ID, 3, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown publication: err = %v", err)
	}
}

func TestRecordPublicationValidation(t *testing.T) {
	ctx := context.Background()
	gdb := newPublicationsTestDB(t)
	svc := New(Deps{DB: gdb})
	u, p := seedProjectVersions(t, gdb, 1)

	_, err := svc.RecordPublication(ctx, u.ID, p.ID, 9, PublicationRecord{RegistryHost: "ghcr.io", Repository: "a/b", Tag: "1"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing version: err = %v", err)
	}
	var verr *ValidationError
	if _, err := svc.RecordPublication(ctx, u.ID, p.ID, 1, PublicationRecord{Repository: "a/b", Tag: "1"}); !errors.As(err, &verr) {
		t.Fatalf("missing host: err = %v", err)
	}
	if _, err := svc.RecordPublication(ctx, u.ID, p.ID, 1, PublicationRecord{RegistryHost: "ghcr.io", Repository: "a/b"}); !errors.As(err, &verr) {
		t.Fatalf("missing tag: err = %v", err)
	}

	// An ID that belongs to another project is never reused.
	_, other := seedProjectVersionsNamed(t, gdb, "other")
	otherRec, err := svc.RecordPublication(ctx, u.ID, other.ID, 1, PublicationRecord{RegistryHost: "quay.io", Repository: "x/y", Tag: "1"})
	if err != nil {
		t.Fatal(err)
	}
	rec, err := svc.RecordPublication(ctx, u.ID, p.ID, 1, PublicationRecord{ID: otherRec.ID, RegistryHost: "quay.io", Repository: "x/y", Tag: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if *rec.ID == *otherRec.ID {
		t.Fatal("reused another project's publication ID")
	}
	byVersion, _ := svc.PublicationsByVersion(ctx, other.ID)
	if len(byVersion[1]) != 1 {
		t.Fatalf("other project's record was touched: %+v", byVersion)
	}
}

func seedProjectVersionsNamed(t *testing.T, gdb *gorm.DB, name string) (models.User, models.Project) {
	t.Helper()
	u := models.User{Username: name + "-owner", Email: name + "@example.com"}
	if err := gdb.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	p := models.Project{Name: name, OwnerID: u.ID, Status: models.ProjectStatusReady}
	if err := gdb.Omit("Owner").Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	v := models.ProjectVersion{ProjectID: p.ID, VersionNumber: 1, ManifestContent: "x", LockFileContent: "y", PackageMetadata: "[]", CreatedBy: u.ID}
	if err := gdb.Omit("Project", "Job", "CreatedByUser").Create(&v).Error; err != nil {
		t.Fatal(err)
	}
	return u, p
}
