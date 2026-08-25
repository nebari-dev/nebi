package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/config"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/limits"
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
	dsn := dbPath + "?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}
	sqlDB.SetMaxOpenConns(30)
	sqlDB.SetMaxIdleConns(5)

	if err := db.AutoMigrate(
		&models.User{},
		&models.FederatedIdentity{},
		&models.FederatedIdentityReview{},
		&models.Role{},
		&models.Workspace{},
		&models.Job{},
		&models.Permission{},
		&models.WorkspaceVersion{},
		&models.WorkspaceTag{},
		&models.AuditLog{},
		&models.Package{},
		&models.OCIRegistry{},
		&models.Publication{},
		&models.Group{},
		&models.GroupMember{},
		&models.GroupPermission{},
		&models.ResourceLock{},
		&models.ResourceMetric{},
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

	svc := New(db, q, exec, isLocal, nil, rbac.NewDefaultProvider(), limits.Defaults())
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

func TestCreate_RejectsOversizedPixiTomlBeforeWrites(t *testing.T) {
	svc, db := testSetup(t, true)
	limitCfg := limits.Defaults()
	limitCfg.ManifestBytes = 8
	svc.limits = limitCfg
	userID := createTestUser(t, db, "alice")

	_, err := svc.Create(context.Background(), CreateRequest{
		Name:     "too-big",
		PixiToml: strings.Repeat("x", 9),
	}, userID)

	var ve *ValidationError
	if !isValidationError(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}

	var workspaces, jobs int64
	db.Model(&models.Workspace{}).Count(&workspaces)
	db.Model(&models.Job{}).Count(&jobs)
	if workspaces != 0 || jobs != 0 {
		t.Fatalf("expected no workspace/job writes, got workspaces=%d jobs=%d", workspaces, jobs)
	}
}

func TestCreate_AllowsManifestWithManyPackages(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")

	var sb strings.Builder
	sb.WriteString("[project]\nname = \"many-manifest-packages\"\n\n[dependencies]\n")
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&sb, "pkg-%d = \"*\"\n", i)
	}

	if _, err := svc.Create(context.Background(), CreateRequest{PixiToml: sb.String()}, userID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var workspaces int64
	db.Model(&models.Workspace{}).Count(&workspaces)
	if workspaces != 1 {
		t.Fatalf("expected 1 workspace, got %d", workspaces)
	}
}

