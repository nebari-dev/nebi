package handlers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/nebari-dev/nebi/internal/audit"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/oci"
	"github.com/nebari-dev/nebi/internal/queue"
	"github.com/nebari-dev/nebi/internal/rbac"
	"github.com/nebari-dev/nebi/internal/utils"
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

	if err := query.Preload("Owner").Order("created_at DESC").Find(&environments).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch environments"})
		return
	}

	// Enrich with size information
	enriched := h.enrichEnvironmentsWithSize(environments)

	c.JSON(http.StatusOK, enriched)
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
// @Param id path string true "Environment ID"
// @Success 200 {object} models.Environment
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /environments/{id} [get]
func (h *EnvironmentHandler) GetEnvironment(c *gin.Context) {
	envID := c.Param("id")

	var env models.Environment
	// Note: RBAC middleware already checked access, so just fetch by ID
	if err := h.db.Preload("Owner").Where("id = ?", envID).First(&env).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Environment not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch environment"})
		return
	}

	// Enrich with size information
	enriched := h.enrichEnvironmentWithSize(&env)

	c.JSON(http.StatusOK, enriched)
}

// DeleteEnvironment godoc
// @Summary Delete an environment
// @Tags environments
// @Security BearerAuth
// @Param id path string true "Environment ID"
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
// @Param id path string true "Environment ID"
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
// @Param id path string true "Environment ID"
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
// @Param id path string true "Environment ID"
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
// @Param id path string true "Environment ID"
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

// EnvironmentResponse includes environment data with formatted size
type EnvironmentResponse struct {
	models.Environment
	SizeFormatted string `json:"size_formatted"`
}

// enrichEnvironmentWithSize adds formatted size to an environment
func (h *EnvironmentHandler) enrichEnvironmentWithSize(env *models.Environment) EnvironmentResponse {
	return EnvironmentResponse{
		Environment:   *env,
		SizeFormatted: utils.FormatBytes(env.SizeBytes),
	}
}

// enrichEnvironmentsWithSize adds formatted size to multiple environments
func (h *EnvironmentHandler) enrichEnvironmentsWithSize(envs []models.Environment) []EnvironmentResponse {
	result := make([]EnvironmentResponse, len(envs))
	for i, env := range envs {
		result[i] = h.enrichEnvironmentWithSize(&env)
	}
	return result
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

// ListVersions godoc
// @Summary List all versions for an environment
// @Tags environments
// @Security BearerAuth
// @Produce json
// @Param id path string true "Environment ID"
// @Success 200 {array} models.EnvironmentVersion
// @Router /environments/{id}/versions [get]
func (h *EnvironmentHandler) ListVersions(c *gin.Context) {
	envID := c.Param("id")

	var versions []models.EnvironmentVersion
	// Exclude large file contents from list view for performance
	err := h.db.
		Select("id", "environment_id", "version_number", "job_id", "created_by", "description", "created_at").
		Where("environment_id = ?", envID).
		Order("version_number DESC").
		Find(&versions).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch versions"})
		return
	}

	c.JSON(http.StatusOK, versions)
}

// GetVersion godoc
// @Summary Get a specific version with full details
// @Tags environments
// @Security BearerAuth
// @Produce json
// @Param id path string true "Environment ID"
// @Param version path int true "Version number"
// @Success 200 {object} models.EnvironmentVersion
// @Router /environments/{id}/versions/{version} [get]
func (h *EnvironmentHandler) GetVersion(c *gin.Context) {
	envID := c.Param("id")
	versionNum := c.Param("version")

	var version models.EnvironmentVersion
	err := h.db.
		Where("environment_id = ? AND version_number = ?", envID, versionNum).
		First(&version).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Version not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch version"})
		return
	}

	c.JSON(http.StatusOK, version)
}

// DownloadLockFile godoc
// @Summary Download pixi.lock for a specific version
// @Tags environments
// @Security BearerAuth
// @Produce text/plain
// @Param id path string true "Environment ID"
// @Param version path int true "Version number"
// @Success 200 {string} string "pixi.lock content"
// @Router /environments/{id}/versions/{version}/pixi-lock [get]
func (h *EnvironmentHandler) DownloadLockFile(c *gin.Context) {
	envID := c.Param("id")
	versionNum := c.Param("version")

	var version models.EnvironmentVersion
	err := h.db.
		Select("lock_file_content").
		Where("environment_id = ? AND version_number = ?", envID, versionNum).
		First(&version).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Version not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch version"})
		return
	}

	// Set headers for file download
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=pixi-lock-v%s.lock", versionNum))
	c.Header("Content-Type", "text/plain")
	c.String(http.StatusOK, version.LockFileContent)
}

