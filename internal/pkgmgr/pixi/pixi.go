package pixi

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/aktech/darb/internal/pkgmgr"
	"github.com/pelletier/go-toml/v2"
)

func init() {
	pkgmgr.Register("pixi", func(customPath string) (pkgmgr.PackageManager, error) {
		return NewWithPath(customPath)
	})
}

// PixiManager implements the PackageManager interface for pixi
type PixiManager struct {
	pixiPath string // Path to pixi binary
}

// New creates a new PixiManager instance
func New() (*PixiManager, error) {
	return NewWithPath("")
}

// NewWithPath creates a new PixiManager with a custom pixi binary path
func NewWithPath(customPath string) (*PixiManager, error) {
	pixiPath := customPath
	if pixiPath == "" {
		// Find pixi binary in PATH
		path, err := exec.LookPath("pixi")
		if err != nil {
			// Pixi not found, attempt automatic installation
			ctx := context.Background()
			installedPath, installErr := InstallPixi(ctx)
			if installErr != nil {
				return nil, fmt.Errorf("pixi not found in PATH and auto-installation failed: %w", installErr)
			}
			pixiPath = installedPath
		} else {
			pixiPath = path
		}
	}

	// Verify pixi is executable
	if err := exec.Command(pixiPath, "--version").Run(); err != nil {
		return nil, fmt.Errorf("pixi binary is not executable: %w", err)
	}

	return &PixiManager{pixiPath: pixiPath}, nil
}

// Name returns the package manager name
func (p *PixiManager) Name() string {
	return "pixi"
}

// BinaryPath returns the path to the pixi binary
func (p *PixiManager) BinaryPath() string {
	return p.pixiPath
}

// Init creates a new pixi environment
func (p *PixiManager) Init(ctx context.Context, opts pkgmgr.InitOptions) error {
	if opts.EnvPath == "" {
		return fmt.Errorf("environment path is required")
	}
	if opts.Name == "" {
		return fmt.Errorf("environment name is required")
	}

	// Create the directory if it doesn't exist
	if err := os.MkdirAll(opts.EnvPath, 0755); err != nil {
		return fmt.Errorf("failed to create environment directory: %w", err)
	}

	// Build pixi init command
	// We run pixi init in the target directory
	// The project name will be set to the directory name by pixi
	args := []string{"init"}

	// Add channels if specified
	for _, channel := range opts.Channels {
		args = append(args, "--channel", channel)
	}

	// Execute pixi init in the target directory
	cmd := exec.CommandContext(ctx, p.pixiPath, args...)
	cmd.Dir = opts.EnvPath

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pixi init failed: %w, stderr: %s", err, stderr.String())
	}

	return nil
}

// Install adds packages to an environment
func (p *PixiManager) Install(ctx context.Context, opts pkgmgr.InstallOptions) error {
	if opts.EnvPath == "" {
		return fmt.Errorf("environment path is required")
	}
	if len(opts.Packages) == 0 {
		return fmt.Errorf("at least one package is required")
	}

	// Build pixi add command
	args := append([]string{"add"}, opts.Packages...)

	// Execute pixi add
	cmd := exec.CommandContext(ctx, p.pixiPath, args...)
	cmd.Dir = opts.EnvPath

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pixi add failed: %w, stderr: %s", err, stderr.String())
	}

	return nil
}

// Remove removes packages from an environment
func (p *PixiManager) Remove(ctx context.Context, opts pkgmgr.RemoveOptions) error {
	if opts.EnvPath == "" {
		return fmt.Errorf("environment path is required")
	}
	if len(opts.Packages) == 0 {
		return fmt.Errorf("at least one package is required")
	}

	// Build pixi remove command
	args := append([]string{"remove"}, opts.Packages...)

	// Execute pixi remove
	cmd := exec.CommandContext(ctx, p.pixiPath, args...)
	cmd.Dir = opts.EnvPath

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pixi remove failed: %w, stderr: %s", err, stderr.String())
	}

	return nil
}

// List returns installed packages in an environment
func (p *PixiManager) List(ctx context.Context, opts pkgmgr.ListOptions) ([]pkgmgr.Package, error) {
	if opts.EnvPath == "" {
		return nil, fmt.Errorf("environment path is required")
	}

	// Get packages from manifest file instead of running pixi list
	// This is more reliable and doesn't require the environment to be activated
	manifest, err := p.GetManifest(ctx, opts.EnvPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get manifest: %w", err)
	}

	packages := make([]pkgmgr.Package, 0, len(manifest.Packages))
	for name, version := range manifest.Packages {
		packages = append(packages, pkgmgr.Package{
			Name:    name,
			Version: version,
			Channel: "", // Channel info not directly available in simple list
		})
	}

	return packages, nil
}

// Update updates packages in an environment
func (p *PixiManager) Update(ctx context.Context, opts pkgmgr.UpdateOptions) error {
	if opts.EnvPath == "" {
		return fmt.Errorf("environment path is required")
	}

	// Build pixi update command
	args := []string{"update"}
	if len(opts.Packages) > 0 {
		args = append(args, opts.Packages...)
	}

	// Execute pixi update
	cmd := exec.CommandContext(ctx, p.pixiPath, args...)
	cmd.Dir = opts.EnvPath

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pixi update failed: %w, stderr: %s", err, stderr.String())
	}

	return nil
}

// pixiManifest represents the structure of a pixi.toml file
type pixiManifest struct {
	Project struct {
		Name     string   `toml:"name"`
		Channels []string `toml:"channels"`
	} `toml:"project"`
	Dependencies map[string]interface{} `toml:"dependencies"`
}

// GetManifest returns the parsed pixi.toml manifest file
func (p *PixiManager) GetManifest(ctx context.Context, envPath string) (*pkgmgr.Manifest, error) {
	if envPath == "" {
		return nil, fmt.Errorf("environment path is required")
	}

	manifestPath := filepath.Join(envPath, "pixi.toml")

	// Read the manifest file
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read pixi.toml: %w", err)
	}

	// Parse TOML
	var pixiManifest pixiManifest
	if err := toml.Unmarshal(content, &pixiManifest); err != nil {
		return nil, fmt.Errorf("failed to parse pixi.toml: %w", err)
	}

	// Convert dependencies to map[string]string
	packages := make(map[string]string)
	for name, value := range pixiManifest.Dependencies {
		// Dependencies can be strings (version) or tables (complex spec)
		switch v := value.(type) {
		case string:
			packages[name] = v
		case map[string]interface{}:
			// If it's a table, try to extract version
			if version, ok := v["version"].(string); ok {
				packages[name] = version
			} else {
				packages[name] = "*" // Unknown version
			}
		default:
			packages[name] = "*" // Unknown version
		}
	}

	manifest := &pkgmgr.Manifest{
		Name:     pixiManifest.Project.Name,
		Packages: packages,
		Channels: pixiManifest.Project.Channels,
		Raw:      content,
	}

	return manifest, nil
}

// executeCommand is a helper to execute pixi commands with proper error handling
func (p *PixiManager) executeCommand(ctx context.Context, workDir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, p.pixiPath, args...)
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("command failed: %w, stderr: %s", err, stderr.String())
	}

	return strings.TrimSpace(stdout.String()), nil
}
