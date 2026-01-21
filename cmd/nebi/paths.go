package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// EnvIndex maps workspace names to their UUIDs for local lookup.
type EnvIndex struct {
	Workspaces map[string]string `json:"workspaces"` // name -> UUID
}

// getDataDir returns the XDG data directory for nebi (~/.local/share/nebi).
func getDataDir() (string, error) {
	// Check XDG_DATA_HOME first, then fall back to ~/.local/share
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		dataHome = filepath.Join(homeDir, ".local", "share")
	}
	return filepath.Join(dataHome, "nebi"), nil
}

// getEnvsDir returns the central environments directory (~/.local/share/nebi/envs).
func getEnvsDir() (string, error) {
	dataDir, err := getDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, "envs"), nil
}

// getIndexPath returns the path to the index.json file.
func getIndexPath() (string, error) {
	envsDir, err := getEnvsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(envsDir, "index.json"), nil
}

// loadIndex loads the index.json file, creating it if it doesn't exist.
func loadIndex() (*EnvIndex, error) {
	indexPath, err := getIndexPath()
	if err != nil {
		return nil, err
	}

	// Ensure the envs directory exists
	envsDir := filepath.Dir(indexPath)
	if err := os.MkdirAll(envsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create envs directory: %w", err)
	}

	// Try to read existing index
	data, err := os.ReadFile(indexPath)
	if os.IsNotExist(err) {
		// Return empty index
		return &EnvIndex{
			Workspaces: make(map[string]string),
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read index: %w", err)
	}

	var index EnvIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("failed to parse index: %w", err)
	}

	// Initialize map if nil
	if index.Workspaces == nil {
		index.Workspaces = make(map[string]string)
	}

	return &index, nil
}

// saveIndex saves the index.json file.
func saveIndex(index *EnvIndex) error {
	indexPath, err := getIndexPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal index: %w", err)
	}

	if err := os.WriteFile(indexPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write index: %w", err)
	}

	return nil
}

// getOrCreateWorkspaceUUID gets the UUID for a workspace from the index,
// or adds it if not present. The workspaceID should come from the server.
func getOrCreateWorkspaceUUID(index *EnvIndex, workspaceName, workspaceID string) string {
	// Check if we already have this workspace
	if existingID, ok := index.Workspaces[workspaceName]; ok {
		// If the ID matches, use it; otherwise update to new ID
		if existingID == workspaceID {
			return existingID
		}
		// Workspace was recreated with new ID, update
	}

	// Add or update the mapping
	index.Workspaces[workspaceName] = workspaceID
	return workspaceID
}

// lookupWorkspaceUUID looks up a workspace UUID by name from the local index.
// Returns the UUID and true if found, or empty string and false if not found.
func lookupWorkspaceUUID(workspaceName string) (string, bool) {
	index, err := loadIndex()
	if err != nil {
		return "", false
	}
	uuid, ok := index.Workspaces[workspaceName]
	return uuid, ok
}

// sanitizePathComponent sanitizes a string for safe use as a directory name.
// Replaces special characters that are problematic on filesystems.
func sanitizePathComponent(s string) string {
	// Replace characters that are problematic on various filesystems
	// : is invalid on Windows, / is a path separator, etc.
	replacer := strings.NewReplacer(
		":", "_",
		"/", "_",
		"\\", "_",
		"<", "_",
		">", "_",
		"\"", "_",
		"|", "_",
		"?", "_",
		"*", "_",
		" ", "_",
	)
	result := replacer.Replace(s)

	// Collapse multiple underscores
	re := regexp.MustCompile(`_+`)
	result = re.ReplaceAllString(result, "_")

	// Trim leading/trailing underscores
	result = strings.Trim(result, "_")

	return result
}

// getCentralEnvPath returns the path for a centrally stored environment.
// Structure: ~/.local/share/nebi/envs/<workspace-uuid>/<sanitized-registry>/<tag>/
func getCentralEnvPath(workspaceUUID, registryName, tag string) (string, error) {
	envsDir, err := getEnvsDir()
	if err != nil {
		return "", err
	}

	sanitizedRegistry := sanitizePathComponent(registryName)
	sanitizedTag := sanitizePathComponent(tag)
	if sanitizedTag == "" {
		sanitizedTag = "latest"
	}

	return filepath.Join(envsDir, workspaceUUID, sanitizedRegistry, sanitizedTag), nil
}

// envExistsLocally checks if a pixi.toml exists in the given directory.
func envExistsLocally(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "pixi.toml"))
	return err == nil
}

// findLocalPixiToml looks for pixi.toml in the current directory.
// Returns the directory path and true if found, or empty string and false if not found.
func findLocalPixiToml() (string, bool) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", false
	}
	pixiPath := filepath.Join(cwd, "pixi.toml")
	if _, err := os.Stat(pixiPath); err == nil {
		return cwd, true
	}
	return "", false
}

// expandPath expands ~ to the home directory and resolves relative paths.
func expandPath(path string) (string, error) {
	if path == "" {
		return "", nil
	}

	// Expand ~
	if strings.HasPrefix(path, "~") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		path = filepath.Join(homeDir, path[1:])
	}

	// Convert to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}

	return absPath, nil
}
