package handlers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/aktech/darb/internal/audit"
	"github.com/aktech/darb/internal/executor"
	"github.com/aktech/darb/internal/models"
	"github.com/aktech/darb/internal/queue"
	"github.com/aktech/darb/internal/rbac"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type EnvironmentHandler struct {
	db       *gorm.DB
	queue    queue.Queue
	executor executor.Executor
}

func NewEnvironmentHandler(db *gorm.DB, q queue.Queue, exec executor.Executor) *EnvironmentHandler {
	return &EnvironmentHandler{db: db, queue: q, executor: exec}
}

// ListEnvironments godoc
// @Summary List all environments for the current user
// @Tags environments
// @Security BearerAuth
// @Produce json
// @Success 200 {array} models.Environment
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /environments [get]
func (h *EnvironmentHandler) ListEnvironments(c *gin.Context) {
	userID := getUserID(c)

	var environments []models.Environment

	// Get environments where user is owner
	query := h.db.Where("owner_id = ?", userID)

	// OR where user has permissions
	var permissions []models.Permission
	h.db.Where("user_id = ?", userID).Find(&permissions)

	envIDs := []uuid.UUID{}
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

// CreateEnvironment godoc
// @Summary Create a new environment
// @Tags environments
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param environment body CreateEnvironmentRequest true "Environment details"
// @Success 201 {object} models.Environment
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /environments [post]
func (h *EnvironmentHandler) CreateEnvironment(c *gin.Context) {
	userID := getUserID(c)

	var req CreateEnvironmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	// Default to pixi if not specified
	packageManager := req.PackageManager
	if packageManager == "" {
		packageManager = "pixi"
	}

	// Create environment record
	env := models.Environment{
		Name:           req.Name,
		OwnerID:        userID,
		Status:         models.EnvStatusPending,
		PackageManager: packageManager,
	}

	if err := h.db.Create(&env).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to create environment"})
		return
	}

	// Queue creation job
	metadata := map[string]interface{}{}
	if req.PixiToml != "" {
		metadata["pixi_toml"] = req.PixiToml
	}

	job := &models.Job{
		Type:          models.JobTypeCreate,
		EnvironmentID: env.ID,
		Status:        models.JobStatusPending,
		Metadata:      metadata,
	}

	if err := h.db.Create(job).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to create job"})
		return
	}

	if err := h.queue.Enqueue(c.Request.Context(), job); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to queue job"})
		return
	}

	// Grant owner access automatically
	if err := rbac.GrantEnvironmentAccess(userID, env.ID, "owner"); err != nil {
		// Log error but don't fail
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to grant owner access"})
		return
	}

	// Audit log
	audit.LogAction(h.db, userID, audit.ActionCreateEnvironment, fmt.Sprintf("env:%s", env.ID.String()), map[string]interface{}{
		"name":            env.Name,
		"package_manager": env.PackageManager,
	})

	c.JSON(http.StatusCreated, env)
}

// GetEnvironment godoc
// @Summary Get an environment by ID
// @Tags environments
// @Security BearerAuth
// @Produce json
// @Param id path int true "Environment ID"
// @Success 200 {object} models.Environment
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /environments/{id} [get]
func (h *EnvironmentHandler) GetEnvironment(c *gin.Context) {
	envID := c.Param("id")

	var env models.Environment
	// Note: RBAC middleware already checked access, so just fetch by ID
	if err := h.db.Where("id = ?", envID).First(&env).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Environment not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch environment"})
		return
	}

	c.JSON(http.StatusOK, env)
}

