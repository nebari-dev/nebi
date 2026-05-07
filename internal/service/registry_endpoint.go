package service

import (
	"fmt"

	nebicrypto "github.com/nebari-dev/nebi/internal/crypto"
	"github.com/nebari-dev/nebi/internal/models"
	"github.com/nebari-dev/nebi/internal/oci"
	"gorm.io/gorm"
)

// registryEndpoint captures everything a service needs to talk to an OCI
// registry: the persisted record (for ID/Name in audit + publication
// rows), the parsed URL, and the decrypted credentials. Construct via
// loadRegistryEndpoint — never directly.
type registryEndpoint struct {
	Registry  *models.OCIRegistry
	Host      string
	Namespace string // may be empty
	Username  string
	Password  string
	PlainHTTP bool
}

// NamespaceRelativeRepoRef composes "host/[namespace/]repo" for callers
// whose repository value is relative to the registry's configured namespace.
func (e *registryEndpoint) NamespaceRelativeRepoRef(repo string) string {
	if e.Namespace != "" {
		return fmt.Sprintf("%s/%s/%s", e.Host, e.Namespace, repo)
	}
	return fmt.Sprintf("%s/%s", e.Host, repo)
}

// RepositoryPathRef composes "host/repository" when repository is already
// the full OCI repository path under the registry host, e.g. "org/name".
func (e *registryEndpoint) RepositoryPathRef(repository string) string {
	return fmt.Sprintf("%s/%s", e.Host, repository)
}

// loadRegistryEndpoint loads a registry by ID, decrypts its password
// (empty input is tolerated for anonymous registries), and parses the
// URL into host/plainHTTP. Returns ErrNotFound if the registry row
// does not exist. id may be a uuid.UUID, a uuid string, or any value
// gorm can compare to the OCIRegistry primary key.
func (s *WorkspaceService) loadRegistryEndpoint(id any) (*registryEndpoint, error) {
	var reg models.OCIRegistry
	if err := s.db.Where("id = ?", id).First(&reg).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var password string
	if reg.Password != "" {
		p, err := nebicrypto.DecryptField(reg.Password, s.encKey)
		if err != nil {
			return nil, fmt.Errorf("decrypt registry credentials: %w", err)
		}
		password = p
	}
	host, _, plainHTTP := oci.ParseRegistryURLFull(reg.URL)
	return &registryEndpoint{
		Registry:  &reg,
		Host:      host,
		Namespace: reg.Namespace,
		Username:  reg.Username,
		Password:  password,
		PlainHTTP: plainHTTP,
	}, nil
}
