package handlers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/service"
)

type ProjectHandler struct {
	svc *service.ProjectService
}

func NewProjectHandler(svc *service.ProjectService) *ProjectHandler {
	return &ProjectHandler{svc: svc}
}

// ListProjects godoc
// @Summary List all projects for the current user
// @Tags projects
// @Security BearerAuth
// @Produce json
// @Success 200 {array} models.Project
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /projects [get]
func (h *ProjectHandler) ListProjects(c *gin.Context) {
	projects, err := h.svc.List(getUserID(c))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, projects)
}

// CreateProject godoc
// @Summary Create a new project
// @Tags projects
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param project body CreateProjectRequest true "Project details"
// @Success 201 {object} models.Project
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /projects [post]
func (h *ProjectHandler) CreateProject(c *gin.Context) {
	var req CreateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		handleBindError(c, err)
		return
	}

	project, err := h.svc.Create(c.Request.Context(), service.CreateRequest{
		Name:     req.Name,
		PixiToml: req.PixiToml,
		Source:   req.Source,
		Path:     req.Path,
	}, getUserID(c))
	if err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusCreated, project)
}

// GetProject godoc
// @Summary Get a project by ID
// @Tags projects
// @Security BearerAuth
// @Produce json
// @Param id path string true "Project ID"
// @Success 200 {object} models.Project
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /projects/{id} [get]
func (h *ProjectHandler) GetProject(c *gin.Context) {
	project, err := h.svc.Get(c.Param("id"))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, project)
}

// DeleteProject godoc
// @Summary Delete a project
// @Tags projects
// @Security BearerAuth
// @Param id path string true "Project ID"
// @Success 204
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /projects/{id} [delete]
func (h *ProjectHandler) DeleteProject(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), c.Param("id"), getUserID(c)); err != nil {
		handleServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// GetPixiToml godoc
// @Summary Get pixi.toml content for a project
// @Tags projects
// @Security BearerAuth
// @Produce json
// @Param id path string true "Project ID"
// @Success 200 {object} PixiTomlResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /projects/{id}/pixi-toml [get]
func (h *ProjectHandler) GetPixiToml(c *gin.Context) {
	content, err := h.svc.GetPixiToml(c.Param("id"))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, PixiTomlResponse{Content: content})
}

// SavePixiToml godoc
// @Summary Save pixi.toml content for a project
// @Tags projects
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Project ID"
// @Param request body SavePixiTomlRequest true "pixi.toml content"
// @Success 200 {object} PixiTomlResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /projects/{id}/pixi-toml [put]
func (h *ProjectHandler) SavePixiToml(c *gin.Context) {
	var req SavePixiTomlRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		handleBindError(c, err)
		return
	}

	if err := h.svc.SavePixiToml(c.Param("id"), req.Content); err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, PixiTomlResponse(req))
}

// SolveProject godoc
// @Summary Solve the environment (refresh pixi.lock) from current pixi.toml
// @Tags projects
// @Security BearerAuth
// @Produce json
// @Param id path string true "Project ID"
// @Success 202 {object} models.Job
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /projects/{id}/solve [post]
func (h *ProjectHandler) SolveProject(c *gin.Context) {
	job, err := h.svc.SolveProject(c.Request.Context(), c.Param("id"), getUserID(c))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, job)
}

// InstallProject godoc
// @Summary Install the project environment from its lockfile (local mode)
// @Tags projects
// @Security BearerAuth
// @Produce json
// @Param id path string true "Project ID"
// @Success 202 {object} models.Job
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /projects/{id}/install [post]
func (h *ProjectHandler) InstallProject(c *gin.Context) {
	job, err := h.svc.InstallProjectEnv(c.Request.Context(), c.Param("id"), getUserID(c))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, job)
}

// UninstallProject godoc
// @Summary Remove the project's installed environment (local mode)
// @Tags projects
// @Security BearerAuth
// @Produce json
// @Param id path string true "Project ID"
// @Success 202 {object} models.Job
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /projects/{id}/uninstall [post]
func (h *ProjectHandler) UninstallProject(c *gin.Context) {
	job, err := h.svc.UninstallProjectEnv(c.Request.Context(), c.Param("id"), getUserID(c))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, job)
}

// PushVersion godoc
// @Summary Push a new version to the server
// @Description Create a new project version and assign a tag
// @Tags projects
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Project ID"
// @Param request body PushVersionRequest true "Push request"
// @Success 201 {object} PushVersionResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /projects/{id}/push [post]
func (h *ProjectHandler) PushVersion(c *gin.Context) {
	var req PushVersionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		handleBindError(c, err)
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
// @Summary List all versions for a project
// @Tags projects
// @Security BearerAuth
// @Produce json
// @Param id path string true "Project ID"
// @Success 200 {array} models.ProjectVersion
// @Router /projects/{id}/versions [get]
func (h *ProjectHandler) ListVersions(c *gin.Context) {
	versions, err := h.svc.ListVersions(c.Param("id"))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, versions)
}

