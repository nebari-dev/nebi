package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/audit"
	"github.com/nebari-dev/nebi/internal/executor"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/oci"
	"github.com/nebari-dev/nebi/internal/queue"
	"github.com/nebari-dev/nebi/internal/rbac"
	"github.com/nebari-dev/nebi/internal/service"
	"github.com/nebari-dev/nebi/internal/utils"
	"gorm.io/gorm"
)

type WorkspaceHandler struct {
	svc      *service.WorkspaceService
	db       *gorm.DB
	queue    queue.Queue
	executor executor.Executor
	isLocal  bool
}

func NewWorkspaceHandler(svc *service.WorkspaceService, db *gorm.DB, q queue.Queue, exec executor.Executor, isLocal bool) *WorkspaceHandler {
	return &WorkspaceHandler{svc: svc, db: db, queue: q, executor: exec, isLocal: isLocal}
}

// handleServiceError maps service-layer errors to HTTP status codes.
func handleServiceError(c *gin.Context, err error) {
	if errors.Is(err, service.ErrNotFound) {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Not found"})
		return
	}
	var validationErr *service.ValidationError
	if errors.As(err, &validationErr) {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: validationErr.Message})
		return
	}
	var conflictErr *service.ConflictError
	if errors.As(err, &conflictErr) {
		c.JSON(http.StatusConflict, ErrorResponse{Error: conflictErr.Message})
		return
	}
	slog.Error("unhandled service error", "error", err)
	c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Internal server error"})
}

// ListWorkspaces godoc
// @Summary List all workspaces for the current user
// @Tags workspaces
// @Security BearerAuth
// @Produce json
// @Success 200 {array} models.Workspace
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /workspaces [get]
func (h *WorkspaceHandler) ListWorkspaces(c *gin.Context) {
	userID := getUserID(c)

	workspaces, err := h.svc.List(userID)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	enriched := h.enrichWorkspacesWithSize(workspaces)
	c.JSON(http.StatusOK, enriched)
}

// CreateWorkspace godoc
// @Summary Create a new workspace
// @Tags workspaces
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param workspace body CreateWorkspaceRequest true "Workspace details"
// @Success 201 {object} models.Workspace
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /workspaces [post]
func (h *WorkspaceHandler) CreateWorkspace(c *gin.Context) {
	userID := getUserID(c)

	var req CreateWorkspaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	ws, err := h.svc.Create(c.Request.Context(), service.CreateRequest{
		Name:           req.Name,
		PackageManager: req.PackageManager,
		PixiToml:       req.PixiToml,
		Source:         req.Source,
		Path:           req.Path,
	}, userID)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusCreated, ws)
}

// GetWorkspace godoc
// @Summary Get a workspace by ID
// @Tags workspaces
// @Security BearerAuth
// @Produce json
// @Param id path string true "Workspace ID"
// @Success 200 {object} models.Workspace
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /workspaces/{id} [get]
func (h *WorkspaceHandler) GetWorkspace(c *gin.Context) {
	wsID := c.Param("id")

	ws, err := h.svc.Get(wsID)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	enriched := h.enrichWorkspaceWithSize(ws)
	c.JSON(http.StatusOK, enriched)
}

// DeleteWorkspace godoc
// @Summary Delete an workspace
// @Tags workspaces
// @Security BearerAuth
// @Param id path string true "Workspace ID"
// @Success 204
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /workspaces/{id} [delete]
func (h *WorkspaceHandler) DeleteWorkspace(c *gin.Context) {
	userID := getUserID(c)
	wsID := c.Param("id")

	if err := h.svc.Delete(c.Request.Context(), wsID, userID); err != nil {
		handleServiceError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// GetPixiToml godoc
// @Summary Get pixi.toml content for an workspace
// @Tags workspaces
// @Security BearerAuth
// @Produce json
// @Param id path string true "Workspace ID"
// @Success 200 {object} PixiTomlResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /workspaces/{id}/pixi-toml [get]
func (h *WorkspaceHandler) GetPixiToml(c *gin.Context) {
	wsID := c.Param("id")

	content, err := h.svc.GetPixiToml(wsID)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, PixiTomlResponse{Content: content})
}

// SavePixiToml godoc
// @Summary Save pixi.toml content for a workspace
// @Tags workspaces
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Param request body SavePixiTomlRequest true "pixi.toml content"
// @Success 200 {object} PixiTomlResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /workspaces/{id}/pixi-toml [put]
func (h *WorkspaceHandler) SavePixiToml(c *gin.Context) {
	wsID := c.Param("id")

	var req SavePixiTomlRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	if err := h.svc.SavePixiToml(wsID, req.Content); err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, PixiTomlResponse(req))
}

