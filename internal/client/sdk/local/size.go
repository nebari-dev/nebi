package local

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

// sizeCache holds project sizes in memory. Sizes are never stored in the
// database.
type sizeCache struct {
	mu       sync.Mutex
	entries  map[uuid.UUID]sizeEntry
	inflight map[uuid.UUID]bool
}

type sizeEntry struct {
	bytes      int64
	measuredAt time.Time
}

func newSizeCache() sizeCache {
	return sizeCache{entries: make(map[uuid.UUID]sizeEntry), inflight: make(map[uuid.UUID]bool)}
}

func (c *sizeCache) get(id uuid.UUID) (int64, time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[id]
	return e.bytes, e.measuredAt, ok
}

func (c *sizeCache) store(id uuid.UUID, bytes int64, at time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[id] = sizeEntry{bytes: bytes, measuredAt: at}
}

func (c *sizeCache) forget(id uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, id)
}

// claim marks the projects whose size is missing or older than ttl as being
// measured and returns them. Projects already being measured are skipped.
func (c *sizeCache) claim(projects []Project, now time.Time, ttl time.Duration) []Project {
	c.mu.Lock()
	defer c.mu.Unlock()
	var due []Project
	for _, p := range projects {
		if c.inflight[p.ID] {
			continue
		}
		if e, ok := c.entries[p.ID]; ok && now.Sub(e.measuredAt) < ttl {
			continue
		}
		c.inflight[p.ID] = true
		due = append(due, p)
	}
	return due
}

func (c *sizeCache) release(id uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.inflight, id)
}

// refreshSizes measures, in the background, the projects whose cached size
// is missing or stale. It returns at once.
func (s *Service) refreshSizes(projects []Project) {
	due := s.sizes.claim(projects, s.now(), s.sizeTTL)
	if len(due) == 0 {
		return
	}
	s.bg.Add(1)
	go func() {
		defer s.bg.Done()
		for _, p := range due {
			if s.bgCtx.Err() == nil {
				if size, err := s.measureDir(s.bgCtx, p.Path); err == nil {
					s.sizes.store(p.ID, size, s.now())
				}
			}
			s.sizes.release(p.ID)
		}
	}()
}

// measureDir returns the total size of the regular files under dir.
// Symlinks are not followed or counted, unreadable entries are skipped, and
// a missing dir has size 0.
func measureDir(ctx context.Context, dir string) (int64, error) {
	var total int64
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			if d == nil && errors.Is(err, fs.ErrNotExist) {
				return fs.SkipAll
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		return 0, err
	}
	return total, nil
}
