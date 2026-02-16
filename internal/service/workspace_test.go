package service

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/config"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/queue"
	"github.com/nebari-dev/nebi/internal/rbac"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// testSetup creates an in-memory DB, migrates models, initializes RBAC,
// and returns a WorkspaceService ready for testing.
func testSetup(t *testing.T, isLocal bool) (*WorkspaceService, *gorm.DB) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	if err := db.AutoMigrate(
		&models.User{},
		&models.Role{},
		&models.Workspace{},
		&models.Job{},
		&models.Permission{},
		&models.WorkspaceVersion{},
		&models.WorkspaceTag{},
		&models.AuditLog{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// RBAC enforcer is global — initialize per test
	if err := rbac.InitEnforcer(db, slog.Default()); err != nil {
		t.Fatalf("init rbac: %v", err)
	}

	q := queue.NewMemoryQueue(100)
	t.Cleanup(func() { q.Close() })

	dir := t.TempDir()
	cfg := &config.Config{
		Storage: config.StorageConfig{WorkspacesDir: dir},
	}
	exec, err := executor.NewLocalExecutor(cfg)
	if err != nil {
		t.Fatalf("new executor: %v", err)
	}

	svc := New(db, q, exec, isLocal)
	return svc, db
}

// createTestUser inserts a user and returns its ID.
func createTestUser(t *testing.T, db *gorm.DB, username string) uuid.UUID {
	t.Helper()
	user := models.User{Username: username, Email: username + "@test.com"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return user.ID
}

// createReadyWorkspace is a shortcut that creates a workspace and marks it ready.
func createReadyWorkspace(t *testing.T, svc *WorkspaceService, db *gorm.DB, name string, userID uuid.UUID) *models.Workspace {
	t.Helper()
	ws, err := svc.Create(context.Background(), CreateRequest{Name: name}, userID)
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	db.Model(ws).Update("status", models.WsStatusReady)
	ws.Status = models.WsStatusReady
	return ws
}

// --- Create validation tests ---

func TestCreate_Defaults(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")

	ws, err := svc.Create(context.Background(), CreateRequest{Name: "test-ws"}, userID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ws.PackageManager != "pixi" {
		t.Errorf("expected default package_manager=pixi, got %q", ws.PackageManager)
	}
	if ws.Status != models.WsStatusPending {
		t.Errorf("expected status=pending, got %q", ws.Status)
	}
}

func TestCreate_InvalidSource(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")

	_, err := svc.Create(context.Background(), CreateRequest{
		Name:   "bad",
		Source: "invalid",
	}, userID)

	if err == nil {
		t.Fatal("expected error for invalid source")
	}
	var ve *ValidationError
	if !isValidationError(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
}

func TestCreate_LocalSourceRejectedInTeamMode(t *testing.T) {
	svc, db := testSetup(t, false) // team mode
	userID := createTestUser(t, db, "alice")

	_, err := svc.Create(context.Background(), CreateRequest{
		Name:   "local-ws",
		Source: "local",
		Path:   "/tmp/some/path",
	}, userID)

	if err == nil {
		t.Fatal("expected error for local source in team mode")
	}
	var ve *ValidationError
	if !isValidationError(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
}

func TestCreate_LocalSourceRequiresAbsPath(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")

	_, err := svc.Create(context.Background(), CreateRequest{
		Name:   "local-ws",
		Source: "local",
		Path:   "relative/path",
	}, userID)

	if err == nil {
		t.Fatal("expected error for relative path")
	}
	var ve *ValidationError
	if !isValidationError(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
}

func TestCreate_LocalSourceAccepted(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")

	ws, err := svc.Create(context.Background(), CreateRequest{
		Name:   "local-ws",
		Source: "local",
		Path:   "/tmp/my-project",
	}, userID)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ws.Source != "local" || ws.Path != "/tmp/my-project" {
		t.Errorf("unexpected source=%q path=%q", ws.Source, ws.Path)
	}
}

// --- List tests (local vs team mode) ---

func TestList_LocalModeReturnsAll(t *testing.T) {
	svc, db := testSetup(t, true) // local mode
	alice := createTestUser(t, db, "alice")
	bob := createTestUser(t, db, "bob")

	svc.Create(context.Background(), CreateRequest{Name: "ws-alice"}, alice)
	svc.Create(context.Background(), CreateRequest{Name: "ws-bob"}, bob)

	// In local mode, any user sees all workspaces
	workspaces, err := svc.List(alice)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(workspaces) != 2 {
		t.Errorf("expected 2 workspaces in local mode, got %d", len(workspaces))
	}
}

func TestList_TeamModeFiltersToOwner(t *testing.T) {
	svc, db := testSetup(t, false) // team mode
	alice := createTestUser(t, db, "alice")
	bob := createTestUser(t, db, "bob")

	svc.Create(context.Background(), CreateRequest{Name: "ws-alice"}, alice)
	svc.Create(context.Background(), CreateRequest{Name: "ws-bob"}, bob)

	workspaces, err := svc.List(alice)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(workspaces) != 1 {
		t.Errorf("expected 1 workspace for alice in team mode, got %d", len(workspaces))
	}
	if len(workspaces) > 0 && workspaces[0].Name != "ws-alice" {
		t.Errorf("expected ws-alice, got %q", workspaces[0].Name)
	}
}

// --- Get tests ---

func TestGet_Found(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")

	created, _ := svc.Create(context.Background(), CreateRequest{Name: "test"}, userID)

	ws, err := svc.Get(created.ID.String())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ws.Name != "test" {
		t.Errorf("expected name=test, got %q", ws.Name)
	}
}

func TestGet_NotFound(t *testing.T) {
	svc, _ := testSetup(t, true)

	_, err := svc.Get(uuid.New().String())
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// --- Delete tests ---

func TestDelete_NotFound(t *testing.T) {
	svc, _ := testSetup(t, true)

	err := svc.Delete(context.Background(), uuid.New().String(), uuid.New())
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// --- PushVersion tag conflict tests ---

func TestPushVersion_TagConflictWithoutForce(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "push-test", userID)

	// First push succeeds
	_, err := svc.PushVersion(context.Background(), ws.ID.String(), PushRequest{
		Tag:      "v1",
		PixiToml: "[project]\nname = \"test\"",
	}, userID)
	if err != nil {
		t.Fatalf("first push failed: %v", err)
	}

	// Second push with same tag (no force) should fail with ConflictError
	_, err = svc.PushVersion(context.Background(), ws.ID.String(), PushRequest{
		Tag:      "v1",
		PixiToml: "[project]\nname = \"test-v2\"",
	}, userID)
	if err == nil {
		t.Fatal("expected conflict error")
	}
	var ce *ConflictError
	if !isConflictError(err, &ce) {
		t.Fatalf("expected ConflictError, got %T: %v", err, err)
	}

	// Verify no orphaned version was created (only 1 version should exist)
	var count int64
	db.Model(&models.WorkspaceVersion{}).Where("workspace_id = ?", ws.ID).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 version (no orphan), got %d", count)
	}
}

func TestPushVersion_TagConflictWithForce(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "push-test", userID)

	// First push
	r1, err := svc.PushVersion(context.Background(), ws.ID.String(), PushRequest{
		Tag:      "v1",
		PixiToml: "[project]\nname = \"test\"",
	}, userID)
	if err != nil {
		t.Fatalf("first push: %v", err)
	}

	// Force push same tag
	r2, err := svc.PushVersion(context.Background(), ws.ID.String(), PushRequest{
		Tag:      "v1",
		PixiToml: "[project]\nname = \"test-v2\"",
		Force:    true,
	}, userID)
	if err != nil {
		t.Fatalf("force push: %v", err)
	}
	if r2.VersionNumber <= r1.VersionNumber {
		t.Errorf("expected new version > old version, got %d <= %d", r2.VersionNumber, r1.VersionNumber)
	}

	// Tag should point to new version
	var tag models.WorkspaceTag
	db.Where("workspace_id = ? AND tag = ?", ws.ID, "v1").First(&tag)
	if tag.VersionNumber != r2.VersionNumber {
		t.Errorf("tag should point to version %d, got %d", r2.VersionNumber, tag.VersionNumber)
	}
}

func TestPushVersion_WorkspaceNotReady(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")

	// Create workspace but don't mark it ready (stays pending)
	ws, err := svc.Create(context.Background(), CreateRequest{Name: "pending-ws"}, userID)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	_, err = svc.PushVersion(context.Background(), ws.ID.String(), PushRequest{
		Tag:      "v1",
		PixiToml: "test",
	}, userID)

	var ve *ValidationError
	if !isValidationError(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
}

// --- GetPixiToml / SavePixiToml tests ---

func TestPixiToml_RoundTrip(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "toml-test", userID)

	// Create the workspace directory and file
	wsPath := svc.executor.GetWorkspacePath(ws)
	os.MkdirAll(wsPath, 0755)

	content := "[project]\nname = \"my-project\""
	if err := svc.SavePixiToml(ws.ID.String(), content); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := svc.GetPixiToml(ws.ID.String())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != content {
		t.Errorf("round-trip mismatch: got %q, want %q", got, content)
	}
}

func TestGetPixiToml_NotFoundWorkspace(t *testing.T) {
	svc, _ := testSetup(t, true)

	_, err := svc.GetPixiToml(uuid.New().String())
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetPixiToml_MissingFile(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "no-toml", userID)

	// Workspace dir exists but no pixi.toml
	wsPath := svc.executor.GetWorkspacePath(ws)
	os.MkdirAll(wsPath, 0755)

	_, err := svc.GetPixiToml(ws.ID.String())
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound for missing file, got %v", err)
	}
}

func TestGetPixiToml_PermissionError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping permission test as root")
	}

	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "perm-test", userID)

	// Create file then remove read permission
	wsPath := svc.executor.GetWorkspacePath(ws)
	os.MkdirAll(wsPath, 0755)
	pixiPath := filepath.Join(wsPath, "pixi.toml")
	os.WriteFile(pixiPath, []byte("test"), 0644)
	os.Chmod(pixiPath, 0000)
	t.Cleanup(func() { os.Chmod(pixiPath, 0644) })

	_, err := svc.GetPixiToml(ws.ID.String())
	// Should NOT be ErrNotFound — it's a permission error (→ 500)
	if err == ErrNotFound {
		t.Error("permission error should not be mapped to ErrNotFound")
	}
	if err == nil {
		t.Error("expected error for unreadable file")
	}
}

// --- ListVersions / GetVersion tests ---

func TestListVersions_Empty(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "no-versions", userID)

	versions, err := svc.ListVersions(ws.ID.String())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(versions) != 0 {
		t.Errorf("expected 0 versions, got %d", len(versions))
	}
}

