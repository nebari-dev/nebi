package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/aktech/darb/internal/models"
	"github.com/aktech/darb/internal/queue"
	"github.com/glebarez/sqlite"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// mockExecutor implements executor.Executor for testing.
type mockExecutor struct {
	envPath string
}

func (m *mockExecutor) CreateEnvironment(_ context.Context, _ *models.Environment, _ io.Writer, _ ...string) error {
	return nil
}
func (m *mockExecutor) InstallPackages(_ context.Context, _ *models.Environment, _ []string, _ io.Writer) error {
	return nil
}
func (m *mockExecutor) RemovePackages(_ context.Context, _ *models.Environment, _ []string, _ io.Writer) error {
	return nil
}
func (m *mockExecutor) DeleteEnvironment(_ context.Context, _ *models.Environment, _ io.Writer) error {
	return nil
}
func (m *mockExecutor) GetEnvironmentPath(_ *models.Environment) string {
	return m.envPath
}

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	err = db.AutoMigrate(
		&models.User{},
		&models.Environment{},
		&models.EnvironmentVersion{},
		&models.OCIRegistry{},
		&models.Publication{},
	)
	if err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

// TestPublishEnvironment_CreatesVersionFromClientContent verifies that when a
// client provides pixi_toml and pixi_lock in the publish request, the handler
// creates a new EnvironmentVersion with that content (rather than reusing the
// latest existing version). This is the test that would have caught the original
// push/pull round-trip bug where pushed content was lost.
func TestPublishEnvironment_CreatesVersionFromClientContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)

	// Create test user
	userID := uuid.New()
	user := models.User{ID: userID, Username: "testuser", Email: "test@test.com", PasswordHash: "x"}
	db.Create(&user)

	// Create environment in "ready" state with an initial version
	envID := uuid.New()
	env := models.Environment{
		ID:             envID,
		Name:           "test-ws",
		OwnerID:        userID,
		Status:         models.EnvStatusReady,
		PackageManager: "pixi",
	}
	db.Create(&env)

	initialVersion := models.EnvironmentVersion{
		EnvironmentID:   envID,
		ManifestContent: "initial pixi.toml",
		LockFileContent: "initial pixi.lock",
		PackageMetadata: "[]",
		CreatedBy:       userID,
		Description:     "Initial creation",
	}
	db.Create(&initialVersion)
	if initialVersion.VersionNumber != 1 {
		t.Fatalf("expected initial version 1, got %d", initialVersion.VersionNumber)
	}

	// Create registry
	registryID := uuid.New()
	registry := models.OCIRegistry{
		ID:        registryID,
		Name:      "test-registry",
		URL:       "localhost:0", // Invalid — OCI publish will fail, which is expected
		CreatedBy: userID,
	}
	db.Create(&registry)

	// Set up mock executor pointing to a temp dir
	tmpDir := t.TempDir()
	exec := &mockExecutor{envPath: tmpDir}

	handler := NewEnvironmentHandler(db, queue.NewMemoryQueue(100), exec)

	// Build the publish request with client-provided content
	clientPixiToml := "[project]\nname = \"test-ws\"\n\n[dependencies]\nscipy = \"*\"\n"
	clientPixiLock := "version: 6\npackages:\n  scipy:\n    version: 1.11.0\n"

	reqBody := PublishRequest{
		RegistryID: registryID,
		Repository: "test-ws",
		Tag:        "v2",
		PixiToml:   clientPixiToml,
		PixiLock:   clientPixiLock,
	}
	body, _ := json.Marshal(reqBody)

	// Set up gin context
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user", &user)
	c.Params = gin.Params{{Key: "id", Value: envID.String()}}
	c.Request = httptest.NewRequest(http.MethodPost, "/environments/"+envID.String()+"/publish", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	// Call handler (OCI publish will fail — that's fine)
	handler.PublishEnvironment(c)

	// --- Assertions ---

	// 1. A new EnvironmentVersion (version 2) should exist with the client content
	var newVersion models.EnvironmentVersion
	err := db.Where("environment_id = ? AND version_number = ?", envID, 2).First(&newVersion).Error
	if err != nil {
		t.Fatalf("expected version 2 to be created, got error: %v", err)
	}

	if newVersion.ManifestContent != clientPixiToml {
		t.Errorf("version ManifestContent mismatch:\n  got:  %q\n  want: %q", newVersion.ManifestContent, clientPixiToml)
	}
	if newVersion.LockFileContent != clientPixiLock {
		t.Errorf("version LockFileContent mismatch:\n  got:  %q\n  want: %q", newVersion.LockFileContent, clientPixiLock)
	}
	if newVersion.CreatedBy != userID {
		t.Errorf("version CreatedBy = %v, want %v", newVersion.CreatedBy, userID)
	}

	// 2. Files should be written to envPath
	tomlBytes, err := os.ReadFile(filepath.Join(tmpDir, "pixi.toml"))
	if err != nil {
		t.Fatalf("pixi.toml not written to envPath: %v", err)
	}
	if string(tomlBytes) != clientPixiToml {
		t.Errorf("pixi.toml on disk mismatch:\n  got:  %q\n  want: %q", string(tomlBytes), clientPixiToml)
	}

	lockBytes, err := os.ReadFile(filepath.Join(tmpDir, "pixi.lock"))
	if err != nil {
		t.Fatalf("pixi.lock not written to envPath: %v", err)
	}
	if string(lockBytes) != clientPixiLock {
		t.Errorf("pixi.lock on disk mismatch:\n  got:  %q\n  want: %q", string(lockBytes), clientPixiLock)
	}

	// 3. The initial version should be unchanged
	var v1 models.EnvironmentVersion
	db.Where("environment_id = ? AND version_number = ?", envID, 1).First(&v1)
	if v1.ManifestContent != "initial pixi.toml" {
		t.Errorf("version 1 was mutated: ManifestContent = %q", v1.ManifestContent)
	}
}