// GetVersion godoc
// @Summary Get a specific version with full details
// @Tags projects
// @Security BearerAuth
// @Produce json
// @Param id path string true "Project ID"
// @Param version path int true "Version number"
// @Success 200 {object} models.ProjectVersion
// @Router /projects/{id}/versions/{version} [get]
func (h *ProjectHandler) GetVersion(c *gin.Context) {
	version, err := h.svc.GetVersion(c.Param("id"), c.Param("version"))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, version)
}

// DownloadLockFile godoc
// @Summary Download pixi.lock for a specific version
// @Tags projects
// @Security BearerAuth
// @Produce text/plain
// @Param id path string true "Project ID"
// @Param version path int true "Version number"
// @Success 200 {string} string "pixi.lock content"
// @Router /projects/{id}/versions/{version}/pixi-lock [get]
func (h *ProjectHandler) DownloadLockFile(c *gin.Context) {
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
// @Tags projects
// @Security BearerAuth
// @Produce text/plain
// @Param id path string true "Project ID"
// @Param version path int true "Version number"
// @Success 200 {string} string "pixi.toml content"
// @Router /projects/{id}/versions/{version}/pixi-toml [get]
func (h *ProjectHandler) DownloadManifestFile(c *gin.Context) {
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
// @Summary List tags for a project
// @Tags projects
// @Security BearerAuth
// @Produce json
// @Param id path string true "Project ID"
// @Success 200 {array} ProjectTagResponse
// @Router /projects/{id}/tags [get]
func (h *ProjectHandler) ListTags(c *gin.Context) {
	tags, err := h.svc.ListTags(c.Param("id"))
	if err != nil {
		handleServiceError(c, err)
		return
	}

	response := make([]ProjectTagResponse, len(tags))
	for i, t := range tags {
		response[i] = ProjectTagResponse{
			Tag:           t.Tag,
			VersionNumber: t.VersionNumber,
			CreatedAt:     t.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt:     t.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}
	c.JSON(http.StatusOK, response)
}

// InstallPackages godoc
// @Summary Install packages in a project
// @Tags projects
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Project ID"
// @Param packages body InstallPackagesRequest true "Packages to install"
// @Success 202 {object} models.Job
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /projects/{id}/packages [post]
func (h *ProjectHandler) InstallPackages(c *gin.Context) {
	var req InstallPackagesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		handleBindError(c, err)
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
// @Summary Remove packages from a project
// @Tags projects
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Project ID"
// @Param package path string true "Package name"
// @Success 202 {object} models.Job
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /projects/{id}/packages/{package} [delete]
func (h *ProjectHandler) RemovePackages(c *gin.Context) {
	job, err := h.svc.RemovePackage(c.Request.Context(), c.Param("id"), c.Param("package"), getUserID(c))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, job)
}

// ListPackages godoc
// @Summary List packages in a project
// @Tags projects
// @Security BearerAuth
// @Produce json
// @Param id path string true "Project ID"
// @Success 200 {array} models.Package
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /projects/{id}/packages [get]
func (h *ProjectHandler) ListPackages(c *gin.Context) {
	packages, err := h.svc.ListPackages(c.Param("id"))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, packages)
}

// ShareProject godoc
// @Summary Share project with another user (owner only)
// @Tags projects
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Project ID"
// @Param share body ShareProjectRequest true "Share details"
// @Success 201 {object} models.Permission
// @Router /projects/{id}/share [post]
func (h *ProjectHandler) ShareProject(c *gin.Context) {
	var req ShareProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		handleBindError(c, err)
		return
	}

	perm, err := h.svc.ShareProject(c.Param("id"), getUserID(c), req.UserID, req.Role)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, perm)
}

// UnshareProject godoc
// @Summary Revoke user access to project (owner only)
// @Tags projects
// @Security BearerAuth
// @Param id path string true "Project ID"
// @Param user_id path string true "User ID to revoke"
// @Success 204
// @Router /projects/{id}/share/{user_id} [delete]
func (h *ProjectHandler) UnshareProject(c *gin.Context) {
	targetUserID, err := uuid.Parse(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid user ID"})
		return
	}

	if err := h.svc.UnshareProject(c.Param("id"), getUserID(c), targetUserID); err != nil {
		handleServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// ListCollaborators godoc
// @Summary List all users with access to project
// @Tags projects
// @Security BearerAuth
// @Produce json
// @Param id path string true "Project ID"
// @Success 200 {array} service.CollaboratorResult
// @Router /projects/{id}/collaborators [get]
func (h *ProjectHandler) ListCollaborators(c *gin.Context) {
	collaborators, err := h.svc.ListCollaborators(c.Param("id"))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, collaborators)
}

// RollbackToVersion godoc
// @Summary Rollback project to a previous version
// @Tags projects
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Project ID"
// @Param request body RollbackRequest true "Rollback request"
// @Success 202 {object} models.Job
// @Router /projects/{id}/rollback [post]
func (h *ProjectHandler) RollbackToVersion(c *gin.Context) {
	var req RollbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		handleBindError(c, err)
		return
	}

	job, err := h.svc.RollbackToVersion(c.Request.Context(), c.Param("id"), req.VersionNumber, getUserID(c))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, job)
}

