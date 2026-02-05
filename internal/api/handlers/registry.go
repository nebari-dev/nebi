package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
)

// RegistryHandler handles OCI registry operations
type RegistryHandler struct {
	db *gorm.DB
}

// NewRegistryHandler creates a new registry handler
func NewRegistryHandler(db *gorm.DB) *RegistryHandler {
	return &RegistryHandler{db: db}
}

// Request/Response types

type CreateRegistryRequest struct {
	Name              string `json:"name" binding:"required"`
	URL               string `json:"url" binding:"required"`
	Username          string `json:"username"`
	Password          string `json:"password"`
	IsDefault         bool   `json:"is_default"`
	DefaultRepository string `json:"default_repository"`
}

type UpdateRegistryRequest struct {
	Name              *string `json:"name"`
	URL               *string `json:"url"`
	Username          *string `json:"username"`
	Password          *string `json:"password"`
	IsDefault         *bool   `json:"is_default"`
	DefaultRepository *string `json:"default_repository"`
}

type RegistryResponse struct {
	ID                uuid.UUID `json:"id"`
	Name              string    `json:"name"`
	URL               string    `json:"url"`
	Username          string    `json:"username"`
	IsDefault         bool      `json:"is_default"`
	DefaultRepository string    `json:"default_repository"`
	CreatedAt         string    `json:"created_at"`
}

// ListRegistries godoc
// @Summary List all OCI registries
// @Description Get list of all configured OCI registries (admin only)
// @Tags admin
// @Security BearerAuth
// @Accept json
// @Produce json
// @Success 200 {array} RegistryResponse
// @Failure 500 {object} ErrorResponse
// @Router /admin/registries [get]
func (h *RegistryHandler) ListRegistries(c *gin.Context) {
	var registries []models.OCIRegistry
	if err := h.db.Find(&registries).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch registries"})
		return
	}

	response := make([]RegistryResponse, len(registries))
	for i, reg := range registries {
		response[i] = RegistryResponse{
			ID:                reg.ID,
			Name:              reg.Name,
			URL:               reg.URL,
			Username:          reg.Username,
			IsDefault:         reg.IsDefault,
			DefaultRepository: reg.DefaultRepository,
			CreatedAt:         reg.CreatedAt.Format("2006-01-02 15:04:05"),
		}
	}

	c.JSON(http.StatusOK, response)
}

// CreateRegistry godoc
// @Summary Create a new OCI registry
// @Description Add a new OCI registry configuration (admin only)
// @Tags admin
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param registry body CreateRegistryRequest true "Registry details"
// @Success 201 {object} RegistryResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /admin/registries [post]
func (h *RegistryHandler) CreateRegistry(c *gin.Context) {
	var req CreateRegistryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	// Get current user from context
	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "User not found"})
		return
	}
	userID := user.(*models.User).ID

	// If setting as default, unset other defaults
	if req.IsDefault {
		h.db.Model(&models.OCIRegistry{}).Where("is_default = ?", true).Update("is_default", false)
	}

	registry := models.OCIRegistry{
		Name:              req.Name,
		URL:               req.URL,
		Username:          req.Username,
		Password:          req.Password, // TODO: Encrypt password
		IsDefault:         req.IsDefault,
		DefaultRepository: req.DefaultRepository,
		CreatedBy:         userID,
	}

	if err := h.db.Create(&registry).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to create registry"})
		return
	}

	c.JSON(http.StatusCreated, RegistryResponse{
		ID:                registry.ID,
		Name:              registry.Name,
		URL:               registry.URL,
		Username:          registry.Username,
		IsDefault:         registry.IsDefault,
		DefaultRepository: registry.DefaultRepository,
		CreatedAt:         registry.CreatedAt.Format("2006-01-02 15:04:05"),
	})
}

