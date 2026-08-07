package service

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	nebicrypto "github.com/nebari-dev/nebi/internal/crypto"
	"github.com/nebari-dev/nebi/internal/models"
	"gorm.io/gorm"
)

var buildEnvKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

var workspaceArtifactFilenames = []string{"pixi.toml", "pixi.lock"}

// BuildEnvVarResult is the public shape for a configured build variable.
type BuildEnvVarResult struct {
	ID        uuid.UUID `json:"id"`
	Key       string    `json:"key"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BuildEnvVarReq holds parameters for creating or updating a build variable.
type BuildEnvVarReq struct {
	Key   string
	Value string
}

// ListBuildEnvVars returns public metadata for the current user's build variables.
func (s *WorkspaceService) ListBuildEnvVars(userID uuid.UUID) ([]BuildEnvVarResult, error) {
	var vars []models.BuildEnvVar
	if err := s.db.Where("user_id = ?", userID).Order("key ASC").Find(&vars).Error; err != nil {
		return nil, fmt.Errorf("fetch build environment variables: %w", err)
	}

	result := make([]BuildEnvVarResult, len(vars))
	for i, v := range vars {
		result[i] = buildEnvVarToResult(v)
	}
	return result, nil
}

// UpsertBuildEnvVar creates or replaces a single current-user build variable.
func (s *WorkspaceService) UpsertBuildEnvVar(userID uuid.UUID, req BuildEnvVarReq) (*BuildEnvVarResult, error) {
	key, err := normalizeBuildEnvKey(req.Key)
	if err != nil {
		return nil, err
	}
	if req.Value == "" {
		return nil, &ValidationError{Message: "value is required"}
	}

	encryptedValue, err := nebicrypto.EncryptField(req.Value, s.encKey)
	if err != nil {
		return nil, fmt.Errorf("encrypt build environment variable: %w", err)
	}

	var envVar models.BuildEnvVar
	err = s.db.Where("user_id = ? AND key = ?", userID, key).First(&envVar).Error
	switch {
	case err == nil:
		envVar.Value = encryptedValue
		if err := s.db.Save(&envVar).Error; err != nil {
			return nil, fmt.Errorf("update build environment variable: %w", err)
		}
	case err == gorm.ErrRecordNotFound:
		envVar = models.BuildEnvVar{
			UserID: userID,
			Key:    key,
			Value:  encryptedValue,
		}
		if err := s.db.Create(&envVar).Error; err != nil {
			if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "duplicate") {
				return nil, &ConflictError{Message: fmt.Sprintf("build environment variable %q already exists", key)}
			}
			return nil, fmt.Errorf("create build environment variable: %w", err)
		}
	default:
		return nil, err
	}

	result := buildEnvVarToResult(envVar)
	return &result, nil
}

// DeleteBuildEnvVar removes a current-user build variable by key.
func (s *WorkspaceService) DeleteBuildEnvVar(userID uuid.UUID, key string) error {
	normalizedKey, err := normalizeBuildEnvKey(key)
	if err != nil {
		return err
	}

	result := s.db.Where("user_id = ? AND key = ?", userID, normalizedKey).Delete(&models.BuildEnvVar{})
	if result.Error != nil {
		return fmt.Errorf("delete build environment variable: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// BuildEnvironmentSecretsForUser returns decrypted build variables and their values for leak checks.
func (s *WorkspaceService) BuildEnvironmentSecretsForUser(userID uuid.UUID) (map[string]string, []string, error) {
	return s.buildEnvironmentForUser(userID)
}

func (s *WorkspaceService) buildEnvironmentForUser(userID uuid.UUID) (map[string]string, []string, error) {
	var vars []models.BuildEnvVar
	if err := s.db.Where("user_id = ?", userID).Order("key ASC").Find(&vars).Error; err != nil {
		return nil, nil, fmt.Errorf("fetch build environment variables: %w", err)
	}

	env := make(map[string]string, len(vars))
	for _, v := range vars {
		value, err := nebicrypto.DecryptField(v.Value, s.encKey)
		if err != nil {
			return nil, nil, fmt.Errorf("decrypt build environment variable %q: %w", v.Key, err)
		}
		env[v.Key] = value
	}
	return env, buildEnvironmentSecretValues(env), nil
}

// buildEnvironmentSecretValues returns deterministic, non-empty values to check before persisting artifacts.
func buildEnvironmentSecretValues(env map[string]string) []string {
	seen := make(map[string]bool, len(env))
	for _, value := range env {
		if value != "" {
			seen[value] = true
		}
	}

	values := make([]string, 0, len(seen))
	for value := range seen {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

// ReadWorkspaceArtifactContents reads the Pixi files that can be persisted or published.
func ReadWorkspaceArtifactContents(wsPath string) (map[string]string, error) {
	contents := make(map[string]string, len(workspaceArtifactFilenames))
	for _, filename := range workspaceArtifactFilenames {
		content, err := os.ReadFile(filepath.Join(wsPath, filename))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", filename, err)
		}
		contents[filename] = string(content)
	}
	return contents, nil
}

// CheckBuildEnvironmentSecretLeak rejects artifacts that contain a configured build variable value.
func CheckBuildEnvironmentSecretLeak(contents map[string]string, secretValues []string) error {
	if len(contents) == 0 || len(secretValues) == 0 {
		return nil
	}

	filenames := make([]string, 0, len(contents))
	for filename := range contents {
		filenames = append(filenames, filename)
	}
	sort.Strings(filenames)

	for _, filename := range filenames {
		content := contents[filename]
		for _, secretValue := range secretValues {
			if secretValue != "" && strings.Contains(content, secretValue) {
				return &ValidationError{
					Message: fmt.Sprintf("build environment secret value leaked into %s; refusing to persist or publish artifact", filename),
				}
			}
		}
	}

	return nil
}

// EnsureNoBuildEnvironmentSecretLeak checks artifact contents against the current user's build variables.
func (s *WorkspaceService) EnsureNoBuildEnvironmentSecretLeak(userID uuid.UUID, contents map[string]string) error {
	_, secretValues, err := s.buildEnvironmentForUser(userID)
	if err != nil {
		return err
	}
	return CheckBuildEnvironmentSecretLeak(contents, secretValues)
}

func normalizeBuildEnvKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", &ValidationError{Message: "key is required"}
	}
	if !buildEnvKeyPattern.MatchString(key) {
		return "", &ValidationError{Message: "key must be a valid environment variable name"}
	}
	return key, nil
}

func buildEnvVarToResult(v models.BuildEnvVar) BuildEnvVarResult {
	return BuildEnvVarResult{
		ID:        v.ID,
		Key:       v.Key,
		CreatedAt: v.CreatedAt,
		UpdatedAt: v.UpdatedAt,
	}
}
