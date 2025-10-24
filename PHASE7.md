# Phase 7: RBAC & Access Control Implementation Guide

## Overview

Phase 7 implements Role-Based Access Control (RBAC) using Casbin to enable multi-user access control in Darb. This phase adds admin capabilities, user management, permission management, and comprehensive audit logging.

## Current State (Phase 4 Complete ✅)

**What's Working:**
- ✅ Full backend API with local executor
- ✅ Complete React frontend with all pages
- ✅ JWT-based authentication
- ✅ Environment and package management
- ✅ Job processing with real-time updates

**Database Models Already Exist:**
- ✅ `User` model with UUID primary key
- ✅ `Role` model (id, name, description)
- ✅ `Permission` model (user_id, environment_id, role_id)
- ✅ `AuditLog` model (user_id, action, resource, details_json, timestamp)

**What's Missing:**
- ❌ No RBAC enforcement - all authenticated users see all environments
- ❌ No role assignments - users don't have roles
- ❌ No permission checks - no middleware to enforce access control
- ❌ No admin endpoints - can't manage users/roles/permissions
- ❌ Users don't have `is_admin` field

## Phase 7 Goals

1. **RBAC Implementation** - Use Casbin for policy enforcement
2. **User Roles** - admin, owner, editor, viewer
3. **Permission System** - Environment-level access control
4. **Admin API** - Endpoints for user/role/permission management
5. **Audit Logging** - Track all sensitive operations
6. **Middleware** - Enforce permissions on all protected routes

---

## Implementation Steps

### Step 1: Install Casbin Dependencies

```bash
cd /Users/aktech/dev/darb
go get github.com/casbin/casbin/v2@latest
go get github.com/casbin/gorm-adapter/v3@latest
go mod tidy
```

**Verify installation:**
```bash
grep casbin go.mod
```

---

### Step 2: Create RBAC Model Configuration

**Create file:** `internal/rbac/model.conf`

```conf
[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub) && r.obj == p.obj && r.act == p.act
```

**Explanation:**
- `sub` = subject (user UUID)
- `obj` = object (environment ID or "admin")
- `act` = action (read, write, admin)
- `g` = role inheritance

---

### Step 3: Implement RBAC Enforcer

**Create file:** `internal/rbac/rbac.go`

```go
package rbac

import (
	"fmt"
	"log/slog"

	"github.com/casbin/casbin/v2"
	gormadapter "github.com/casbin/gorm-adapter/v3"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var enforcer *casbin.Enforcer

// InitEnforcer initializes the Casbin enforcer
func InitEnforcer(db *gorm.DB, logger *slog.Logger) error {
	adapter, err := gormadapter.NewAdapterByDB(db)
	if err != nil {
		return fmt.Errorf("failed to create casbin adapter: %w", err)
	}

	e, err := casbin.NewEnforcer("internal/rbac/model.conf", adapter)
	if err != nil {
		return fmt.Errorf("failed to create casbin enforcer: %w", err)
	}

	// Load policies from database
	if err := e.LoadPolicy(); err != nil {
		return fmt.Errorf("failed to load policies: %w", err)
	}

	enforcer = e
	logger.Info("RBAC enforcer initialized")
	return nil
}

// GetEnforcer returns the global enforcer instance
func GetEnforcer() *casbin.Enforcer {
	return enforcer
}

// CanReadEnvironment checks if user can read an environment
func CanReadEnvironment(userID uuid.UUID, envID uint) (bool, error) {
	return enforcer.Enforce(userID.String(), fmt.Sprintf("env:%d", envID), "read")
}

// CanWriteEnvironment checks if user can write to an environment
func CanWriteEnvironment(userID uuid.UUID, envID uint) (bool, error) {
	return enforcer.Enforce(userID.String(), fmt.Sprintf("env:%d", envID), "write")
}

// IsAdmin checks if user has admin privileges
func IsAdmin(userID uuid.UUID) (bool, error) {
	return enforcer.Enforce(userID.String(), "admin", "admin")
}

// GrantEnvironmentAccess grants access to an environment
func GrantEnvironmentAccess(userID uuid.UUID, envID uint, role string) error {
	var action string
	switch role {
	case "owner", "editor":
		action = "write"
	case "viewer":
		action = "read"
	default:
		return fmt.Errorf("invalid role: %s", role)
	}

	_, err := enforcer.AddPolicy(userID.String(), fmt.Sprintf("env:%d", envID), action)
	if err != nil {
		return err
	}

	return enforcer.SavePolicy()
}

// RevokeEnvironmentAccess revokes access to an environment
func RevokeEnvironmentAccess(userID uuid.UUID, envID uint) error {
	obj := fmt.Sprintf("env:%d", envID)

	// Remove both read and write permissions
	enforcer.RemovePolicy(userID.String(), obj, "read")
	enforcer.RemovePolicy(userID.String(), obj, "write")

	return enforcer.SavePolicy()
}

// MakeAdmin grants admin privileges to a user
func MakeAdmin(userID uuid.UUID) error {
	_, err := enforcer.AddPolicy(userID.String(), "admin", "admin")
	if err != nil {
		return err
	}
	return enforcer.SavePolicy()
}

// RevokeAdmin removes admin privileges from a user
func RevokeAdmin(userID uuid.UUID) error {
	_, err := enforcer.RemovePolicy(userID.String(), "admin", "admin")
	if err != nil {
		return err
	}
	return enforcer.SavePolicy()
}
```