// DownloadManifestFile godoc
// @Summary Download pixi.toml for a specific version
// @Tags environments
// @Security BearerAuth
// @Produce text/plain
// @Param id path string true "Environment ID"
// @Param version path int true "Version number"
// @Success 200 {string} string "pixi.toml content"
// @Router /environments/{id}/versions/{version}/pixi-toml [get]
func (h *EnvironmentHandler) DownloadManifestFile(c *gin.Context) {
	envID := c.Param("id")
	versionNum := c.Param("version")

	var version models.EnvironmentVersion
	err := h.db.
		Select("manifest_content").
		Where("environment_id = ? AND version_number = ?", envID, versionNum).
		First(&version).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Version not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch version"})
		return
	}

	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=pixi-toml-v%s.toml", versionNum))
	c.Header("Content-Type", "text/plain")
	c.String(http.StatusOK, version.ManifestContent)
}

// RollbackToVersion godoc
// @Summary Rollback environment to a previous version
// @Tags environments
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Environment ID"
// @Param request body RollbackRequest true "Rollback request"
// @Success 202 {object} models.Job
// @Router /environments/{id}/rollback [post]
func (h *EnvironmentHandler) RollbackToVersion(c *gin.Context) {
	userID := getUserID(c)
	envID := c.Param("id")

	var req RollbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	// Verify environment exists
	var env models.Environment
	if err := h.db.Where("id = ?", envID).First(&env).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Environment not found"})
		return
	}

	// Check environment is ready
	if env.Status != models.EnvStatusReady {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Environment is not ready"})
		return
	}

	// Verify version exists
	var version models.EnvironmentVersion
	err := h.db.
		Where("environment_id = ? AND version_number = ?", envID, req.VersionNumber).
		First(&version).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Version not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch version"})
		return
	}

	// Create rollback job
	job := &models.Job{
		Type:          models.JobTypeRollback,
		EnvironmentID: env.ID,
		Status:        models.JobStatusPending,
		Metadata: map[string]interface{}{
			"version_id":     version.ID.String(),
			"version_number": version.VersionNumber,
			"user_id":        userID.String(),
		},
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
	audit.LogAction(h.db, userID, "rollback_environment", fmt.Sprintf("env:%s", env.ID.String()), map[string]interface{}{
		"version_number": req.VersionNumber,
	})

	c.JSON(http.StatusAccepted, job)
}

type RollbackRequest struct {
	VersionNumber int `json:"version_number" binding:"required"`
}

// Publishing to OCI Registry

type PublishRequest struct {
	RegistryID uuid.UUID `json:"registry_id" binding:"required"`
	Repository string    `json:"repository" binding:"required"` // e.g., "myorg/myenv"
	Tag        string    `json:"tag" binding:"required"`        // e.g., "v1.0.0"
}

type PublicationResponse struct {
	ID            uuid.UUID `json:"id"`
	VersionNumber int       `json:"version_number"`
	RegistryName  string    `json:"registry_name"`
	RegistryURL   string    `json:"registry_url"`
	Repository    string    `json:"repository"`
	Tag           string    `json:"tag"`
	Digest        string    `json:"digest"`
	PublishedBy   string    `json:"published_by"`
	PublishedAt   string    `json:"published_at"`
}

