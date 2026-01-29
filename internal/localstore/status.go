package localstore

import (
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"
)

const (
	StatusClean    = "clean"
	StatusModified = "modified"
	StatusMissing  = "missing"
)

// ComputeStatus compares the current spec files against snapshots.
// Returns "clean", "modified", or "missing".
func (s *Store) ComputeStatus(ws *Workspace) string {
	// Check if the workspace directory still exists
	if _, err := os.Stat(ws.Path); err != nil {
		return StatusMissing
	}

	// Check if pixi.toml exists (required)
	if _, err := os.Stat(filepath.Join(ws.Path, "pixi.toml")); err != nil {
		return StatusMissing
	}

	// Compare each spec file
	for _, name := range SpecFiles {
		current := filepath.Join(ws.Path, name)
		snapshot := filepath.Join(s.SnapshotDir(ws.ID), name)

		currentHash, errC := fileHash(current)
		snapshotHash, errS := fileHash(snapshot)

		// Both missing is fine (e.g. no pixi.lock)
		if errors.Is(errC, os.ErrNotExist) && errors.Is(errS, os.ErrNotExist) {
			continue
		}

		// One exists but not the other
		if (errC != nil) != (errS != nil) {
			return StatusModified
		}

		// Both exist, compare hashes
		if currentHash != snapshotHash {
			return StatusModified
		}
	}

	return StatusClean
}

func fileHash(path string) ([sha256.Size]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return [sha256.Size]byte{}, err
	}

	var result [sha256.Size]byte
	copy(result[:], h.Sum(nil))
	return result, nil
}