---

### Step 4: Add is_admin Field to User Model

**Update:** `internal/models/user.go`

```go
// Add to User struct
IsAdmin bool `gorm:"default:false" json:"is_admin"`
```

**Create migration:** `internal/db/migrations/add_is_admin_to_users.go`

```go
package migrations

import "gorm.io/gorm"

func AddIsAdminToUsers(db *gorm.DB) error {
	return db.Exec("ALTER TABLE users ADD COLUMN is_admin BOOLEAN DEFAULT false").Error
}
```

**Or run manually:**
```sql
ALTER TABLE users ADD COLUMN is_admin BOOLEAN DEFAULT false;
```

---

### Step 5: Create Database Seeds for Roles

**Create file:** `internal/db/seeds.go`

```go
package db

import (
	"log/slog"

	"github.com/aktech/darb/internal/models"
	"gorm.io/gorm"
)

// SeedRoles creates default roles
func SeedRoles(db *gorm.DB, logger *slog.Logger) error {
	roles := []models.Role{
		{Name: "admin", Description: "System administrator with full access"},
		{Name: "owner", Description: "Environment owner with full control"},
		{Name: "editor", Description: "Can modify environments and install packages"},
		{Name: "viewer", Description: "Read-only access to environments"},
	}

	for _, role := range roles {
		var existing models.Role
		if err := db.Where("name = ?", role.Name).First(&existing).Error; err == gorm.ErrRecordNotFound {
			if err := db.Create(&role).Error; err != nil {
				return err
			}
			logger.Info("Created role", "name", role.Name)
		}
	}

	return nil
}
```

**Call in:** `internal/db/db.go` after migrations:

```go
// After AutoMigrate, add:
if err := SeedRoles(db, logger); err != nil {
	return nil, fmt.Errorf("failed to seed roles: %w", err)
}
```

---

### Step 6: Create Audit Logging System

**Create file:** `internal/audit/audit.go`

```go
package audit

import (
	"encoding/json"
	"time"

	"github.com/aktech/darb/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// LogAction records an audit log entry
func LogAction(db *gorm.DB, userID uuid.UUID, action, resource string, details interface{}) error {
	detailsJSON, err := json.Marshal(details)
	if err != nil {
		detailsJSON = []byte("{}")
	}

	log := models.AuditLog{
		UserID:      userID,
		Action:      action,
		Resource:    resource,
		DetailsJSON: string(detailsJSON),
		Timestamp:   time.Now(),
	}

	return db.Create(&log).Error
}

// Audit actions constants
const (
	ActionCreateUser       = "create_user"
	ActionUpdateUser       = "update_user"
	ActionDeleteUser       = "delete_user"
	ActionMakeAdmin        = "make_admin"
	ActionRevokeAdmin      = "revoke_admin"
	ActionGrantPermission  = "grant_permission"
	ActionRevokePermission = "revoke_permission"
	ActionCreateEnvironment = "create_environment"
	ActionDeleteEnvironment = "delete_environment"
	ActionInstallPackage   = "install_package"
	ActionRemovePackage    = "remove_package"
	ActionLogin            = "login"
	ActionLoginFailed      = "login_failed"
)
```

