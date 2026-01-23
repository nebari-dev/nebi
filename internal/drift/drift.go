// Package drift provides drift detection for Nebi workspaces.
//
// Drift detection compares local file content against the layer digests
// stored in the .nebi metadata file. This allows offline detection of
// whether files have been modified since they were pulled.
package drift

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/aktech/darb/internal/nebifile"
)

// Status represents the drift status of a workspace or file.
type Status string

const (
	// StatusClean means the local file matches the pulled layer digest.
	StatusClean Status = "clean"

	// StatusModified means the local file differs from the pulled layer digest.
	StatusModified Status = "modified"

	// StatusMissing means the local file no longer exists.
	StatusMissing Status = "missing"

	// StatusUnknown means drift cannot be determined (e.g., .nebi metadata missing).
	StatusUnknown Status = "unknown"
)

// FileStatus represents the drift status of a single file.
type FileStatus struct {
	Filename      string `json:"filename"`
	Status        Status `json:"status"`
	OriginDigest  string `json:"origin_digest,omitempty"`
	CurrentDigest string `json:"current_digest,omitempty"`
}

// WorkspaceStatus represents the overall drift status of a workspace.
type WorkspaceStatus struct {
	Overall Status       `json:"overall"`
	Files   []FileStatus `json:"files"`
}

// Check performs drift detection on a workspace directory.
// It reads the .nebi metadata file and compares local file hashes
// against the stored layer digests.
func Check(dir string) (*WorkspaceStatus, error) {
	nf, err := nebifile.Read(dir)
	if err != nil {
		return &WorkspaceStatus{
			Overall: StatusUnknown,
		}, fmt.Errorf("failed to read .nebi metadata: %w", err)
	}

	return CheckWithNebiFile(dir, nf), nil
}

// CheckWithNebiFile performs drift detection using an already-loaded .nebi file.
// This is useful when the caller already has the NebiFile loaded.
func CheckWithNebiFile(dir string, nf *nebifile.NebiFile) *WorkspaceStatus {
	ws := &WorkspaceStatus{
		Overall: StatusClean,
		Files:   make([]FileStatus, 0, len(nf.Layers)),
	}

	for filename, layer := range nf.Layers {
		fs := checkFile(dir, filename, layer.Digest)
		ws.Files = append(ws.Files, fs)

		// Missing takes precedence over modified in overall status
		if fs.Status == StatusMissing {
			ws.Overall = StatusMissing
		} else if fs.Status != StatusClean && ws.Overall != StatusMissing {
			ws.Overall = StatusModified
		}
	}

	return ws
}

// checkFile checks the drift status of a single file.
func checkFile(dir, filename, originDigest string) FileStatus {
	path := filepath.Join(dir, filename)

	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return FileStatus{
				Filename:     filename,
				Status:       StatusMissing,
				OriginDigest: originDigest,
			}
		}
		// Can't read file - treat as unknown
		return FileStatus{
			Filename:     filename,
			Status:       StatusUnknown,
			OriginDigest: originDigest,
		}
	}

	currentDigest := nebifile.ComputeDigest(content)

	if currentDigest == originDigest {
		return FileStatus{
			Filename:      filename,
			Status:        StatusClean,
			OriginDigest:  originDigest,
			CurrentDigest: currentDigest,
		}
	}

	return FileStatus{
		Filename:      filename,
		Status:        StatusModified,
		OriginDigest:  originDigest,
		CurrentDigest: currentDigest,
	}
}

// IsModified returns true if any file in the workspace has been modified.
func (ws *WorkspaceStatus) IsModified() bool {
	return ws.Overall == StatusModified || ws.Overall == StatusMissing
}

// GetFileStatus returns the status of a specific file, or nil if not tracked.
func (ws *WorkspaceStatus) GetFileStatus(filename string) *FileStatus {
	for i := range ws.Files {
		if ws.Files[i].Filename == filename {
			return &ws.Files[i]
		}
	}
	return nil
}