func TestCreate_PixiTomlDoesNotConsumeGenericMetadataLimit(t *testing.T) {
	svc, db := testSetup(t, true)
	limitCfg := limits.Defaults()
	limitCfg.ManifestBytes = 256
	limitCfg.MetadataBytes = 64
	svc.limits = limitCfg
	userID := createTestUser(t, db, "alice")

	manifest := "[project]\nname = \"metadata-limit\"\n# " + strings.Repeat("x", 64) + "\n"
	if _, err := svc.Create(context.Background(), CreateRequest{PixiToml: manifest}, userID); err != nil {
		t.Fatalf("expected pixi_toml to be governed by manifest limit, got %v", err)
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

func TestDelete_BypassesActiveJobQuotas(t *testing.T) {
	svc, db := testSetup(t, true)
	limitCfg := limits.Defaults()
	limitCfg.ActiveJobsGlobal = 1
	limitCfg.ActiveJobsPerUser = 1
	limitCfg.ActiveJobsPerWorkspace = 1
	svc.limits = limitCfg

	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "delete-quota", userID)
	if err := db.Model(&models.Job{}).Where("workspace_id = ?", ws.ID).Update("status", models.JobStatusCompleted).Error; err != nil {
		t.Fatalf("complete setup jobs: %v", err)
	}
	if err := db.Create(&models.Job{
		WorkspaceID: ws.ID,
		UserID:      userID,
		Type:        models.JobTypeUpdate,
		Status:      models.JobStatusPending,
	}).Error; err != nil {
		t.Fatalf("create active job: %v", err)
	}

	if err := svc.Delete(context.Background(), ws.ID.String(), userID); err != nil {
		t.Fatalf("delete should bypass active job quotas: %v", err)
	}

	var deleteJobs int64
	db.Model(&models.Job{}).
		Where("workspace_id = ? AND type = ? AND status = ?", ws.ID, models.JobTypeDelete, models.JobStatusPending).
		Count(&deleteJobs)
	if deleteJobs != 1 {
		t.Fatalf("expected one pending delete job, got %d", deleteJobs)
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

func TestPushVersion_RejectsOversizedLockBeforeVersionWrite(t *testing.T) {
	svc, db := testSetup(t, true)
	limitCfg := limits.Defaults()
	limitCfg.LockBytes = 8
	svc.limits = limitCfg
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "push-lock-limit", userID)

	_, err := svc.PushVersion(context.Background(), ws.ID.String(), PushRequest{
		PixiToml: "[project]\nname = \"test\"",
		PixiLock: strings.Repeat("x", 9),
	}, userID)

	var ve *ValidationError
	if !isValidationError(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}

	var versions int64
	db.Model(&models.WorkspaceVersion{}).Where("workspace_id = ?", ws.ID).Count(&versions)
	if versions != 0 {
		t.Fatalf("expected no version writes, got %d", versions)
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

func TestSavePixiToml_RejectsOversizedBeforeWrite(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "toml-limit", userID)
	limitCfg := limits.Defaults()
	limitCfg.ManifestBytes = 8
	svc.limits = limitCfg

	wsPath := svc.executor.GetWorkspacePath(ws)
	if err := os.MkdirAll(wsPath, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	manifestPath := filepath.Join(wsPath, "pixi.toml")
	original := "[project]\nname = \"toml-limit\"\n"
	if err := os.WriteFile(manifestPath, []byte(original), 0o644); err != nil {
		t.Fatalf("write original manifest: %v", err)
	}

	err := svc.SavePixiToml(ws.ID.String(), strings.Repeat("x", 9))

	var ve *ValidationError
	if !isValidationError(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
	got, readErr := os.ReadFile(manifestPath)
	if readErr != nil {
		t.Fatalf("read manifest: %v", readErr)
	}
	if string(got) != original {
		t.Fatalf("expected existing manifest unchanged, got %q", string(got))
	}
}

func TestGetPixiToml_UsesPersistedPathForManagedWorkspace(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")
	ws := createReadyWorkspace(t, svc, db, "persisted-path", userID)

	wsPath := svc.executor.GetWorkspacePath(ws)
	if err := os.MkdirAll(wsPath, 0o755); err != nil {
		t.Fatalf("mkdir ws path: %v", err)
	}

	content := "[workspace]\nname = \"persisted-path\"\n"
	if err := os.WriteFile(filepath.Join(wsPath, "pixi.toml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write pixi.toml: %v", err)
	}

	// Simulate worker-persisted path in DB.
	if err := db.Model(&models.Workspace{}).Where("id = ?", ws.ID).Update("path", wsPath).Error; err != nil {
		t.Fatalf("persist path: %v", err)
	}

	// Simulate a later process with a different configured base dir.
	cfg := &config.Config{Storage: config.StorageConfig{WorkspacesDir: t.TempDir()}}
	exec2, err := executor.NewLocalExecutor(cfg)
	if err != nil {
		t.Fatalf("new executor: %v", err)
	}
	q := queue.NewMemoryQueue(10)
	t.Cleanup(func() { q.Close() })
	svc2 := New(db, q, exec2, true, nil, rbac.NewDefaultProvider(), limits.Defaults())

	got, err := svc2.GetPixiToml(ws.ID.String())
	if err != nil {
		t.Fatalf("get pixi.toml via second executor: %v", err)
	}
	if got != content {
		t.Errorf("persisted-path read mismatch: got %q, want %q", got, content)
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

	if _, err := svc.PushVersion(context.Background(), ws.ID.String(), PushRequest{
		Tag:      "latest",
		PixiToml: "[project]\nname = \"tag-test\"\n",
	}, userID); err != nil {
		t.Fatalf("push version: %v", err)
	}

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
	var ve *ValidationError
	ok := errors.As(err, &ve)
	if ok && target != nil {
		*target = ve
	}
	return ok
}

func isConflictError(err error, target **ConflictError) bool {
	var ce *ConflictError
	ok := errors.As(err, &ce)
	if ok && target != nil {
		*target = ce
	}
	return ok
}

func TestCreate_ForwardsImportStagingDirToJob(t *testing.T) {
	svc, db := testSetup(t, true)
	userID := createTestUser(t, db, "alice")

	ws, err := svc.Create(context.Background(), CreateRequest{
		Name:             "import-seeded",
		ImportStagingDir: "/tmp/fake-staging-path",
	}, userID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	var job models.Job
	if err := db.Where("workspace_id = ? AND type = ?", ws.ID, models.JobTypeCreate).First(&job).Error; err != nil {
		t.Fatalf("find create job: %v", err)
	}
	got, _ := job.Metadata["import_staging_dir"].(string)
	if got != "/tmp/fake-staging-path" {
		t.Errorf("job metadata import_staging_dir = %q, want %q", got, "/tmp/fake-staging-path")
	}
}

func TestWorkspaceService_IsLocal(t *testing.T) {
	localSvc, _ := testSetup(t, true)
	if !localSvc.IsLocal() {
		t.Error("expected IsLocal()=true when service constructed with isLocal=true")
	}

	teamSvc, _ := testSetup(t, false)
	if teamSvc.IsLocal() {
		t.Error("expected IsLocal()=false when service constructed with isLocal=false")
	}
}