// PushVersion godoc
// @Summary Push a new version to the server
// @Description Create a new workspace version and assign a tag
// @Tags workspaces
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Param request body PushVersionRequest true "Push request"
// @Success 201 {object} PushVersionResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /workspaces/{id}/push [post]
func (h *WorkspaceHandler) PushVersion(c *gin.Context) {
	wsID := c.Param("id")
	userID := getUserID(c)

	var req PushVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	result, err := h.svc.PushVersion(c.Request.Context(), wsID, service.PushRequest{
		Tag:      req.Tag,
		PixiToml: req.PixiToml,
		PixiLock: req.PixiLock,
		Force:    req.Force,
	}, userID)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusCreated, PushVersionResponse{
		VersionNumber: result.VersionNumber,
		Tag:           result.Tag,
	})
}

// ListVersions godoc
// @Summary List all versions for an workspace
// @Tags workspaces
// @Security BearerAuth
// @Produce json
// @Param id path string true "Workspace ID"
// @Success 200 {array} models.WorkspaceVersion
// @Router /workspaces/{id}/versions [get]
func (h *WorkspaceHandler) ListVersions(c *gin.Context) {
	wsID := c.Param("id")

	versions, err := h.svc.ListVersions(wsID)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, versions)
}

// GetVersion godoc
// @Summary Get a specific version with full details
// @Tags workspaces
// @Security BearerAuth
// @Produce json
// @Param id path string true "Workspace ID"
// @Param version path int true "Version number"
// @Success 200 {object} models.WorkspaceVersion
// @Router /workspaces/{id}/versions/{version} [get]
func (h *WorkspaceHandler) GetVersion(c *gin.Context) {
	wsID := c.Param("id")
	versionNum := c.Param("version")

	version, err := h.svc.GetVersion(wsID, versionNum)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusOK, version)
}

// DownloadLockFile godoc
// @Summary Download pixi.lock for a specific version
// @Tags workspaces
// @Security BearerAuth
// @Produce text/plain
// @Param id path string true "Workspace ID"
// @Param version path int true "Version number"
// @Success 200 {string} string "pixi.lock content"
// @Router /workspaces/{id}/versions/{version}/pixi-lock [get]
func (h *WorkspaceHandler) DownloadLockFile(c *gin.Context) {
	wsID := c.Param("id")
	versionNum := c.Param("version")

	content, err := h.svc.GetVersionFile(wsID, versionNum, "lock")
	if err != nil {
		handleServiceError(c, err)
		return
	}

	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=pixi-lock-v%s.lock", versionNum))
	c.Header("Content-Type", "text/plain")
	c.String(http.StatusOK, content)
}

// DownloadManifestFile godoc
// @Summary Download pixi.toml for a specific version
// @Tags workspaces
// @Security BearerAuth
// @Produce text/plain
// @Param id path string true "Workspace ID"
// @Param version path int true "Version number"
// @Success 200 {string} string "pixi.toml content"
// @Router /workspaces/{id}/versions/{version}/pixi-toml [get]
func (h *WorkspaceHandler) DownloadManifestFile(c *gin.Context) {
	wsID := c.Param("id")
	versionNum := c.Param("version")

	content, err := h.svc.GetVersionFile(wsID, versionNum, "manifest")
	if err != nil {
		handleServiceError(c, err)
		return
	}

	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=pixi-toml-v%s.toml", versionNum))
	c.Header("Content-Type", "text/plain")
	c.String(http.StatusOK, content)
}