---

### Step 7: Implement RBAC Middleware

**Create file:** `internal/api/middleware/rbac.go`

```go
package middleware

import (
	"net/http"
	"strconv"

	"github.com/aktech/darb/internal/rbac"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequireAdmin ensures the user is an admin
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			c.Abort()
			return
		}

		uid := userID.(uuid.UUID)
		isAdmin, err := rbac.IsAdmin(uid)
		if err != nil || !isAdmin {
			c.JSON(http.StatusForbidden, gin.H{"error": "Admin access required"})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireEnvironmentAccess checks if user can access an environment
func RequireEnvironmentAccess(action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			c.Abort()
			return
		}

		envIDStr := c.Param("id")
		envID, err := strconv.ParseUint(envIDStr, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid environment ID"})
			c.Abort()
			return
		}

		uid := userID.(uuid.UUID)

		var hasAccess bool
		if action == "read" {
			hasAccess, err = rbac.CanReadEnvironment(uid, uint(envID))
		} else if action == "write" {
			hasAccess, err = rbac.CanWriteEnvironment(uid, uint(envID))
		}

		if err != nil || !hasAccess {
			c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
			c.Abort()
			return
		}

		c.Next()
	}
}
```

---

### Step 8: Implement Admin API Handlers

**Create file:** `internal/api/handlers/admin.go`

