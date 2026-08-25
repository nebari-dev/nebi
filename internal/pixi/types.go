package pixi

import (
	"fmt"
	"io"

	"github.com/nebari-dev/nebi/internal/limits"
)

// InitOptions contains parameters for initializing a new environment
type InitOptions struct {
	EnvPath        string               // Path where environment will be created
	Name           string               // Environment name
	Python         string               // Python version (if applicable)
	Channels       []string             // Conda channels
	LogWriter      io.Writer            // Optional writer for streaming command output
	ResourceLimits limits.ProcessLimits // Optional OS process limits
}

// InstallOptions contains parameters for installing packages
type InstallOptions struct {
	EnvPath        string               // Path to environment
	Packages       []string             // Package names (e.g., "numpy==1.24.0")
	LogWriter      io.Writer            // Optional writer for streaming command output
	NoInstall      bool                 // Only update manifest and lockfile, don't install the environment
	ResourceLimits limits.ProcessLimits // Optional OS process limits
}

// RemoveOptions contains parameters for removing packages
type RemoveOptions struct {
	EnvPath        string               // Path to environment
	Packages       []string             // Package names to remove
	LogWriter      io.Writer            // Optional writer for streaming command output
	NoInstall      bool                 // Only update manifest and lockfile, don't modify the environment
	ResourceLimits limits.ProcessLimits // Optional OS process limits
}

// ListOptions contains parameters for listing packages
type ListOptions struct {
	EnvPath        string               // Path to environment
	ResourceLimits limits.ProcessLimits // Optional OS process limits
	MaxOutputBytes int                  // Optional cap for pixi list output
}

// OutputLimitError reports that a pixi command produced more output
// than the caller was willing to keep in memory.
type OutputLimitError struct {
	Command string
	Stream  string
	Limit   int
}

func (e *OutputLimitError) Error() string {
	return fmt.Sprintf("%s %s exceeds %d bytes", e.Command, e.Stream, e.Limit)
}

// UpdateOptions contains parameters for updating packages
type UpdateOptions struct {
	EnvPath        string               // Path to environment
	Packages       []string             // Packages to update (empty = update all)
	ResourceLimits limits.ProcessLimits // Optional OS process limits
}

// Package represents an installed package
type Package struct {
	Name    string
	Version string
	Channel string // Conda channel, when known
}

// Manifest represents a parsed pixi.toml manifest file
type Manifest struct {
	Name     string
	Packages map[string]string // name -> version
	Channels []string
	Raw      []byte // Raw manifest content
}
