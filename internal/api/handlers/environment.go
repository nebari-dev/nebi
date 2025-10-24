package handlers

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/aktech/darb/internal/executor"
	"github.com/aktech/darb/internal/models"
	"github.com/aktech/darb/internal/queue"
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
	if err := h.db.Where("owner_id = ?", userID).Order("created_at DESC").Find(&environments).Error; err != nil {
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
	userID := getUserID(c)
	envID := c.Param("id")

	var env models.Environment
	if err := h.db.Where("id = ? AND owner_id = ?", envID, userID).First(&env).Error; err != nil {
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
	if err := h.db.Where("id = ? AND owner_id = ?", envID, userID).First(&env).Error; err != nil {
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
	if err := h.db.Where("id = ? AND owner_id = ?", envID, userID).First(&env).Error; err != nil {
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
	if err := h.db.Where("id = ? AND owner_id = ?", envID, userID).First(&env).Error; err != nil {
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
	userID := getUserID(c)
	envID := c.Param("id")

	var env models.Environment
	if err := h.db.Where("id = ? AND owner_id = ?", envID, userID).First(&env).Error; err != nil {
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
	userID := getUserID(c)
	envID := c.Param("id")

	var env models.Environment
	if err := h.db.Where("id = ? AND owner_id = ?", envID, userID).First(&env).Error; err != nil {
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

// Helper function to get user ID from context
func getUserID(c *gin.Context) uuid.UUID {
	user, exists := c.Get("user")
	if !exists {
		return uuid.Nil
	}
	return user.(*models.User).ID
}
