package localstore

import (
	"errors"
	"os"
	"path/filepath"
)

// FileStatus describes the state of a single spec file.
type FileStatus int

const (
	FileUnchanged FileStatus = iota
	FileModified
	FileNew
	FileDeleted
	FileBothMissing
)

// DiffResult holds the diff status for a single spec file.
type DiffResult struct {
	Name         string
	Status       FileStatus
	CurrentPath  string
	SnapshotPath string
}

// ComputeDiff returns per-file diff results for a workspace.
func (s *Store) ComputeDiff(ws *Workspace) ([]DiffResult, error) {
	var results []DiffResult

	for _, name := range SpecFiles {
		current := filepath.Join(ws.Path, name)
		snapshot := filepath.Join(s.SnapshotDir(ws.ID), name)

		_, errC := os.Stat(current)
		_, errS := os.Stat(snapshot)

		bothMissing := errors.Is(errC, os.ErrNotExist) && errors.Is(errS, os.ErrNotExist)
		if bothMissing {
			results = append(results, DiffResult{Name: name, Status: FileBothMissing})
			continue
		}

		if errors.Is(errS, os.ErrNotExist) {
			results = append(results, DiffResult{Name: name, Status: FileNew, CurrentPath: current})
			continue
		}

		if errors.Is(errC, os.ErrNotExist) {
			results = append(results, DiffResult{Name: name, Status: FileDeleted, SnapshotPath: snapshot})
			continue
		}

		// Both exist — compare hashes
		hC, err := fileHash(current)
		if err != nil {
			return nil, err
		}
		hS, err := fileHash(snapshot)
		if err != nil {
			return nil, err
		}

		if hC == hS {
			results = append(results, DiffResult{Name: name, Status: FileUnchanged})
		} else {
			results = append(results, DiffResult{
				Name:         name,
				Status:       FileModified,
				CurrentPath:  current,
				SnapshotPath: snapshot,
			})
		}
	}

	return results, nil
}