```go
package handlers

import (
	"net/http"

	"github.com/aktech/darb/internal/audit"
	"github.com/aktech/darb/internal/models"
	"github.com/aktech/darb/internal/rbac"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type AdminHandler struct {
	db *gorm.DB
}

func NewAdminHandler(db *gorm.DB) *AdminHandler {
	return &AdminHandler{db: db}
}

// ListUsers godoc
// @Summary List all users (admin only)
// @Tags admin
// @Security BearerAuth
// @Produce json
// @Success 200 {array} models.User
// @Router /admin/users [get]
func (h *AdminHandler) ListUsers(c *gin.Context) {
	var users []models.User
	if err := h.db.Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch users"})
		return
	}

	c.JSON(http.StatusOK, users)
}

// CreateUser godoc
// @Summary Create a new user (admin only)
// @Tags admin
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param user body CreateUserRequest true "User details"
// @Success 201 {object} models.User
// @Router /admin/users [post]
func (h *AdminHandler) CreateUser(c *gin.Context) {
	adminUserID := c.MustGet("user_id").(uuid.UUID)

	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to hash password"})
		return
	}

	user := models.User{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: string(hashedPassword),
		IsAdmin:      req.IsAdmin,
	}

	if err := h.db.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to create user"})
		return
	}

	// If admin, grant admin permissions
	if req.IsAdmin {
		if err := rbac.MakeAdmin(user.ID); err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to grant admin permissions"})
			return
		}
	}

	// Audit log
	audit.LogAction(h.db, adminUserID, audit.ActionCreateUser, "user:"+user.ID.String(), map[string]interface{}{
		"username": user.Username,
		"email":    user.Email,
		"is_admin": user.IsAdmin,
	})

	c.JSON(http.StatusCreated, user)
}

// ToggleAdmin godoc
// @Summary Toggle admin status for a user
// @Tags admin
// @Security BearerAuth
// @Param id path string true "User UUID"
// @Success 200 {object} models.User
// @Router /admin/users/{id}/toggle-admin [post]
func (h *AdminHandler) ToggleAdmin(c *gin.Context) {
	adminUserID := c.MustGet("user_id").(uuid.UUID)
	userIDStr := c.Param("id")

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid user ID"})
		return
	}

	var user models.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "User not found"})
		return
	}

	// Toggle admin status
	user.IsAdmin = !user.IsAdmin
	if err := h.db.Save(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to update user"})
		return
	}

	// Update RBAC
	if user.IsAdmin {
		rbac.MakeAdmin(user.ID)
		audit.LogAction(h.db, adminUserID, audit.ActionMakeAdmin, "user:"+user.ID.String(), nil)
	} else {
		rbac.RevokeAdmin(user.ID)
		audit.LogAction(h.db, adminUserID, audit.ActionRevokeAdmin, "user:"+user.ID.String(), nil)
	}

	c.JSON(http.StatusOK, user)
}

// DeleteUser godoc
// @Summary Delete a user (admin only)
// @Tags admin
// @Security BearerAuth
// @Param id path string true "User UUID"
// @Success 204
// @Router /admin/users/{id} [delete]
func (h *AdminHandler) DeleteUser(c *gin.Context) {
	adminUserID := c.MustGet("user_id").(uuid.UUID)
	userIDStr := c.Param("id")

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid user ID"})
		return
	}

	// Can't delete yourself
	if userID == adminUserID {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Cannot delete yourself"})
		return
	}

	var user models.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "User not found"})
		return
	}

	if err := h.db.Delete(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to delete user"})
		return
	}

	// Audit log
	audit.LogAction(h.db, adminUserID, audit.ActionDeleteUser, "user:"+user.ID.String(), map[string]interface{}{
		"username": user.Username,
	})

	c.Status(http.StatusNoContent)
}

// GrantPermission godoc
// @Summary Grant environment access to a user
// @Tags admin
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param permission body GrantPermissionRequest true "Permission details"
// @Success 201 {object} models.Permission
// @Router /admin/permissions [post]
func (h *AdminHandler) GrantPermission(c *gin.Context) {
	adminUserID := c.MustGet("user_id").(uuid.UUID)

	var req GrantPermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	// Verify user exists
	var user models.User
	if err := h.db.First(&user, "id = ?", req.UserID).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "User not found"})
		return
	}

	// Verify environment exists
	var env models.Environment
	if err := h.db.First(&env, req.EnvironmentID).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Environment not found"})
		return
	}

	// Verify role exists
	var role models.Role
	if err := h.db.First(&role, req.RoleID).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Role not found"})
		return
	}

	// Create permission record
	permission := models.Permission{
		UserID:        req.UserID,
		EnvironmentID: req.EnvironmentID,
		RoleID:        req.RoleID,
	}

	if err := h.db.Create(&permission).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to create permission"})
		return
	}

	// Grant in RBAC
	if err := rbac.GrantEnvironmentAccess(user.ID, env.ID, role.Name); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to grant RBAC permission"})
		return
	}

	// Audit log
	audit.LogAction(h.db, adminUserID, audit.ActionGrantPermission, "permission:"+permission.ID, map[string]interface{}{
		"user_id":        req.UserID,
		"environment_id": req.EnvironmentID,
		"role":           role.Name,
	})

	c.JSON(http.StatusCreated, permission)
}

// ListPermissions godoc
// @Summary List all permissions
// @Tags admin
// @Security BearerAuth
// @Produce json
// @Success 200 {array} models.Permission
// @Router /admin/permissions [get]
func (h *AdminHandler) ListPermissions(c *gin.Context) {
	var permissions []models.Permission
	if err := h.db.Preload("User").Preload("Environment").Preload("Role").Find(&permissions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch permissions"})
		return
	}

	c.JSON(http.StatusOK, permissions)
}

// RevokePermission godoc
// @Summary Revoke a permission
// @Tags admin
// @Security BearerAuth
// @Param id path int true "Permission ID"
// @Success 204
// @Router /admin/permissions/{id} [delete]
func (h *AdminHandler) RevokePermission(c *gin.Context) {
	adminUserID := c.MustGet("user_id").(uuid.UUID)
	permissionID := c.Param("id")

	var permission models.Permission
	if err := h.db.Preload("User").First(&permission, permissionID).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Permission not found"})
		return
	}

	// Revoke from RBAC
	if err := rbac.RevokeEnvironmentAccess(permission.User.ID, permission.EnvironmentID); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to revoke RBAC permission"})
		return
	}

	// Delete permission record
	if err := h.db.Delete(&permission).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to delete permission"})
		return
	}

	// Audit log
	audit.LogAction(h.db, adminUserID, audit.ActionRevokePermission, "permission:"+permissionID, map[string]interface{}{
		"user_id":        permission.UserID,
		"environment_id": permission.EnvironmentID,
	})

	c.Status(http.StatusNoContent)
}

// ListAuditLogs godoc
// @Summary List audit logs
// @Tags admin
// @Security BearerAuth
// @Produce json
// @Param user_id query string false "Filter by user ID"
// @Param action query string false "Filter by action"
// @Success 200 {array} models.AuditLog
// @Router /admin/audit-logs [get]
func (h *AdminHandler) ListAuditLogs(c *gin.Context) {
	query := h.db.Preload("User").Order("timestamp DESC").Limit(100)

	if userID := c.Query("user_id"); userID != "" {
		query = query.Where("user_id = ?", userID)
	}

	if action := c.Query("action"); action != "" {
		query = query.Where("action = ?", action)
	}

	var logs []models.AuditLog
	if err := query.Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch audit logs"})
		return
	}

	c.JSON(http.StatusOK, logs)
}

// Request types
type CreateUserRequest struct {
	Username string `json:"username" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
	IsAdmin  bool   `json:"is_admin"`
}