// PublishProject godoc
// @Summary Publish project to OCI registry
// @Description Publish pixi.toml and pixi.lock to an OCI registry
// @Tags projects
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Project ID"
// @Param request body PublishRequest true "Publish request"
// @Success 201 {object} service.PublicationResult
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /projects/{id}/publish [post]
func (h *ProjectHandler) PublishProject(c *gin.Context) {
	var req PublishRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		handleBindError(c, err)
		return
	}

	result, err := h.svc.PublishProject(c.Request.Context(), c.Param("id"), service.PublishProjectRequest{
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
// @Summary List publications for a project
// @Description Get all publications (registry pushes) for a project
// @Tags projects
// @Security BearerAuth
// @Produce json
// @Param id path string true "Project ID"
// @Success 200 {array} service.PublicationResult
// @Failure 404 {object} ErrorResponse
// @Router /projects/{id}/publications [get]
func (h *ProjectHandler) ListPublications(c *gin.Context) {
	publications, err := h.svc.ListPublications(c.Param("id"), getUserID(c))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, publications)
}

// UpdatePublication godoc
// @Summary Update a publication's visibility
// @Description Toggle the public/private visibility of a publication
// @Tags projects
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Project ID"
// @Param pubId path string true "Publication ID"
// @Param request body UpdatePublicationRequest true "Update request"
// @Success 200 {object} service.PublicationResult
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /projects/{id}/publications/{pubId} [patch]
func (h *ProjectHandler) UpdatePublication(c *gin.Context) {
	var req UpdatePublicationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		handleBindError(c, err)
		return
	}

	result, err := h.svc.UpdatePublication(c.Request.Context(), c.Param("id"), c.Param("pubId"), *req.IsPublic, getUserID(c))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// GetPublishDefaults godoc
// @Summary Get default values for publishing a project
// @Description Returns suggested registry, repository name, and next tag for publishing
// @Tags projects
// @Security BearerAuth
// @Produce json
// @Param id path string true "Project ID"
// @Param registry_id query string false "Registry ID to compute defaults against"
// @Success 200 {object} service.PublishDefaultsResult
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /projects/{id}/publish-defaults [get]
func (h *ProjectHandler) GetPublishDefaults(c *gin.Context) {
	registryID := uuid.Nil
	if rawRegistryID := c.Query("registry_id"); rawRegistryID != "" {
		parsedID, err := uuid.Parse(rawRegistryID)
		if err != nil {
			c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid registry ID"})
			return
		}
		registryID = parsedID
	}

	defaults, err := h.svc.GetPublishDefaults(c.Param("id"), getUserID(c), registryID)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, defaults)
}

// --- Request/Response types ---

type CreateProjectRequest struct {
	Name     string `json:"name"`
	PixiToml string `json:"pixi_toml"`
	Source   string `json:"source"`
	Path     string `json:"path"`
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

type ProjectTagResponse struct {
	Tag           string `json:"tag"`
	VersionNumber int    `json:"version_number"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type RollbackRequest struct {
	VersionNumber int `json:"version_number" binding:"required"`
}

type ShareProjectRequest struct {
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

type ShareProjectWithGroupRequest struct {
	GroupID uuid.UUID `json:"group_id" binding:"required"`
	Role    string    `json:"role" binding:"required"` // "viewer" or "editor"
}

// ShareProjectWithGroup godoc
// @Summary Share project with a group (owner only)
// @Tags projects
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Project ID"
// @Param share body ShareProjectWithGroupRequest true "Group share details"
// @Success 201 {object} models.GroupPermission
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /projects/{id}/share-group [post]
func (h *ProjectHandler) ShareProjectWithGroup(c *gin.Context) {
	var req ShareProjectWithGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		handleBindError(c, err)
		return
	}
	perm, err := h.svc.ShareProjectWithGroup(c.Param("id"), getUserID(c), req.GroupID, req.Role)
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, perm)
}

// UnshareProjectWithGroup godoc
// @Summary Revoke a group's access to a project (owner only)
// @Tags projects
// @Security BearerAuth
// @Param id path string true "Project ID"
// @Param group_id path string true "Group ID to revoke"
// @Success 204
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /projects/{id}/share-group/{group_id} [delete]
func (h *ProjectHandler) UnshareProjectWithGroup(c *gin.Context) {
	groupID, err := uuid.Parse(c.Param("group_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Invalid group ID"})
		return
	}
	if err := h.svc.UnshareProjectFromGroup(c.Param("id"), getUserID(c), groupID); err != nil {
		handleServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// getUserID is in helpers.go