// DeleteEnvironment godoc
// @Summary Delete an environment
// @Tags environments
// @Security BearerAuth
// @Param id path int true "Environment ID"
// @Success 204
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /environments/{id} [delete]
func (h *EnvironmentHandler) DeleteEnvironment(c *gin.Context) {
	userID := getUserID(c)
	envID := c.Param("id")

	var env models.Environment
	// Note: RBAC middleware already checked write access
	if err := h.db.Where("id = ?", envID).First(&env).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Environment not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch environment"})
		return
	}

	// Queue deletion job
	job := &models.Job{
		Type:          models.JobTypeDelete,
		EnvironmentID: env.ID,
		Status:        models.JobStatusPending,
	}

	if err := h.db.Create(job).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to create job"})
		return
	}

	if err := h.queue.Enqueue(c.Request.Context(), job); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to queue job"})
		return
	}

	// Audit log
	audit.LogAction(h.db, userID, audit.ActionDeleteEnvironment, fmt.Sprintf("env:%s", env.ID.String()), map[string]interface{}{
		"name": env.Name,
	})

	c.Status(http.StatusNoContent)
}

// InstallPackages godoc
// @Summary Install packages in an environment
// @Tags environments
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "Environment ID"
// @Param packages body InstallPackagesRequest true "Packages to install"
// @Success 202 {object} models.Job
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /environments/{id}/packages [post]
func (h *EnvironmentHandler) InstallPackages(c *gin.Context) {
	userID := getUserID(c)
	envID := c.Param("id")

	var req InstallPackagesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	var env models.Environment
	// Note: RBAC middleware already checked write access
	if err := h.db.Where("id = ?", envID).First(&env).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Environment not found"})
		return
	}

	// Check if environment is ready
	if env.Status != models.EnvStatusReady {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Environment is not ready"})
		return
	}

	// Queue install job
	job := &models.Job{
		Type:          models.JobTypeInstall,
		EnvironmentID: env.ID,
		Status:        models.JobStatusPending,
		Metadata:      map[string]interface{}{"packages": req.Packages},
	}

	if err := h.db.Create(job).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to create job"})
		return
	}

	if err := h.queue.Enqueue(c.Request.Context(), job); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to queue job"})
		return
	}

	// Audit log
	audit.LogAction(h.db, userID, audit.ActionInstallPackage, fmt.Sprintf("env:%s", env.ID.String()), map[string]interface{}{
		"packages": req.Packages,
	})

	c.JSON(http.StatusAccepted, job)
}

// RemovePackages godoc
// @Summary Remove packages from an environment
// @Tags environments
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "Environment ID"
// @Param package path string true "Package name"
// @Success 202 {object} models.Job
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /environments/{id}/packages/{package} [delete]
func (h *EnvironmentHandler) RemovePackages(c *gin.Context) {
	userID := getUserID(c)
	envID := c.Param("id")
	packageName := c.Param("package")

	var env models.Environment
	// Note: RBAC middleware already checked write access
	if err := h.db.Where("id = ?", envID).First(&env).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Environment not found"})
		return
	}

	// Check if environment is ready
	if env.Status != models.EnvStatusReady {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Environment is not ready"})
		return
	}

	// Queue remove job
	job := &models.Job{
		Type:          models.JobTypeRemove,
		EnvironmentID: env.ID,
		Status:        models.JobStatusPending,
		Metadata:      map[string]interface{}{"packages": []string{packageName}},
	}

	if err := h.db.Create(job).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to create job"})
		return
	}

	if err := h.queue.Enqueue(c.Request.Context(), job); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to queue job"})
		return
	}

	// Audit log
	audit.LogAction(h.db, userID, audit.ActionRemovePackage, fmt.Sprintf("env:%s", env.ID.String()), map[string]interface{}{
		"package": packageName,
	})

	c.JSON(http.StatusAccepted, job)
}

// ListPackages godoc
// @Summary List packages in an environment
// @Tags environments
// @Security BearerAuth
// @Produce json
// @Param id path int true "Environment ID"
// @Success 200 {array} models.Package
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /environments/{id}/packages [get]
func (h *EnvironmentHandler) ListPackages(c *gin.Context) {
	envID := c.Param("id")

	var env models.Environment
	// Note: RBAC middleware already checked read access
	if err := h.db.Where("id = ?", envID).First(&env).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Environment not found"})
		return
	}

	var packages []models.Package
	if err := h.db.Where("environment_id = ?", env.ID).Find(&packages).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch packages"})
		return
	}

	c.JSON(http.StatusOK, packages)
}