// ListTags godoc
// @Summary List tags for an workspace
// @Tags workspaces
// @Security BearerAuth
// @Produce json
// @Param id path string true "Workspace ID"
// @Success 200 {array} WorkspaceTagResponse
// @Router /workspaces/{id}/tags [get]
func (h *WorkspaceHandler) ListTags(c *gin.Context) {
	wsID := c.Param("id")

	tags, err := h.svc.ListTags(wsID)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	response := make([]WorkspaceTagResponse, len(tags))
	for i, t := range tags {
		response[i] = WorkspaceTagResponse{
			Tag:           t.Tag,
			VersionNumber: t.VersionNumber,
			CreatedAt:     t.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt:     t.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	c.JSON(http.StatusOK, response)
}

// --- Non-extracted methods (remain using h.db directly) ---

// InstallPackages godoc
// @Summary Install packages in an workspace
// @Tags workspaces
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Param packages body InstallPackagesRequest true "Packages to install"
// @Success 202 {object} models.Job
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /workspaces/{id}/packages [post]
func (h *WorkspaceHandler) InstallPackages(c *gin.Context) {
	userID := getUserID(c)
	wsID := c.Param("id")

	var req InstallPackagesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	var ws models.Workspace
	// Note: RBAC middleware already checked write access
	if err := h.db.Where("id = ?", wsID).First(&ws).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Workspace not found"})
		return
	}

	// Check if workspace is ready
	if ws.Status != models.WsStatusReady {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Workspace is not ready"})
		return
	}

	// Queue install job
	job := &models.Job{
		Type:        models.JobTypeInstall,
		WorkspaceID: ws.ID,
		Status:      models.JobStatusPending,
		Metadata:    map[string]interface{}{"packages": req.Packages},
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
	audit.LogAction(h.db, userID, audit.ActionInstallPackage, fmt.Sprintf("ws:%s", ws.ID.String()), map[string]interface{}{
		"packages": req.Packages,
	})

	c.JSON(http.StatusAccepted, job)
}

// RemovePackages godoc
// @Summary Remove packages from an workspace
// @Tags workspaces
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Param package path string true "Package name"
// @Success 202 {object} models.Job
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /workspaces/{id}/packages/{package} [delete]
func (h *WorkspaceHandler) RemovePackages(c *gin.Context) {
	userID := getUserID(c)
	wsID := c.Param("id")
	packageName := c.Param("package")

	var ws models.Workspace
	// Note: RBAC middleware already checked write access
	if err := h.db.Where("id = ?", wsID).First(&ws).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Workspace not found"})
		return
	}

	// Check if workspace is ready
	if ws.Status != models.WsStatusReady {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Workspace is not ready"})
		return
	}

	// Queue remove job
	job := &models.Job{
		Type:        models.JobTypeRemove,
		WorkspaceID: ws.ID,
		Status:      models.JobStatusPending,
		Metadata:    map[string]interface{}{"packages": []string{packageName}},
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
	audit.LogAction(h.db, userID, audit.ActionRemovePackage, fmt.Sprintf("ws:%s", ws.ID.String()), map[string]interface{}{
		"package": packageName,
	})

	c.JSON(http.StatusAccepted, job)
}

// ListPackages godoc
// @Summary List packages in an workspace
// @Tags workspaces
// @Security BearerAuth
// @Produce json
// @Param id path string true "Workspace ID"
// @Success 200 {array} models.Package
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /workspaces/{id}/packages [get]
func (h *WorkspaceHandler) ListPackages(c *gin.Context) {
	wsID := c.Param("id")

	var ws models.Workspace
	// Note: RBAC middleware already checked read access
	if err := h.db.Where("id = ?", wsID).First(&ws).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Workspace not found"})
		return
	}

	var packages []models.Package
	if err := h.db.Where("workspace_id = ?", ws.ID).Find(&packages).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch packages"})
		return
	}

	c.JSON(http.StatusOK, packages)
}

// ShareWorkspace godoc
// @Summary Share workspace with another user (owner only)
// @Tags workspaces
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Param share body ShareWorkspaceRequest true "Share details"
// @Success 201 {object} models.Permission
// @Router /workspaces/{id}/share [post]
func (h *WorkspaceHandler) ShareWorkspace(c *gin.Context) {
	ownerID := getUserID(c)
	wsID := c.Param("id")

	var req ShareWorkspaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	// Parse workspace ID
	wsUUID, err := uuid.Parse(wsID)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid workspace ID"})
		return
	}

	// Get workspace and check ownership
	var ws models.Workspace
	if err := h.db.Where("id = ?", wsUUID).First(&ws).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Workspace not found"})
		return
	}

	// Check if user is the owner
	if ws.OwnerID != ownerID {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Only the owner can share this workspace"})
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
		UserID:      req.UserID,
		WorkspaceID: wsUUID,
		RoleID:      role.ID,
	}

	if err := h.db.Create(&permission).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to create permission"})
		return
	}

	// Grant in RBAC
	if err := rbac.GrantWorkspaceAccess(req.UserID, wsUUID, req.Role); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to grant RBAC permission"})
		return
	}

	// Audit log
	audit.LogAction(h.db, ownerID, audit.ActionGrantPermission, fmt.Sprintf("ws:%s", wsUUID.String()), map[string]interface{}{
		"target_user_id": req.UserID,
		"role":           req.Role,
	})

	c.JSON(http.StatusCreated, permission)
}

