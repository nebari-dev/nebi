package service

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	nebicrypto "github.com/nebari-dev/nebi/internal/crypto"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
)

// RegistryService contains business logic for OCI registry operations.
type RegistryService struct {
	db     *gorm.DB
	encKey []byte
}

// NewRegistryService creates a new RegistryService.
func NewRegistryService(db *gorm.DB, encKey []byte) *RegistryService {
	return &RegistryService{db: db, encKey: encKey}
}

// RegistryResult is the response type for registry operations.
type RegistryResult struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	URL         string    `json:"url"`
	Username    string    `json:"username"`
	HasAPIToken bool      `json:"has_api_token"`
	IsDefault   bool      `json:"is_default"`
	Namespace   string    `json:"namespace"`
	CreatedAt   string    `json:"created_at"`
}

// CreateRegistryRequest holds parameters for creating a registry.
type CreateRegistryReq struct {
	Name      string
	URL       string
	Username  string
	Password  string
	APIToken  string
	IsDefault bool
	Namespace string
	CreatedBy uuid.UUID
}

// UpdateRegistryReq holds parameters for updating a registry.
type UpdateRegistryReq struct {
	Name      *string
	URL       *string
	Username  *string
	Password  *string
	APIToken  *string
	IsDefault *bool
	Namespace *string
}

// ListRegistries returns all registries with admin-level detail (includes username, token status).
func (s *RegistryService) ListRegistries() ([]RegistryResult, error) {
	var registries []models.OCIRegistry
	if err := s.db.Find(&registries).Error; err != nil {
		return nil, fmt.Errorf("fetch registries: %w", err)
	}

	result := make([]RegistryResult, len(registries))
	for i, reg := range registries {
		apiToken, err := nebicrypto.DecryptField(reg.APIToken, s.encKey)
		if err != nil {
			slog.Error("Failed to decrypt API token", "registry_id", reg.ID, "error", err)
		}
		result[i] = registryToResult(reg, reg.Username, apiToken != "")
	}
	return result, nil
}

// ListPublicRegistries returns registries with public-safe info (no credentials).
func (s *RegistryService) ListPublicRegistries() ([]RegistryResult, error) {
	var registries []models.OCIRegistry
	if err := s.db.Find(&registries).Error; err != nil {
		return nil, fmt.Errorf("fetch registries: %w", err)
	}

	result := make([]RegistryResult, len(registries))
	for i, reg := range registries {
		result[i] = registryToResult(reg, "", false)
	}
	return result, nil
}

// GetRegistry returns a single registry by ID with admin-level detail.
func (s *RegistryService) GetRegistry(id string) (*RegistryResult, error) {
	var registry models.OCIRegistry
	if err := s.db.Where("id = ?", id).First(&registry).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}

	apiToken, err := nebicrypto.DecryptField(registry.APIToken, s.encKey)
	if err != nil {
		slog.Error("Failed to decrypt API token", "registry_id", registry.ID, "error", err)
	}

	r := registryToResult(registry, registry.Username, apiToken != "")
	return &r, nil
}

// CreateRegistry creates a new registry with encrypted credentials.
func (s *RegistryService) CreateRegistry(req CreateRegistryReq) (*RegistryResult, error) {
	if req.IsDefault {
		s.db.Model(&models.OCIRegistry{}).Where("is_default = ?", true).Update("is_default", false)
	}

	encPassword, err := nebicrypto.EncryptField(req.Password, s.encKey)
	if err != nil {
		return nil, fmt.Errorf("encrypt credentials: %w", err)
	}
	encAPIToken, err := nebicrypto.EncryptField(req.APIToken, s.encKey)
	if err != nil {
		return nil, fmt.Errorf("encrypt credentials: %w", err)
	}

	registry := models.OCIRegistry{
		Name:      req.Name,
		URL:       req.URL,
		Username:  req.Username,
		Password:  encPassword,
		APIToken:  encAPIToken,
		IsDefault: req.IsDefault,
		Namespace: req.Namespace,
		CreatedBy: req.CreatedBy,
	}

	if err := s.db.Create(&registry).Error; err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "duplicate") {
			return nil, &ConflictError{Message: fmt.Sprintf("Registry with name '%s' already exists", req.Name)}
		}
		return nil, fmt.Errorf("create registry: %w", err)
	}

	r := registryToResult(registry, registry.Username, req.APIToken != "")
	return &r, nil
}