// GetPixiToml godoc
// @Summary Get pixi.toml content for an environment
// @Tags environments
// @Security BearerAuth
// @Produce json
// @Param id path int true "Environment ID"
// @Success 200 {object} PixiTomlResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /environments/{id}/pixi-toml [get]
func (h *EnvironmentHandler) GetPixiToml(c *gin.Context) {
	envID := c.Param("id")

	var env models.Environment
	// Note: RBAC middleware already checked read access
	if err := h.db.Where("id = ?", envID).First(&env).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Environment not found"})
		return
	}

	// Get environment path using executor
	envPath := h.executor.GetEnvironmentPath(&env)
	pixiTomlPath := filepath.Join(envPath, "pixi.toml")

	// Read pixi.toml file
	content, err := os.ReadFile(pixiTomlPath)
	if err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "pixi.toml not found"})
		return
	}

	c.JSON(http.StatusOK, PixiTomlResponse{
		Content: string(content),
	})
}

// Request types
type CreateEnvironmentRequest struct {
	Name           string `json:"name" binding:"required"`
	PackageManager string `json:"package_manager"`
	PixiToml       string `json:"pixi_toml"`
}

type PixiTomlResponse struct {
	Content string `json:"content"`
}

type InstallPackagesRequest struct {
	Packages []string `json:"packages" binding:"required"`
}

// ShareEnvironment godoc
// @Summary Share environment with another user (owner only)
// @Tags environments
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Environment ID"
// @Param share body ShareEnvironmentRequest true "Share details"
// @Success 201 {object} models.Permission
// @Router /environments/{id}/share [post]
func (h *EnvironmentHandler) ShareEnvironment(c *gin.Context) {
	ownerID := getUserID(c)
	envID := c.Param("id")

	var req ShareEnvironmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	// Parse environment ID
	envUUID, err := uuid.Parse(envID)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid environment ID"})
		return
	}

	// Get environment and check ownership
	var env models.Environment
	if err := h.db.Where("id = ?", envUUID).First(&env).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Environment not found"})
		return
	}

	// Check if user is the owner
	if env.OwnerID != ownerID {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Only the owner can share this environment"})
		return
	}

	// Verify target user exists
	var targetUser models.User
	if err := h.db.First(&targetUser, "id = ?", req.UserID).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "User not found"})
		return
	}

	// Validate role
	if req.Role != "viewer" && req.Role != "editor" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Role must be 'viewer' or 'editor'"})
		return
	}

	// Get role ID
	var role models.Role
	if err := h.db.Where("name = ?", req.Role).First(&role).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Role not found"})
		return
	}

	// Create permission record
	permission := models.Permission{
		UserID:        req.UserID,
		EnvironmentID: envUUID,
		RoleID:        role.ID,
	}

	if err := h.db.Create(&permission).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to create permission"})
		return
	}

	// Grant in RBAC
	if err := rbac.GrantEnvironmentAccess(req.UserID, envUUID, req.Role); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to grant RBAC permission"})
		return
	}

	// Audit log
	audit.LogAction(h.db, ownerID, audit.ActionGrantPermission, fmt.Sprintf("env:%s", envUUID.String()), map[string]interface{}{
		"target_user_id": req.UserID,
		"role":           req.Role,
	})

	c.JSON(http.StatusCreated, permission)
}