// UnshareWorkspace godoc
// @Summary Revoke user access to workspace (owner only)
// @Tags workspaces
// @Security BearerAuth
// @Param id path string true "Workspace ID"
// @Param user_id path string true "User ID to revoke"
// @Success 204
// @Router /workspaces/{id}/share/{user_id} [delete]
func (h *WorkspaceHandler) UnshareWorkspace(c *gin.Context) {
	ownerID := getUserID(c)
	wsID := c.Param("id")
	targetUserID := c.Param("user_id")

	// Parse UUIDs
	wsUUID, err := uuid.Parse(wsID)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid workspace ID"})
		return
	}

	targetUUID, err := uuid.Parse(targetUserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid user ID"})
		return
	}

	// Get workspace and check ownership
	var ws models.Workspace
	if err := h.db.Where("id = ?", wsUUID).First(&ws).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Workspace not found"})
		return
	}

	// Check if user is the owner
	if ws.OwnerID != ownerID {
		c.JSON(http.StatusForbidden, ErrorResponse{Error: "Only the owner can unshare this workspace"})
		return
	}

	// Cannot remove owner's own access
	if targetUUID == ownerID {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Cannot remove owner's access"})
		return
	}

	// Find and delete permission
	var permission models.Permission
	if err := h.db.Where("user_id = ? AND workspace_id = ?", targetUUID, wsUUID).First(&permission).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Permission not found"})
		return
	}

	// Revoke from RBAC
	if err := rbac.RevokeWorkspaceAccess(targetUUID, wsUUID); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to revoke RBAC permission"})
		return
	}

	// Delete permission record
	if err := h.db.Delete(&permission).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to delete permission"})
		return
	}

	// Audit log
	audit.LogAction(h.db, ownerID, audit.ActionRevokePermission, fmt.Sprintf("ws:%s", wsUUID.String()), map[string]interface{}{
		"target_user_id": targetUUID,
	})

	c.Status(http.StatusNoContent)
}