func TestGetVersion_NotFound(t *testing.T) {
	svc, _ := testSetup(t, true)

	_, err := svc.GetVersion(uuid.New().String(), "1")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// --- ListTags tests ---

func TestListTags_Empty(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "no-tags", userID)

	tags, err := svc.ListTags(ws.ID.String())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tags) != 0 {
		t.Errorf("expected 0 tags, got %d", len(tags))
	}
}

func TestListTags_AfterPush(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "tag-test", userID)

	svc.PushVersion(context.Background(), ws.ID.String(), PushRequest{
		Tag:      "latest",
		PixiToml: "test",
	}, userID)

	tags, err := svc.ListTags(ws.ID.String())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Push now creates: content hash tag + "latest" (auto) + "latest" (user, deduped) → 2 unique tags
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags (hash + latest), got %d", len(tags))
	}
	tagNames := map[string]bool{}
	for _, tg := range tags {
		tagNames[tg.Tag] = true
	}
	if !tagNames["latest"] {
		t.Errorf("expected a 'latest' tag, got tags: %v", tagNames)
	}
}

// --- helpers ---

func isValidationError(err error, target **ValidationError) bool {
	ve, ok := err.(*ValidationError)
	if ok && target != nil {
		*target = ve
	}
	return ok
}

func isConflictError(err error, target **ConflictError) bool {
	ce, ok := err.(*ConflictError)
	if ok && target != nil {
		*target = ce
	}
	return ok
}
