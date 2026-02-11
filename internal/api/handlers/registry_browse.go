package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	nebicrypto "github.com/nebari-dev/nebi/internal/crypto"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/oci"
	"github.com/nebari-dev/nebi/internal/service"
	"gorm.io/gorm"
)

// RegistryBrowseHandler handles browsing and importing from OCI registries
type RegistryBrowseHandler struct {
	db     *gorm.DB
	svc    *service.WorkspaceService
	encKey []byte
}

// NewRegistryBrowseHandler creates a new registry browse handler
func NewRegistryBrowseHandler(db *gorm.DB, svc *service.WorkspaceService, encKey []byte) *RegistryBrowseHandler {
	return &RegistryBrowseHandler{db: db, svc: svc, encKey: encKey}
}

// ImportRequest is the JSON body for the import endpoint
type ImportRequest struct {
	Repository string `json:"repository" binding:"required"`
	Tag        string `json:"tag" binding:"required"`
	Name       string `json:"name" binding:"required"`
}

// ListRepositories lists repositories in a registry.
// Falls back to Quay.io API or known publications if /v2/_catalog is not supported.
func (h *RegistryBrowseHandler) ListRepositories(c *gin.Context) {
	registryID := c.Param("id")
	search := c.Query("search")

	var registry models.OCIRegistry
	if err := h.db.Where("id = ?", registryID).First(&registry).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Registry not found"})
		return
	}

	password, err := nebicrypto.DecryptField(registry.Password, h.encKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to decrypt registry credentials"})
		return
	}

	host, namespace := oci.ParseRegistryURL(registry.URL)

	opts := oci.BrowseOptions{
		RegistryHost: host,
		Username:     registry.Username,
		Password:     password,
	}

	catalogFailed := false
	repos, err := oci.ListRepositories(c.Request.Context(), opts)
	if err != nil || len(repos) == 0 {
		catalogFailed = true

		// Try Quay.io API fallback if the host is quay.io
		if strings.Contains(host, "quay.io") {
			// Derive namespace from URL path or default_repository
			ns := namespace
			if ns == "" && registry.DefaultRepository != "" {
				parts := strings.SplitN(registry.DefaultRepository, "/", 2)
				ns = parts[0]
			}
			if ns != "" {
				apiToken, _ := nebicrypto.DecryptField(registry.APIToken, h.encKey)
				quayRepos, quayErr := oci.ListRepositoriesViaQuayAPI(c.Request.Context(), host, ns, apiToken)
				if quayErr == nil && len(quayRepos) > 0 {
					repos = quayRepos
					catalogFailed = false
				}
			}
		}

		if repos == nil {
			repos = []oci.RepositoryInfo{}
		}
	}

	// Always merge in known publications from the DB
	knownRepos := h.fallbackRepositories(registry.ID.String(), "")
	seen := make(map[string]bool)
	for _, r := range repos {
		seen[r.Name] = true
	}
	for _, r := range knownRepos {
		if !seen[r.Name] {
			repos = append(repos, r)
			seen[r.Name] = true
		}
	}

	// Apply search filter if provided
	if search != "" {
		var filtered []oci.RepositoryInfo
		for _, repo := range repos {
			if strings.Contains(strings.ToLower(repo.Name), strings.ToLower(search)) {
				filtered = append(filtered, repo)
			}
		}
		repos = filtered
	}

	if repos == nil {
		repos = []oci.RepositoryInfo{}
	}

	c.JSON(http.StatusOK, gin.H{
		"repositories": repos,
		"fallback":     catalogFailed,
	})
}

// fallbackRepositories returns distinct repositories from publication records
func (h *RegistryBrowseHandler) fallbackRepositories(registryID, search string) []oci.RepositoryInfo {
	var repositories []string
	query := h.db.Model(&models.Publication{}).
		Where("registry_id = ?", registryID).
		Distinct("repository").
		Pluck("repository", &repositories)

	if query.Error != nil {
		return []oci.RepositoryInfo{}
	}

	var result []oci.RepositoryInfo
	for _, repo := range repositories {
		if search == "" || strings.Contains(strings.ToLower(repo), strings.ToLower(search)) {
			result = append(result, oci.RepositoryInfo{Name: repo})
		}
	}

	if result == nil {
		result = []oci.RepositoryInfo{}
	}
	return result
}

// ListTags lists tags for a repository in a registry
func (h *RegistryBrowseHandler) ListTags(c *gin.Context) {
	registryID := c.Param("id")
	repoName := c.Query("repo")

	if repoName == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "repo query parameter is required"})
		return
	}

	var registry models.OCIRegistry
	if err := h.db.Where("id = ?", registryID).First(&registry).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Registry not found"})
		return
	}

	password, err := nebicrypto.DecryptField(registry.Password, h.encKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to decrypt registry credentials"})
		return
	}

	host, _ := oci.ParseRegistryURL(registry.URL)
	repoRef := fmt.Sprintf("%s/%s", host, repoName)
	opts := oci.BrowseOptions{
		RegistryHost: host,
		Username:     registry.Username,
		Password:     password,
	}

	tags, err := oci.ListTags(c.Request.Context(), repoRef, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: fmt.Sprintf("Failed to list tags: %v", err)})
		return
	}

	if tags == nil {
		tags = []oci.TagInfo{}
	}

	c.JSON(http.StatusOK, gin.H{"tags": tags})
}

// ImportEnvironment pulls an environment from a registry and creates a workspace
func (h *RegistryBrowseHandler) ImportEnvironment(c *gin.Context) {
	registryID := c.Param("id")
	userID := getUserID(c)

	var req ImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	var registry models.OCIRegistry
	if err := h.db.Where("id = ?", registryID).First(&registry).Error; err != nil {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Registry not found"})
		return
	}

	password, err := nebicrypto.DecryptField(registry.Password, h.encKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: "Failed to decrypt registry credentials"})
		return
	}

	host, _ := oci.ParseRegistryURL(registry.URL)
	repoRef := fmt.Sprintf("%s/%s", host, req.Repository)
	opts := oci.BrowseOptions{
		RegistryHost: host,
		Username:     registry.Username,
		Password:     password,
	}

	result, err := oci.PullEnvironment(c.Request.Context(), repoRef, req.Tag, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{Error: fmt.Sprintf("Failed to pull environment: %v", err)})
		return
	}

	ws, err := h.svc.Create(c.Request.Context(), service.CreateRequest{
		Name:           req.Name,
		PackageManager: "pixi",
		PixiToml:       result.PixiToml,
	}, userID)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	c.JSON(http.StatusCreated, ws)
}