// ListCollaborators godoc
// @Summary List all users with access to workspace
// @Tags workspaces
// @Security BearerAuth
// @Produce json
// @Param id path string true "Workspace ID"
// @Success 200 {array} CollaboratorResponse
// @Router /workspaces/{id}/collaborators [get]
func (h *WorkspaceHandler) ListCollaborators(c *gin.Context) {
	wsID := c.Param("id")

	// Parse workspace ID
	wsUUID, err := uuid.Parse(wsID)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid workspace ID"})
		return
	}

	// Note: RBAC middleware already checked read access
	var ws models.Workspace
	if err := h.db.Where("id = ?", wsUUID).First(&ws).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Workspace not found"})
		return
	}

	// Get all permissions for this workspace
	var permissions []models.Permission
	if err := h.db.Preload("User").Preload("Role").Where("workspace_id = ?", wsUUID).Find(&permissions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch collaborators"})
		return
	}

	// Start with owner
	var owner models.User
	collaborators := []CollaboratorResponse{}

	if err := h.db.First(&owner, "id = ?", ws.OwnerID).Error; err == nil {
		collaborators = append(collaborators, CollaboratorResponse{
			UserID:   ws.OwnerID,
			Username: owner.Username,
			Email:    owner.Email,
			Role:     "owner",
			IsOwner:  true,
		})
	}

	// Add other collaborators (excluding owner if they have a permission record)
	for _, perm := range permissions {
		if perm.UserID != ws.OwnerID {
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

// RollbackToVersion godoc
// @Summary Rollback workspace to a previous version
// @Tags workspaces
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Param request body RollbackRequest true "Rollback request"
// @Success 202 {object} models.Job
// @Router /workspaces/{id}/rollback [post]
func (h *WorkspaceHandler) RollbackToVersion(c *gin.Context) {
	userID := getUserID(c)
	wsID := c.Param("id")

	var req RollbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	// Verify workspace exists
	var ws models.Workspace
	if err := h.db.Where("id = ?", wsID).First(&ws).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Workspace not found"})
		return
	}

	// Check workspace is ready
	if ws.Status != models.WsStatusReady {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Workspace is not ready"})
		return
	}

	// Verify version exists
	var version models.WorkspaceVersion
	err := h.db.
		Where("workspace_id = ? AND version_number = ?", wsID, req.VersionNumber).
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
		Type:        models.JobTypeRollback,
		WorkspaceID: ws.ID,
		Status:      models.JobStatusPending,
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
	audit.LogAction(h.db, userID, "rollback_workspace", fmt.Sprintf("ws:%s", ws.ID.String()), map[string]interface{}{
		"version_number": req.VersionNumber,
	})

	c.JSON(http.StatusAccepted, job)
}

// PublishWorkspace godoc
// @Summary Publish workspace to OCI registry
// @Description Publish pixi.toml and pixi.lock to an OCI registry
// @Tags workspaces
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Param request body PublishRequest true "Publish request"
// @Success 201 {object} PublicationResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /workspaces/{id}/publish [post]
func (h *WorkspaceHandler) PublishWorkspace(c *gin.Context) {
	wsID := c.Param("id")
	userID := getUserID(c)

	var req PublishRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	// Get workspace
	var ws models.Workspace
	if err := h.db.Where("id = ?", wsID).First(&ws).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Workspace not found"})
		return
	}

	// Check if workspace is ready
	if ws.Status != models.WsStatusReady {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Workspace must be in ready state to publish"})
		return
	}

	// Get the latest version number for this workspace
	var latestVersion models.WorkspaceVersion
	if err := h.db.Where("workspace_id = ?", wsID).Order("version_number DESC").First(&latestVersion).Error; err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Workspace has no versions to publish"})
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
	wsPath := h.executor.GetWorkspacePath(&ws)

	digest, err := oci.PublishWorkspace(c.Request.Context(), wsPath, oci.PublishOptions{
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
		WorkspaceID:   ws.ID,
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
	audit.Log(h.db, userID, audit.ActionPublishWorkspace, audit.ResourceWorkspace, ws.ID, map[string]interface{}{
		"registry":   registry.Name,
		"repository": req.Repository,
		"tag":        req.Tag,
	})

	c.JSON(http.StatusCreated, response)
}

// ListPublications godoc
// @Summary List publications for an workspace
// @Description Get all publications (registry pushes) for an workspace
// @Tags workspaces
// @Security BearerAuth
// @Produce json
// @Param id path string true "Workspace ID"
// @Success 200 {array} PublicationResponse
// @Failure 404 {object} ErrorResponse
// @Router /workspaces/{id}/publications [get]
func (h *WorkspaceHandler) ListPublications(c *gin.Context) {
	wsID := c.Param("id")

	// Check workspace exists
	var ws models.Workspace
	if err := h.db.Where("id = ?", wsID).First(&ws).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Workspace not found"})
		return
	}

	// Get publications
	var publications []models.Publication
	if err := h.db.Where("workspace_id = ?", wsID).
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

// --- Request/Response types ---

type CreateWorkspaceRequest struct {
	Name           string `json:"name" binding:"required"`
	PackageManager string `json:"package_manager"`
	PixiToml       string `json:"pixi_toml"`
	Source         string `json:"source"`
	Path           string `json:"path"`
}

type PixiTomlResponse struct {
	Content string `json:"content"`
}

type InstallPackagesRequest struct {
	Packages []string `json:"packages" binding:"required"`
}

type SavePixiTomlRequest struct {
	Content string `json:"content" binding:"required"`
}

type PushVersionRequest struct {
	Tag      string `json:"tag" binding:"required"`
	PixiToml string `json:"pixi_toml" binding:"required"`
	PixiLock string `json:"pixi_lock"`
	Force    bool   `json:"force"`
}

type PushVersionResponse struct {
	VersionNumber int    `json:"version_number"`
	Tag           string `json:"tag"`
}

type WorkspaceTagResponse struct {
	Tag           string `json:"tag"`
	VersionNumber int    `json:"version_number"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type RollbackRequest struct {
	VersionNumber int `json:"version_number" binding:"required"`
}

type ShareWorkspaceRequest struct {
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

// WorkspaceResponse includes workspace data with formatted size
type WorkspaceResponse struct {
	models.Workspace
	SizeFormatted string `json:"size_formatted"`
}

// enrichWorkspaceWithSize adds formatted size to a workspace
func (h *WorkspaceHandler) enrichWorkspaceWithSize(ws *models.Workspace) WorkspaceResponse {
	return WorkspaceResponse{
		Workspace:     *ws,
		SizeFormatted: utils.FormatBytes(ws.SizeBytes),
	}
}

// enrichWorkspacesWithSize adds formatted size to multiple workspaces
func (h *WorkspaceHandler) enrichWorkspacesWithSize(workspaces []models.Workspace) []WorkspaceResponse {
	result := make([]WorkspaceResponse, len(workspaces))
	for i, ws := range workspaces {
		result[i] = h.enrichWorkspaceWithSize(&ws)
	}
	return result
}

// Helper function to get user ID from context
func getUserID(c *gin.Context) uuid.UUID {
	user, exists := c.Get("user")
	if !exists {
		return uuid.Nil
	}
	return user.(*models.User).ID
}