// UpdateRegistry updates an existing registry.
func (s *RegistryService) UpdateRegistry(id string, req UpdateRegistryReq) (*RegistryResult, error) {
	var registry models.OCIRegistry
	if err := s.db.Where("id = ?", id).First(&registry).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}

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
		enc, err := nebicrypto.EncryptField(*req.Password, s.encKey)
		if err != nil {
			return nil, fmt.Errorf("encrypt credentials: %w", err)
		}
		registry.Password = enc
	}
	if req.APIToken != nil {
		enc, err := nebicrypto.EncryptField(*req.APIToken, s.encKey)
		if err != nil {
			return nil, fmt.Errorf("encrypt credentials: %w", err)
		}
		registry.APIToken = enc
	}
	if req.IsDefault != nil {
		if *req.IsDefault {
			s.db.Model(&models.OCIRegistry{}).Where("is_default = ?", true).Update("is_default", false)
		}
		registry.IsDefault = *req.IsDefault
	}
	if req.Namespace != nil {
		registry.Namespace = *req.Namespace
	}

	if err := s.db.Save(&registry).Error; err != nil {
		return nil, fmt.Errorf("update registry: %w", err)
	}

	apiToken, err := nebicrypto.DecryptField(registry.APIToken, s.encKey)
	if err != nil {
		slog.Error("Failed to decrypt API token", "registry_id", registry.ID, "error", err)
	}

	r := registryToResult(registry, registry.Username, apiToken != "")
	return &r, nil
}

// DeleteRegistry deletes a registry by ID.
func (s *RegistryService) DeleteRegistry(id string) error {
	var registry models.OCIRegistry
	if err := s.db.Where("id = ?", id).First(&registry).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return ErrNotFound
		}
		return err
	}

	if err := s.db.Delete(&registry).Error; err != nil {
		return fmt.Errorf("delete registry: %w", err)
	}
	return nil
}

// RegistryWithCredentials holds a registry and its decrypted credentials for OCI operations.
type RegistryWithCredentials struct {
	Registry models.OCIRegistry
	Password string
	APIToken string
}

// GetRegistryWithCredentials returns a registry with decrypted credentials for OCI operations.
func (s *RegistryService) GetRegistryWithCredentials(id string) (*RegistryWithCredentials, error) {
	var registry models.OCIRegistry
	if err := s.db.Where("id = ?", id).First(&registry).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}

	password, err := nebicrypto.DecryptField(registry.Password, s.encKey)
	if err != nil {
		return nil, fmt.Errorf("decrypt registry credentials: %w", err)
	}

	apiToken, _ := nebicrypto.DecryptField(registry.APIToken, s.encKey)

	return &RegistryWithCredentials{
		Registry: registry,
		Password: password,
		APIToken: apiToken,
	}, nil
}

// FallbackRepositories returns distinct repository names from publication records for a registry.
func (s *RegistryService) FallbackRepositories(registryID string) []string {
	var repositories []string
	s.db.Model(&models.Publication{}).
		Where("registry_id = ?", registryID).
		Distinct("repository").
		Pluck("repository", &repositories)
	return repositories
}

func registryToResult(reg models.OCIRegistry, username string, hasAPIToken bool) RegistryResult {
	return RegistryResult{
		ID:          reg.ID,
		Name:        reg.Name,
		URL:         reg.URL,
		Username:    username,
		HasAPIToken: hasAPIToken,
		IsDefault:   reg.IsDefault,
		Namespace:   reg.Namespace,
		CreatedAt:   reg.CreatedAt.Format("2006-01-02 15:04:05"),
	}
}