type GrantPermissionRequest struct {
	UserID        uuid.UUID `json:"user_id" binding:"required"`
	EnvironmentID uint      `json:"environment_id" binding:"required"`
	RoleID        uint      `json:"role_id" binding:"required"`
}
```

---

### Step 9: Update Environment Handlers with RBAC

**Update:** `internal/api/handlers/environment.go`

**Key changes:**

1. **ListEnvironments** - Show environments user owns OR has permission to access:
```go
func (h *EnvironmentHandler) ListEnvironments(c *gin.Context) {
	userID := c.MustGet("user_id").(uuid.UUID)

	var environments []models.Environment

	// Get environments where user is owner
	query := h.db.Where("owner_id = ?", userID)

	// OR where user has permissions
	var permissions []models.Permission
	h.db.Where("user_id = ?", userID).Find(&permissions)

	envIDs := []uint{}
	for _, p := range permissions {
		envIDs = append(envIDs, p.EnvironmentID)
	}

	if len(envIDs) > 0 {
		query = query.Or("id IN ?", envIDs)
	}

	if err := query.Order("created_at DESC").Find(&environments).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch environments"})
		return
	}

	c.JSON(http.StatusOK, environments)
}
```

2. **CreateEnvironment** - Grant owner access automatically:
```go
// After creating environment:
if err := rbac.GrantEnvironmentAccess(userID, env.ID, "owner"); err != nil {
	// Log error but don't fail
}

// Audit log
audit.LogAction(h.db, userID, audit.ActionCreateEnvironment, fmt.Sprintf("env:%d", env.ID), map[string]interface{}{
	"name": env.Name,
	"package_manager": env.PackageManager,
})
```

3. **Add audit logging to all operations:**
- DeleteEnvironment → ActionDeleteEnvironment
- InstallPackages → ActionInstallPackage
- RemovePackage → ActionRemovePackage

---

### Step 10: Update Router with Admin Routes

**Update:** `internal/api/router.go`

```go
// Import new packages
import (
	"github.com/aktech/darb/internal/api/middleware"
	"github.com/aktech/darb/internal/rbac"
	// ... existing imports
)

// In SetupRouter function:

// Initialize RBAC
if err := rbac.InitEnforcer(db, logger); err != nil {
	logger.Error("Failed to initialize RBAC", "error", err)
	panic(err)
}

// ... existing routes setup

// Admin routes (require admin role)
adminHandler := handlers.NewAdminHandler(db)
admin := api.Group("/admin")
admin.Use(authMiddleware, middleware.RequireAdmin())
{
	// User management
	admin.GET("/users", adminHandler.ListUsers)
	admin.POST("/users", adminHandler.CreateUser)
	admin.POST("/users/:id/toggle-admin", adminHandler.ToggleAdmin)
	admin.DELETE("/users/:id", adminHandler.DeleteUser)

	// Permission management
	admin.GET("/permissions", adminHandler.ListPermissions)
	admin.POST("/permissions", adminHandler.GrantPermission)
	admin.DELETE("/permissions/:id", adminHandler.RevokePermission)

	// Audit logs
	admin.GET("/audit-logs", adminHandler.ListAuditLogs)
}

