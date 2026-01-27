// Package drift provides drift detection for Nebi repos.
//
// Drift detection compares local file content against the layer digests
// stored in index.json. This allows offline detection of whether files
// have been modified since they were pulled.
package drift

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/aktech/darb/internal/localindex"
	"github.com/aktech/darb/internal/nebifile"
)

// Status represents the drift status of a repo or file.
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

// RepoStatus represents the overall drift status of a repo.
type RepoStatus struct {
	Overall Status       `json:"overall"`
	Files   []FileStatus `json:"files"`
}

// Check performs drift detection on a repo directory.
// It reads the layer digests from index.json and compares local file hashes.
func Check(dir string) (*RepoStatus, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		absDir = dir
	}

	// Get layers from local index
	store := localindex.NewStore()
	entry, err := store.FindByPath(absDir)
	if err != nil || entry == nil {
		// Try to use nebifile to find the entry by other means
		nf, nfErr := nebifile.Read(dir)
		if nfErr != nil {
			return &RepoStatus{
				Overall: StatusUnknown,
			}, fmt.Errorf("failed to find entry in index: %w", err)
		}
		// Try to find entry by spec name and version
		entries, findErr := store.FindBySpecVersion(nf.Origin.SpecName, nf.Origin.VersionName)
		if findErr != nil || len(entries) == 0 {
			return &RepoStatus{
				Overall: StatusUnknown,
			}, fmt.Errorf("entry not found in local index")
		}
		// Use the first matching entry
		entry = &entries[0]
	}

	return CheckWithLayers(dir, entry.Layers), nil
}

// CheckWithLayers performs drift detection using layer digests.
// This is the main drift checking function that compares local files
// against the provided layer digests.
func CheckWithLayers(dir string, layers map[string]string) *RepoStatus {
	ws := &RepoStatus{
		Overall: StatusClean,
		Files:   make([]FileStatus, 0, len(layers)),
	}

	for filename, digest := range layers {
		fs := checkFile(dir, filename, digest)
		ws.Files = append(ws.Files, fs)

		if fs.Status != StatusClean {
			ws.Overall = StatusModified
		}
	}

	return ws
}

// CheckWithNebiFile performs drift detection by looking up layers in the index.
// This is a convenience function that uses the nebifile to find the index entry.
// Deprecated: Use CheckWithLayers or Check instead.
func CheckWithNebiFile(dir string, nf *nebifile.NebiFile) *RepoStatus {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		absDir = dir
	}

	// Get layers from local index
	store := localindex.NewStore()
	entry, err := store.FindByPath(absDir)
	if err != nil || entry == nil {
		// Try by spec name and version
		entries, _ := store.FindBySpecVersion(nf.Origin.SpecName, nf.Origin.VersionName)
		if len(entries) > 0 {
			entry = &entries[0]
		}
	}

	if entry != nil && entry.Layers != nil {
		return CheckWithLayers(dir, entry.Layers)
	}

	// Fallback: return clean status if no layers found
	return &RepoStatus{
		Overall: StatusClean,
		Files:   []FileStatus{},
	}
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

// IsModified returns true if any file in the repo has been modified.
func (ws *RepoStatus) IsModified() bool {
	return ws.Overall == StatusModified
}

// GetFileStatus returns the status of a specific file, or nil if not tracked.
func (ws *RepoStatus) GetFileStatus(filename string) *FileStatus {
	for i := range ws.Files {
		if ws.Files[i].Filename == filename {
			return &ws.Files[i]
		}
	}
	return nil
}