// UnshareEnvironment godoc
// @Summary Revoke user access to environment (owner only)
// @Tags environments
// @Security BearerAuth
// @Param id path string true "Environment ID"
// @Param user_id path string true "User ID to revoke"
// @Success 204
// @Router /environments/{id}/share/{user_id} [delete]
func (h *EnvironmentHandler) UnshareEnvironment(c *gin.Context) {
	ownerID := getUserID(c)
	envID := c.Param("id")
	targetUserID := c.Param("user_id")

	// Parse UUIDs
	envUUID, err := uuid.Parse(envID)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid environment ID"})
		return
	}

	targetUUID, err := uuid.Parse(targetUserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid user ID"})
		return
	}

	// Get environment and check ownership
	var env models.Environment
	if err := h.db.Where("id = ?", envUUID).First(&env).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Environment not found"})
		return
	}

	// Check if user is the owner
	if env.OwnerID != ownerID {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Only the owner can unshare this environment"})
		return
	}

	// Cannot remove owner's own access
	if targetUUID == ownerID {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Cannot remove owner's access"})
		return
	}

	// Find and delete permission
	var permission models.Permission
	if err := h.db.Where("user_id = ? AND environment_id = ?", targetUUID, envUUID).First(&permission).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Permission not found"})
		return
	}

	// Revoke from RBAC
	if err := rbac.RevokeEnvironmentAccess(targetUUID, envUUID); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to revoke RBAC permission"})
		return
	}

	// Delete permission record
	if err := h.db.Delete(&permission).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to delete permission"})
		return
	}

	// Audit log
	audit.LogAction(h.db, ownerID, audit.ActionRevokePermission, fmt.Sprintf("env:%s", envUUID.String()), map[string]interface{}{
		"target_user_id": targetUUID,
	})

	c.Status(http.StatusNoContent)
}

// ListCollaborators godoc
// @Summary List all users with access to environment
// @Tags environments
// @Security BearerAuth
// @Produce json
// @Param id path string true "Environment ID"
// @Success 200 {array} CollaboratorResponse
// @Router /environments/{id}/collaborators [get]
func (h *EnvironmentHandler) ListCollaborators(c *gin.Context) {
	envID := c.Param("id")

	// Parse environment ID
	envUUID, err := uuid.Parse(envID)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid environment ID"})
		return
	}

	// Note: RBAC middleware already checked read access
	var env models.Environment
	if err := h.db.Where("id = ?", envUUID).First(&env).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Environment not found"})
		return
	}

	// Get all permissions for this environment
	var permissions []models.Permission
	if err := h.db.Preload("User").Preload("Role").Where("environment_id = ?", envUUID).Find(&permissions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch collaborators"})
		return
	}

	// Start with owner
	var owner models.User
	collaborators := []CollaboratorResponse{}

	if err := h.db.First(&owner, "id = ?", env.OwnerID).Error; err == nil {
		collaborators = append(collaborators, CollaboratorResponse{
			UserID:   env.OwnerID,
			Username: owner.Username,
			Email:    owner.Email,
			Role:     "owner",
			IsOwner:  true,
		})
	}

	// Add other collaborators (excluding owner if they have a permission record)
	for _, perm := range permissions {
		if perm.UserID != env.OwnerID {
			collaborators = append(collaborators, CollaboratorResponse{
				UserID:   perm.UserID,
				Username: perm.User.Username,
				Email:    perm.User.Email,
				Role:     perm.Role.Name,
				IsOwner:  false,
			})
		}
	}

	c.JSON(http.StatusOK, collaborators)
}

// Request/Response types
type ShareEnvironmentRequest struct {
	UserID uuid.UUID `json:"user_id" binding:"required"`
	Role   string    `json:"role" binding:"required"` // "viewer" or "editor"
}

type CollaboratorResponse struct {
	UserID   uuid.UUID `json:"user_id"`
	Username string    `json:"username"`
	Email    string    `json:"email,omitempty"`
	Role     string    `json:"role"` // "owner", "editor", "viewer"
	IsOwner  bool      `json:"is_owner"`
}

// Helper function to get user ID from context
func getUserID(c *gin.Context) uuid.UUID {
	user, exists := c.Get("user")
	if !exists {
		return uuid.Nil
	}
	return user.(*models.User).ID
}
