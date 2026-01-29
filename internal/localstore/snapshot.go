package localstore

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// SpecFiles are the pixi workspace files we track.
var SpecFiles = []string{"pixi.toml", "pixi.lock"}

// SaveSnapshot copies spec files from srcDir into the snapshot directory for the given workspace.
func (s *Store) SaveSnapshot(workspaceID string, srcDir string) error {
	snapDir := s.SnapshotDir(workspaceID)
	if err := os.MkdirAll(snapDir, 0755); err != nil {
		return fmt.Errorf("creating snapshot directory: %w", err)
	}

	for _, name := range SpecFiles {
		src := filepath.Join(srcDir, name)
		dst := filepath.Join(snapDir, name)

		if err := copyFile(src, dst); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				// pixi.lock may not exist yet, that's fine
				os.Remove(dst) // remove stale snapshot if source is gone
				continue
			}
			return fmt.Errorf("copying %s: %w", name, err)
		}
	}
	return nil
}

// SnapshotExists returns true if the snapshot directory exists for the given workspace.
func (s *Store) SnapshotExists(workspaceID string) bool {
	info, err := os.Stat(s.SnapshotDir(workspaceID))
	return err == nil && info.IsDir()
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
