package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	nebicrypto "github.com/nebari-dev/nebi/internal/crypto"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
)

// RegistryHandler handles OCI registry operations
type RegistryHandler struct {
	db     *gorm.DB
	encKey []byte
}

// NewRegistryHandler creates a new registry handler
func NewRegistryHandler(db *gorm.DB, encKey []byte) *RegistryHandler {
	return &RegistryHandler{db: db, encKey: encKey}
}

// Request/Response types

type CreateRegistryRequest struct {
	Name      string `json:"name" binding:"required"`
	URL       string `json:"url" binding:"required"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	APIToken  string `json:"api_token"`
	IsDefault bool   `json:"is_default"`
	Namespace string `json:"namespace"`
}

type UpdateRegistryRequest struct {
	Name      *string `json:"name"`
	URL       *string `json:"url"`
	Username  *string `json:"username"`
	Password  *string `json:"password"`
	APIToken  *string `json:"api_token"`
	IsDefault *bool   `json:"is_default"`
	Namespace *string `json:"namespace"`
}

type RegistryResponse struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	URL         string    `json:"url"`
	Username    string    `json:"username"`
	HasAPIToken bool      `json:"has_api_token"`
	IsDefault   bool      `json:"is_default"`
	Namespace   string    `json:"namespace"`
	CreatedAt   string    `json:"created_at"`
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
		// Decrypt to check HasAPIToken accurately (encrypted values are non-empty but not real tokens)
		apiToken, err := nebicrypto.DecryptField(reg.APIToken, h.encKey)
		if err != nil {
			slog.Error("Failed to decrypt API token", "registry_id", reg.ID, "error", err)
		}
		response[i] = RegistryResponse{
			ID:          reg.ID,
			Name:        reg.Name,
			URL:         reg.URL,
			Username:    reg.Username,
			HasAPIToken: apiToken != "",
			IsDefault:   reg.IsDefault,
			Namespace:   reg.Namespace,
			CreatedAt:   reg.CreatedAt.Format("2006-01-02 15:04:05"),
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

	encPassword, err := nebicrypto.EncryptField(req.Password, h.encKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to encrypt credentials"})
		return
	}
	encAPIToken, err := nebicrypto.EncryptField(req.APIToken, h.encKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to encrypt credentials"})
		return
	}

	registry := models.OCIRegistry{
		Name:      req.Name,
		URL:       req.URL,
		Username:  req.Username,
		Password:  encPassword,
		APIToken:  encAPIToken,
		IsDefault: req.IsDefault,
		Namespace: req.Namespace,
		CreatedBy: userID,
	}

	if err := h.db.Create(&registry).Error; err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "duplicate") {
			c.JSON(http.StatusConflict, ErrorResponse{Error: fmt.Sprintf("Registry with name '%s' already exists", req.Name)})
			return
		}
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to create registry"})
		return
	}

	c.JSON(http.StatusCreated, RegistryResponse{
		ID:          registry.ID,
		Name:        registry.Name,
		URL:         registry.URL,
		Username:    registry.Username,
		HasAPIToken: req.APIToken != "",
		IsDefault:   registry.IsDefault,
		Namespace:   registry.Namespace,
		CreatedAt:   registry.CreatedAt.Format("2006-01-02 15:04:05"),
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

	apiToken, err := nebicrypto.DecryptField(registry.APIToken, h.encKey)
	if err != nil {
		slog.Error("Failed to decrypt API token", "registry_id", registry.ID, "error", err)
	}

	c.JSON(http.StatusOK, RegistryResponse{
		ID:          registry.ID,
		Name:        registry.Name,
		URL:         registry.URL,
		Username:    registry.Username,
		HasAPIToken: apiToken != "",
		IsDefault:   registry.IsDefault,
		Namespace:   registry.Namespace,
		CreatedAt:   registry.CreatedAt.Format("2006-01-02 15:04:05"),
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
		encPwd, err := nebicrypto.EncryptField(*req.Password, h.encKey)
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to encrypt credentials"})
			return
		}
		registry.Password = encPwd
	}
	if req.APIToken != nil {
		encToken, err := nebicrypto.EncryptField(*req.APIToken, h.encKey)
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to encrypt credentials"})
			return
		}
		registry.APIToken = encToken
	}
	if req.IsDefault != nil {
		if *req.IsDefault {
			// Unset other defaults
			h.db.Model(&models.OCIRegistry{}).Where("is_default = ?", true).Update("is_default", false)
		}
		registry.IsDefault = *req.IsDefault
	}
	if req.Namespace != nil {
		registry.Namespace = *req.Namespace
	}

	if err := h.db.Save(&registry).Error; err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to update registry"})
		return
	}

	updatedAPIToken, err := nebicrypto.DecryptField(registry.APIToken, h.encKey)
	if err != nil {
		slog.Error("Failed to decrypt API token", "registry_id", registry.ID, "error", err)
	}

	c.JSON(http.StatusOK, RegistryResponse{
		ID:          registry.ID,
		Name:        registry.Name,
		URL:         registry.URL,
		Username:    registry.Username,
		HasAPIToken: updatedAPIToken != "",
		IsDefault:   registry.IsDefault,
		Namespace:   registry.Namespace,
		CreatedAt:   registry.CreatedAt.Format("2006-01-02 15:04:05"),
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
			ID:          reg.ID,
			Name:        reg.Name,
			URL:         reg.URL,
			Username:    "",    // Don't expose username to regular users
			HasAPIToken: false, // Don't expose token info to regular users
			IsDefault:   reg.IsDefault,
			Namespace:   reg.Namespace,
			CreatedAt:   reg.CreatedAt.Format("2006-01-02 15:04:05"),
		}
	}

	c.JSON(http.StatusOK, response)
}
