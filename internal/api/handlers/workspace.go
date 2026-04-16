package handlers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/service"
)

type WorkspaceHandler struct {
	svc *service.WorkspaceService
}

func NewWorkspaceHandler(svc *service.WorkspaceService) *WorkspaceHandler {
	return &WorkspaceHandler{svc: svc}
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
	workspaces, err := h.svc.List(getUserID(c))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, workspaces)
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
	}, getUserID(c))
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
	ws, err := h.svc.Get(c.Param("id"))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, ws)
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
	if err := h.svc.Delete(c.Request.Context(), c.Param("id"), getUserID(c)); err != nil {
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
	content, err := h.svc.GetPixiToml(c.Param("id"))
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
	var req SavePixiTomlRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	if err := h.svc.SavePixiToml(c.Param("id"), req.Content); err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, PixiTomlResponse(req))
}

// SolveWorkspace godoc
// @Summary Solve and install environment from current pixi.toml
// @Tags workspaces
// @Security BearerAuth
// @Produce json
// @Param id path string true "Workspace ID"
// @Success 202 {object} models.Job
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /workspaces/{id}/solve [post]
func (h *WorkspaceHandler) SolveWorkspace(c *gin.Context) {
	job, err := h.svc.SolveWorkspace(c.Request.Context(), c.Param("id"), getUserID(c))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, job)
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
	var req PushVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	result, err := h.svc.PushVersion(c.Request.Context(), c.Param("id"), service.PushRequest{
		Tag:      req.Tag,
		PixiToml: req.PixiToml,
		PixiLock: req.PixiLock,
		Force:    req.Force,
	}, getUserID(c))
	if err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusCreated, PushVersionResponse{
		VersionNumber: result.VersionNumber,
		Tags:          result.Tags,
		ContentHash:   result.ContentHash,
		Deduplicated:  result.Deduplicated,
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
	versions, err := h.svc.ListVersions(c.Param("id"))
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
	version, err := h.svc.GetVersion(c.Param("id"), c.Param("version"))
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
	versionNum := c.Param("version")
	content, err := h.svc.GetVersionFile(c.Param("id"), versionNum, "lock")
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
	versionNum := c.Param("version")
	content, err := h.svc.GetVersionFile(c.Param("id"), versionNum, "manifest")
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
	tags, err := h.svc.ListTags(c.Param("id"))
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
	var req InstallPackagesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	job, err := h.svc.InstallPackages(c.Request.Context(), c.Param("id"), req.Packages, getUserID(c))
	if err != nil {
		handleServiceError(c, err)
		return
	}
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
	job, err := h.svc.RemovePackage(c.Request.Context(), c.Param("id"), c.Param("package"), getUserID(c))
	if err != nil {
		handleServiceError(c, err)
		return
	}
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
	packages, err := h.svc.ListPackages(c.Param("id"))
	if err != nil {
		handleServiceError(c, err)
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
	var req ShareWorkspaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	perm, err := h.svc.ShareWorkspace(c.Param("id"), getUserID(c), req.UserID, req.Role)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, perm)
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
	targetUserID, err := uuid.Parse(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid user ID"})
		return
	}

	if err := h.svc.UnshareWorkspace(c.Param("id"), getUserID(c), targetUserID); err != nil {
		handleServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// ListCollaborators godoc
// @Summary List all users with access to workspace
// @Tags workspaces
// @Security BearerAuth
// @Produce json
// @Param id path string true "Workspace ID"
// @Success 200 {array} service.CollaboratorResult
// @Router /workspaces/{id}/collaborators [get]
func (h *WorkspaceHandler) ListCollaborators(c *gin.Context) {
	collaborators, err := h.svc.ListCollaborators(c.Param("id"))
	if err != nil {
		handleServiceError(c, err)
		return
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
	var req RollbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	job, err := h.svc.RollbackToVersion(c.Request.Context(), c.Param("id"), req.VersionNumber, getUserID(c))
	if err != nil {
		handleServiceError(c, err)
		return
	}
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
// @Success 201 {object} service.PublicationResult
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /workspaces/{id}/publish [post]
func (h *WorkspaceHandler) PublishWorkspace(c *gin.Context) {
	var req PublishRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	result, err := h.svc.PublishWorkspace(c.Request.Context(), c.Param("id"), service.PublishWorkspaceRequest{
		RegistryID: req.RegistryID,
		Repository: req.Repository,
		Tag:        req.Tag,
	}, getUserID(c))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, result)
}

// ListPublications godoc
// @Summary List publications for an workspace
// @Description Get all publications (registry pushes) for an workspace
// @Tags workspaces
// @Security BearerAuth
// @Produce json
// @Param id path string true "Workspace ID"
// @Success 200 {array} service.PublicationResult
// @Failure 404 {object} ErrorResponse
// @Router /workspaces/{id}/publications [get]
func (h *WorkspaceHandler) ListPublications(c *gin.Context) {
	publications, err := h.svc.ListPublications(c.Param("id"))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, publications)
}

// UpdatePublication godoc
// @Summary Update a publication's visibility
// @Description Toggle the public/private visibility of a publication
// @Tags workspaces
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Param pubId path string true "Publication ID"
// @Param request body UpdatePublicationRequest true "Update request"
// @Success 200 {object} service.PublicationResult
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /workspaces/{id}/publications/{pubId} [patch]
func (h *WorkspaceHandler) UpdatePublication(c *gin.Context) {
	var req UpdatePublicationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	result, err := h.svc.UpdatePublication(c.Request.Context(), c.Param("id"), c.Param("pubId"), *req.IsPublic)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// GetPublishDefaults godoc
// @Summary Get default values for publishing a workspace
// @Description Returns suggested registry, repository name, and next tag for publishing
// @Tags workspaces
// @Security BearerAuth
// @Produce json
// @Param id path string true "Workspace ID"
// @Success 200 {object} service.PublishDefaultsResult
// @Failure 404 {object} ErrorResponse
// @Router /workspaces/{id}/publish-defaults [get]
func (h *WorkspaceHandler) GetPublishDefaults(c *gin.Context) {
	defaults, err := h.svc.GetPublishDefaults(c.Param("id"))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, defaults)
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
	Tag      string `json:"tag"`
	PixiToml string `json:"pixi_toml" binding:"required"`
	PixiLock string `json:"pixi_lock"`
	Force    bool   `json:"force"`
}

type PushVersionResponse struct {
	VersionNumber int      `json:"version_number"`
	Tags          []string `json:"tags"`
	ContentHash   string   `json:"content_hash"`
	Deduplicated  bool     `json:"deduplicated"`
	Tag           string   `json:"tag"`
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

type PublishRequest struct {
	RegistryID uuid.UUID `json:"registry_id" binding:"required"`
	Repository string    `json:"repository" binding:"required"` // e.g., "myorg/myenv"
	Tag        string    `json:"tag" binding:"required"`        // e.g., "v1.0.0"
}

type UpdatePublicationRequest struct {
	IsPublic *bool `json:"is_public" binding:"required"`
}

// getUserID is in helpers.go