// GetRegistry godoc
// @Summary Get a registry by ID
// @Description Get details of a specific OCI registry (admin only)
// @Tags admin
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Registry ID"
// @Success 200 {object} RegistryResponse
// @Failure 404 {object} ErrorResponse
// @Router /admin/registries/{id} [get]
func (h *RegistryHandler) GetRegistry(c *gin.Context) {
	id := c.Param("id")

	var registry models.OCIRegistry
	if err := h.db.Where("id = ?", id).First(&registry).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Registry not found"})
		return
	}

	c.JSON(http.StatusOK, RegistryResponse{
		ID:                registry.ID,
		Name:              registry.Name,
		URL:               registry.URL,
		Username:          registry.Username,
		IsDefault:         registry.IsDefault,
		DefaultRepository: registry.DefaultRepository,
		CreatedAt:         registry.CreatedAt.Format("2006-01-02 15:04:05"),
	})
}

// UpdateRegistry godoc
// @Summary Update a registry
// @Description Update OCI registry details (admin only)
// @Tags admin
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Registry ID"
// @Param registry body UpdateRegistryRequest true "Registry updates"
// @Success 200 {object} RegistryResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /admin/registries/{id} [put]
func (h *RegistryHandler) UpdateRegistry(c *gin.Context) {
	id := c.Param("id")

	var req UpdateRegistryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	var registry models.OCIRegistry
	if err := h.db.Where("id = ?", id).First(&registry).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Registry not found"})
		return
	}

	// Update fields
	if req.Name != nil {
		registry.Name = *req.Name
	}
	if req.URL != nil {
		registry.URL = *req.URL
	}
	if req.Username != nil {
		registry.Username = *req.Username
	}
	if req.Password != nil {
		registry.Password = *req.Password // TODO: Encrypt
	}
	if req.IsDefault != nil {
		if *req.IsDefault {
			// Unset other defaults
			h.db.Model(&models.OCIRegistry{}).Where("is_default = ?", true).Update("is_default", false)
		}
		registry.IsDefault = *req.IsDefault
	}
	if req.DefaultRepository != nil {
		registry.DefaultRepository = *req.DefaultRepository
	}

	if err := h.db.Save(&registry).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to update registry"})
		return
	}

	c.JSON(http.StatusOK, RegistryResponse{
		ID:                registry.ID,
		Name:              registry.Name,
		URL:               registry.URL,
		Username:          registry.Username,
		IsDefault:         registry.IsDefault,
		DefaultRepository: registry.DefaultRepository,
		CreatedAt:         registry.CreatedAt.Format("2006-01-02 15:04:05"),
	})
}

// DeleteRegistry godoc
// @Summary Delete a registry
// @Description Delete an OCI registry configuration (admin only)
// @Tags admin
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Registry ID"
// @Success 204
// @Failure 404 {object} ErrorResponse
// @Router /admin/registries/{id} [delete]
func (h *RegistryHandler) DeleteRegistry(c *gin.Context) {
	id := c.Param("id")

	var registry models.OCIRegistry
	if err := h.db.Where("id = ?", id).First(&registry).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Registry not found"})
		return
	}

	if err := h.db.Delete(&registry).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to delete registry"})
		return
	}

	c.Status(http.StatusNoContent)
}

// ListPublicRegistries godoc
// @Summary List available registries (public info only)
// @Description Get list of registries for users to select from (no credentials exposed)
// @Tags registries
// @Security BearerAuth
// @Accept json
// @Produce json
// @Success 200 {array} RegistryResponse
// @Router /registries [get]
func (h *RegistryHandler) ListPublicRegistries(c *gin.Context) {
	var registries []models.OCIRegistry
	if err := h.db.Find(&registries).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to fetch registries"})
		return
	}

	response := make([]RegistryResponse, len(registries))
	for i, reg := range registries {
		response[i] = RegistryResponse{
			ID:                reg.ID,
			Name:              reg.Name,
			URL:               reg.URL,
			Username:          "", // Don't expose username to regular users
			IsDefault:         reg.IsDefault,
			DefaultRepository: reg.DefaultRepository,
			CreatedAt:         reg.CreatedAt.Format("2006-01-02 15:04:05"),
		}
	}

	c.JSON(http.StatusOK, response)
}
