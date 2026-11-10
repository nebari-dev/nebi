package registry

// Owner: U7 (publish). Registries page, add/edit/remove registry,
// capability.

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/nebari-dev/nebi/internal/client/sdk/shared"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/oci"
	"github.com/nebari-dev/nebi/internal/store"
	"gorm.io/gorm"
)

// UpdateRegistryRequest edits a registry (PUT /client/registries/:id).
// Nil fields are left unchanged. Password nil keeps the stored password,
// "" removes it. IsDefault true makes this the default registry; false
// clears the flag on this registry only.
type UpdateRegistryRequest struct {
	Name      *string `json:"name,omitempty"`
	URL       *string `json:"url,omitempty"`
	Namespace *string `json:"namespace,omitempty"`
	Username  *string `json:"username,omitempty"`
	Password  *string `json:"password,omitempty"`
	IsDefault *bool   `json:"is_default,omitempty"`
}

var (
	registryHostRE = regexp.MustCompile(`^(?i)[a-z0-9]([a-z0-9.-]*[a-z0-9])?(:\d+)?(/[a-z0-9._/-]+)?$`)
	namespaceRE    = regexp.MustCompile(`^[a-z0-9]([a-z0-9._/-]*[a-z0-9])?$`)
)

// Field names of registry validation errors.
const (
	fieldName      = "name"
	fieldURL       = "url"
	fieldNamespace = "namespace"
	fieldUsername  = "username"
)

// ListRegistries returns configured registries with capabilities, the
// default registry first, then by name.
func (s *Service) ListRegistries(ctx context.Context) ([]shared.RegistryItem, error) {
	var rows []models.OCIRegistry
	if err := s.db(ctx).Where("deleted_at IS NULL").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("listing registries: %w", err)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].IsDefault != rows[j].IsDefault {
			return rows[i].IsDefault
		}
		return strings.ToLower(rows[i].Name) < strings.ToLower(rows[j].Name)
	})
	items := make([]shared.RegistryItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, registryItem(r))
	}
	return items, nil
}

// GetRegistry returns one registry.
func (s *Service) GetRegistry(ctx context.Context, id uuid.UUID) (*shared.RegistryItem, error) {
	reg, err := s.loadRegistry(ctx, id)
	if err != nil {
		return nil, err
	}
	item := registryItem(*reg)
	return &item, nil
}

// ResolveRegistry finds a registry by UUID or by name (case-insensitive).
// An empty ref returns the default registry.
func (s *Service) ResolveRegistry(ctx context.Context, ref string) (*shared.RegistryItem, error) {
	ref = strings.TrimSpace(ref)
	var reg models.OCIRegistry
	q := s.db(ctx).Where("deleted_at IS NULL")
	var err error
	switch id, perr := uuid.Parse(ref); {
	case ref == "":
		err = q.Where("is_default = ?", true).First(&reg).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("no default registry; add one or pass a registry name: %w", shared.ErrNotFound)
		}
	case perr == nil:
		err = q.Where("id = ?", id).First(&reg).Error
	default:
		err = q.Where("LOWER(name) = LOWER(?)", ref).First(&reg).Error
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("registry %q: %w", ref, shared.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	item := registryItem(reg)
	return &item, nil
}

// AddRegistry stores a registry; the password goes to the keyring.
func (s *Service) AddRegistry(ctx context.Context, req shared.AddRegistryRequest) (*shared.RegistryItem, error) {
	name := strings.TrimSpace(req.Name)
	url, err := normalizeRegistryURL(req.URL)
	if err != nil {
		return nil, err
	}
	if err := validateRegistryName(name); err != nil {
		return nil, err
	}
	namespace := strings.Trim(strings.TrimSpace(req.Namespace), "/")
	if err := validateNamespace(namespace); err != nil {
		return nil, err
	}
	username := strings.TrimSpace(req.Username)
	if req.Password != "" && username == "" {
		return nil, &shared.ValidationError{Field: fieldUsername, Message: "Enter the username for this password."}
	}
	if err := s.checkNameFree(ctx, name, uuid.Nil); err != nil {
		return nil, err
	}

	row := models.OCIRegistry{
		Name:      name,
		URL:       url,
		Namespace: namespace,
		Username:  username,
		CreatedBy: s.deps.LocalUserID,
	}
	err = s.db(ctx).Transaction(func(tx *gorm.DB) error {
		var defaults int64
		if err := tx.Model(&models.OCIRegistry{}).Where("deleted_at IS NULL AND is_default = ?", true).Count(&defaults).Error; err != nil {
			return err
		}
		row.IsDefault = req.IsDefault || defaults == 0
		if row.IsDefault {
			if err := tx.Model(&models.OCIRegistry{}).Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
				return err
			}
		}
		return tx.Create(&row).Error
	})
	if err != nil {
		return nil, fmt.Errorf("adding registry: %w", err)
	}
	if req.Password != "" {
		if err := s.local.SetRegistryPassword(ctx, name, req.Password); err != nil {
			_ = s.db(ctx).Delete(&row).Error
			return nil, err
		}
	}
	item := registryItem(row)
	return &item, nil
}

