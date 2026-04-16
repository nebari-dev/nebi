package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/service"
)

// RegistryHandler handles OCI registry operations
type RegistryHandler struct {
	svc *service.RegistryService
}

// NewRegistryHandler creates a new registry handler
func NewRegistryHandler(svc *service.RegistryService) *RegistryHandler {
	return &RegistryHandler{svc: svc}
}

// ListRegistries godoc
// @Summary List all OCI registries
// @Description Get list of all configured OCI registries (admin only)
// @Tags admin
// @Security BearerAuth
// @Produce json
// @Success 200 {array} service.RegistryResult
// @Failure 500 {object} ErrorResponse
// @Router /admin/registries [get]
func (h *RegistryHandler) ListRegistries(c *gin.Context) {
	registries, err := h.svc.ListRegistries()
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, registries)
}

// CreateRegistry godoc
// @Summary Create a new OCI registry
// @Description Add a new OCI registry configuration (admin only)
// @Tags admin
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param registry body CreateRegistryRequest true "Registry details"
// @Success 201 {object} service.RegistryResult
// @Failure 400 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /admin/registries [post]
func (h *RegistryHandler) CreateRegistry(c *gin.Context) {
	var req CreateRegistryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	user, exists := c.Get("user")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Error: "User not found"})
		return
	}
	userID := user.(*models.User).ID

	result, err := h.svc.CreateRegistry(service.CreateRegistryReq{
		Name:      req.Name,
		URL:       req.URL,
		Username:  req.Username,
		Password:  req.Password,
		APIToken:  req.APIToken,
		IsDefault: req.IsDefault,
		Namespace: req.Namespace,
		CreatedBy: userID,
	})
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, result)
}

// GetRegistry godoc
// @Summary Get a registry by ID
// @Description Get details of a specific OCI registry (admin only)
// @Tags admin
// @Security BearerAuth
// @Produce json
// @Param id path string true "Registry ID"
// @Success 200 {object} service.RegistryResult
// @Failure 404 {object} ErrorResponse
// @Router /admin/registries/{id} [get]
func (h *RegistryHandler) GetRegistry(c *gin.Context) {
	result, err := h.svc.GetRegistry(c.Param("id"))
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
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
// @Success 200 {object} service.RegistryResult
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /admin/registries/{id} [put]
func (h *RegistryHandler) UpdateRegistry(c *gin.Context) {
	var req UpdateRegistryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	result, err := h.svc.UpdateRegistry(c.Param("id"), service.UpdateRegistryReq{
		Name:      req.Name,
		URL:       req.URL,
		Username:  req.Username,
		Password:  req.Password,
		APIToken:  req.APIToken,
		IsDefault: req.IsDefault,
		Namespace: req.Namespace,
	})
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// DeleteRegistry godoc
// @Summary Delete a registry
// @Description Delete an OCI registry configuration (admin only)
// @Tags admin
// @Security BearerAuth
// @Param id path string true "Registry ID"
// @Success 204
// @Failure 404 {object} ErrorResponse
// @Router /admin/registries/{id} [delete]
func (h *RegistryHandler) DeleteRegistry(c *gin.Context) {
	if err := h.svc.DeleteRegistry(c.Param("id")); err != nil {
		handleServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// ListPublicRegistries godoc
// @Summary List available registries (public info only)
// @Description Get list of registries for users to select from (no credentials exposed)
// @Tags registries
// @Security BearerAuth
// @Produce json
// @Success 200 {array} service.RegistryResult
// @Router /registries [get]
func (h *RegistryHandler) ListPublicRegistries(c *gin.Context) {
	registries, err := h.svc.ListPublicRegistries()
	if err != nil {
		handleServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, registries)
}

// --- Request types ---

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