// PublishEnvironment godoc
// @Summary Publish environment to OCI registry
// @Description Publish pixi.toml and pixi.lock to an OCI registry
// @Tags environments
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Environment ID"
// @Param request body PublishRequest true "Publish request"
// @Success 201 {object} PublicationResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /environments/{id}/publish [post]
func (h *EnvironmentHandler) PublishEnvironment(c *gin.Context) {
	envID := c.Param("id")
	userID := getUserID(c)

	var req PublishRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	// Get environment
	var env models.Environment
	if err := h.db.Where("id = ?", envID).First(&env).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Environment not found"})
		return
	}

	// Check if environment is ready
	if env.Status != models.EnvStatusReady {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Environment must be in ready state to publish"})
		return
	}

	// Get the latest version number for this environment
	var latestVersion models.EnvironmentVersion
	if err := h.db.Where("environment_id = ?", envID).Order("version_number DESC").First(&latestVersion).Error; err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Environment has no versions to publish"})
		return
	}

	// Get registry
	var registry models.OCIRegistry
	if err := h.db.Where("id = ?", req.RegistryID).First(&registry).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Registry not found"})
		return
	}

	// Build full repository path
	fullRepo := fmt.Sprintf("%s/%s", registry.URL, req.Repository)

	// Publish using OCI package
	envPath := h.executor.GetEnvironmentPath(&env)

	digest, err := oci.PublishEnvironment(c.Request.Context(), envPath, oci.PublishOptions{
		Repository:   fullRepo,
		Tag:          req.Tag,
		Username:     registry.Username,
		Password:     registry.Password,
		RegistryHost: registry.URL,
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: fmt.Sprintf("Failed to publish: %v", err)})
		return
	}

	// Create publication record
	publication := models.Publication{
		EnvironmentID: env.ID,
		VersionNumber: latestVersion.VersionNumber,
		RegistryID:    registry.ID,
		Repository:    req.Repository,
		Tag:           req.Tag,
		Digest:        digest,
		PublishedBy:   userID,
	}

	if err := h.db.Create(&publication).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to save publication record"})
		return
	}

	// Load relations for response
	h.db.Preload("Registry").Preload("PublishedByUser").First(&publication, publication.ID)

	response := PublicationResponse{
		ID:            publication.ID,
		VersionNumber: publication.VersionNumber,
		RegistryName:  publication.Registry.Name,
		RegistryURL:   publication.Registry.URL,
		Repository:    publication.Repository,
		Tag:           publication.Tag,
		Digest:        publication.Digest,
		PublishedBy:   publication.PublishedByUser.Username,
		PublishedAt:   publication.CreatedAt.Format("2006-01-02 15:04:05"),
	}

	// Audit log
	audit.Log(h.db, userID, audit.ActionPublishEnvironment, audit.ResourceEnvironment, env.ID, map[string]interface{}{
		"registry":   registry.Name,
		"repository": req.Repository,
		"tag":        req.Tag,
	})

	c.JSON(http.StatusCreated, response)
}

// ListPublications godoc
// @Summary List publications for an environment
// @Description Get all publications (registry pushes) for an environment
// @Tags environments
// @Security BearerAuth
// @Produce json
// @Param id path string true "Environment ID"
// @Success 200 {array} PublicationResponse
// @Failure 404 {object} ErrorResponse
// @Router /environments/{id}/publications [get]
func (h *EnvironmentHandler) ListPublications(c *gin.Context) {
	envID := c.Param("id")

	// Check environment exists
	var env models.Environment
	if err := h.db.Where("id = ?", envID).First(&env).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Environment not found"})
		return
	}

	// Get publications
	var publications []models.Publication
	if err := h.db.Where("environment_id = ?", envID).
		Preload("Registry").
		Preload("PublishedByUser").
		Order("created_at DESC").
		Find(&publications).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch publications"})
		return
	}

	response := make([]PublicationResponse, len(publications))
	for i, pub := range publications {
		response[i] = PublicationResponse{
			ID:            pub.ID,
			VersionNumber: pub.VersionNumber,
			RegistryName:  pub.Registry.Name,
			RegistryURL:   pub.Registry.URL,
			Repository:    pub.Repository,
			Tag:           pub.Tag,
			Digest:        pub.Digest,
			PublishedBy:   pub.PublishedByUser.Username,
			PublishedAt:   pub.CreatedAt.Format("2006-01-02 15:04:05"),
		}
	}

	c.JSON(http.StatusOK, response)
}