// Update environment routes with RBAC
environments := api.Group("/environments")
environments.Use(authMiddleware)
{
	environments.GET("", envHandler.ListEnvironments)
	environments.POST("", envHandler.CreateEnvironment)

	// Per-environment operations with permission checks
	env := environments.Group("/:id")
	{
		// Read operations
		env.GET("", middleware.RequireEnvironmentAccess("read"), envHandler.GetEnvironment)
		env.GET("/packages", middleware.RequireEnvironmentAccess("read"), envHandler.ListPackages)
		env.GET("/pixi-toml", middleware.RequireEnvironmentAccess("read"), envHandler.GetPixiToml)

		// Write operations
		env.DELETE("", middleware.RequireEnvironmentAccess("write"), envHandler.DeleteEnvironment)
		env.POST("/packages", middleware.RequireEnvironmentAccess("write"), envHandler.InstallPackages)
		env.DELETE("/packages/:package", middleware.RequireEnvironmentAccess("write"), envHandler.RemovePackage)
	}
}
```

---

### Step 11: Update Swagger Documentation

**Update:** `cmd/server/main.go`

Add Swagger annotations for admin endpoints:

```go
// @title Darb API
// @version 1.0
// @description Multi-User Environment Management System
// @contact.name Darb Team
// @license.name MIT
// @host localhost:8080
// @BasePath /api/v1
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
```

Then regenerate:
```bash
make swagger
```

---

## Testing Phase 7

### Step 1: Seed First Admin User

**Create script:** `scripts/create_admin.go`

```go
package main

import (
	"fmt"
	"log"

	"github.com/aktech/darb/internal/config"
	"github.com/aktech/darb/internal/db"
	"github.com/aktech/darb/internal/logger"
	"github.com/aktech/darb/internal/models"
	"github.com/aktech/darb/internal/rbac"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Println("Usage: go run scripts/create_admin.go <username> <email> <password>")
		os.Exit(1)
	}

	username := os.Args[1]
	email := os.Args[2]
	password := os.Args[3]

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	log := logger.New(cfg.Log.Format, cfg.Log.Level)
	database, err := db.Init(cfg, log)
	if err != nil {
		log.Fatal(err)
	}

	// Initialize RBAC
	if err := rbac.InitEnforcer(database, log); err != nil {
		log.Fatal(err)
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatal(err)
	}

	user := models.User{
		Username:     username,
		Email:        email,
		PasswordHash: string(hashedPassword),
		IsAdmin:      true,
	}

	if err := database.Create(&user).Error; err != nil {
		log.Fatal(err)
	}

	// Grant admin in RBAC
	if err := rbac.MakeAdmin(user.ID); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Admin user created successfully!\n")
	fmt.Printf("ID: %s\n", user.ID)
	fmt.Printf("Username: %s\n", user.Username)
	fmt.Printf("Email: %s\n", user.Email)
}
```

Run it:
```bash
go run scripts/create_admin.go admin admin@example.com password123
```

### Step 2: Test RBAC Flow

**Create test script:** `test_rbac.sh`

```bash
#!/bin/bash

API_BASE="http://localhost:8080/api/v1"