// UpdateRegistry edits a registry. Renaming moves the stored password to
// the new name.
func (s *Service) UpdateRegistry(ctx context.Context, id uuid.UUID, req UpdateRegistryRequest) (*shared.RegistryItem, error) {
	reg, err := s.loadRegistry(ctx, id)
	if err != nil {
		return nil, err
	}
	oldName := reg.Name
	updated := *reg
	if req.Name != nil {
		updated.Name = strings.TrimSpace(*req.Name)
		if err := validateRegistryName(updated.Name); err != nil {
			return nil, err
		}
		if err := s.checkNameFree(ctx, updated.Name, id); err != nil {
			return nil, err
		}
	}
	if req.URL != nil {
		if updated.URL, err = normalizeRegistryURL(*req.URL); err != nil {
			return nil, err
		}
	}
	if req.Namespace != nil {
		updated.Namespace = strings.Trim(strings.TrimSpace(*req.Namespace), "/")
		if err := validateNamespace(updated.Namespace); err != nil {
			return nil, err
		}
	}
	if req.Username != nil {
		updated.Username = strings.TrimSpace(*req.Username)
	}
	if req.Password != nil && *req.Password != "" && updated.Username == "" {
		return nil, &shared.ValidationError{Field: fieldUsername, Message: "Enter the username for this password."}
	}

	// Read the password before a rename so it can move to the new name.
	password := ""
	if req.Password != nil {
		password = *req.Password
	} else if updated.Name != oldName {
		password, _ = s.local.RegistryPassword(ctx, oldName)
	}

	err = s.db(ctx).Transaction(func(tx *gorm.DB) error {
		if req.IsDefault != nil {
			if *req.IsDefault {
				if err := tx.Model(&models.OCIRegistry{}).Where("is_default = ? AND id <> ?", true, id).Update("is_default", false).Error; err != nil {
					return err
				}
			}
			updated.IsDefault = *req.IsDefault
		}
		if err := tx.Model(&models.OCIRegistry{}).Where("id = ?", id).Updates(map[string]any{
			"name":       updated.Name,
			"url":        updated.URL,
			"namespace":  updated.Namespace,
			"username":   updated.Username,
			"is_default": updated.IsDefault,
		}).Error; err != nil {
			return err
		}
		return dropLegacyRegistry(tx, id, oldName)
	})
	if err != nil {
		return nil, fmt.Errorf("updating registry: %w", err)
	}

	switch {
	case updated.Name != oldName:
		if password != "" {
			if err := s.local.SetRegistryPassword(ctx, updated.Name, password); err != nil {
				return nil, err
			}
		}
		_ = s.local.DeleteRegistryPassword(ctx, oldName)
	case req.Password != nil:
		if err := s.local.SetRegistryPassword(ctx, updated.Name, *req.Password); err != nil {
			return nil, err
		}
	}
	item := registryItem(updated)
	return &item, nil
}

