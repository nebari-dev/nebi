package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"github.com/aktech/darb/internal/models"
	"github.com/glebarez/sqlite"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupInfoTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	err = db.AutoMigrate(&models.ServerConfig{})
	if err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func TestGetInfo_ReturnsServerInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupInfoTestDB(t)

	// Create a server ID in the database
	serverID := "test-server-id-12345"
	config := models.ServerConfig{
		Key:   models.ServerConfigKeyServerID,
		Value: serverID,
	}
	db.Create(&config)

	// Set up handler
	handler := NewInfoHandler(db)

	// Create test request
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/info", nil)

	// Call handler
	handler.GetInfo(c)

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response InfoResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if response.ServerID != serverID {
		t.Errorf("expected server ID %s, got %s", serverID, response.ServerID)
	}

	if response.GoVersion != runtime.Version() {
		t.Errorf("expected go version %s, got %s", runtime.Version(), response.GoVersion)
	}

	if response.OS != runtime.GOOS {
		t.Errorf("expected OS %s, got %s", runtime.GOOS, response.OS)
	}

	if response.Arch != runtime.GOARCH {
		t.Errorf("expected arch %s, got %s", runtime.GOARCH, response.Arch)
	}
}

func TestGetInfo_ErrorsWhenServerIDNotInitialized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupInfoTestDB(t)

	// Don't create a server ID - simulate uninitialized state

	handler := NewInfoHandler(db)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/info", nil)

	handler.GetInfo(c)

	// Should return 500 error
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}
}
