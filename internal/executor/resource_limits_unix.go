//go:build unix

package executor

import (
	"os"
	"syscall"
)

type diskUsageTracker struct {
	seen map[fileID]struct{}
}

type fileID struct {
	dev uint64
	ino uint64
}

func newDiskUsageTracker() *diskUsageTracker {
	return &diskUsageTracker{seen: make(map[fileID]struct{})}
}

func (t *diskUsageTracker) size(info os.FileInfo) int64 {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return info.Size()
	}
	if stat.Nlink > 1 {
		id := fileID{dev: uint64(stat.Dev), ino: uint64(stat.Ino)}
		if _, ok := t.seen[id]; ok {
			return 0
		}
		t.seen[id] = struct{}{}
	}
	if stat.Blocks > 0 {
		return stat.Blocks * 512
	}
	return info.Size()
}