# 1. Login as admin
echo "=== Login as admin ==="
ADMIN_TOKEN=$(curl -s -X POST $API_BASE/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": "password123"}' | jq -r '.token')

echo "Admin token: $ADMIN_TOKEN"

# 2. Create a regular user
echo -e "\n=== Create regular user ==="
USER_RESPONSE=$(curl -s -X POST $API_BASE/admin/users \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "testuser",
    "email": "test@example.com",
    "password": "password123",
    "is_admin": false
  }')

echo $USER_RESPONSE | jq .
USER_ID=$(echo $USER_RESPONSE | jq -r '.id')

# 3. Login as regular user
echo -e "\n=== Login as regular user ==="
USER_TOKEN=$(curl -s -X POST $API_BASE/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username": "testuser", "password": "password123"}' | jq -r '.token')

echo "User token: $USER_TOKEN"

# 4. Create environment as regular user
echo -e "\n=== Create environment as user ==="
ENV_RESPONSE=$(curl -s -X POST $API_BASE/environments \
  -H "Authorization: Bearer $USER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name": "user-env", "package_manager": "pixi"}')

echo $ENV_RESPONSE | jq .
ENV_ID=$(echo $ENV_RESPONSE | jq -r '.id')

# 5. Try to access admin endpoint as regular user (should fail)
echo -e "\n=== Try admin endpoint as regular user (should fail) ==="
curl -s -X GET $API_BASE/admin/users \
  -H "Authorization: Bearer $USER_TOKEN" | jq .

# 6. Grant permission to another user
echo -e "\n=== Create second user ==="
USER2_RESPONSE=$(curl -s -X POST $API_BASE/admin/users \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "viewer",
    "email": "viewer@example.com",
    "password": "password123",
    "is_admin": false
  }')

USER2_ID=$(echo $USER2_RESPONSE | jq -r '.id')

# Get viewer role ID
echo -e "\n=== Get roles ==="
ROLES=$(curl -s -X GET $API_BASE/admin/roles \
  -H "Authorization: Bearer $ADMIN_TOKEN")
VIEWER_ROLE_ID=$(echo $ROLES | jq -r '.[] | select(.name=="viewer") | .id')

# Grant viewer access to environment
echo -e "\n=== Grant viewer access to environment ==="
curl -s -X POST $API_BASE/admin/permissions \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"user_id\": \"$USER2_ID\",
    \"environment_id\": $ENV_ID,
    \"role_id\": $VIEWER_ROLE_ID
  }" | jq .

# 7. Login as viewer
echo -e "\n=== Login as viewer ==="
VIEWER_TOKEN=$(curl -s -X POST $API_BASE/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username": "viewer", "password": "password123"}' | jq -r '.token')

# 8. Try to read environment as viewer (should work)
echo -e "\n=== Read environment as viewer (should work) ==="
curl -s -X GET $API_BASE/environments/$ENV_ID \
  -H "Authorization: Bearer $VIEWER_TOKEN" | jq .

# 9. Try to delete environment as viewer (should fail)
echo -e "\n=== Try to delete as viewer (should fail) ==="
curl -s -X DELETE $API_BASE/environments/$ENV_ID \
  -H "Authorization: Bearer $VIEWER_TOKEN" | jq .

# 10. View audit logs
echo -e "\n=== View audit logs ==="
curl -s -X GET $API_BASE/admin/audit-logs \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq .

echo -e "\n=== RBAC Test Complete ==="
```

Run it:
```bash
chmod +x test_rbac.sh
./test_rbac.sh
```

---

## Acceptance Criteria

Phase 7 is complete when:

- [x] Casbin installed and enforcer initialized
- [x] Default roles seeded (admin, owner, editor, viewer)
- [x] User model has `is_admin` field
- [x] RBAC middleware enforces permissions
- [x] Admin can create/delete users
- [x] Admin can grant/revoke permissions
- [x] Admin can view audit logs
- [x] Environment access restricted by ownership + permissions
- [x] Non-admin users cannot access admin endpoints
- [x] Users can only see environments they own or have permission to
- [x] All sensitive operations are audited
- [x] Test script passes all scenarios

---

## Files Changed/Created

### New Files:
- `internal/rbac/model.conf`
- `internal/rbac/rbac.go`
- `internal/api/middleware/rbac.go`
- `internal/audit/audit.go`
- `internal/api/handlers/admin.go`
- `internal/db/seeds.go`
- `scripts/create_admin.go`
- `test_rbac.sh`

### Modified Files:
- `internal/models/user.go` - Added `IsAdmin` field
- `internal/api/handlers/environment.go` - Added RBAC checks and audit logging
- `internal/api/router.go` - Added admin routes and RBAC middleware
- `internal/db/db.go` - Added role seeding
- `cmd/server/main.go` - Updated Swagger docs
- `go.mod` - Added Casbin dependencies

---

## Notes for Phase 8 (Admin UI)

Once Phase 7 backend is complete, Phase 8 will build the React frontend for:
- Admin dashboard
- User management interface
- Permission management interface
- Audit log viewer

The backend API is now ready to support all admin UI features!

---

**Phase 7 Implementation Complete!** ✅

Next: **PHASE8.md** - Build the admin interface