// PushVersion godoc
// @Summary Push a new version to the server
// @Description Create a new environment version and assign a tag
// @Tags environments
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Environment ID"
// @Param request body PushVersionRequest true "Push request"
// @Success 201 {object} PushVersionResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /environments/{id}/push [post]
func (h *EnvironmentHandler) PushVersion(c *gin.Context) {
	envID := c.Param("id")
	userID := getUserID(c)

	var req PushVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	var env models.Environment
	if err := h.db.Where("id = ?", envID).First(&env).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Environment not found"})
		return
	}

	if env.Status != models.EnvStatusReady {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Environment must be in ready state to push"})
		return
	}

	// Write files to env path (for future publish operations)
	envPath := h.executor.GetEnvironmentPath(&env)
	if err := os.MkdirAll(envPath, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to create environment directory"})
		return
	}

	if err := os.WriteFile(filepath.Join(envPath, "pixi.toml"), []byte(req.PixiToml), 0644); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to write pixi.toml"})
		return
	}

	if req.PixiLock != "" {
		if err := os.WriteFile(filepath.Join(envPath, "pixi.lock"), []byte(req.PixiLock), 0644); err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to write pixi.lock"})
			return
		}
	}

	// Create version record
	newVersion := models.EnvironmentVersion{
		EnvironmentID:   env.ID,
		ManifestContent: req.PixiToml,
		LockFileContent: req.PixiLock,
		PackageMetadata: "[]",
		CreatedBy:       userID,
		Description:     fmt.Sprintf("Pushed as %s:%s", env.Name, req.Tag),
	}
	if err := h.db.Create(&newVersion).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to create version record"})
		return
	}

	// Check if tag already exists
	var existingTag models.EnvironmentTag
	result := h.db.Where("environment_id = ? AND tag = ?", env.ID, req.Tag).First(&existingTag)
	if result.Error == nil {
		if !req.Force {
			c.JSON(http.StatusConflict, ErrorResponse{
				Error: fmt.Sprintf("tag %q already exists at version %d; use --force to reassign", req.Tag, existingTag.VersionNumber),
			})
			return
		}
		oldVersion := existingTag.VersionNumber
		existingTag.VersionNumber = newVersion.VersionNumber
		if err := h.db.Save(&existingTag).Error; err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to update tag"})
			return
		}
		audit.Log(h.db, userID, audit.ActionReassignTag, audit.ResourceEnvironment, env.ID, map[string]interface{}{
			"tag":         req.Tag,
			"old_version": oldVersion,
			"new_version": newVersion.VersionNumber,
		})
	} else {
		newTag := models.EnvironmentTag{
			EnvironmentID: env.ID,
			Tag:           req.Tag,
			VersionNumber: newVersion.VersionNumber,
			CreatedBy:     userID,
		}
		if err := h.db.Create(&newTag).Error; err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to create tag"})
			return
		}
	}

	audit.Log(h.db, userID, audit.ActionPush, audit.ResourceEnvironment, env.ID, map[string]interface{}{
		"tag":     req.Tag,
		"version": newVersion.VersionNumber,
	})

	c.JSON(http.StatusCreated, PushVersionResponse{
		VersionNumber: newVersion.VersionNumber,
		Tag:           req.Tag,
	})
}

// ListTags godoc
// @Summary List tags for an environment
// @Tags environments
// @Security BearerAuth
// @Produce json
// @Param id path string true "Environment ID"
// @Success 200 {array} EnvironmentTagResponse
// @Router /environments/{id}/tags [get]
func (h *EnvironmentHandler) ListTags(c *gin.Context) {
	envID := c.Param("id")

	var tags []models.EnvironmentTag
	if err := h.db.Where("environment_id = ?", envID).Order("created_at DESC").Find(&tags).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to list tags"})
		return
	}

	response := make([]EnvironmentTagResponse, len(tags))
	for i, t := range tags {
		response[i] = EnvironmentTagResponse{
			Tag:           t.Tag,
			VersionNumber: t.VersionNumber,
			CreatedAt:     t.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt:     t.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	c.JSON(http.StatusOK, response)
}

// PushVersionRequest is the request body for pushing a new version.
type PushVersionRequest struct {
	Tag      string `json:"tag" binding:"required"`
	PixiToml string `json:"pixi_toml" binding:"required"`
	PixiLock string `json:"pixi_lock"`
	Force    bool   `json:"force"`
}

// PushVersionResponse is returned after a successful push.
type PushVersionResponse struct {
	VersionNumber int    `json:"version_number"`
	Tag           string `json:"tag"`
}

// EnvironmentTagResponse represents a tag in API responses.
type EnvironmentTagResponse struct {
	Tag           string `json:"tag"`
	VersionNumber int    `json:"version_number"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// Helper function to get user ID from context
func getUserID(c *gin.Context) uuid.UUID {
	user, exists := c.Get("user")
	if !exists {
		return uuid.Nil
	}
	return user.(*models.User).ID
}