// TestPublishEnvironment_FallbackToLatestVersion verifies backward compatibility:
// when no pixi_toml/pixi_lock is provided, the handler uses the latest existing
// version number (the old behavior).
func TestPublishEnvironment_FallbackToLatestVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupTestDB(t)

	userID := uuid.New()
	user := models.User{ID: userID, Username: "testuser", Email: "test@test.com", PasswordHash: "x"}
	db.Create(&user)

	envID := uuid.New()
	env := models.Environment{
		ID:             envID,
		Name:           "test-ws",
		OwnerID:        userID,
		Status:         models.EnvStatusReady,
		PackageManager: "pixi",
	}
	db.Create(&env)

	// Create two versions — latest is version 2
	v1 := models.EnvironmentVersion{
		EnvironmentID:   envID,
		ManifestContent: "v1 toml",
		LockFileContent: "v1 lock",
		PackageMetadata: "[]",
		CreatedBy:       userID,
	}
	db.Create(&v1)
	v2 := models.EnvironmentVersion{
		EnvironmentID:   envID,
		ManifestContent: "v2 toml",
		LockFileContent: "v2 lock",
		PackageMetadata: "[]",
		CreatedBy:       userID,
	}
	db.Create(&v2)

	registryID := uuid.New()
	registry := models.OCIRegistry{
		ID:        registryID,
		Name:      "test-registry",
		URL:       "localhost:0",
		CreatedBy: userID,
	}
	db.Create(&registry)

	// Write existing files to envPath (since OCI publisher reads from disk)
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "pixi.toml"), []byte("v2 toml"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "pixi.lock"), []byte("v2 lock"), 0644)

	exec := &mockExecutor{envPath: tmpDir}
	handler := NewEnvironmentHandler(db, queue.NewMemoryQueue(100), exec)

	// Publish WITHOUT providing content (old-style request)
	reqBody := PublishRequest{
		RegistryID: registryID,
		Repository: "test-ws",
		Tag:        "v2",
	}
	body, _ := json.Marshal(reqBody)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user", &user)
	c.Params = gin.Params{{Key: "id", Value: envID.String()}}
	c.Request = httptest.NewRequest(http.MethodPost, "/environments/"+envID.String()+"/publish", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.PublishEnvironment(c)

	// No new version should have been created (still only 2 versions)
	var count int64
	db.Model(&models.EnvironmentVersion{}).Where("environment_id = ?", envID).Count(&count)
	if count != 2 {
		t.Errorf("expected 2 versions (no new version created), got %d", count)
	}
}