// RemoveRegistry deletes a registry and its keyring password. Local
// publication records that point at it are removed too: with the registry
// gone this machine can no longer manage those tags (the tags themselves
// stay in the registry). When the removed registry was the default, the
// first remaining registry by name becomes the default.
func (s *Service) RemoveRegistry(ctx context.Context, id uuid.UUID) error {
	reg, err := s.loadRegistry(ctx, id)
	if err != nil {
		return err
	}
	err = s.db(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("registry_id = ?", id).Delete(&models.Publication{}).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", id).Delete(&models.OCIRegistry{}).Error; err != nil {
			return err
		}
		if err := dropLegacyRegistry(tx, id, reg.Name); err != nil {
			return err
		}
		if reg.IsDefault {
			var next models.OCIRegistry
			err := tx.Where("deleted_at IS NULL").Order("name").First(&next).Error
			if err == nil {
				return tx.Model(&next).Update("is_default", true).Error
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("removing registry: %w", err)
	}
	return s.local.DeleteRegistryPassword(ctx, reg.Name)
}

// ---------------------------------------------------------------- helpers

func (s *Service) db(ctx context.Context) *gorm.DB { return s.deps.DB.WithContext(ctx) }

func (s *Service) loadRegistry(ctx context.Context, id uuid.UUID) (*models.OCIRegistry, error) {
	var reg models.OCIRegistry
	err := s.db(ctx).Where("id = ? AND deleted_at IS NULL", id).First(&reg).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("registry %s: %w", id, shared.ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	return &reg, nil
}

func (s *Service) checkNameFree(ctx context.Context, name string, except uuid.UUID) error {
	var count int64
	q := s.db(ctx).Model(&models.OCIRegistry{}).Where("LOWER(name) = LOWER(?)", name)
	if except != uuid.Nil {
		q = q.Where("id <> ?", except)
	}
	if err := q.Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return &shared.ValidationError{Field: fieldName, Message: fmt.Sprintf("A registry called %s already exists.", name)}
	}
	return nil
}

// dropLegacyRegistry deletes the pre-#649 CLI copy of a registry
// (local_registries) so local.Migrate does not import it again after the
// registry was renamed or removed.
func dropLegacyRegistry(tx *gorm.DB, id uuid.UUID, name string) error {
	if !tx.Migrator().HasTable(&store.LocalRegistry{}) {
		return nil
	}
	return tx.Unscoped().Where("id = ? OR name = ?", id, name).Delete(&store.LocalRegistry{}).Error
}

func validateRegistryName(name string) error {
	if name == "" {
		return &shared.ValidationError{Field: fieldName, Message: "Enter a registry name."}
	}
	if len(name) > 100 {
		return &shared.ValidationError{Field: fieldName, Message: "Use 100 characters or fewer."}
	}
	return nil
}

func validateNamespace(ns string) error {
	if ns == "" {
		return nil
	}
	if !namespaceRE.MatchString(ns) {
		return &shared.ValidationError{Field: fieldNamespace, Message: "Use lowercase letters, numbers, dots, hyphens, underscores or slashes."}
	}
	return nil
}

// normalizeRegistryURL trims the URL, drops an https:// scheme and a
// trailing slash, and keeps an explicit http:// (plain-HTTP registries for
// local testing).
func normalizeRegistryURL(raw string) (string, error) {
	u := strings.TrimSpace(raw)
	if u == "" {
		return "", &shared.ValidationError{Field: fieldURL, Message: "Enter the registry’s URL."}
	}
	stripped, plainHTTP := oci.StripScheme(u)
	stripped = strings.TrimRight(stripped, "/")
	if !registryHostRE.MatchString(stripped) {
		return "", &shared.ValidationError{Field: fieldURL, Message: "That doesn’t look like a registry URL. Try ghcr.io."}
	}
	if plainHTTP {
		return "http://" + stripped, nil
	}
	return stripped, nil
}

// endpoint splits a registry row into its host, the effective namespace
// (the namespace field, else the URL path) and whether it uses plain HTTP.
func endpoint(reg models.OCIRegistry) (host, namespace string, plainHTTP bool) {
	host, namespace, plainHTTP = oci.ParseRegistryURLFull(reg.URL)
	if reg.Namespace != "" {
		namespace = reg.Namespace
	}
	return host, strings.Trim(namespace, "/"), plainHTTP
}

func registryItem(r models.OCIRegistry) shared.RegistryItem {
	host, namespace, _ := endpoint(r)
	can, reason := oci.TagDeletionSupport(host)
	return shared.RegistryItem{
		ID:              r.ID,
		Name:            r.Name,
		URL:             r.URL,
		Namespace:       namespace,
		Username:        r.Username,
		HasCredentials:  r.Username != "",
		IsDefault:       r.IsDefault,
		CanUnpublish:    can,
		UnpublishReason: reason,
	}
}

// ociEndpoint builds the oci.Registry (host + credentials, no namespace:
// repositories carry their full path) for a registry row.
func (s *Service) ociEndpoint(ctx context.Context, reg models.OCIRegistry) (oci.Registry, error) {
	host, _, plainHTTP := endpoint(reg)
	out := oci.Registry{Host: host, Username: reg.Username, PlainHTTP: plainHTTP}
	if reg.Username != "" {
		pw, err := s.local.RegistryPassword(ctx, reg.Name)
		if err != nil {
			return out, err
		}
		out.Password = pw
	}
	return out, nil
}
